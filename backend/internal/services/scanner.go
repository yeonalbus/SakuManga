package services

import (
	"SakuManga/internal/database"
	"SakuManga/internal/models"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

func generateID(path string) string {
	hash := md5.Sum([]byte(path))
	return hex.EncodeToString(hash[:])
}

// ─────────────────────────────────────────────────────────────
// 扫描进度（内存态，供前端轮询 /scan-paths/:id/scan/progress）
// ─────────────────────────────────────────────────────────────

// ScanPhase 扫描阶段
type ScanPhase string

const (
	ScanPhaseCounting    ScanPhase = "counting"    // 统计待扫描条目
	ScanPhaseScanning    ScanPhase = "scanning"    // 正在逐条处理
	ScanPhaseReconciling ScanPhase = "reconciling" // Round54：扫描完成后的失效对账（仅全量）
	ScanPhaseDone        ScanPhase = "done"        // 完成
)

// ScanProgress 单条扫描路径的进度快照结构（线程安全）
type ScanProgress struct {
	mu           sync.Mutex
	PathID       string    `json:"pathId"`
	Mode         string    `json:"mode"` // full | incremental
	Phase        ScanPhase `json:"phase"`
	Total        int       `json:"total"`        // 待扫描条目总数
	Processed    int       `json:"processed"`    // 已处理条目数
	Found        int       `json:"found"`        // 实际入库的漫画数
	Skipped      int       `json:"skipped"`      // 跳过的条目数（增量模式已存在等）
	CurrentTitle string    `json:"currentTitle"` // 当前正在处理的条目
	Error        string    `json:"error,omitempty"`
	Done         bool      `json:"done"`
	StartedAt    int64     `json:"startedAt"`
	FinishedAt   int64     `json:"finishedAt,omitempty"`
	ComicCount   int       `json:"comicCount,omitempty"` // 完成后的入库数量（同 Found）
	// D17：跳过项统计（Warnings 为前若干条摘要，供前端告警展示）
	SkippedEmpty       int      `json:"skippedEmpty,omitempty"`       // 直接子目录递归图片数为 0
	SkippedUnsupported int      `json:"skippedUnsupported,omitempty"` // 不可读归档（rar/7z 等）
	SkippedExisting    int      `json:"skippedExisting,omitempty"`    // 增量模式已存在
	SkippedGid         int      `json:"skippedGid,omitempty"`         // 同 gid 冲突
	Failed             int      `json:"failed,omitempty"`             // 写库失败
	Warnings           []string `json:"warnings,omitempty"`

	// ── Round54：失效对账阶段（全量扫描完成后；增量不做）──
	PathMissing        bool `json:"pathMissing,omitempty"`        // 扫描根路径不存在（整条路径判失效）
	ReconcileTotal     int  `json:"reconcileTotal,omitempty"`     // 对账待检查记录数
	ReconcileProcessed int  `json:"reconcileProcessed,omitempty"` // 对账已检查记录数
	MissingCount       int  `json:"missingCount,omitempty"`       // 判定失效的记录数
	CandidateCount     int  `json:"candidateCount,omitempty"`     // 恢复候选数
	FallenCount        int  `json:"fallenCount,omitempty"`        // 阵亡条目数
	HealedCount        int  `json:"healedCount,omitempty"`        // 自愈清除条目数
}

func newScanProgress(pathID, mode string) *ScanProgress {
	return &ScanProgress{
		PathID:    pathID,
		Mode:      mode,
		Phase:     ScanPhaseCounting,
		StartedAt: time.Now().UnixMilli(),
	}
}

// Snapshot 返回一个安全的浅拷贝快照指针，供外部读取（JSON 序列化）。
// 返回指针而非值：ScanProgress 内含 sync.Mutex，值拷贝会触发 go vet
// 「copies lock value」告警（append/return 拷贝锁）。
func (p *ScanProgress) Snapshot() *ScanProgress {
	if p == nil {
		return &ScanProgress{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return &ScanProgress{
		PathID:       p.PathID,
		Mode:         p.Mode,
		Phase:        p.Phase,
		Total:        p.Total,
		Processed:    p.Processed,
		Found:        p.Found,
		Skipped:      p.Skipped,
		CurrentTitle: p.CurrentTitle,
		Error:        p.Error,
		Done:         p.Done,
		StartedAt:    p.StartedAt,
		FinishedAt:   p.FinishedAt,
		ComicCount:   p.ComicCount,
		SkippedEmpty: p.SkippedEmpty, SkippedUnsupported: p.SkippedUnsupported,
		SkippedExisting: p.SkippedExisting, SkippedGid: p.SkippedGid, Failed: p.Failed,
		Warnings: append([]string(nil), p.Warnings...),
		// Round54：对账阶段字段
		PathMissing: p.PathMissing, ReconcileTotal: p.ReconcileTotal,
		ReconcileProcessed: p.ReconcileProcessed, MissingCount: p.MissingCount,
		CandidateCount: p.CandidateCount, FallenCount: p.FallenCount, HealedCount: p.HealedCount,
	}
}

// IsDone 线程安全地判断扫描是否结束
func (p *ScanProgress) IsDone() bool {
	if p == nil {
		return true
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.Done
}

func (p *ScanProgress) setPhase(phase ScanPhase) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Phase = phase
}

func (p *ScanProgress) setTotal(total int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Total = total
}

// setSkipBreakdown 写入跳过项分原因统计与告警摘要（D17；Warnings 已由调用方截断）
func (p *ScanProgress) setSkipBreakdown(stats ScanStats, warnings []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.SkippedEmpty = stats.SkippedEmpty
	p.SkippedUnsupported = stats.SkippedUnsupported
	p.SkippedExisting = stats.SkippedExisting
	p.SkippedGid = stats.SkippedGid
	p.Failed = stats.Failed
	p.Warnings = append([]string(nil), warnings...)
}

func (p *ScanProgress) setCurrentTitle(title string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.CurrentTitle = title
}

func (p *ScanProgress) incProcessed() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Processed++
}

func (p *ScanProgress) incFound() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Found++
}

func (p *ScanProgress) incSkipped() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Skipped++
}

