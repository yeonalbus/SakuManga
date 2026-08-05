package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"SakuHentai/internal/models"
	"SakuHentai/internal/services"
)

type EHSettingHandler struct {
	db        *gorm.DB
	ehService *services.EHService
}

func NewEHSettingHandler(db *gorm.DB, ehService *services.EHService) *EHSettingHandler {
	return &EHSettingHandler{db: db, ehService: ehService}
}

// ============================================================
// 内部辅助：EHSetting 单例 / 默认 Profile / 生效配置同步
// ============================================================

// getOrCreateEHSettings 确保 EHSetting 单例存在
func (h *EHSettingHandler) getOrCreateEHSettings() *models.EHSetting {
	var setting models.EHSetting
	if err := h.db.First(&setting, 1).Error; err != nil {
		setting = models.EHSetting{
			ID:             1,
			Site:           "e-hentai",
			PreferRedirect: true,
			SelectedProfile: "",
		}
		h.db.Create(&setting)
	}
	return &setting
}

// ensureDefaultProfile 无任何 Profile 时创建默认档并选中
func (h *EHSettingHandler) ensureDefaultProfile() error {
	var count int64
	if err := h.db.Model(&models.EHProfile{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	profile := models.EHProfile{
		Name:           "默认",
		IsDefault:      true,
		Site:           "e-hentai",
		PreferRedirect: true,
		RowsPerPage:    40,
		TopListSize:    10,
		Resolution:     "auto",
	}
	if err := h.db.Create(&profile).Error; err != nil {
		return err
	}

	setting := h.getOrCreateEHSettings()
	setting.SelectedProfile = strconv.FormatUint(uint64(profile.ID), 10)
	setting.Site = profile.Site
	setting.PreferRedirect = profile.PreferRedirect
	setting.UpdatedAt = time.Now()
	return h.db.Save(setting).Error
}

// getActiveProfile 返回当前选中的 Profile（不存在时回退默认）
func (h *EHSettingHandler) getActiveProfile() (*models.EHProfile, error) {
	setting := h.getOrCreateEHSettings()

	if setting.SelectedProfile != "" {
		var profile models.EHProfile
		if err := h.db.First(&profile, setting.SelectedProfile).Error; err == nil {
			return &profile, nil
		}
	}

	var profile models.EHProfile
	if err := h.db.Where("is_default = ?", true).First(&profile).Error; err == nil {
		return &profile, nil
	}
	// 无默认档时取第一条
	if err := h.db.Order("id asc").First(&profile).Error; err == nil {
		return &profile, nil
	}
	return nil, gorm.ErrRecordNotFound
}

// syncEHSetting 将 Profile 的站点配置同步到 EHSetting 生效快照
func (h *EHSettingHandler) syncEHSetting(profile *models.EHProfile) error {
	setting := h.getOrCreateEHSettings()
	setting.Site = profile.Site
	setting.PreferRedirect = profile.PreferRedirect
	setting.SelectedProfile = strconv.FormatUint(uint64(profile.ID), 10)
	setting.UpdatedAt = time.Now()
	return h.db.Save(setting).Error
}

// requireAccount 校验已绑定 E 站账号
func (h *EHSettingHandler) requireAccount() (*models.AccountSetting, *models.EHSetting, bool) {
	var account models.AccountSetting
	if err := h.db.First(&account, 1).Error; err != nil || account.IPBMemberID == "" {
		return nil, nil, false
	}
	return &account, h.getOrCreateEHSettings(), true
}

// ============================================================
// EHSetting 基础接口
// ============================================================

// GetEHSettings 获取当前生效配置 + 选中 Profile 名称
func (h *EHSettingHandler) GetEHSettings(c *gin.Context) {
	_ = h.ensureDefaultProfile()

	setting := h.getOrCreateEHSettings()
	profileName := ""
	if profile, err := h.getActiveProfile(); err == nil {
		profileName = profile.Name
	}

	c.JSON(http.StatusOK, gin.H{
		"id":                 setting.ID,
		"site":               setting.Site,
		"preferRedirect":     setting.PreferRedirect,
		"selectedProfile":    setting.SelectedProfile,
		"selectedProfileName": profileName,
		"updatedAt":          setting.UpdatedAt,
	})
}

// SaveEHSettings 保存站点配置（直接写入 EHSetting 单例生效快照）
// Profile 的完整配置（含站点各项）统一通过 uconfig 接口在 E 站侧管理
func (h *EHSettingHandler) SaveEHSettings(c *gin.Context) {
	var req struct {
		Site           *string `json:"site"`
		PreferRedirect *bool   `json:"preferRedirect"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数解析失败: " + err.Error()})
		return
	}

	setting := h.getOrCreateEHSettings()
	if req.Site != nil {
		setting.Site = *req.Site
	}
	if req.PreferRedirect != nil {
		setting.PreferRedirect = *req.PreferRedirect
	}
	setting.UpdatedAt = time.Now()
	if err := h.db.Save(setting).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存设置失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "设置更新成功"})
}

// ============================================================
// Profile 管理
// ============================================================

// GetProfiles 列出全部 Profile
func (h *EHSettingHandler) GetProfiles(c *gin.Context) {
	_ = h.ensureDefaultProfile()

	var profiles []models.EHProfile
	if err := h.db.Order("is_default desc, id asc").Find(&profiles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取 Profile 列表失败"})
		return
	}

	setting := h.getOrCreateEHSettings()

	c.JSON(http.StatusOK, gin.H{
		"profiles":       profiles,
		"selectedProfile": setting.SelectedProfile,
	})
}

// CreateProfile 新建 Profile，可按 select 字段决定是否立即切换使用
func (h *EHSettingHandler) CreateProfile(c *gin.Context) {
	_ = h.ensureDefaultProfile()

	var req struct {
		Name           string `json:"name" binding:"required"`
		Site           string `json:"site"`
		PreferRedirect *bool  `json:"preferRedirect"`
		RowsPerPage    *int   `json:"rowsPerPage"`
		TopListSize    *int   `json:"topListSize"`
		Resolution     string `json:"resolution"`
		Select         bool   `json:"select"` // 创建后是否立即切换
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Profile 名称不能为空"})
		return
	}

	profile := models.EHProfile{
		Name:           req.Name,
		Site:           req.Site,
		PreferRedirect: true,
		RowsPerPage:    40,
		TopListSize:    10,
		Resolution:     req.Resolution,
	}
	if profile.Site == "" {
		profile.Site = "e-hentai"
	}
	if profile.Resolution == "" {
		profile.Resolution = "auto"
	}
	if req.PreferRedirect != nil {
		profile.PreferRedirect = *req.PreferRedirect
	}
	if req.RowsPerPage != nil {
		profile.RowsPerPage = *req.RowsPerPage
	}
	if req.TopListSize != nil {
		profile.TopListSize = *req.TopListSize
	}

	if err := h.db.Create(&profile).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建 Profile 失败: " + err.Error()})
		return
	}

	// 第一个 Profile 自动设为默认
	var count int64
	h.db.Model(&models.EHProfile{}).Count(&count)
	if count == 1 {
		profile.IsDefault = true
		h.db.Save(&profile)
	}

	if req.Select {
		_ = h.syncEHSetting(&profile)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Profile 已创建", "data": profile})
}

// UpdateProfile 更新指定 Profile 的站点配置并保存
func (h *EHSettingHandler) UpdateProfile(c *gin.Context) {
	id := c.Param("id")
	var profile models.EHProfile
	if err := h.db.First(&profile, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Profile 不存在"})
		return
	}

	var req struct {
		Name           *string `json:"name"`
		Site           *string `json:"site"`
		PreferRedirect *bool   `json:"preferRedirect"`
		RowsPerPage    *int    `json:"rowsPerPage"`
		TopListSize    *int    `json:"topListSize"`
		Resolution     *string `json:"resolution"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数解析失败: " + err.Error()})
		return
	}

	if req.Name != nil {
		profile.Name = *req.Name
	}
	if req.Site != nil {
		profile.Site = *req.Site
	}
	if req.PreferRedirect != nil {
		profile.PreferRedirect = *req.PreferRedirect
	}
	if req.RowsPerPage != nil {
		profile.RowsPerPage = *req.RowsPerPage
	}
	if req.TopListSize != nil {
		profile.TopListSize = *req.TopListSize
	}
	if req.Resolution != nil {
		profile.Resolution = *req.Resolution
	}
	profile.UpdatedAt = time.Now()

	if err := h.db.Save(&profile).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存 Profile 失败: " + err.Error()})
		return
	}

	// 若是当前选中的 Profile，同步生效快照
	setting := h.getOrCreateEHSettings()
	if setting.SelectedProfile == strconv.FormatUint(uint64(profile.ID), 10) {
		setting.Site = profile.Site
		setting.PreferRedirect = profile.PreferRedirect
		setting.UpdatedAt = time.Now()
		h.db.Save(setting)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Profile 已保存", "data": profile})
}

// DeleteProfile 删除指定 Profile（默认与当前使用的不可删除）
func (h *EHSettingHandler) DeleteProfile(c *gin.Context) {
	id := c.Param("id")
	var profile models.EHProfile
	if err := h.db.First(&profile, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Profile 不存在"})
		return
	}

	if profile.IsDefault {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不能删除默认 Profile"})
		return
	}

	setting := h.getOrCreateEHSettings()
	if setting.SelectedProfile == strconv.FormatUint(uint64(profile.ID), 10) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不能删除当前正在使用的 Profile，请先切换"})
		return
	}

	if err := h.db.Delete(&profile).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除 Profile 失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Profile 已删除"})
}

// SelectProfile 切换当前使用的 Profile
func (h *EHSettingHandler) SelectProfile(c *gin.Context) {
	id := c.Param("id")
	var profile models.EHProfile
	if err := h.db.First(&profile, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Profile 不存在"})
		return
	}

	if err := h.syncEHSetting(&profile); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "切换 Profile 失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "已切换使用 Profile", "data": profile})
}

// ============================================================
// 图片配额与资产
// ============================================================

// GetEHUserStatus 实时读取图片配额与资产（GP / Credits / Hath）
func (h *EHSettingHandler) GetEHUserStatus(c *gin.Context) {
	account, setting, ok := h.requireAccount()
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先绑定并保存 E 站账户凭证"})
		return
	}

	status, err := h.ehService.FetchEHUserStatus(account, setting)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, status)
}

