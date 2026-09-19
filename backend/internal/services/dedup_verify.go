package services

import (
	"fmt"
	"log"
	"strings"

	"SakuManga/internal/models"

	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// Round42 决策 D2：联网复核（默认关闭）+ 查重设置读写
//
// 定位：名称归一（Tier 1）与多字段打分（Tier 2）都是**纯本地**证据；对「差不多但不完全像」的
// 候选，用 E 站反查做一次外部权威校验，能把可信的近似簇升为高置信、并给未通过的明确标注。
// 默认关闭：联网有成本（每本 ~1.2s 限流）且依赖已绑定 IPB 账号，需用户显式开启。
// ─────────────────────────────────────────────────────────────

// dedupSettingID 查重设置单行主键
const dedupSettingID uint = 1

// maxOnlineVerifyPerRun 单次维护的联网复核上限（限流 ~1.2s/本 → 30 本约 36s）
const maxOnlineVerifyPerRun = 30

// GetDedupSetting 读取查重设置（无记录时返回默认值：联网复核关闭）
func GetDedupSetting(db *gorm.DB) models.DedupSetting {
	out := models.DedupSetting{ID: dedupSettingID, OnlineVerify: false}
	if db == nil {
		return out
	}
	var s models.DedupSetting
	if err := db.Where("id = ?", dedupSettingID).First(&s).Error; err != nil {
		return out
	}
	return s
}

// SaveDedupSetting 保存查重设置（单行覆盖写）
func SaveDedupSetting(db *gorm.DB, s models.DedupSetting) error {
	if db == nil {
		return fmt.Errorf("非法参数：db 不能为空")
	}
	s.ID = dedupSettingID
	if err := db.Where("id = ?", dedupSettingID).Delete(&models.DedupSetting{}).Error; err != nil {
		return err
	}
	return db.Create(&s).Error
}

// matchOnlineCandidate 判定 E 站搜索结果是否佐证该簇为「同一作品」：
// 结果中存在一条，其标题经同一套清洗后与簇核心名一致 → 复核通过。
// （列表结果的 Title 即主标题；跨语言翻译标题天然不匹配 → 复核"未确认"，符合"宁缺毋滥"）
func matchOnlineCandidate(clusterKey string, results []OnlineComicDTO) bool {
	want := fingerprintTitle(clusterKey, "").Core
	if want == "" {
		want = strings.ToLower(strings.TrimSpace(clusterKey))
	}
	if want == "" {
		return false
	}
	for _, r := range results {
		if strings.TrimSpace(r.Title) == "" {
			continue
		}
		if got := fingerprintTitle(r.Title, "").Core; got != "" && got == want {
			return true
		}
	}
	return false
}

// approxClusterTarget 是否为「近似层（Tier 2）」产出的、需要联网复核的簇。
// Tier 1 的精确归一键结果不参与（本地证据已足够），只复核 Tier 2 的近似判定。
func approxClusterTarget(cl DedupCluster) bool {
	return strings.Contains(cl.Reason, "多重相似")
}

// VerifyApproxClustersOnline 对近似层疑似重复簇做 E 站反查复核（原地更新 confidence/reason）。
//
// 返回复核通过的簇数（仅用于日志/测试）。约束：
//   - 未绑定 IPB 账号 / ehService 为空 → 直接跳过（不报错）；
//   - 单次上限 maxOnlineVerifyPerRun 个簇，逐簇限流；
//   - 通过 → 置信度升为 high 并注明依据；未通过 → 保持原档并标注「未确认」，供人工优先看。
func VerifyApproxClustersOnline(db *gorm.DB, ehService *EHService, clusters []DedupCluster, onProgress OfflineProgressFn) int {
	if db == nil || ehService == nil || len(clusters) == 0 {
		return 0
	}
	account := LoadAdminAccount(db)
	if account == nil || account.IPBMemberID == "" {
		log.Printf("%s [dedup-verify] 未绑定 IPB 账号，跳过联网复核", dlWarnTag)
		return 0
	}
	ehSetting := loadEHSetting(db, LoadAdminUserID(db))

	targets := make([]int, 0, len(clusters))
	for i := range clusters {
		if approxClusterTarget(clusters[i]) {
			targets = append(targets, i)
		}
	}
	if len(targets) == 0 {
		return 0
	}
	if len(targets) > maxOnlineVerifyPerRun {
		targets = targets[:maxOnlineVerifyPerRun]
	}

	verified := 0
	for n, ci := range targets {
		// 任务级暂停/取消检查点
		if err := OfflineTaskCheckpoint(); err != nil {
			log.Printf("%s [dedup-verify] 任务被中断，联网复核提前结束（已复核 %d 组）", dlWarnTag, n)
			break
		}
		cl := &clusters[ci]
		if onProgress != nil {
			onProgress(n+1, len(targets), cl.TitleKey, "联网复核（E 站标题反查）")
		}
		q := buildSearchTitle(cl.TitleKey)
		if q == "" {
			continue
		}
		res, err := ehService.FetchGalleryList(account, SearchParams{Keyword: q}, ehSetting)
		if err != nil || res == nil || len(res.Comics) == 0 {
			cl.Reason += "；联网复核未确认（E 站无搜索结果，请人工确认）"
			log.Printf("%s [dedup-verify] 簇 %q 反查无结果（%v）", dlWarnTag, cl.TitleKey, err)
			ehRateLimiter.Mark(false)
			ehRateLimiter.Wait()
			continue
		}
		if matchOnlineCandidate(cl.TitleKey, res.Comics) {
			cl.Reason += "；**联网复核通过**（E 站存在同标题的同作品条目）"
			if cl.Confidence == "medium" {
				cl.Confidence = "high"
			}
			verified++
		} else {
			cl.Reason += "；联网复核未确认（E 站结果标题不一致，可能为不同作品或跨语言翻译标题）"
		}
		ehRateLimiter.Mark(true)
		ehRateLimiter.Wait()
	}
	log.Printf("%s [dedup-verify] 联网复核完成：复核 %d 组，通过 %d 组", dlLogTag, len(targets), verified)
	return verified
}