// ── Round54：失效对账阶段进度 ──

func (p *ScanProgress) setPathMissing(missing bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.PathMissing = missing
}

func (p *ScanProgress) setReconcileTotal(total int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ReconcileTotal = total
}

func (p *ScanProgress) incReconcileProcessed() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ReconcileProcessed++
}

// setReconcileResult 写入对账统计（路径缺失标记已在 PathMissing 单独维护）
func (p *ScanProgress) setReconcileResult(res ScanReconcileResult) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.MissingCount = res.Missing
	p.CandidateCount = res.Candidates
	p.FallenCount = res.Fallen
	p.HealedCount = res.Healed
}

func (p *ScanProgress) finish(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Done = true
	p.FinishedAt = time.Now().UnixMilli()
	p.Phase = ScanPhaseDone
	p.ComicCount = p.Found
	if err != nil {
		p.Error = err.Error()
	}
}

// ─────────────────────────────────────────────────────────────
// 扫描口径（D1/D2/D4）
// ─────────────────────────────────────────────────────────────

// ScanSkipReason 扫描跳过原因
type ScanSkipReason string

const (
	SkipReasonEmpty       ScanSkipReason = "empty"               // 直接子目录递归图片数为 0（D2）
	SkipReasonUnsupported ScanSkipReason = "unsupported_archive" // 不可读归档（rar/7z，D4）
)

// ScanSkip 被跳过的条目（供告警展示）
type ScanSkip struct {
	Path   string
	Reason ScanSkipReason
}

// ScanCandidate 待扫描条目（一个本子）
type ScanCandidate struct {
	Path  string
	IsDir bool
}

// isHiddenEntry 名称以 "." 开头的隐藏项（D1 全局忽略：.saku_trash / .git 等）
func isHiddenEntry(name string) bool {
	return strings.HasPrefix(name, ".")
}

