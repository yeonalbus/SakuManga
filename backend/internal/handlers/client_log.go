package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

// GetClientLog 读取前端错误日志内容（供设置页「前端错误」Tab 展示）。
// 分页返回：倒序（最新在前），limit 默认 50（上限 200），offset 用于翻页。
func GetClientLog(c *gin.Context) {
	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	offset := 0
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	entries, err := readClientLogEntries()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"total": 0, "entries": []clientLogEntry{}})
		return
	}
	total := len(entries)
	if offset >= total {
		c.JSON(http.StatusOK, gin.H{"total": total, "entries": []clientLogEntry{}})
		return
	}
	end := offset + limit
	if end > total {
		end = total
	}
	c.JSON(http.StatusOK, gin.H{"total": total, "entries": entries[offset:end]})
}

// readClientLogEntries 按行解析 client.log 为结构化条目（ts 升序）。
// 文件异常膨胀时仅解析尾部 512KB，避免读入内存过大；非 JSON 行降级为 message 展示。
func readClientLogEntries() ([]clientLogEntry, error) {
	data, err := os.ReadFile(clientLogPath)
	if err != nil {
		return nil, err
	}
	const maxTail = 512 * 1024
	if len(data) > maxTail {
		data = data[len(data)-maxTail:]
	}
	entries := make([]clientLogEntry, 0, 64)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e clientLogEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			entries = append(entries, clientLogEntry{Level: "info", Message: line})
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// ClearClientLog 清除前端错误日志文件。
func ClearClientLog(c *gin.Context) {
	if err := os.Remove(clientLogPath); err != nil && !os.IsNotExist(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "清除日志失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
