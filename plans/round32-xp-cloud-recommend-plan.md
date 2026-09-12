# Round32：XP 词云（阶段一）+ 本地偏好推荐（阶段二）

> 立项日期：2026-09-12 · 目标版本：v2.1.0
> 上游材料：`JHentai/词云功能文档.md`（Gemini 讨论稿，本机测试目录，不入库）
> 关联模块：本地排行榜（`OfflineToplist.vue`）、随机抽卡（`RandomView.vue` + `handlers/random.go`）、Tag 双轨三态维护（`tag_maintain.go`）

---

## 一、目标与阶段划分

### 阶段一：XP 词云

把本地排行榜从「只有阅读次数 Top25」扩展为 **「📊 阅读榜 | ☁️ XP 词云」双视图**：

- 两个口径：**库藏词云**（搜集广度，每本计 1）与 **阅读词云**（真实消耗，按阅读信号加权）
- 三段式分组：**核心 XP**（female/male/mixed/other）· **角色原作**（character/parody）· **其他**（location 及未知命名空间）
- 画师/社团（artist/group）不做词云，剥离为 **Top 20 列表**（长尾分散，词云排版杂乱）
- 语言/重分类（language/reclass）全程排除

### 阶段二：本地偏好推荐

在随机抽卡页新增 **「🎲 纯随机 | ✨ 偏好推荐」卡池模式**：

- 推荐**只作用于本地库**：离线部分按词云权重加权采样
- **在线部分保持纯随机**，前端明确标注（决策 D1）
- 推荐参数暴露给用户（决策 D2）

阶段顺序不可颠倒：推荐复用阶段一的权重体系与统计表。

---

## 二、设计依据（2026-09-12 实测数据）

对两个真实库（`Server/manga.db` 3418 本、`JHentai/DataBase/manga.db` 3792 本）直接查库实测：

| 指标 | 实测值 | 对设计的影响 |
| --- | --- | --- |
| `online_tags` 填充率 | **100%**（3418/3418、3792/3792） | tag 数据完整，无需补抓 Online Tag |
| 平均 tag 数 | ~19 / 本（全库约 7.2 万 tag 实例） | 词云候选词量充足，Top 120 展示可行 |
| namespace 分布 | female 40318 · language 6170 · male 5252 · character 5205 · parody 3999 · artist 3887 · other 3650 · group 1654 · mixed 1546 · location 26 · reclass 3 | female 占 56%，**混排必然淹没长尾 → 必须分段** |
| `read_count > 0` | **67 / 3418（2%）**，最高 9 次 | 阅读侧单靠 readCount 不可用 → 多信号融合（决策 D3） |
| 离线历史 / 个人评分 / 在线收藏 / 在线历史 | 48 / 3 / 200 / 200 条 | 评分极稀疏，主力为 readCount + 离线历史 |
| `users` | 1 | 阅读侧仍按 `user_id` 设计，避免多用户串味 |

**关键结论**：库藏侧数据质量高、可直接支撑词云；阅读侧稀疏，必须多信号融合并**在界面上暴露覆盖率**，避免用户误读为"我的 XP 就这些"。

---

## 三、决策记录

| 编号 | 决策点 | 结论 |
| --- | --- | --- |
| **D1** | 推荐是否覆盖在线 | **只做本地**。在线部分保持纯随机，前端明确标注「在线部分为纯随机」 |
| **D2** | 推荐参数是否暴露 | **暴露给用户**（抽卡页推荐参数区：偏好侧重 θ / 探索率 ε / 温度 T / 排除已读 / 排除书架已有） |
| **D3** | 阅读侧权重隔离 | **阅读侧按 `user_id` 隔离，库藏侧全局共享** |
| **D4** | 词云与推荐的落点 | 词云**并入本地排行榜页**（页内 Segmented 切换）；推荐**作为随机页的卡池模式**（复用全部抽卡 UI，不新增页面/路由） |
| **D5** | 统计计算位置 | **全部后端**，权重落库缓存（新增统计表）+ 增量更新，附带全量重建兜底 |
| **D6** | 落地节奏 | 表结构与即时聚合先行，增量差分随后逐个触发点接入（避免阶段一被基础设施拖住） |
| **D7** | `other` 命名空间归属 | **归「其他」分组**（元信息型 tag：马赛克修正/无修正/全彩/渣翻等），核心 XP 只留 female/male/mixed（2026-09-12 实施） |

