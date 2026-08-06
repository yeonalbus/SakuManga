package services

import (
	"archive/zip"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"SakuHentai/internal/models"

	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// 本地漫画 Tag 维护系统
//
// 双轨三态模型：
//   - OnlineTags         E站官方 tag（每日刷新覆盖）
//   - OfflineAddTags     本地新增 tag（用户客制化，不写回）
//   - OfflineRemoveTags  本地删除的 online tag（刷新时略过、写回时剔除）
//
// 展示规则：   展示 = (Online ∪ OfflineAdd) − OfflineRemove
// 刷新规则：   新 OnlineTags 中处于 OfflineRemoveTags 的 tag 不恢复
// 写回规则：   ComicInfo.Tags = OnlineTags − OfflineRemoveTags
// ─────────────────────────────────────────────────────────────

// 东八区时区（Windows 可能缺少 tzdata，回退 FixedZone）
var tagMaintainLoc = loadTagMaintainLocation()

func loadTagMaintainLocation() *time.Location {
	if loc, err := time.LoadLocation("Asia/Shanghai"); err == nil {
		return loc
	}
	return time.FixedZone("CST", 8*3600)
}

// TagMaintainProgress 刷新/写回任务进度（供界面轮询）
type TagMaintainProgress struct {
	Status    string `json:"status"`    // idle | running | success | error
	Type      string `json:"type"`      // refresh | writeback
	Done      int    `json:"done"`      // 已处理数
	Total     int    `json:"total"`     // 总数
	Updated   int    `json:"updated"`   // 实际变更数
	Written   int    `json:"written"`   // 写回成功数
	Failed    int    `json:"failed"`    // 失败数
	NoTags    int    `json:"noTags"`    // 无 tags 跳过数
	Message   string `json:"message"`   // 最近提示
	StartedAt int64  `json:"startedAt"` // 开始时间戳(ms)
}

// TagMaintainService 双轨 Tag 维护服务
type TagMaintainService struct {
	db        *gorm.DB
	ehService *EHService

	mu       sync.Mutex
	progress TagMaintainProgress
	running  bool
}

// NewTagMaintainService 构造 Tag 维护服务
func NewTagMaintainService(db *gorm.DB, ehService *EHService) *TagMaintainService {
	return &TagMaintainService{
		db:        db,
		ehService: ehService,
		progress:  TagMaintainProgress{Status: "idle"},
	}
}

// GetProgress 返回当前进度副本（线程安全）
func (s *TagMaintainService) GetProgress() TagMaintainProgress {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.progress
}

// IsRunning 是否正在执行
func (s *TagMaintainService) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *TagMaintainService) setProgress(p TagMaintainProgress) {
	s.mu.Lock()
	s.progress = p
	s.mu.Unlock()
}

func (s *TagMaintainService) beginRun(typ string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return false
	}
	s.running = true
	s.progress = TagMaintainProgress{
		Status:    "running",
		Type:      typ,
		StartedAt: time.Now().UnixMilli(),
	}
	return true
}

func (s *TagMaintainService) endRun() {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
}

// ─────────────────────────────────────────────────────────────
// 设置读写
// ─────────────────────────────────────────────────────────────

// LoadTagMaintainSetting 读取单例设置（不存在则创建默认值）
func LoadTagMaintainSetting(db *gorm.DB) *models.TagMaintainSetting {
	var setting models.TagMaintainSetting
	if err := db.First(&setting, 1).Error; err != nil {
		setting = models.TagMaintainSetting{
			ID:                   1,
			EnableDailyRefresh:   true,
			EnableWeeklyWriteback: true,
			RefreshHour:          6,
			WritebackWeekday:     0,
			WritebackHour:        6,
		}
		if err := db.Create(&setting).Error; err != nil {
			log.Printf("%s 初始化 TagMaintainSetting 失败: %v", dlErrTag, err)
		}
	}
	return &setting
}

// SaveTagMaintainSetting 保存设置并返回最新值
func SaveTagMaintainSetting(db *gorm.DB, setting *models.TagMaintainSetting) (*models.TagMaintainSetting, error) {
	setting.ID = 1
	setting.UpdatedAt = time.Now()
	if err := db.Save(setting).Error; err != nil {
		return nil, err
	}
	return setting, nil
}

