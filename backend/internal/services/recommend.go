package services

import (
	"errors"
	"math"
	"math/rand"
	"sort"
	"time"

	"SakuManga/internal/models"
)

// ─────────────────────────────────────────────────────────────
// Round32 阶段二：本地偏好推荐
//
// 定位：推荐 = 「带偏好的随机抽卡」，只作用于本地库（在线部分保持纯随机，决策 D1）。
// 数据源：阶段一的 XP 权重表（XpCloudService.WeightTable），不重复维护统计口径。
//
// 算法（详见 plans/round32-xp-cloud-recommend-plan.md）：
//   1. 打分  score(c) = Σ_{t∈tags(c)} w'(t) / |tags(c)|^γ
//      w'(t) = [(1-θ)·libNorm(t) + θ·readNorm(t)] · IDF(t)   （θ=偏好侧重，IDF=泛化抑制）
//   2. 采样  p(c) ∝ exp((score(c) - max) / T)                 （温度 T 越大越平缓）
//      ε 概率改为均匀随机（探索率：防止 XP 固化在同一批本子）
//   3. 多样性：单轮内同一 parody / artist 最多 2 本（软约束，候选不足时自动放宽）
//   4. 降级：权重表为空 / 候选全为 0 分 → 返回 ErrRecommendNoWeights，由调用方退回纯随机
// ─────────────────────────────────────────────────────────────

// 推荐参数默认值（前端可覆盖，决策 D2）
const (
	RecoDefaultTheta       = 0.60 // 偏好侧重：0=纯库藏，1=纯阅读
	RecoDefaultExplore     = 0.15 // 探索率 ε
	RecoDefaultTemperature = 0.50 // 采样温度 T
	RecoDefaultGamma       = 0.50 // 长度归一指数 γ（抑制 tag 多的本子刷分）

	RecoMaxTemperature      = 2.0
	RecoMinTemperature      = 0.2
	RecoMaxExplore          = 0.5
	RecoDiversityMaxSameKey = 2 // 同一 parody/artist 单轮最多本数
)

// ErrRecommendNoWeights 无可用权重（权重表为空或候选全零分）→ 调用方退回纯随机
var ErrRecommendNoWeights = errors.New("本地库暂无可用偏好权重")

// RecommendOptions 推荐参数（已归一化）
type RecommendOptions struct {
	Theta       float64 // 0~1
	Explore     float64 // 0~0.5
	Temperature float64 // 0.2~2
	Gamma       float64 // 0~1
}

// NormalizeRecommendOptions 夹取参数到合法区间（非法/缺省值回退默认）
func NormalizeRecommendOptions(opts RecommendOptions) RecommendOptions {
	out := opts
	if out.Theta < 0 || out.Theta > 1 || math.IsNaN(out.Theta) {
		out.Theta = RecoDefaultTheta
	}
	if out.Explore < 0 || out.Explore > RecoMaxExplore || math.IsNaN(out.Explore) {
		out.Explore = RecoDefaultExplore
	}
	if out.Temperature < RecoMinTemperature || out.Temperature > RecoMaxTemperature || math.IsNaN(out.Temperature) {
		out.Temperature = RecoDefaultTemperature
	}
	if out.Gamma < 0 || out.Gamma > 1 || math.IsNaN(out.Gamma) {
		out.Gamma = RecoDefaultGamma
	}
	return out
}

// RankedComic 推荐结果项（含命中理由）
type RankedComic struct {
	Comic       models.OfflineComic
	Score       float64
	MatchedTags []string // 贡献最高的前 3 个 tag（"namespace:key"，前端做命中理由徽标）

	// divKeys 多样性键缓存（parody/artist/group）：采样循环逐轮判定，
	// 预计算一次可避免对数千候选反复解析 tag JSON。
	divKeys []string
}

// RecommendService 本地偏好推荐服务
type RecommendService struct {
	xp *XpCloudService
}

// NewRecommendService 构造推荐服务（复用阶段一的统计服务权重表与其缓存）
func NewRecommendService(xp *XpCloudService) *RecommendService {
	return &RecommendService{xp: xp}
}

// Recommend 从候选集中按偏好加权抽取 count 本
//
// candidates 由调用方按硬约束（分类/页数/评分/负向排除等）预筛；本方法只负责打分与采样。
func (s *RecommendService) Recommend(userID uint, candidates []models.OfflineComic, count int, opts RecommendOptions) ([]RankedComic, error) {
	if len(candidates) == 0 || count <= 0 {
		return nil, nil
	}
	opts = NormalizeRecommendOptions(opts)

	table, err := s.xp.WeightTable(userID)
	if err != nil {
		return nil, err
	}
	if len(table.Lib) == 0 && len(table.Read) == 0 {
		return nil, ErrRecommendNoWeights
	}

	ranked := make([]RankedComic, 0, len(candidates))
	usable := 0
	for i := range candidates {
		comic := &candidates[i]
		tags := XpEffectiveTags(comic)
		score, matched := ScoreComic(tags, table, opts)
		if score > 0 {
			usable++
		}
		ranked = append(ranked, RankedComic{
			Comic:       *comic,
			Score:       score,
			MatchedTags: matched,
			divKeys:     comicDiversityKeys(comic),
		})
	}
	if usable == 0 {
		return nil, ErrRecommendNoWeights
	}

	if count > len(ranked) {
		count = len(ranked)
	}
	return sampleRanked(ranked, count, opts), nil
}