---

## 四、统一权重模型（词云与推荐的唯一公式源）

### 4.1 有效 tag 口径

与详情页展示完全一致，复用 `services.MergeTags`：

```
有效 tags(c) = MergeTags(onlineTags, offlineAddTags, offlineRemoveTags)
              若三态全空 → 回退 parseRawTags(c.Tags)
```

### 4.2 命名空间分组

| 分组 | 命名空间 | 用途 |
| --- | --- | --- |
| `core` | female · male · mixed · other | 核心 XP 词云 |
| `ip` | character · parody | 角色/原作词云 |
| `misc` | location 及其他未知命名空间 | 「其他」词云 |
| `artist` | artist · group | 不进词云 → Top 列表 |
| — | language · reclass | 全程排除 |

### 4.3 每本信号

**库藏侧**：`lib(c) = 1`

**阅读侧（按用户）**：

```
r(c) = 0.60 · log1p(readCount)
     + 0.25 · exp(-Δdays / 90)          // Δdays = now - 离线历史 lastReadAt，无记录则该项为 0（半衰期 90 天，反映 XP 漂移）
     + 0.15 · (myRating - 1) / 4        // 个人评分 1-5 → 0~1，未评分为 0
```

三项分别对全体库做 max 归一后加权（保证量纲一致）。

### 4.4 tag 权重（按文档的稀释公式）

```
libWeight(t)  = Σ_{c ∋ t}  lib(c) / |tags(c)|      // 防止含大量泛化标签的单本淹没冷门核心 tag
readWeight(t) = Σ_{c ∋ t}  r(c)   / |tags(c)|
```

查询时归一：`libNorm(t) = libWeight(t) / max(libWeight)`，`readNorm(t)` 同理。

**推荐融合权重**：

```
w(t)  = (1 - θ) · libNorm(t) + θ · readNorm(t)        // θ 默认 0.6；readWeight 全零时自动退化 θ = 0
w'(t) = w(t) · log(1 + N / (1 + comicCount(t)))       // 泛化抑制：压低 female:big breasts 这类全库高频词
```

**词云字号映射**：

```
fontSize = 12 + 24 · (log(w) - log(wMin)) / (log(wMax) - log(wMin))
```

### 4.5 推荐打分与采样

```
score(c) = Σ_{t ∈ tags(c)} w'(t) / |tags(c)|^γ        // γ 默认 0.5，抑制 tag 多的本子刷分
p(c)     ∝ exp(score(c) / T)                          // 温度采样，T 默认 0.5
ε 概率    → 纯均匀随机（探索率默认 0.15，防 XP 固化）
```

**多样性**：单轮抽取中，同一 `parody` 或 `artist` 最多 2 本；同一本不重复。

**硬约束继承**：现有抽卡过滤器全部继续生效（分类 / 页数 / 评分 / 语言 / 负向排除 / 仅已下载）。

---

## 五、阶段一：XP 词云

### 5.1 数据层（`backend/internal/models/xp_stat.go`）