// ─────────────────────────────────────────────────────────────
// Tag 集合工具
// ─────────────────────────────────────────────────────────────

// MarshalTagSlice JSON 序列化 tag 数组（统一入口，空则存 "[]"）
func MarshalTagSlice(tags []string) string {
	data, err := json.Marshal(tags)
	if err != nil {
		return "[]"
	}
	return string(data)
}

// UnmarshalTagSlice 解析 tag 数组字符串（兼容 JSON 数组与逗号分隔）
func UnmarshalTagSlice(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	var tags []string
	if strings.HasPrefix(raw, "[") {
		if err := json.Unmarshal([]byte(raw), &tags); err == nil {
			return normalizeTags(tags)
		}
		return []string{}
	}
	parts := strings.Split(raw, ",")
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			tags = append(tags, t)
		}
	}
	return normalizeTags(tags)
}

func normalizeTags(tags []string) []string {
	set := map[string]bool{}
	out := []string{}
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" || set[t] {
			continue
		}
		set[t] = true
		out = append(out, t)
	}
	return out
}

// MergeTags 展示规则：(online ∪ offlineAdd) − offlineRemove
func MergeTags(online, offlineAdd, offlineRemove []string) []string {
	removeSet := map[string]bool{}
	for _, r := range offlineRemove {
		removeSet[r] = true
	}
	merged := normalizeTags(append(append([]string{}, online...), offlineAdd...))
	out := []string{}
	for _, t := range merged {
		if !removeSet[t] {
			out = append(out, t)
		}
	}
	return out
}

// filterRemovedTags 刷新规则：剔除用户删除过的 online tag
func filterRemovedTags(newOnline, offlineRemove []string) []string {
	removeSet := map[string]bool{}
	for _, r := range offlineRemove {
		removeSet[r] = true
	}
	out := []string{}
	for _, t := range newOnline {
		if !removeSet[t] {
			out = append(out, t)
		}
	}
	return out
}

// ─────────────────────────────────────────────────────────────
// 每日 Tag 刷新
// ─────────────────────────────────────────────────────────────

// TagRefreshResult 单次刷新结果
type TagRefreshResult struct {
	Total               int     `json:"total"`               // 参与刷新的漫画数（含 gid）
	Skipped             int     `json:"skipped"`             // 跳过（无 gid / 拉取失败）
	Updated             int     `json:"updated"`             // tag 有变更的漫画数
	Unchanged           int     `json:"unchanged"`           // 无变更
	PublishedBackfilled int     `json:"publishedBackfilled"` // 本次补写的发布时间数量（问题2：从 E 站详情 Posted 回填）
	ElapsedSec          float64 `json:"elapsedSec"`          // 耗时(秒)
}

// RefreshAllTags 联网刷新全部含 gid 漫画的 OnlineTags
func (s *TagMaintainService) RefreshAllTags() (*TagRefreshResult, error) {
	if !s.beginRun("refresh") {
		return nil, fmt.Errorf("已有 Tag 维护任务正在执行，请稍后再试")
	}
	defer s.endRun()
	return s.refreshAllTagsLocked()
}

