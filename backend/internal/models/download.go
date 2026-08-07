package models

import "time"

// DownloadTaskStatus 下载任务状态
type DownloadTaskStatus string

const (
	DownloadQueued      DownloadTaskStatus = "queued"       // 排队中
	DownloadDownloading DownloadTaskStatus = "downloading"  // 下载中
	DownloadPaused      DownloadTaskStatus = "paused"       // 已暂停
	DownloadCompleted   DownloadTaskStatus = "completed"    // 已完成
	DownloadError       DownloadTaskStatus = "error"        // 出错（可重试）
	DownloadErrorLock   DownloadTaskStatus = "error_lock"   // GP/配额不足被锁定
	DownloadCancelled   DownloadTaskStatus = "cancelled"    // 已取消
)

// DownloadMode 下载模式
type DownloadMode string

const (
	DownloadModeGallery DownloadMode = "gallery" // 普通画廊下载（逐图）
	DownloadModeArchive DownloadMode = "archive" // 归档下载（zip）
)

// ArchiveType 归档图片类型
type ArchiveType string

const (
	ArchiveTypeOriginal ArchiveType = "original" // 原图
	ArchiveTypeResample ArchiveType = "resample" // 压缩图
)

// DownloadDefaultScheme 默认下载配置（替代旧的 defaultDownloadOriginal 布尔值）
type DownloadDefaultScheme string

const (
	DefaultSchemeGallery           DownloadDefaultScheme = "gallery"           // 画廊下载（逐图）
	DefaultSchemeGalleryOriginal   DownloadDefaultScheme = "galleryOriginal"   // 画廊原图
	DefaultSchemeArchiveResample   DownloadDefaultScheme = "archiveResample"   // 归档压缩
	DefaultSchemeArchiveOriginal   DownloadDefaultScheme = "archiveOriginal"   // 归档原图
)

