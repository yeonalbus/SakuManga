package models

// ServerSetting 服务器与存储配置（单例 ID=1）
type ServerSetting struct {
	ID           uint   `gorm:"primaryKey;default:1" json:"id"`
	BindHost     string `gorm:"default:'0.0.0.0'" json:"bindHost"`
	Port         int    `gorm:"default:8081" json:"port"`
	HistoryLimit int    `gorm:"default:200" json:"historyLimit"` // 每用户历史记录上限

	SystemLogsEnabled bool `gorm:"default:true" json:"systemLogsEnabled"` // 是否启用四类系统日志落盘（Round4 任务七）
}