// refreshAllTagsLocked 实际刷新逻辑（调用方须持有 running 锁）
func (s *TagMaintainService) refreshAllTagsLocked() (*TagRefreshResult, error) {
	start := time.Now()
	result := &TagRefreshResult{}

	account := LoadAdminAccount(s.db)
	if account.IPBMemberID == "" {
		s.setProgress(TagMaintainProgress{Status: "error", Type: "refresh", Message: "请先绑定并保存 E 站账户凭证"})
		return nil, fmt.Errorf("请先绑定并保存 E 站账户凭证")
	}
	ehSetting := loadEHSetting(s.db, LoadAdminUserID(s.db))

	var comics []models.OfflineComic
	if err := s.db.Where("g_id != ''").Find(&comics).Error; err != nil {
		s.setProgress(TagMaintainProgress{Status: "error", Type: "refresh", Message: "读取离线漫画失败"})
		return nil, fmt.Errorf("读取离线漫画失败: %v", err)
	}

	result.Total = len(comics)
	s.setProgress(TagMaintainProgress{Status: "running", Type: "refresh", Total: len(comics), StartedAt: start.UnixMilli(), Message: "开始刷新 Tag…"})
	log.Printf("%s [tagm] 开始 Tag 刷新：共 %d 个含 gid 漫画", dlLogTag, len(comics))

	for i := range comics {
		c := &comics[i]
		// 进度更新
		s.mu.Lock()
		s.progress.Done = i + 1
		s.progress.Message = fmt.Sprintf("刷新 %q …", c.Title)
		s.mu.Unlock()

		if c.GID == "" {
			result.Skipped++
			continue
		}

		detail, err := s.ehService.FetchGalleryDetail(account, c.GID, c.Token, ehSetting)
		if err != nil || detail == nil {
			log.Printf("%s [tagm] 漫画 %q(gid=%s) 详情拉取失败（跳过）: %v", dlWarnTag, c.Title, c.GID, err)
			result.Skipped++
			// 限流退避（与 CheckUpdates 一致）
			time.Sleep(1200 * time.Millisecond)
			continue
		}

		// ── 顺带补齐发布时间（问题2）：外部导入的画廊本地 metadata 无 publishTime，
		//    用本次拉取的 E 站详情 Posted 字段解析并回写 published_at，无需额外请求 ──
		if c.PublishedAt == nil && detail.UpdatedAt != "" {
			if pt := parsePublishTime(detail.UpdatedAt); pt != nil {
				if err := s.db.Model(c).Update("published_at", *pt).Error; err != nil {
					log.Printf("%s [tagm] 漫画 %q 补写发布时间失败: %v", dlWarnTag, c.Title, err)
				} else {
					c.PublishedAt = pt
					result.PublishedBackfilled++
				}
			}
		}

		// 刷新规则：剔除用户删除过的 online tag
		newOnline := filterRemovedTags(detail.Tags, UnmarshalTagSlice(c.OfflineRemoveTags))

		oldOnline := UnmarshalTagSlice(c.OnlineTags)
		changed := !sameTagSet(oldOnline, newOnline)

		c.OnlineTags = MarshalTagSlice(newOnline)
		if changed {
			c.LastTagRefreshAt = time.Now().UnixMilli()
			c.TagRefreshCount++
			c.UpdatedAt = time.Now()
			if err := s.db.Model(c).Updates(map[string]interface{}{
				"online_tags":         c.OnlineTags,
				"last_tag_refresh_at": c.LastTagRefreshAt,
				"tag_refresh_count":   c.TagRefreshCount,
				"updated_at":          c.UpdatedAt,
			}).Error; err != nil {
				log.Printf("%s [tagm] 保存漫画 %s tag 刷新失败: %v", dlErrTag, c.ID, err)
			} else {
				result.Updated++
			}
		} else {
			// 无变化也记录刷新时间（刷新计数不计）
			c.LastTagRefreshAt = time.Now().UnixMilli()
			_ = s.db.Model(c).Update("last_tag_refresh_at", c.LastTagRefreshAt).Error
			result.Unchanged++
		}
		// 限流退避
		time.Sleep(1200 * time.Millisecond)
	}

	result.ElapsedSec = time.Since(start).Seconds()

	// 更新设置里的最近执行时间
	setting := LoadTagMaintainSetting(s.db)
	setting.LastDailyRunAt = time.Now().UnixMilli()
	_ = s.db.Model(setting).Update("last_daily_run_at", setting.LastDailyRunAt).Error

	s.setProgress(TagMaintainProgress{Status: "success", Type: "refresh", Done: result.Total, Total: result.Total, Updated: result.Updated,
		Message: fmt.Sprintf("刷新完成：更新 %d，无变化 %d，跳过 %d，补写发布时间 %d", result.Updated, result.Unchanged, result.Skipped, result.PublishedBackfilled), StartedAt: start.UnixMilli()})
	log.Printf("%s [tagm] Tag 刷新完成：更新 %d / 无变化 %d / 跳过 %d / 补写发布时间 %d，耗时 %.1fs", dlLogTag, result.Updated, result.Unchanged, result.Skipped, result.PublishedBackfilled, result.ElapsedSec)
	return result, nil
}

func sameTagSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := map[string]bool{}
	for _, x := range a {
		set[x] = true
	}
	for _, x := range b {
		if !set[x] {
			return false
		}
	}
	return true
}