// ScoreComic 单本打分：Σ w'(t) / |tags|^γ，并返回贡献最高的前 3 个 tag 作为命中理由
//
// 导出以便单测直接断言打分口径。
func ScoreComic(tags []string, table *XpWeightTable, opts RecommendOptions) (float64, []string) {
	if len(tags) == 0 || table == nil {
		return 0, nil
	}
	type contribution struct {
		tag string
		w   float64
	}
	contribs := make([]contribution, 0, len(tags))
	sum := 0.0
	for _, tag := range tags {
		libW := table.Lib[tag]
		readW := table.Read[tag]
		w := (1-opts.Theta)*libW + opts.Theta*readW
		if w <= 0 {
			continue
		}
		w *= table.IDF[tag] // 泛化抑制
		if w <= 0 {
			continue
		}
		sum += w
		contribs = append(contribs, contribution{tag: tag, w: w})
	}
	if sum <= 0 {
		return 0, nil
	}

	score := sum / math.Pow(float64(len(tags)), opts.Gamma)

	sort.Slice(contribs, func(i, j int) bool {
		if contribs[i].w == contribs[j].w {
			return contribs[i].tag < contribs[j].tag // 稳定输出（便于测试）
		}
		return contribs[i].w > contribs[j].w
	})
	top := make([]string, 0, 3)
	for i := 0; i < len(contribs) && i < 3; i++ {
		top = append(top, contribs[i].tag)
	}
	return score, top
}

// sampleRanked 温度采样 + 探索率混合 + 多样性限流（不放回抽取）
func sampleRanked(ranked []RankedComic, count int, opts RecommendOptions) []RankedComic {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// 温度权重：减去 max 防止 exp 溢出
	maxScore := ranked[0].Score
	for i := range ranked {
		if ranked[i].Score > maxScore {
			maxScore = ranked[i].Score
		}
	}
	weights := make([]float64, len(ranked))
	for i := range ranked {
		weights[i] = math.Exp((ranked[i].Score - maxScore) / opts.Temperature)
	}

	avail := make([]int, 0, len(ranked))
	for i := range ranked {
		avail = append(avail, i)
	}
	picked := make([]RankedComic, 0, count)
	keyCount := map[string]int{}

	for len(picked) < count && len(avail) > 0 {
		// 1. 多样性限流（软约束：候选全被限流时必须放宽，否则抽不满）
		candidates := make([]int, 0, len(avail))
		for _, idx := range avail {
			if diversityOK(&ranked[idx], keyCount) {
				candidates = append(candidates, idx)
			}
		}
		if len(candidates) == 0 {
			// 放宽时仍优先取「超限程度最轻」的候选，避免同 parody/artist 扎堆
			best := -1
			for _, idx := range avail {
				m := maxKeyCount(&ranked[idx], keyCount)
				if best < 0 || m < best {
					best = m
					candidates = candidates[:0]
				}
				if m == best {
					candidates = append(candidates, idx)
				}
			}
		}

		// 2. 探索：ε 概率均匀随机；否则温度加权轮盘赌
		chosen := candidates[len(candidates)-1]
		if rng.Float64() < opts.Explore {
			chosen = candidates[rng.Intn(len(candidates))]
		} else {
			total := 0.0
			for _, idx := range candidates {
				total += weights[idx]
			}
			if total <= 0 {
				chosen = candidates[rng.Intn(len(candidates))]
			} else {
				target := rng.Float64() * total
				acc := 0.0
				for _, idx := range candidates {
					acc += weights[idx]
					if acc >= target {
						chosen = idx
						break
					}
				}
			}
		}

		picked = append(picked, ranked[chosen])
		for _, key := range ranked[chosen].divKeys {
			keyCount[key]++
		}

		// 3. 不放回：从可用集合移除
		for i, idx := range avail {
			if idx == chosen {
				avail = append(avail[:i], avail[i+1:]...)
				break
			}
		}
	}
	return picked
}

// comicDiversityKeys 取本子的多样性键（parody / artist / group），采样前预计算一次
func comicDiversityKeys(comic *models.OfflineComic) []string {
	keys := make([]string, 0, 2)
	for _, tag := range XpEffectiveTags(comic) {
		ns, _ := SplitXpTag(tag)
		if ns == "parody" || ns == "artist" || ns == "group" {
			keys = append(keys, tag)
		}
	}
	return keys
}

// diversityOK 该本的所有多样性键是否都未超限
func diversityOK(item *RankedComic, keyCount map[string]int) bool {
	for _, key := range item.divKeys {
		if keyCount[key] >= RecoDiversityMaxSameKey {
			return false
		}
	}
	return true
}

// maxKeyCount 该本多样性键中已选次数最大值（放宽限流时用来挑「超限最轻」的候选）
func maxKeyCount(item *RankedComic, keyCount map[string]int) int {
	m := 0
	for _, key := range item.divKeys {
		if keyCount[key] > m {
			m = keyCount[key]
		}
	}
	return m
}