// CollectScanCandidates 按 D1/D2/D4 口径收集扫描候选。
//
// includeSubfolders=true：扫描路径根自身直接含图 → 根即一本（不再切子目录）；
// 否则根的直接子目录各一本（递归图片数为 0 → 跳过），根下直接归档各一本。
// includeSubfolders=false：仅扫描路径根自身视为单本（递归图片数为 0 → 跳过）。
//
// 遍历一律跳过名称以 "." 开头的隐藏目录/文件；rar/7z 记为 unsupported_archive。
func CollectScanCandidates(rootPath string, includeSubfolders bool) ([]ScanCandidate, []ScanSkip, error) {
	if _, err := os.Stat(rootPath); err != nil {
		return nil, nil, err
	}
	var cands []ScanCandidate
	var skips []ScanSkip
	addSkip := func(path string, reason ScanSkipReason) {
		skips = append(skips, ScanSkip{Path: path, Reason: reason})
	}

	// 根自身是否直接含图（隐藏项不参与）
	rootHasImage := false
	if entries, err := os.ReadDir(rootPath); err == nil {
		for _, e := range entries {
			if e.IsDir() || isHiddenEntry(e.Name()) {
				continue
			}
			if IsImage(e.Name()) {
				rootHasImage = true
				break
			}
		}
	}

	if !includeSubfolders {
		n, err := countImagesRecursive(rootPath)
		if err != nil || n == 0 {
			addSkip(rootPath, SkipReasonEmpty)
			return nil, skips, nil
		}
		return []ScanCandidate{{Path: rootPath, IsDir: true}}, skips, nil
	}

	if rootHasImage {
		// D1：根自身含图 → 根一本，子目录不再单独切本
		return []ScanCandidate{{Path: rootPath, IsDir: true}}, skips, nil
	}

	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return nil, nil, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if isHiddenEntry(name) {
			continue
		}
		full := filepath.Join(rootPath, name)
		if entry.IsDir() {
			n, err := countImagesRecursive(full)
			if err != nil || n == 0 {
				addSkip(full, SkipReasonEmpty)
				continue
			}
			cands = append(cands, ScanCandidate{Path: full, IsDir: true})
			continue
		}
		switch {
		case IsSupportedArchive(name):
			cands = append(cands, ScanCandidate{Path: full, IsDir: false})
		case IsUnsupportedArchive(name):
			addSkip(full, SkipReasonUnsupported)
		}
	}
	return cands, skips, nil
}

// countImagesRecursive 目录内图片数量（= 递归页表长度，与阅读器同一口径）
func countImagesRecursive(dir string) (int, error) {
	images, err := listImagesRecursive(dir)
	if err != nil {
		return 0, err
	}
	return len(images), nil
}

// countImagesInZip 归档内图片数量（= 归档页表长度）
func countImagesInZip(zipPath string) (int, error) {
	images, err := listImagesInZip(zipPath)
	if err != nil {
		return 0, err
	}
	return len(images), nil
}

// ─────────────────────────────────────────────────────────────
// 单个条目入库
// ─────────────────────────────────────────────────────────────

// SaveComicResult 单条入库结果（D17：扫描统计按原因细分）
type SaveComicResult int

const (
	// SaveComicCreated 已写入（新增或按白名单更新）
	SaveComicCreated SaveComicResult = iota
	// SaveComicSkippedExisting 增量模式：路径已存在，跳过
	SaveComicSkippedExisting
	// SaveComicSkippedGid 同 gid 冲突（已存在文件夹形态时跳过压缩包）
	SaveComicSkippedGid
	// SaveComicFailed 写库失败
	SaveComicFailed
)

// saveComic 兼容包装：只有 Created 视为成功。
func saveComic(localPath string, isDir bool, incremental bool, scanPathID string) bool {
	return saveComicWithResult(localPath, isDir, incremental, scanPathID) == SaveComicCreated
}