// ─────────────────────────────────────────────────────────────
// 每周反向写回 ComicInfo
// ─────────────────────────────────────────────────────────────

// WritebackResult 单次写回结果
type WritebackResult struct {
	Total      int    `json:"total"`      // 处理漫画总数
	Written    int    `json:"written"`    // 成功写回数
	Failed     int    `json:"failed"`     // 失败数
	NoTags     int    `json:"noTags"`     // 无 tags 可写跳过
	ElapsedSec float64 `json:"elapsedSec"` // 耗时(秒)
}

// WritebackComicInfo 将数据库 online tag 反向写入 ComicInfo.xml
func (s *TagMaintainService) WritebackComicInfo() (*WritebackResult, error) {
	if !s.beginRun("writeback") {
		return nil, fmt.Errorf("已有 Tag 维护任务正在执行，请稍后再试")
	}
	defer s.endRun()
	return s.writebackLocked()
}

// writebackLocked 实际写回逻辑
func (s *TagMaintainService) writebackLocked() (*WritebackResult, error) {
	start := time.Now()
	result := &WritebackResult{}

	var comics []models.OfflineComic
	if err := s.db.Find(&comics).Error; err != nil {
		s.setProgress(TagMaintainProgress{Status: "error", Type: "writeback", Message: "读取离线漫画失败"})
		return nil, fmt.Errorf("读取离线漫画失败: %v", err)
	}

	result.Total = len(comics)
	s.setProgress(TagMaintainProgress{Status: "running", Type: "writeback", Total: len(comics), StartedAt: start.UnixMilli(), Message: "开始反向写回…"})
	log.Printf("%s [tagm] 开始反向写回 ComicInfo：共 %d 个漫画", dlLogTag, len(comics))

	for i := range comics {
		c := &comics[i]
		s.mu.Lock()
		s.progress.Done = i + 1
		s.progress.Message = fmt.Sprintf("写回 %q …", c.Title)
		s.mu.Unlock()

		// 写回规则：OnlineTags − OfflineRemoveTags（不含 OfflineAddTags）
		writeTags := MergeTags(UnmarshalTagSlice(c.OnlineTags), nil, UnmarshalTagSlice(c.OfflineRemoveTags))
		if len(writeTags) == 0 {
			result.NoTags++
			continue
		}

		if c.LocalPath == "" {
			result.Failed++
			continue
		}

		var err error
		fi, statErr := os.Stat(c.LocalPath)
		if statErr != nil {
			log.Printf("%s [tagm] 漫画 %s 路径不存在（跳过）: %v", dlWarnTag, c.ID, statErr)
			result.Failed++
			continue
		}
		if fi.IsDir() {
			err = writeComicInfoToDir(c.LocalPath, writeTags)
		} else if IsArchive(c.LocalPath) {
			err = writeComicInfoToZip(c.LocalPath, writeTags)
		} else {
			result.Failed++
			continue
		}

		if err != nil {
			log.Printf("%s [tagm] 漫画 %q(%s) 写回 ComicInfo 失败: %v", dlErrTag, c.Title, c.LocalPath, err)
			result.Failed++
		} else {
			result.Written++
			log.Printf("%s [tagm] 已写回 %q：tags=%d（%s）", dlLogTag, c.Title, len(writeTags), c.LocalPath)
		}
	}

	result.ElapsedSec = time.Since(start).Seconds()

	setting := LoadTagMaintainSetting(s.db)
	setting.LastWeeklyRunAt = time.Now().UnixMilli()
	_ = s.db.Model(setting).Update("last_weekly_run_at", setting.LastWeeklyRunAt).Error

	s.setProgress(TagMaintainProgress{Status: "success", Type: "writeback", Done: result.Total, Total: result.Total, Written: result.Written,
		Message: fmt.Sprintf("写回完成：成功 %d，失败 %d，无 tags %d", result.Written, result.Failed, result.NoTags), StartedAt: start.UnixMilli()})
	log.Printf("%s [tagm] 反向写回完成：成功 %d / 失败 %d / 无 tags %d，耗时 %.1fs", dlLogTag, result.Written, result.Failed, result.NoTags, result.ElapsedSec)
	return result, nil
}

// ─────────────────────────────────────────────────────────────
// ComicInfo.xml 写回实现
// ─────────────────────────────────────────────────────────────