```go
// XpComicStat 单本漫画的 XP 贡献快照（增量差分的基准，按 用户×漫画）
type XpComicStat struct {
    UserID     uint      `gorm:"primaryKey" json:"userId"`
    ComicID    string    `gorm:"primaryKey" json:"comicId"`
    LibWeight  float64   `json:"libWeight"`   // 库藏侧信号（默认 1）
    ReadWeight float64   `json:"readWeight"`  // 阅读侧信号（归一前原始值）
    TagCount   int       `json:"tagCount"`    // 有效 tag 数
    TagsHash   string    `json:"tagsHash"`    // 有效 tag 集合指纹：变化才重算
    UpdatedAt  time.Time `json:"updatedAt"`
}

// XpTagStat tag 聚合权重（按 用户×namespace×key）
type XpTagStat struct {
    UserID     uint      `gorm:"primaryKey" json:"userId"`
    Namespace  string    `gorm:"primaryKey" json:"namespace"`
    TagKey     string    `gorm:"primaryKey" json:"tagKey"`
    LibWeight  float64   `json:"libWeight"`
    ReadWeight float64   `json:"readWeight"`
    ComicCount int       `json:"comicCount"`
    UpdatedAt  time.Time `json:"updatedAt"`
}

// XpMeta 统计元信息（单例 ID=1）
type XpMeta struct {
    ID             uint  `gorm:"primaryKey;default:1" json:"id"`
    SchemaVersion  int   `json:"schemaVersion"`
    FormulaVersion int   `json:"formulaVersion"` // 公式变更 → 自动全量重建
    LastRebuildAt  int64 `json:"lastRebuildAt"`
    Dirty          bool  `json:"dirty"` // 增量写入失败/数据迁移 → 下次查询自动重建
}
```

`database/db.go` 的 `AutoMigrate` 追加这三张表。

### 5.2 服务层（`backend/internal/services/xp_cloud.go`）

| 函数 | 职责 |
| --- | --- |
| `EffectiveTags(comic) []string` | 合并三态 tag（复用 `MergeTags`），三态全空回退 `Tags` |
| `ComposeComic(comic, userID) (stat, deltas)` | 计算单本贡献（库藏 + 阅读信号 → tag 增量表） |
| `RecomposeComic(comicID, userID) error` | 差分更新：读旧 `XpComicStat` → 聚合表减旧 → 加新 → 写回（**事务内，幂等**） |
| `RecomposeAll(userID) error` | 该用户全量重建（清表 → 遍历库 → 重建） |
| `EnsureFresh() error` | `FormulaVersion` 不匹配 / `Dirty` → 自动 `RecomposeAll` |
| `TagWeights(userID) map[string]float64` | 输出 `w'(t)` 权重表（**推荐复用**，带内存缓存 + 版本失效） |
| `Query(group, view, limit, userID)` | 词云查询：按分组/视图取词条 + artist Top 列表 + 覆盖率元信息 |

- tag 展示名走既有 `GlobalTagEngine.TranslateTags`（中文翻译复用，零额外数据源）
- 归一化在查询时计算（表内只存原始值，改公式不需重写数据，只需 bump `FormulaVersion`）

### 5.3 接口（`backend/internal/handlers/xp_cloud.go` + `router.go`）

```
GET  /api/v1/offline/xp-cloud?group=core|ip|misc&view=library|reading&limit=120
     → {
         tags:   [{ namespace, key, name, weight, libWeight, readWeight, comicCount }],
         artists:[{ key, name, comicCount, readCount }],
         meta:   { libComics, readComics, totalComics, updatedAt, formulaVersion }
       }
POST /api/v1/offline/xp-cloud/rebuild      // 仅管理员：手动全量重算
```

- `GET` 挂登录组（只读），`POST rebuild` 挂 `admin` 组
- `meta.readComics` 用于前端「阅读信号覆盖 X / Y 本」提示

### 5.4 增量触发点（每处一行调用，全部幂等）

| 触发场景 | 接入位置 |
| --- | --- |
| 漫画新增/更新（扫描、下载入库） | `services/scanner.go`、`download.go` 入库回调 |
| 漫画删除 | `handlers/comic.go` `DeleteOfflineComic` |
| Tag 每日刷新 / 每周写回 | `services/tag_maintain.go`（单本粒度，随刷新循环） |
| 单本 tag 增删（详情页编辑） | `services/tag_maintain.go` `editTags` 或 `handlers/tag_maintain.go` |
| 阅读次数自增 | `handlers/comic.go` `RecordComicClick` |
| 离线历史新增（`lastReadAt` 变化） | `handlers/library.go` `AddHistory` |
| 个人评分增改删 | `handlers/library.go` `SetComicRating` / `DeleteComicRating` |
| 兜底 | `FormulaVersion` 变更 / `Dirty` / 管理员手动重建 |