// saveComicWithResult 处理单个候选条目：解析元数据并入库，返回细分结果（D17）。
// scanPathID 为来源额外扫描路径 ID；空字符串 = 下载导入（问题3 来源识别）。
func saveComicWithResult(localPath string, isDir bool, incremental bool, scanPathID string) SaveComicResult {
	title := filepath.Base(localPath)
	if !isDir {
		title = strings.TrimSuffix(title, filepath.Ext(title))
	}

	// 🎯 1. 尝试解析本地/压缩包内部的 metadata、ametadata 以及 ComicInfo.xml
	var meta *ParsedMetadata
	if isDir {
		meta = ParseDirMetadata(localPath)
	} else {
		meta = ParseZipMetadata(localPath)
	}

	// 优先使用元数据中的标题与分类（问题2：titleJpn 单独保留，供日语优先显示）
	if meta.Title != "" {
		title = meta.Title
	}
	titleJpn := meta.TitleJpn
	category := "Doujinshi"
	if meta.Category != "" {
		category = meta.Category
	}

	// 默认兜底标签
	tags := meta.Tags
	if len(tags) == 0 {
		tags = []string{"本地扫描"}
	}
	tagsJSON, _ := json.Marshal(tags)

	// D3：页数 = 递归页表长度（与阅读器一致）
	pageCount := 0
	var fileSize int64
	fileModifiedAt := time.Time{}
	if isDir {
		entries, _ := os.ReadDir(localPath)
		for _, entry := range entries {
			if entry.IsDir() || isHiddenEntry(entry.Name()) {
				continue
			}
			if fi, err := entry.Info(); err == nil {
				fileSize += fi.Size()
				if fi.ModTime().After(fileModifiedAt) {
					fileModifiedAt = fi.ModTime()
				}
			}
		}
		if n, err := countImagesRecursive(localPath); err == nil {
			pageCount = n
		}
		// 目录自身修改时间兜底（问题1）
		if fileModifiedAt.IsZero() {
			if fi, err := os.Stat(localPath); err == nil {
				fileModifiedAt = fi.ModTime()
			}
		}
	} else {
		if n, err := countImagesInZip(localPath); err == nil {
			pageCount = n
		}
		if fi, err := os.Stat(localPath); err == nil {
			fileSize = fi.Size()
			fileModifiedAt = fi.ModTime()
		}
	}

	comicID := generateID(localPath)
	coverURL := "/api/v1/comics/" + comicID + "/cover"

	// 🎯 1.5 增量模式：该路径已在库中 → 默认跳过；
	//    但若已有记录缺 GID/Token（此前无 sidecar 元数据），重新解析本地元数据补提
	//    （S6：用户补放 metadata/ComicInfo.xml 后再次增量扫描即可自动回填 GID，
	//     使跨路径「同 GID / 父画廊关系」查重自动生效）。
	if incremental {
		var existing models.OfflineComic
		if err := database.DB.Where("local_path = ?", localPath).First(&existing).Error; err == nil {
			if existing.FileMissing {
				_ = database.DB.Model(&existing).Update("file_missing", false).Error
			}
			if existing.GID == "" && meta.GID != "" {
				updates := map[string]interface{}{"g_id": meta.GID}
				if meta.Token != "" {
					updates["token"] = meta.Token
				}
				if meta.ParentGID != "" {
					updates["parent_g_id"] = meta.ParentGID
				}
				if err := database.DB.Model(&existing).Updates(updates).Error; err == nil {
					log.Printf("%s [scan] 增量补提元数据 gid=%s: %q", dlLogTag, meta.GID, localPath)
				}
			}
			// D8：来源不可变——已存在记录不改写 scan_path_id（含增量模式），
			// 避免「下载目录被设为扫描路径」时把来源从「下载」翻成「额外路径」。
			log.Printf("%s [scan] 增量模式跳过已存在路径: %q", dlLogTag, localPath)
			return SaveComicSkippedExisting
		}
	}

	// 🎯 2. gid 查重：同一 gid 已有记录时，优先保留「文件夹」形态（画廊下载），跳过「压缩包」形态（归档）
	if meta.GID != "" {
		var existing models.OfflineComic
		err := database.DB.Where("g_id = ? AND id != ?", meta.GID, comicID).First(&existing).Error
		if err == nil {
			existingIsDir := existing.SourceMode == "gallery" ||
				!strings.HasSuffix(strings.ToLower(existing.LocalPath), ".zip")
			if existingIsDir && !isDir {
				// 已存在文件夹形态（画廊下载），跳过压缩包形态（归档）
				log.Printf("%s [scan] 跳过同 gid=%s 的压缩包（已存在文件夹形态 %q）: %q",
					dlLogTag, meta.GID, existing.LocalPath, localPath)
				return SaveComicSkippedGid
			}
			if !existingIsDir && isDir {
				// 新的是文件夹形态，删除旧的压缩包记录，保留文件夹
				log.Printf("%s [scan] gid=%s 由压缩包 %q 升级为文件夹 %q，移除旧压缩包记录",
					dlLogTag, meta.GID, existing.LocalPath, localPath)
				database.DB.Delete(&existing)
				// Round20-Bug1/Bug4：压缩包记录被替换 → 历史/书架/阅读清单引用迁移到新文件夹记录
				CleanupComicReferences(database.DB, existing.ID, comicID)
			}
		}
	}

	sourceMode := "gallery"
	if !isDir {
		sourceMode = "archive"
	}

	// 首次入库时间：已存在记录保留原值（AddedAt 与 UpdatedAt 不同，不会被 CheckUpdates 覆盖，问题1）
	addedAt := time.Now()
	var existingAdded models.OfflineComic
	if err := database.DB.Select("added_at").Where("local_path = ?", localPath).First(&existingAdded).Error; err == nil && !existingAdded.AddedAt.IsZero() {
		addedAt = existingAdded.AddedAt
	}

	// Round11：OriginalTitle 首次入库 = 当前标题；已有记录由下方 existingFull 保留
	originalTitle := title
	// BUG2：OriginalTitleJpn 首次入库 = 当前日文标题（用户修改主标题后可恢复）
	originalTitleJpn := titleJpn
	remarkPreserve := ""

	// Round11：OriginalTitle / Remark 保留策略。
	// OriginalTitle 仅首次入库时记录（供「修改标题 → 恢复原标题」）；
	// 已存在记录（重新扫描/更新）不覆盖 OriginalTitle 与用户备注。
	// Round23：HiddenPages / OriginalPageCount 为用户客制化（自定义删除页面），重新扫描同样保留，
	// PageCount 按「物理页数 − 有效隐藏数」重算（隐藏索引随物理页数裁剪）。
	// BUG2：OriginalTitleJpn 同 OriginalTitle 策略（老数据无此列时用当前 TitleJpn 回填基准）；
	// 同时保护用户修改的主标题——TitleJpn 与首次入库基准不同 = 用户改过，
	// 扫描不覆盖，避免重新扫描/离线更新丢失用户自定义主标题。
	// 画质升级：DownloadScheme 由下载任务回填，重新扫描不得覆盖（存量零值保留为空）。
	var existingFull models.OfflineComic
	if err := database.DB.Where("local_path = ?", localPath).First(&existingFull).Error; err == nil {
		if existingFull.OriginalTitle != "" {
			originalTitle = existingFull.OriginalTitle
		}
		if existingFull.Remark != "" {
			remarkPreserve = existingFull.Remark
		}
		if existingFull.OriginalTitleJpn == "" && existingFull.TitleJpn != "" {
			existingFull.OriginalTitleJpn = existingFull.TitleJpn // 存量回填：补首次入库日文基准
		}
		if existingFull.OriginalTitleJpn != "" {
			originalTitleJpn = existingFull.OriginalTitleJpn
		}
		// 用户修改过主标题（TitleJpn ≠ 首次入库基准）→ 保留用户值，metadata 新值不覆盖
		if existingFull.OriginalTitleJpn != "" && existingFull.TitleJpn != "" &&
			existingFull.TitleJpn != existingFull.OriginalTitleJpn {
			titleJpn = existingFull.TitleJpn
		}
	}

	// 隐藏页与原始页数：首次入库为空；已存在记录保留用户隐藏配置。
	// D10：以相对路径存储；旧的纯数字索引按当前页表映射后再归一。
	var pagePaths []string
	if isDir {
		if ps, err := listImagesRecursive(localPath); err == nil {
			pagePaths = ps
		}
	} else {
		if ps, err := listImagesInZip(localPath); err == nil {
			pagePaths = ps
		}
	}
	hidden := ParseHiddenSet(existingFull.HiddenPages, pagePaths)
	originalPageCount := existingFull.OriginalPageCount
	if originalPageCount <= 0 {
		originalPageCount = pageCount
	}
	if pageCount > originalPageCount {
		originalPageCount = pageCount // 物理页变多时同步抬升原页数
	}
	hidden = NormalizeHiddenSet(hidden, pagePaths) // 页表中已不存在的路径自动丢弃
	effectivePageCount := EffectivePageCount(pagePaths, hidden)

	// 发布时间：metadata publishTime / ComicInfo 日期（问题1 排序）
	publishedAt := parsePublishTime(meta.PublishTime)

	// Round52：社区评分——本地 metadata/ComicInfo 有值时写入；
	// 库中已有值（联网核对写入的更权威）则不覆盖，避免重新扫描把新值退化成旧值。
	communityRating := existingFull.CommunityRating
	if communityRating <= 0 {
		communityRating = meta.Rating
	}

	initialOnlineTags := ""
	if s := strings.TrimSpace(string(tagsJSON)); s != "" && s != "[]" {
		initialOnlineTags = s
	}

	comic := models.OfflineComic{
		ID:                comicID,
		Title:             title,
		TitleJpn:          titleJpn,
		OriginalTitle:     originalTitle,
		OriginalTitleJpn:  originalTitleJpn,
		Remark:            remarkPreserve,
		CoverURL:          coverURL,
		Source:            models.SourceOffline,
		Category:          category,           // 写入解析出的分类
		Tags:              string(tagsJSON),   // 写入解析出的多元标签数组 JSON
		OnlineTags:        initialOnlineTags,  // 首次入库同步写 OnlineTags（与二次扫描 Merge 口径一致）
		CommunityRating:   communityRating,    // Round52：社区评分（本地解析 / 保留库中已有）
		PageCount:         effectivePageCount, // Round23：有效页数（物理 − 隐藏）
		HiddenPages:       hidden.Marshal(),   // D10：相对路径数组
		OriginalPageCount: originalPageCount,
		UpdatedAt:         time.Now(),
		AddedAt:           addedAt,
		FileModifiedAt:    fileModifiedAt,
		PublishedAt:       publishedAt,
		IsDownloaded:      true,
		LocalPath:         localPath,
		FileSize:          fileSize,
		ScanPathID:        scanPathID, // 来源额外路径 ID；空 = 下载导入（问题3）
		GID:               meta.GID,
		Token:             meta.Token,
		ParentGID:         meta.ParentGID,
		SourceMode:        sourceMode,
		DownloadScheme:    existingFull.DownloadScheme, // 画质升级：下载任务回填的方案，扫描不覆盖（存量零值保留）
	}

	// 问题3修复：捕获入库错误，避免「扫描发现 N 本」与实际落库数量不一致的静默失败。
	// D7：禁止整行 Save —— 只写白名单允许的列（状态位/用户数据/阅读信号一律保留）。
	if err := applyScanFields(database.DB, &existingFull, &comic); err != nil {
		log.Printf("%s [scan] 入库失败 %q: %v", dlErrTag, localPath, err)
		return SaveComicFailed
	}
	return SaveComicCreated
}