// DownloadTask 下载任务（对应数据库 download_tasks 表）
type DownloadTask struct {
	ID          string             `gorm:"primaryKey" json:"id"`             // 唯一 ID（毫秒时间戳 + 随机字节）
	UserID      uint               `gorm:"index" json:"userId"`              // 发起者（执行用其 E 站账号）
	GID         string             `gorm:"index" json:"gid"`                 // 画廊 GID
	Token       string             `json:"token"`                            // 画廊 Token
	Title       string             `json:"title"`                            // 画廊标题
	CoverURL    string             `json:"coverUrl,omitempty"`               // 封面地址
	Mode        DownloadMode       `gorm:"index" json:"mode"`                // gallery | archive
	ArchiveType ArchiveType        `json:"archiveType"`                      // original | resample（仅归档）
	Status      DownloadTaskStatus `gorm:"index" json:"status"`              // 任务状态
	Priority    int                `gorm:"default:0" json:"priority"`        // 优先级
	Group       string             `json:"group,omitempty"`                  // 默认分组（下载/归档）
	TotalFiles  int                `json:"totalFiles"`                       // 总文件数
	DoneFiles   int                `json:"doneFiles"`                        // 已完成文件数
	TotalBytes  int64              `json:"totalBytes"`                       // 总字节
	DoneBytes   int64              `json:"doneBytes"`                        // 已完成字节
	Speed       float64            `json:"speed"`                            // 实时速度（字节/秒）
	ArchivePath string             `json:"archivePath,omitempty"`            // 压缩包路径
	ExtractPath string             `json:"extractPath,omitempty"`            // 解压后文件夹路径
	Error             string `json:"error,omitempty"`       // 最近错误信息
	AutoUnlockCount   int    `json:"autoUnlockCount"`       // 自动 GP 解锁已重试次数（归档遇锁自动解锁，上限 3 次；手动解锁/重试后清零）
	// 更新关联：本任务是「离线更新」下载时记录被更新漫画的 ID，完成后用于清理旧版
	UpdateForComicID string    `json:"updateForComicId,omitempty"`
	// Username 发起者用户名（仅展示用，非数据库字段；管理员管理多用户任务时标识发起者）
	Username string `gorm:"-" json:"username,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// DownloadSetting 下载设置（单例 ID=1，与前端 downloadSettings 对齐）
type DownloadSetting struct {
	ID uint `gorm:"primaryKey;default:1" json:"id"`

	// ── 下载路径 ──
	ArchivePath        string `json:"archivePath"`        // 压缩包路径
	ExtractPath        string `json:"extractPath"`        // 解压后的文件夹存储路径
	SingleImageSavePath string `json:"singleImageSavePath"` // 单张图片保存路径

	// ── 下载行为 ──
	DefaultDownloadScheme          DownloadDefaultScheme `json:"defaultDownloadScheme"` // 默认下载配置 gallery | galleryOriginal | archiveResample | archiveOriginal
	DefaultDownloadOriginal        bool                   `json:"-"`                       // 已废弃，仅用于旧数据迁移（旧版「默认选中下载原图」）
	ConcurrentImageDownloads       int                    `json:"concurrentImageDownloads"`        // 同时下载图片数量
	SpeedLimitImages                int    `json:"speedLimitImages"`                // 速度限制（图片）
	SpeedLimitInterval              string `json:"speedLimitInterval"`              // 速度限制间隔 1s|2s|5s
	DownloadAllGalleriesSamePriority bool   `json:"downloadAllGalleriesSamePriority"` // 同一优先级下同时下载所有画廊

	// ── 归档设置 ──
	ArchiveThreads                int  `json:"archiveThreads"`                // 单个归档文件分片下载的并发线程数（全局线程配额池统一调度，上限 10）
	ControlArchiveConcurrency     bool `json:"controlArchiveConcurrency"`      // 控制归档下载并发：开启后归档任务需先获取全局线程配额（不足则排队等待）
	MaxArchiveConcurrency         int  `gorm:"default:1" json:"maxArchiveConcurrency"` // 最大归档并发数（1-10，且 ≤ ArchiveThreads；默认 1=单归档全线程，需多归档并行时调高）
	DeleteZipAfterArchiveDownload bool `json:"deleteZipAfterArchiveDownload"` // 归档下载完成后删除原压缩包
	AutoReduceThreadsOnEOF        bool `gorm:"default:true" json:"autoReduceThreadsOnEOF"` // 归档下载遇 EOF（连接中断）自动降低线程数规避：开启后自动降线程重试，关闭则直接报错提示手动调低
	AutoUnlockArchiveOnLock       bool `json:"autoUnlockArchiveOnLock"`       // 归档任务遇锁(error_lock)时自动消耗 GP 解锁重试（默认关闭，开启可能消耗大量 GP）

	// ── 下载任务 ──
	AutoResumeTasks bool `json:"autoResumeTasks"` // 自动恢复下载任务

	// ── 自动更新画廊（步骤 7 会同步扩展前端，后端字段一并预留）──
	AutoUpdateGallery           bool   `json:"autoUpdateGallery"`           // 是否自动更新画廊
	AutoUpdateScheme            string `json:"autoUpdateScheme"`            // 更新下载方案 gallery | archive
	AutoUpdateFallbackToGallery bool   `json:"autoUpdateFallbackToGallery"` // 无 H@H 时自动降级为画廊下载
	AutoUpdateDeleteOriginal    bool   `json:"autoUpdateDeleteOriginal"`    // 下载新版本后是否删除旧版本文件夹

	UpdatedAt time.Time `json:"updatedAt"`
}

// ArchiveDownloadOption archiver.php 解析出的单个归档方案（原图/压缩图）
type ArchiveDownloadOption struct {
	Label string `json:"label"` // "original" | "resample"
	Name  string `json:"name"`  // 显示名：原图 / 压缩图
	Cost  string `json:"cost"`  // e.g. "Free!" / "30 Credits"
	Size  string `json:"size"`  // e.g. "18.56 MiB"
}

// ArchiveInfo archiver.php 解析结果
type ArchiveInfo struct {
	GID       string                  `json:"gid"`
	Token     string                  `json:"token"`
	Options   []ArchiveDownloadOption `json:"options"`             // 顺序：原图、压缩图
	SizeBytes int64                   `json:"sizeBytes,omitempty"` // 预估字节数（按所选方案 Size 解析，0 表示未知）
}

// DownloadGPInfo GP 面板信息（余额 + archiver 报价）
type DownloadGPInfo struct {
	GP        string       `json:"gp"`
	Credits   string       `json:"credits"`
	Hath      string       `json:"hath"`
	QuotaUsed int          `json:"quotaUsed"`
	QuotaMax  int          `json:"quotaMax"`
	Archive   *ArchiveInfo `json:"archive,omitempty"`
}