### 5.5 前端（阶段一）

| 文件 | 内容 |
| --- | --- |
| `src/api/xpCloud.ts` | 接口封装 + 响应类型（类型补进 `types/comic.ts`） |
| `src/utils/tagColor.ts` | 从 `TagChip.vue` 抽取 `namespace → 配色`，词云与胶囊共用，避免两套色板 |
| `src/components/XpWordCloud.vue` | Canvas 自研螺旋布局（阿基米德螺线 + 矩形碰撞检测，**不引新依赖**）：DPR 适配、主题色、hover tooltip（权重/本数）、点击命中、容器 resize 重排、词条上限保护 |
| `src/components/ArtistTopList.vue` | artist/group Top 20（名称 + 本数 + 阅读次数），点击跳对应搜索 |
| `src/views/offline/OfflineToplist.vue` | 顶部 Segmented「📊 阅读榜 / ☁️ XP 词云」；词云面板含 视图切换（库藏/阅读）× 分组切换（核心XP/角色原作/其他）、覆盖率提示、管理员「重算统计」入口 |

**交互**：词条点击复用 `TagChip` 既有快捷搜索行为（离线跳 `/offline/home` 带 `f_search` 语义关键词）。

**空态/低覆盖**：`meta.readComics / meta.totalComics < 5%` 时，阅读视图顶部提示「阅读信号仅覆盖 X 本，建议以库藏视图为主」。

---

## 六、阶段二：本地偏好推荐

### 6.1 后端（`services/recommend.go`）

| 函数 | 职责 |
| --- | --- |
| `LoadWeights(userID) (map[string]float64, error)` | 取 `w'(t)`（阶段一 `XpCloudService.TagWeights` 复用） |
| `ScoreComic(comic, weights) float64` | `Σ w'(t)/|tags|^γ` |
| `Sample(candidates, scores, T, ε) []comic` | 温度 softmax 轮盘赌 + ε 探索混合 |
| `Diversify(picked) []comic` | 同 parody/artist 限流 + 去重 |
| `Recommend(userID, opts, filters) ([]comic, error)` | 编排：候选查询（继承硬约束）→ 打分 → 采样 → 多样性 |

- 候选池与权重表加**内存缓存**（含失效版本号），避免每次抽卡重扫 3400 行
- **降级路径**：权重表为空 / 本地库为空 / 全零权重 → 自动退化为纯随机，并在响应 `warning` 中说明

### 6.2 接口扩展（`handlers/random.go`，向后兼容）

`GET /comics/random` 新增参数（**默认值即旧行为，老路径零改动**）：

| 参数 | 默认 | 说明 |
| --- | --- | --- |
| `mode` | `random` | `random` \| `recommend` |
| `recoTheta` | `0.6` | 偏好侧重（0=纯库藏，1=纯阅读） |
| `recoExplore` | `0.15` | 探索率 ε（0~0.5） |
| `recoTemp` | `0.5` | 采样温度 T（0.2~2） |
| `recoExcludeRead` | `false` | 排除读过的（`read_count > 0`） |
| `recoExcludeShelf` | `false` | 排除已在书架的本子 |

- `RandomComicItem` 增加 `matchedTags []string`（命中理由 Top 3）与 `score float64`
- **在线部分**：`mode=recommend` 时仍走 `FetchRandomGalleryList` 纯随机，响应中 `onlineRandom: true` 供前端标注（D1）

### 6.3 前端（阶段二）