// applyScanFields 按 D7 三类白名单落库：
//   - 新记录（existing.ID 为空）：Create，扫描值即初始值；
//   - 已存在记录：只写 Scan-Owned 与满足条件的 Merge 列（禁止整行 Save）。
//
// updated_at 仅在存在实际变更列时才刷新，避免「扫一次全亮」污染列表默认排序。
func applyScanFields(db *gorm.DB, existing *models.OfflineComic, incoming *models.OfflineComic) error {
	if existing == nil || existing.ID == "" {
		return db.Create(incoming).Error
	}
	updates := scanFieldUpdates(existing, incoming)
	if len(updates) == 0 {
		return nil
	}
	updates["updated_at"] = time.Now()
	return db.Model(&models.OfflineComic{}).Where("id = ?", incoming.ID).Updates(updates).Error
}

// scanFieldUpdates 计算已存在记录的扫描落库列（D7 字段白名单）。
func scanFieldUpdates(existing *models.OfflineComic, incoming *models.OfflineComic) map[string]interface{} {
	u := map[string]interface{}{}

	// ── Scan-Owned：扫描可完全决定 ──
	if existing.FileMissing {
		u["file_missing"] = false
	}
	u["page_count"] = incoming.PageCount
	u["original_page_count"] = incoming.OriginalPageCount
	u["file_size"] = incoming.FileSize
	u["file_modified_at"] = incoming.FileModifiedAt
	u["source_mode"] = incoming.SourceMode
	u["category"] = incoming.Category

	// ── Merge：标题（未手改才写，写入时同步基线，避免下次扫描误判为手改而永久锁定）──
	titleEdited := existing.OriginalTitle != "" && existing.Title != existing.OriginalTitle
	if !titleEdited && incoming.Title != "" {
		u["title"] = incoming.Title
		u["original_title"] = incoming.Title
	}
	jpnEdited := existing.OriginalTitleJpn != "" && existing.TitleJpn != existing.OriginalTitleJpn
	if !jpnEdited && incoming.TitleJpn != "" {
		u["title_jpn"] = incoming.TitleJpn
		u["original_title_jpn"] = incoming.TitleJpn
	}

	// ── Merge：空值填充（保护在线回填值与用户数据）──
	if existing.GID == "" && incoming.GID != "" {
		u["g_id"] = incoming.GID
	}
	if existing.Token == "" && incoming.Token != "" {
		u["token"] = incoming.Token
	}
	if existing.ParentGID == "" && incoming.ParentGID != "" {
		u["parent_g_id"] = incoming.ParentGID
	}
	if existing.PublishedAt == nil && incoming.PublishedAt != nil {
		u["published_at"] = incoming.PublishedAt
	}
	if existing.CommunityRating <= 0 && incoming.CommunityRating > 0 {
		u["community_rating"] = incoming.CommunityRating
	}

	// ── Merge：OnlineTags 仅空时填充（用侧车解析结果兜底），Tags 由三态合成 ──
	onlineTags := existing.OnlineTags
	if onlineTags == "" {
		if sideTags := strings.TrimSpace(incoming.Tags); sideTags != "" && sideTags != "[]" {
			u["online_tags"] = sideTags
			onlineTags = sideTags
		}
	}
	if merged := mergeTagJSON(onlineTags, existing.OfflineAddTags, existing.OfflineRemoveTags, incoming.Tags); merged != "" && merged != existing.Tags {
		u["tags"] = merged
	}

	// ── D8 来源不可变：已存在记录一律不改写 scan_path_id（仅首次入库在 Create 时写入）──
	return u
}

