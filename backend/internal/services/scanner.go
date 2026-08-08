package services

import (
	"SakuHentai/internal/database"
	"SakuHentai/internal/models"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
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
	ScanPhaseCounting ScanPhase = "counting" // 统计待扫描条目
	ScanPhaseScanning ScanPhase = "scanning" // 正在逐条处理
	ScanPhaseDone     ScanPhase = "done"     // 完成
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
}

func newScanProgress(pathID, mode string) *ScanProgress {
	return &ScanProgress{
		PathID:    pathID,
		Mode:      mode,
		Phase:     ScanPhaseCounting,
		StartedAt: time.Now().UnixMilli(),
	}
}

// Snapshot 返回一个安全的浅拷贝快照，供外部读取（JSON 序列化）
func (p *ScanProgress) Snapshot() ScanProgress {
	if p == nil {
		return ScanProgress{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return ScanProgress{
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
// 候选收集
// ─────────────────────────────────────────────────────────────

// scanCandidate 待扫描条目
type scanCandidate struct {
	path  string
	isDir bool
}

// collectCandidates 收集扫描路径下的所有候选条目（含图片的文件夹 + 归档压缩包）
func collectCandidates(rootPath string, includeSubfolders bool) ([]scanCandidate, error) {
	var candidates []scanCandidate

	if includeSubfolders {
		err := filepath.WalkDir(rootPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}

			if d.IsDir() {
				entries, readErr := os.ReadDir(path)
				if readErr != nil {
					return nil
				}

				hasImage := false
				for _, entry := range entries {
					if !entry.IsDir() && IsImage(entry.Name()) {
						hasImage = true
						break
					}
				}

				if hasImage {
					candidates = append(candidates, scanCandidate{path: path, isDir: true})
					if path != rootPath {
						return filepath.SkipDir
					}
				}
				return nil
			}

			if !d.IsDir() && IsArchive(d.Name()) {
				candidates = append(candidates, scanCandidate{path: path, isDir: false})
			}

			return nil
		})
		return candidates, err
	}

	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return nil, err
	}

	// 根目录自身直接平铺图片（归档解压落地后 extractDir 直接含图片，非子目录形态）
	rootHasImage := false
	for _, entry := range entries {
		if !entry.IsDir() && IsImage(entry.Name()) {
			rootHasImage = true
			break
		}
	}
	if rootHasImage {
		return []scanCandidate{{path: rootPath, isDir: true}}, nil
	}

	for _, entry := range entries {
		fullPath := filepath.Join(rootPath, entry.Name())
		if entry.IsDir() {
			subEntries, subErr := os.ReadDir(fullPath)
			if subErr != nil {
				continue
			}
			for _, sub := range subEntries {
				if !sub.IsDir() && IsImage(sub.Name()) {
					candidates = append(candidates, scanCandidate{path: fullPath, isDir: true})
					break
				}
			}
		} else if IsArchive(entry.Name()) {
			candidates = append(candidates, scanCandidate{path: fullPath, isDir: false})
		}
	}
	return candidates, nil
}

// ─────────────────────────────────────────────────────────────
// 单个条目入库
// ─────────────────────────────────────────────────────────────

// saveComic 处理单个候选条目：解析元数据并入库。
// scanPathID 为来源额外扫描路径 ID；空字符串 = 下载导入（问题3 来源识别）。
// 返回是否真正写入（增量模式下已存在的路径 / 同 gid 冲突跳过的会返回 false）。
func saveComic(localPath string, isDir bool, incremental bool, scanPathID string) bool {
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

	pageCount := 0
	var fileSize int64
	fileModifiedAt := time.Time{}
	if isDir {
		entries, _ := os.ReadDir(localPath)
		for _, entry := range entries {
			if !entry.IsDir() && IsImage(entry.Name()) {
				pageCount++
			}
			if !entry.IsDir() {
				if fi, err := entry.Info(); err == nil {
					fileSize += fi.Size()
					if fi.ModTime().After(fileModifiedAt) {
						fileModifiedAt = fi.ModTime()
					}
				}
			}
		}
		// 目录自身修改时间兜底（问题1）
		if fileModifiedAt.IsZero() {
			if fi, err := os.Stat(localPath); err == nil {
				fileModifiedAt = fi.ModTime()
			}
		}
	} else {
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
			log.Printf("%s [scan] 增量模式跳过已存在路径: %q", dlLogTag, localPath)
			return false
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
				return false
			}
			if !existingIsDir && isDir {
				// 新的是文件夹形态，删除旧的压缩包记录，保留文件夹
				log.Printf("%s [scan] gid=%s 由压缩包 %q 升级为文件夹 %q，移除旧压缩包记录",
					dlLogTag, meta.GID, existing.LocalPath, localPath)
				database.DB.Delete(&existing)
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

	// 发布时间：metadata publishTime / ComicInfo 日期（问题1 排序）
	publishedAt := parsePublishTime(meta.PublishTime)

	comic := models.OfflineComic{
		ID:             comicID,
		Title:          title,
		TitleJpn:       titleJpn,
		CoverURL:       coverURL,
		Source:         models.SourceOffline,
		Category:       category,             // 写入解析出的分类
		Tags:           string(tagsJSON),     // 写入解析出的多元标签数组 JSON
		PageCount:      pageCount,
		UpdatedAt:      time.Now(),
		AddedAt:        addedAt,
		FileModifiedAt: fileModifiedAt,
		PublishedAt:    publishedAt,
		IsDownloaded:   true,
		LocalPath:      localPath,
		FileSize:       fileSize,
		ScanPathID:     scanPathID, // 来源额外路径 ID；空 = 下载导入（问题3）
		GID:            meta.GID,
		Token:          meta.Token,
		ParentGID:      meta.ParentGID,
		SourceMode:     sourceMode,
	}

	// 问题3修复：捕获入库错误，避免「扫描发现 N 本」与实际落库数量不一致的静默失败
	if err := database.DB.Save(&comic).Error; err != nil {
		log.Printf("%s [scan] 入库失败 %q: %v", dlErrTag, localPath, err)
		return false
	}
	return true
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

// scanDirectory 扫描指定路径并入库，返回入库数量。
// mode: "full" 全量 | "incremental" 增量；progress 可为 nil（不汇报进度）；
// pathID 为来源额外扫描路径 ID（空 = 下载导入）。
func scanDirectory(rootPath string, includeSubfolders bool, mode string, progress *ScanProgress, pathID string) (int, error) {
	if _, err := os.Stat(rootPath); os.IsNotExist(err) {
		return 0, err
	}

	if progress != nil {
		progress.setPhase(ScanPhaseCounting)
	}
	candidates, err := collectCandidates(rootPath, includeSubfolders)
	if err != nil {
		return 0, err
	}
	if progress != nil {
		progress.setTotal(len(candidates))
		progress.setPhase(ScanPhaseScanning)
	}

	incremental := mode == "incremental"
	comicCount := 0

	for _, cand := range candidates {
		if progress != nil {
			progress.setCurrentTitle(filepath.Base(cand.path))
		}
		if saveComic(cand.path, cand.isDir, incremental, pathID) {
			comicCount++
			if progress != nil {
				progress.incFound()
			}
		} else if progress != nil {
			progress.incSkipped()
		}
		if progress != nil {
			progress.incProcessed()
		}
	}

	return comicCount, nil
}

// ScanAndSaveDirectory 兼容旧调用：全量扫描，不汇报进度；pathID 为空（下载导入，问题3）
func ScanAndSaveDirectory(rootPath string, includeSubfolders bool) (int, error) {
	return scanDirectory(rootPath, includeSubfolders, "full", nil, "")
}