| 文件 | 内容 |
| --- | --- |
| `src/stores/recommendSettings.ts` | 推荐参数持久化（localStorage，风格同 `preferenceSettings`） |
| `src/views/RandomView.vue` | 顶部卡池模式 Segmented「🎲 纯随机 / ✨ 偏好推荐」；推荐模式展示推荐参数区（θ / ε / T / 排除已读 / 排除书架）；范围含在线时显示「✈️ 在线部分为纯随机」标注；结果卡片显示「命中：tagA tagB」理由徽标 |
| `src/types/comic.ts` · `src/api/comic.ts` | `RandomComicParams` 与响应类型扩展 |

---

## 七、验证与验收

**后端**：`cd backend && go test ./...`（新增 `xp_cloud_test.go`、`recommend_test.go`）
**前端**：`npm run type-check`、`npm run lint`

| 验收项 | 标准 |
| --- | --- |
| 词云数值正确性 | Top 20 词条与直接 SQL 聚合（`json_each(online_tags)`）逐项比对一致 |
| 增量与重建一致 | 对同一库：增量路径结果 == 全量 `RecomposeAll` 结果（测试断言） |
| 差分幂等 | 同一本连续 `RecomposeComic` 多次，聚合表数值不变 |
| 推荐有效性 | 连抽 10 次，命中高权重 tag 比例显著高于纯随机（统计脚本对比） |
| 降级路径 | 权重全零 / 空库 → 自动纯随机 + 提示，不报错 |
| 兼容性 | `mode=random` 行为与改动前逐字节一致（回归既有抽卡用例） |
| 性能 | 词云接口 < 100ms（落库后应达个位数 ms）；推荐抽卡 < 300ms |
| 实机 | 用 `sakuhentai-e2e` 跑词云页切换/点击跳转、推荐连抽与在线标注 |

**收尾**（`AGENTS.md`）：验证 → `git status` / `git diff --stat` → 按模块拆分 Conventional Commits → `git push`。

---

## 八、风险与回退

| 风险 | 应对 |
| --- | --- |
| 阅读信号稀疏（2%） | 多信号融合（D3）+ 覆盖率提示；早期推荐偏库藏侧，θ 用户可调 |
| 差分累积浮点漂移 | `FormulaVersion` 版本化 + 手动/自动全量重建；增量⇄重建一致性由单测守住 |
| 统计表体积 | `XpTagStat` 行数 ≈ 库内唯一 tag 数 × 用户数（数万级），SQLite 无压力 |
| 推荐同质化 | ε 探索率 + 多样性限流 + 温度可调（D2 全部暴露给用户） |
| 抽卡老路径回归 | `mode` 默认 `random`，老路径代码零改动 |
| 阶段一被基础设施拖住 | D6：即时聚合先上，增量差分随后接入；未接入期间由查询时 `Dirty` 兜底 |

---

## 九、任务拆分与提交计划

**阶段一（词云）**

1. `feat(xp): 新增 XP 统计模型与数据库迁移`
2. `feat(xp): XP 词云聚合服务（合并/加权/差分/重建）`
3. `feat(xp): 词云查询与重算接口`
4. `feat(xp): 增量触发点接入（扫描/删除/Tag/阅读/历史/评分）`
5. `feat(xp): 排行榜页新增 XP 词云面板（Canvas 词云 + 画师 Top）`
6. `test(xp): 聚合口径/差分幂等/重建一致性单测`

**阶段二（推荐）**

7. `feat(reco): 本地偏好推荐服务（打分/采样/多样性/降级）`
8. `feat(reco): 随机抽卡接口新增推荐模式与参数`
9. `feat(reco): 随机页卡池模式切换与推荐参数面板`
10. `test(reco): 采样分布/探索率/降级路径单测`

**收尾**：`docs: 更新 PROJECT_TREE 与 README 功能索引`（如涉及）

---

## 十、实施结果与实机验证（2026-09-12）

两个阶段均已实施完成，并在**隔离库副本**（`Server/manga.db` 3418 本 → `Test/pw-bug/xp-verify/`）＋ 本地后端 ＋ Playwright 上完成实机验证。