// mergeTagJSON 由三态源合成展示用 tags；三态为空时回退侧车解析出的标签。
func mergeTagJSON(onlineRaw, addRaw, removeRaw, fallback string) string {
	merged := MergeTags(UnmarshalTagSlice(onlineRaw), UnmarshalTagSlice(addRaw), UnmarshalTagSlice(removeRaw))
	if len(merged) == 0 {
		return fallback
	}
	b, err := json.Marshal(merged)
	if err != nil {
		return fallback
	}
	return string(b)
}

// parsePublishTime 尽量解析发布时间字符串（问题1）。支持多种格式：
// JHentai "2016-05-05 14:00"、E-H Posted "07 June 2016, 14:00"、纯日期等。
// 解析失败返回 nil（PublishedAt 置空，排序回退到其他时间字段）。
func parsePublishTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	layouts := []string{
		"2006-01-02 15:04",
		"2006-01-02 15:04:05",
		"2006-01-02",
		"02 January 2006, 15:04",
		"02 January 2006",
		"January 2, 2006",
		"2006/01/02 15:04",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}

// ─────────────────────────────────────────────────────────────
// 主扫描流程
// ─────────────────────────────────────────────────────────────

// ScanStats 单次扫描的统计结果（D17：found 为写入成功；跳过项分原因计数）
type ScanStats struct {
	Found              int
	SkippedExisting    int // 增量模式已存在
	SkippedGid         int // 同 gid 冲突跳过
	SkippedEmpty       int // 递归图片数为 0（D2）
	SkippedUnsupported int // 不可读归档 rar/7z（D4）
	Failed             int // 写库失败
}

