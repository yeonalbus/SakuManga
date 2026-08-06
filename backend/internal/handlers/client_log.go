package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
)

// clientLogEntry 前端上报的单条错误日志
type clientLogEntry struct {
	Ts      string `json:"ts"`
	Level   string `json:"level"`
	Message string `json:"message"`
	Stack   string `json:"stack"`
	URL     string `json:"url"`
	Info    string `json:"info"`
}

// clientLogPath 前端错误日志落盘路径（相对后端工作目录，如 backend/logs/client.log）
var clientLogPath = filepath.Join("logs", "client.log")

// ReportClientLog 接收前端 errorHandler / 错误边界上报的错误，追加写入本地日志文件。
// 用途：诊断「搜索栏输入特定内容时页面消失」等难以本地复现的前端崩溃（问题8）。
// 逐行 JSON 追加，便于用日志工具或脚本分析。
func ReportClientLog(c *gin.Context) {
	var e clientLogEntry
	if err := c.ShouldBindJSON(&e); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的日志数据"})
		return
	}
	if e.Ts == "" {
		e.Ts = time.Now().Format(time.RFC3339)
	}
	if e.Level == "" {
		e.Level = "info"
	}
	if e.Message == "" {
		e.Message = "(empty)"
	}

	if err := appendClientLog(e); err != nil {
		// 写盘失败不向客户端报错，仅记录服务端日志，避免干扰前端
		log.Printf("[client-log] 写入失败: %v", err)
		c.JSON(http.StatusOK, gin.H{"ok": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// appendClientLog 确保 logs 目录存在后，以追加模式写入一行 JSON。
func appendClientLog(e clientLogEntry) error {
	if err := os.MkdirAll(filepath.Dir(clientLogPath), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(clientLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}

// GetClientLogSize 返回前端错误日志文件大小（字节），供设置页展示真实占用。
func GetClientLogSize(c *gin.Context) {
	info, err := os.Stat(clientLogPath)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"size": 0})
		return
	}
	c.JSON(http.StatusOK, gin.H{"size": info.Size()})
}

// ClearClientLog 清除前端错误日志文件。
func ClearClientLog(c *gin.Context) {
	if err := os.Remove(clientLogPath); err != nil && !os.IsNotExist(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "清除日志失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