### 10.1 阶段一（XP 词云）验证结果

| 验证项 | 结果 |
| --- | --- |
| 统计接口耗时 | 首次 686ms（含全量重建 3418 本）；落库后 19ms（库藏）/ 14ms（阅读） |
| 统计规模 | 库藏 3416/3418 本参与统计，阅读信号 67 本（与实测库一致） |
| 词云布局 | 放入 140/140 词、**两两无重叠、无越界**、字号 12~34px（布局快照断言） |
| 交互 | hover tooltip（权重/本数/阅读权重）；点击词条 → 离线首页并带入 `female:"multiple orgasms$"` |
| 分组/视图 | core/ip/misc × 库藏/阅读 切换各触发一次请求，全部 200 |
| 覆盖率提示 | 阅读视图按预期出现（67/3418 < 5%） |
| console error | 词云页 0 条（其余为隔离库缺封面文件、无 E 站凭证、登录前字典请求等环境因素） |

**过程中发现并修复的问题**

1. **标签展示名含图标 markdown**（真 bug）：EH 翻译词典 `name` 形如 `![长筒袜图标](https://…)长筒袜`，词云会画出整段 URL → 新增 `services.CleanTagDisplayName`（词条与画师名统一清洗，纯图标退回 alt 文本），并补单测。
2. **round 编号冲突**：立项时按 `plans/` 最大编号取 Round30，但 `OnlineSidebar.vue` 已占用 Round30（邮件列表式排版）、`Test/` 下另有 round31 脚本 → 全量改号为 **Round32**（含计划文档改名）。

### 10.2 阶段二（本地偏好推荐）验证结果

**量化对比**（`source=offline`，各 15 轮 × 8 本；探针＝阅读视图 Top 词中覆盖率 < 600 本的偏好词）

| 卡池 | 命中探针词的本子占比 | 平均 score |
| --- | --- | --- |
| 🎲 纯随机（对照） | 42.5% | n/a |
| ✨ 推荐 θ=0（纯库藏） | 55.8% | 1.592 |
| ✨ 推荐 θ=0.6（默认） | 53.3% | 1.811 |
| ✨ 推荐 θ=1.0（纯阅读） | **65.0%** | 2.073 |

θ 越大（越偏阅读侧信号）命中越高，符合权重模型预期；推荐结果 100% 携带命中理由。

**UI 实机验证：21 项全 PASS** —— 卡池模式切换、默认纯随机（旧行为不变）、推荐参数面板（3 滑块 + 3 开关）、参数即时下发与持久化、命中理由徽标、「在线部分为纯随机」标注（范围含在线时显示 / 仅本地时隐藏）、切回纯随机后无 `mode` 参数且无命中理由、抽卡页 0 条 console error。

**接口契约**：`mode=random`（默认）响应与旧版逐字段一致；`mode=recommend` 返回 `mode` 与 `onlineRandom=true`，每本附 `matchedTags` / `score`；非法参数（如 `recoTheta=abc`）回退默认值不报错。

**附带修复（既有问题）**：随机抽卡的卡片标签此前直接使用旧 `Tags` 字段，与列表/详情的双轨三态合并口径不一致（实测 3418 本中 **1424 本 / 42%** 两字段不同），会导致卡片标签与推荐命中理由对不上、负向排除漏掉「本地新增/已删除」tag → 统一改走 `offlineDisplayTags`（`MergeTags` ＋ 旧数据回退）。

### 10.3 `other` 命名空间归属（2026-09-12 决策并实施）

实测发现 `other` 归入「核心 XP」时，`other:mosaic censorship`（马赛克修正，629 本）等**元信息型 tag** 会挤进核心 XP 前列。珱垣决策：**`other` 改归「其他」分组，核心 XP 只保留 female / male / mixed**。

