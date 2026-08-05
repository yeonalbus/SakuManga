package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"SakuHentai/internal/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// 角色常量
const (
	RoleAdmin  = "admin"
	RoleMember = "member"
)

var (
	ErrInvalidCredentials = errors.New("用户名或密码错误")
	ErrUnauthorized       = errors.New("未登录或会话已失效")
	ErrForbidden          = errors.New("没有权限执行此操作")
)

// AuthService 负责用户认证与会话管理
type AuthService struct {
	db *gorm.DB
}

func NewAuthService(db *gorm.DB) *AuthService {
	return &AuthService{db: db}
}

// HashPassword 使用 bcrypt 生成密码哈希
func HashPassword(plain string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// VerifyPassword 校验明文密码与哈希是否匹配
func VerifyPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// Login 校验用户名密码，成功后创建会话并返回 token
func (s *AuthService) Login(username, password string) (string, *models.User, error) {
	var user models.User
	if err := s.db.Where("username = ?", username).First(&user).Error; err != nil {
		return "", nil, ErrInvalidCredentials
	}
	if !VerifyPassword(user.PasswordHash, password) {
		return "", nil, ErrInvalidCredentials
	}
	token, err := s.NewSession(user.ID)
	if err != nil {
		return "", nil, err
	}
	return token, &user, nil
}

// NewSession 生成随机 token（crypto/rand 32 字节）并存入 user_sessions
func (s *AuthService) NewSession(userID uint) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := hex.EncodeToString(buf)
	session := models.UserSession{Token: token, UserID: userID, CreatedAt: time.Now()}
	if err := s.db.Create(&session).Error; err != nil {
		return "", err
	}
	return token, nil
}

// Logout 删除指定会话 token
func (s *AuthService) Logout(token string) error {
	return s.db.Delete(&models.UserSession{}, "token = ?", token).Error
}

// GetUserByToken 根据 token 加载会话对应的用户
func (s *AuthService) GetUserByToken(token string) (*models.User, error) {
	var session models.UserSession
	if err := s.db.Where("token = ?", token).First(&session).Error; err != nil {
		return nil, ErrUnauthorized
	}
	var user models.User
	if err := s.db.First(&user, session.UserID).Error; err != nil {
		return nil, ErrUnauthorized
	}
	return &user, nil
}
