package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ============================================================
// EH uconfig.php 代理接口：应用内直接读取 / 修改 / 保存 E 站配置
// ============================================================

// GetUConfig 读取 E 站 uconfig.php 配置（含 profile 列表与全部分组字段）
func (h *EHSettingHandler) GetUConfig(c *gin.Context) {
	account, setting, ok := h.requireAccount()
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先在账号设置中绑定 E 站账号"})
		return
	}

	data, err := h.ehService.FetchUConfig(account, setting)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

// SaveUConfig 保存配置或执行 profile 操作（切换/新建/重命名/删除/设为默认）
// 请求体：
//
//	profile:     目标 profile 的 value（必填）
//	action:      "" 保存配置 | "rename" | "create" | "default" | "delete"
//	profileName: action 为 rename/create 时需要的名字
//	fields:      配置字段 name→value
func (h *EHSettingHandler) SaveUConfig(c *gin.Context) {
	account, setting, ok := h.requireAccount()
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先在账号设置中绑定 E 站账号"})
		return
	}

	var req struct {
		Profile     string            `json:"profile"`
		Action      string            `json:"action"`
		ProfileName string            `json:"profileName"`
		Fields      map[string]string `json:"fields"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数解析失败: " + err.Error()})
		return
	}

	data, err := h.ehService.SaveUConfig(account, setting, req.Profile, req.Action, req.ProfileName, req.Fields)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}