// scanDirectory 扫描指定路径并入库，返回统计结果。
// mode: "full" 全量 | "incremental" 增量；progress 可为 nil（不汇报进度）；
// pathID 为来源额外扫描路径 ID（空 = 下载导入）。
func scanDirectory(rootPath string, includeSubfolders bool, mode string, progress *ScanProgress, pathID string) (ScanStats, error) {
	if _, err := os.Stat(rootPath); os.IsNotExist(err) {
		return ScanStats{}, err
	}

	if progress != nil {
		progress.setPhase(ScanPhaseCounting)
	}
	candidates, skips, err := CollectScanCandidates(rootPath, includeSubfolders)
	if err != nil {
		return ScanStats{}, err
	}
	stats := ScanStats{}
	warnings := make([]string, 0, 16)
	for _, s := range skips {
		switch s.Reason {
		case SkipReasonEmpty:
			stats.SkippedEmpty++
		case SkipReasonUnsupported:
			stats.SkippedUnsupported++
		}
		if len(warnings) < maxScanWarnings {
			warnings = append(warnings, string(s.Reason)+": "+s.Path)
		}
	}
	if progress != nil {
		progress.setSkipBreakdown(stats, warnings)
		progress.setTotal(len(candidates))
		progress.setPhase(ScanPhaseScanning)
	}

	incremental := mode == "incremental"

	for _, cand := range candidates {
		if progress != nil {
			progress.setCurrentTitle(filepath.Base(cand.Path))
		}
		switch saveComicWithResult(cand.Path, cand.IsDir, incremental, pathID) {
		case SaveComicCreated:
			stats.Found++
			if progress != nil {
				progress.incFound()
			}
		case SaveComicSkippedExisting:
			stats.SkippedExisting++
			if progress != nil {
				progress.incSkipped()
			}
		case SaveComicSkippedGid:
			stats.SkippedGid++
			if progress != nil {
				progress.incSkipped()
			}
		case SaveComicFailed:
			stats.Failed++
			if progress != nil {
				progress.incSkipped()
			}
		}
		if progress != nil {
			progress.incProcessed()
			progress.setSkipBreakdown(stats, warnings)
		}
	}

	return stats, nil
}

// maxScanWarnings 告警摘要保留条数上限（避免大库内存膨胀；统计计数仍为全量）
const maxScanWarnings = 50

// ScanAndSaveDirectory 兼容旧调用：全量扫描，不汇报进度；pathID 为空（下载导入，问题3）
func ScanAndSaveDirectory(rootPath string, includeSubfolders bool) (int, error) {
	stats, err := scanDirectory(rootPath, includeSubfolders, "full", nil, "")
	// Round32：下载入库（画廊/压缩包任务完成）会新增本地漫画 → 异步重建 XP 统计表
	if err == nil && stats.Found > 0 {
		XpRebuildAsync("下载入库")
	}
	return stats.Found, err
}