| 改动点 | 内容 |
| --- | --- |
| 后端分组表 | `services/xp_cloud.go`：`xpCoreNamespaces` 去掉 other；`xpGroupNamespaces["core"]` 改为 female/male/mixed；`xpMiscExcludeNamespaces` 去掉 other（它本身属「其他」） |
| 版本号 | `XpFormulaVersion` 1 → 2 → 首次查询自动全量重建，无需手动操作 |
| 前端映射 | `utils/tagColor.ts` 的 `GROUP_MAP`：`other: 'misc'`（与后端 `XpGroupOf` 对齐） |
| 单测 | 分组断言更新（`other:mosaic censorship` / 裸词 → misc；core 分组不得含 other；misc 需含 other 与 location） |

**实机验证**（隔离库 3418 本）：

- `meta.formulaVersion = 2`（证明自动重建已发生）
- core 分组 200 条中 **other = 0**，命名空间分布 `female:173 / male:23 / mixed:4`
- misc 分组 44 条：`other:39`（马赛克修正 629 本、渣翻 377、变体集 400、无修正 394、系列作品 432、外部广告 236…）+ `location:5`
- 词云 UI 复测 20 项全 PASS，布局快照命名空间分布为 `female, male, mixed`

### 10.4 其他遗留

- **阅读信号稀疏**：`read_count > 0` 仅 67/3418（2%），阅读视图与「偏阅读侧」推荐的效果会随使用自然增强；界面已提示覆盖率。
- 未做项（原计划中的可选部分）：词云导出、推荐权重明细展示、在线侧偏好推荐（D1 已定不做）。
- 测试方法备注：词条点击的实机断言已从「像素扫描定位」改为「布局快照定位」（前者在词条分布变化后会落到非词条区域，产生假阴性）。

---

## 十一、词云视觉改版（2026-09-12）

### 11.1 问题与诊断

珱垣反馈词云「看着有点拉胯」并提供两张实机截图。逐条核对（含对一份第三方分析意见的裁定）：

| 现象 | 裁定 | 依据 |
| --- | --- | --- |
| 「不是碰撞算法，是强行居中的行排版」 | ❌ **归因错误** | 实现本就是螺旋 + 空间网格碰撞；旧截图里也不存在行排版，是把「密度过高」误判成了文本段落 |
| 碰撞盒零留白 → 词与词紧贴、密不透风 | ✅ 主因 | 碰撞盒 = 文字外接矩形，padding = 0，螺旋填到无处可放为止 |
| 长双语复合标签拉成长横条 | ✅ | ip 分组 140 词条中 19 条 >12 字、12 条 >20 字、6 条 >30 字，最长 52 字（词典无中文译名时回退拉丁转写名） |
| 配色只有一两种亮色平铺、无景深 | ✅ | 配色按命名空间固定色，而核心 XP 中 female 占约 88% → 视觉近乎单色 |
| 混入彩色 Emoji 打碎字形统一 | ✅（**笔者的初次判断有误，此处更正**） | emoji 来自 EH 翻译词典的中文名本身（`精灵🧝‍♀️`、`马🐴`、`时间停止⏱️`），此前只清洗了 markdown 图标 |
| 「改用 d3-cloud 等词云库」 | ❌ 不采纳 | 算法无问题，症结在留白参数、长词治理与配色梯度；引入依赖违背项目依赖极简 |

### 11.2 决策记录

| 编号 | 决策点 | 结论 |
| --- | --- | --- |
| **D8** | 长词治理 | `|` 双语取**短侧** → 前端按 **14 字**截断（英文按词边界）→ 超过 **20 字**的词条不参与词云（**B1**） |
| **D9** | 词条角度 | **全水平**，取消竖排（原 C1(b) 方案在大词上形成「柱子」抢中心，且与相邻词视觉粘连） |
| **D10** | 配色方向 | 命名空间主色 + 权重明度/透明梯度（**C2(a)**） |
| **D11** | 密度控件 | 保留「词数」滑块（默认 100）与「高级参数」面板，参数持久化（**C3**） |
| **D12** | 画布高度 | 桌面 **520px** / 移动 **360px**（原 400 / 300） |
| **D13** | 译名过滤 | 「仅显示中文译名」**默认开启**（过滤无译名的拉丁转写长名） |