// readComicInfo 读取目录下 ComicInfo.xml（不存在返回 nil, nil）
func readComicInfo(path string) (*ComicInfoXML, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var meta ComicInfoXML
	if err := xml.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("解析 ComicInfo.xml 失败: %v", err)
	}
	return &meta, nil
}

// writeComicInfoFile 将 ComicInfoXML 写盘（含 XML 头）
func writeComicInfoFile(path string, meta *ComicInfoXML) error {
	xmlData, err := xml.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	xmlFull := append([]byte(xml.Header), xmlData...)
	return os.WriteFile(path, xmlFull, 0o644)
}

// writeComicInfoToDir 改写散图文件夹内的 ComicInfo.xml（保留其它字段）
func writeComicInfoToDir(dirPath string, writeTags []string) error {
	ciPath := filepath.Join(dirPath, "ComicInfo.xml")
	meta, err := readComicInfo(ciPath)
	if err != nil {
		return err
	}
	if meta == nil {
		meta = &ComicInfoXML{}
	}
	meta.Tags = strings.Join(writeTags, ",")
	return writeComicInfoFile(ciPath, meta)
}

// writeComicInfoToZip 重打包 zip/cbz 并更新其中的 ComicInfo.xml
func writeComicInfoToZip(zipPath string, writeTags []string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("打开压缩包失败: %v", err)
	}
	defer zr.Close()

	tmpPath := zipPath + ".tmp"
	tmp, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %v", err)
	}

	zw := zip.NewWriter(tmp)
	comicInfoWritten := false

	for _, f := range zr.File {
		lower := strings.ToLower(filepath.Base(f.Name))
		var header *zip.FileHeader
		var rc io.ReadCloser

		if lower == "comicinfo.xml" {
			// 读取旧内容保留其它字段
			var meta ComicInfoXML
			if r, err := f.Open(); err == nil {
				if data, err := io.ReadAll(r); err == nil {
					_ = xml.Unmarshal(data, &meta)
				}
				r.Close()
			}
			meta.Tags = strings.Join(writeTags, ",")
			xmlData, err := xml.MarshalIndent(meta, "", "  ")
			if err != nil {
				zr.Close()
				tmp.Close()
				os.Remove(tmpPath)
				return err
			}
			xmlFull := append([]byte(xml.Header), xmlData...)

			header = &zip.FileHeader{Name: f.Name, Method: zip.Deflate}
			header.SetModTime(f.ModTime())
			w, err := zw.CreateHeader(header)
			if err != nil {
				zr.Close()
				tmp.Close()
				os.Remove(tmpPath)
				return err
			}
			if _, err := w.Write(xmlFull); err != nil {
				zr.Close()
				tmp.Close()
				os.Remove(tmpPath)
				return err
			}
			comicInfoWritten = true
			continue
		}

		// 其它文件原样复制
		header = &zip.FileHeader{Name: f.Name, Method: f.Method}
		header.SetModTime(f.ModTime())
		w, err := zw.CreateHeader(header)
		if err != nil {
			zr.Close()
			tmp.Close()
			os.Remove(tmpPath)
			return err
		}
		rc, err = f.Open()
		if err != nil {
			zr.Close()
			tmp.Close()
			os.Remove(tmpPath)
			return err
		}
		if _, err := io.Copy(w, rc); err != nil {
			rc.Close()
			zr.Close()
			tmp.Close()
			os.Remove(tmpPath)
			return err
		}
		rc.Close()
	}

	// 压缩包内无 ComicInfo.xml 时新增
	if !comicInfoWritten {
		meta := ComicInfoXML{Tags: strings.Join(writeTags, ",")}
		xmlData, err := xml.MarshalIndent(meta, "", "  ")
		if err != nil {
			zr.Close()
			tmp.Close()
			os.Remove(tmpPath)
			return err
		}
		xmlFull := append([]byte(xml.Header), xmlData...)
		header := &zip.FileHeader{Name: "ComicInfo.xml", Method: zip.Deflate}
		header.SetModTime(time.Now())
		w, err := zw.CreateHeader(header)
		if err != nil {
			zr.Close()
			tmp.Close()
			os.Remove(tmpPath)
			return err
		}
		if _, err := w.Write(xmlFull); err != nil {
			zr.Close()
			tmp.Close()
			os.Remove(tmpPath)
			return err
		}
	}

	if err := zw.Close(); err != nil {
		zr.Close()
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("压缩包写入失败: %v", err)
	}
	if err := tmp.Close(); err != nil {
		zr.Close()
		os.Remove(tmpPath)
		return err
	}
	// 关闭读取器后原子替换
	zr.Close()
	if err := os.Remove(zipPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("替换原压缩包失败（移除旧文件）: %v", err)
	}
	if err := os.Rename(tmpPath, zipPath); err != nil {
		return fmt.Errorf("替换原压缩包失败（重命名）: %v", err)
	}
	return nil
}

