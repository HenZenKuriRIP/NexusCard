package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/HenZenKuriRIP/NexusCard/internal/config"
	"github.com/HenZenKuriRIP/NexusCard/internal/models"
)

var ErrAuth = errors.New("auth failed")

type AuthService struct {
	DB  *gorm.DB
	Cfg *config.Config
}

func NewAuthService(db *gorm.DB, cfg *config.Config) *AuthService {
	return &AuthService{DB: db, Cfg: cfg}
}

type sessionClaims struct {
	UID  uint   `json:"uid"`
	User string `json:"user"`
	Exp  int64  `json:"exp"`
}

func (s *AuthService) SeedAdmin() error {
	var n int64
	s.DB.Model(&models.AdminUser{}).Count(&n)
	if n > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(s.Cfg.Admin.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u := models.AdminUser{
		Username:     s.Cfg.Admin.Username,
		PasswordHash: string(hash),
		DisplayName:  "Administrator",
		Enable:       true,
	}
	return s.DB.Create(&u).Error
}

func (s *AuthService) Login(username, password string) (token string, user *models.AdminUser, err error) {
	var u models.AdminUser
	if err := s.DB.Where("username = ?", strings.TrimSpace(username)).First(&u).Error; err != nil {
		return "", nil, ErrAuth
	}
	if !u.Enable {
		return "", nil, ErrAuth
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return "", nil, ErrAuth
	}
	now := time.Now()
	_ = s.DB.Model(&u).Update("last_login_at", now)
	u.LastLoginAt = &now
	tok, err := s.issueToken(&u, 7*24*time.Hour)
	if err != nil {
		return "", nil, err
	}
	return tok, &u, nil
}

func (s *AuthService) issueToken(u *models.AdminUser, ttl time.Duration) (string, error) {
	c := sessionClaims{UID: u.ID, User: u.Username, Exp: time.Now().Add(ttl).Unix()}
	raw, _ := json.Marshal(c)
	payload := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, []byte(s.Cfg.Admin.JWTSecret))
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payload + "." + sig, nil
}

func (s *AuthService) ParseToken(token string) (*models.AdminUser, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return nil, ErrAuth
	}
	mac := hmac.New(sha256.New, []byte(s.Cfg.Admin.JWTSecret))
	mac.Write([]byte(parts[0]))
	expect := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expect), []byte(parts[1])) {
		return nil, ErrAuth
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrAuth
	}
	var c sessionClaims
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, ErrAuth
	}
	if time.Now().Unix() > c.Exp {
		return nil, fmt.Errorf("%w: expired", ErrAuth)
	}
	var u models.AdminUser
	if err := s.DB.First(&u, c.UID).Error; err != nil || !u.Enable {
		return nil, ErrAuth
	}
	return &u, nil
}

func (s *AuthService) ChangePassword(uid uint, oldPwd, newPwd string) error {
	if len(newPwd) < 6 {
		return fmt.Errorf("%w: password too short", ErrBadParam)
	}
	var u models.AdminUser
	if err := s.DB.First(&u, uid).Error; err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(oldPwd)) != nil {
		return ErrAuth
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPwd), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.DB.Model(&u).Update("password_hash", string(hash)).Error
}