### 11.3 实施方案

**后端**（`services/xp_cloud.go`）：`CleanTagDisplayName` 扩展为四步清洗——markdown 图标剥离 → **emoji/ZWJ/变体选择符剥离** → **双语取短侧** → 空白压缩。展示名清洗后，前端的截断/过滤才有效。

**前端**：
- `utils/tagColor.ts`：命名空间 HSL 主色 + `xpTagColor(ns, t, theme)` 权重梯度；**深/浅两套色板**（浅色底用低明度深色，见 11.4 的实机 bug）
- `components/XpWordCloud.vue`：碰撞盒留白（`lineRatio 1.45` / `padX 0.3`）、**归一化椭圆螺旋**（椭圆边界取代矩形填满）、字号分段（前 10 名高档区间）+ **长词降档**（>16 字 ×0.55 / >12 ×0.75 / >9 ×0.9）、无竖排、主题感知
- `stores/xpCloudSettings.ts`（新增）：词数/字号区间/行距系数/字间距/截断字数/超长过滤/仅中文译名，localStorage 持久化
- `views/offline/OfflineToplist.vue`：⚙️ 词云参数面板（7 滑块 + 1 开关 + 恢复默认）、布局统计行（数据源/放入/超限丢弃/超长排除/无译名排除）、高度 520/360

### 11.4 验收样板先行的施工方式

先在 `Test/pw-bug/xp-mock/` 用真实库数据做了**可独立打开的单文件样板**（`wordcloud-mock.html`，内嵌 6 组真实词云数据，含「对照：旧版参数」一键切换与全部参数滑块），珱垣确认样板后才动工，实现严格比照样板参数。

样板阶段自查发现并修正的两点：**① 大词竖排变「柱子」抢中心**（→ 收紧为仅中小号竖排，最终按 D9 全水平）；**② ip 分组容量暴跌**（100 词只放下 49 条，根因是拉丁转写长名 + 大字号横排 → 引入长词降档，修后 64 条）。

### 11.5 实机验证结果：35 项全 PASS

| 组别 | 关键结论 |
| --- | --- |
| 核心 XP / 角色原作 / 其他（库藏） | 词条**零重叠、零越界、零 emoji、展示名不超截断上限、全水平盒**；核心 XP 100 词全部放下；ip 分组 69 条 |
| 阅读视图 | 同上断言全通过 |
| 参数面板 | 7 滑块 + 1 开关齐备；词数降至 40 → 放入 40；行距 1.9 → 放入数由 69 降为 54（留白参数生效）；恢复默认可用 |
| 译名过滤开关 | 切换即时重排 |
| 移动端（UA=Mobi） | 画布高度 **360**、零重叠、零 emoji |
| **主题适配** | 浅色主题平均明度 **42.3%**（深色字）、深色主题 **70.3%**（亮色字） |

**施工中发现并修复的实机 bug（浅色主题）**：移动端首次截图暴露——原配色是为深色底设计的亮色（l 60~86），在浅色主题的白底上几乎看不清。修复：`tagColor.ts` 增加浅色主题色板（l 26~44）并按权重反向调明度；组件通过 `readXpTheme()` 读取 `<html data-theme>`，并用 `MutationObserver` 监听主题切换即时重绘。

**测试方法修正**：设备类别由 UA/触摸点数决定（与视口宽度无关），移动端验证改用移动 UA 的新 context；`assertLayout` 的截断上限修正为 `truncChars + 1`（省略号占 1 字符）。

产物位置（均在被 gitignore 的测试目录）：样板 `Test/pw-bug/xp-mock/wordcloud-mock.html`、截图 `Test/pw-bug/xp-mock/shots/`（`actual-*` 实机、`mock-*` 样板）。