// ─────────────────────────────────────────────────────────────
// 数据迁移：旧 Tags → OnlineTags
// ─────────────────────────────────────────────────────────────

// MigrateLegacyTags 将旧 Tags 字段迁移到 OnlineTags（OnlineTags 为空时）
// 返回迁移的漫画数。
func MigrateLegacyTags(db *gorm.DB) (int, error) {
	var comics []models.OfflineComic
	if err := db.Find(&comics).Error; err != nil {
		return 0, err
	}
	migrated := 0
	for i := range comics {
		c := &comics[i]
		if c.OnlineTags != "" || c.Tags == "" {
			continue
		}
		oldTags := UnmarshalTagSlice(c.Tags)
		if len(oldTags) == 0 {
			continue
		}
		c.OnlineTags = MarshalTagSlice(oldTags)
		if err := db.Model(c).Update("online_tags", c.OnlineTags).Error; err != nil {
			log.Printf("%s [tagm] 迁移漫画 %s 旧 Tags 失败: %v", dlErrTag, c.ID, err)
			continue
		}
		migrated++
	}
	if migrated > 0 {
		log.Printf("%s [tagm] 数据迁移完成：%d 个漫画的旧 Tags 已迁移到 OnlineTags", dlLogTag, migrated)
	}
	return migrated, nil
}

// ─────────────────────────────────────────────────────────────
// 单本 tag 编辑（落库）
// ─────────────────────────────────────────────────────────────

// EditComicTags 应用单本漫画的 tag 增删。
// addTags 加入 OfflineAddTags；removeTags 若属于 online 则记入 OfflineRemoveTags，
// 否则从 OfflineAddTags 移除。
func (s *TagMaintainService) EditComicTags(comicID string, addTags, removeTags []string) error {
	var comic models.OfflineComic
	if err := s.db.First(&comic, "id = ?", comicID).Error; err != nil {
		return fmt.Errorf("找不到该漫画: %v", err)
	}

	online := UnmarshalTagSlice(comic.OnlineTags)
	add := UnmarshalTagSlice(comic.OfflineAddTags)
	remove := UnmarshalTagSlice(comic.OfflineRemoveTags)

	onlineSet := map[string]bool{}
	for _, t := range online {
		onlineSet[t] = true
	}

	// 1. 新增 tag → OfflineAddTags
	for _, t := range normalizeTags(addTags) {
		if onlineSet[t] {
			continue // 已是 online tag，无需重复添加
		}
		if !contains(add, t) {
			add = append(add, t)
		}
	}

	// 2. 删除 tag
	removeSet := map[string]bool{}
	for _, t := range remove {
		removeSet[t] = true
	}
	// 2a. 新增列表里去掉被删除项
	add = filterOut(add, removeTags)
	// 2b. 属于 online 的 → 记入 OfflineRemoveTags
	for _, t := range normalizeTags(removeTags) {
		if onlineSet[t] && !removeSet[t] {
			remove = append(remove, t)
			removeSet[t] = true
		}
	}

	updates := map[string]interface{}{
		"offline_add_tags":    MarshalTagSlice(add),
		"offline_remove_tags": MarshalTagSlice(remove),
		"updated_at":          time.Now(),
	}
	return s.db.Model(&comic).Updates(updates).Error
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func filterOut(list, remove []string) []string {
	removeSet := map[string]bool{}
	for _, r := range remove {
		removeSet[r] = true
	}
	out := []string{}
	for _, x := range list {
		if !removeSet[x] {
			out = append(out, x)
		}
	}
	return out
}