// ============================================================
// 我的标签
// ============================================================

// GetMyTags 从 E 站 mytags 页读取关注与隐藏的标签
func (h *EHSettingHandler) GetMyTags(c *gin.Context) {
	account, setting, ok := h.requireAccount()
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先绑定并保存 E 站账户凭证"})
		return
	}

	tags, err := h.ehService.FetchMyTags(account, setting)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, tags)
}

// AddMyTag 上传添加一个关注/隐藏标签到 E 站
func (h *EHSettingHandler) AddMyTag(c *gin.Context) {
	account, setting, ok := h.requireAccount()
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先绑定并保存 E 站账户凭证"})
		return
	}

	var req struct {
		Action string `json:"action" binding:"required"` // watch | hide
		Tag    string `json:"tag" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数缺失：action 与 tag 为必填"})
		return
	}

	if err := h.ehService.AddMyTag(account, setting, req.Action, req.Tag); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "标签已添加到 E 站"})
}

// RemoveMyTag 从 E 站移除一个关注/隐藏标签
func (h *EHSettingHandler) RemoveMyTag(c *gin.Context) {
	account, setting, ok := h.requireAccount()
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先绑定并保存 E 站账户凭证"})
		return
	}

	var req struct {
		Action string `json:"action" binding:"required"` // watch | hide
		Tag    string `json:"tag" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数缺失：action 与 tag 为必填"})
		return
	}

	if err := h.ehService.RemoveMyTag(account, setting, req.Action, req.Tag); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "标签已从 E 站移除"})
}

// CreateMyTagset 在 E 站新建一个 Tagset
func (h *EHSettingHandler) CreateMyTagset(c *gin.Context) {
	account, setting, ok := h.requireAccount()
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先绑定并保存 E 站账户凭证"})
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数缺失：name 为必填"})
		return
	}

	if err := h.ehService.CreateMyTagset(account, setting, req.Name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Tagset 已创建"})
}
