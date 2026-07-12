package service

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/HenZenKuriRIP/NexusCard/internal/config"
	"github.com/HenZenKuriRIP/NexusCard/internal/models"
)

const settingKeyAlipay = "alipay"

// AlipaySettings is admin-editable payment config (stored in DB, overrides yaml secrets).
type AlipaySettings struct {
	AppID           string `json:"app_id"`
	PrivateKey      string `json:"private_key"`
	AlipayPublicKey string `json:"alipay_public_key"`
	IsProduction    bool   `json:"is_production"`
	Product         string `json:"product"`
	TimeoutExpress  string `json:"timeout_express"`
	MockPay         bool   `json:"mock_pay"`
	BillSubject     string `json:"bill_subject"`
	Enabled         bool   `json:"enabled"` // master switch for showing real alipay button
}

type SettingsService struct {
	DB  *gorm.DB
	Cfg *config.Config
	mu  sync.RWMutex
}

func NewSettingsService(db *gorm.DB, cfg *config.Config) *SettingsService {
	return &SettingsService{DB: db, Cfg: cfg}
}

func (s *SettingsService) GetAlipay() AlipaySettings {
	// defaults from yaml
	out := AlipaySettings{
		AppID:           s.Cfg.Alipay.AppID,
		PrivateKey:      s.Cfg.Alipay.PrivateKey,
		AlipayPublicKey: s.Cfg.Alipay.AlipayPublicKey,
		IsProduction:    s.Cfg.Alipay.IsProduction,
		Product:         s.Cfg.Alipay.Product,
		TimeoutExpress:  s.Cfg.Alipay.TimeoutExpress,
		MockPay:         s.Cfg.Alipay.MockPay,
		BillSubject:     s.Cfg.Alipay.BillSubject,
		Enabled:         true,
	}
	var row models.SiteSetting
	if err := s.DB.Where("key = ?", settingKeyAlipay).First(&row).Error; err != nil {
		return out
	}
	var dbv AlipaySettings
	if json.Unmarshal([]byte(row.Value), &dbv) != nil {
		return out
	}
	// Default enabled=true when field omitted in older JSON
	if !strings.Contains(row.Value, `"enabled"`) {
		dbv.Enabled = true
	}
	// DB overrides non-empty / explicit fields
	if strings.TrimSpace(dbv.AppID) != "" {
		out.AppID = dbv.AppID
	}
	if strings.TrimSpace(dbv.PrivateKey) != "" {
		out.PrivateKey = dbv.PrivateKey
	}
	if strings.TrimSpace(dbv.AlipayPublicKey) != "" {
		out.AlipayPublicKey = dbv.AlipayPublicKey
	}
	if strings.TrimSpace(dbv.Product) != "" {
		out.Product = dbv.Product
	}
	if strings.TrimSpace(dbv.TimeoutExpress) != "" {
		out.TimeoutExpress = dbv.TimeoutExpress
	}
	if strings.TrimSpace(dbv.BillSubject) != "" {
		out.BillSubject = dbv.BillSubject
	}
	out.IsProduction = dbv.IsProduction
	out.MockPay = dbv.MockPay
	out.Enabled = dbv.Enabled
	// If DB row exists, Enabled false is respected; if never set Enabled field from old data, default true
	return out
}

func (s *SettingsService) SaveAlipay(in AlipaySettings) error {
	if in.Product == "" {
		in.Product = "page"
	}
	if in.TimeoutExpress == "" {
		in.TimeoutExpress = "30m"
	}
	if in.BillSubject == "" {
		in.BillSubject = "Digital Goods"
	}
	// Keep previous secrets if client sent empty (UI often blanks secrets)
	cur := s.GetAlipay()
	if strings.TrimSpace(in.PrivateKey) == "" {
		in.PrivateKey = cur.PrivateKey
	}
	if strings.TrimSpace(in.AlipayPublicKey) == "" {
		in.AlipayPublicKey = cur.AlipayPublicKey
	}
	b, _ := json.Marshal(in)
	row := models.SiteSetting{Key: settingKeyAlipay, Value: string(b), UpdatedAt: time.Now()}
	return s.DB.Save(&row).Error
}

func (s *SettingsService) ToConfigAlipay() config.AlipayConfig {
	a := s.GetAlipay()
	return config.AlipayConfig{
		AppID:           a.AppID,
		PrivateKey:      a.PrivateKey,
		AlipayPublicKey: a.AlipayPublicKey,
		IsProduction:    a.IsProduction,
		Product:         a.Product,
		TimeoutExpress:  a.TimeoutExpress,
		MockPay:         a.MockPay,
		BillSubject:     a.BillSubject,
	}
}

func (s *SettingsService) AlipayPublicView() map[string]any {
	a := s.GetAlipay()
	configured := strings.TrimSpace(a.AppID) != "" &&
		strings.TrimSpace(a.PrivateKey) != "" &&
		strings.TrimSpace(a.AlipayPublicKey) != ""
	return map[string]any{
		"app_id_masked":     maskMid(a.AppID),
		"has_private_key":   strings.TrimSpace(a.PrivateKey) != "",
		"has_public_key":    strings.TrimSpace(a.AlipayPublicKey) != "",
		"is_production":     a.IsProduction,
		"product":           a.Product,
		"timeout_express":   a.TimeoutExpress,
		"mock_pay":          a.MockPay,
		"bill_subject":      a.BillSubject,
		"enabled":           a.Enabled,
		"configured":        configured,
		"effective_enabled": a.Enabled && configured,
	}
}

func maskMid(id string) string {
	id = strings.TrimSpace(id)
	if len(id) <= 4 {
		return id
	}
	return id[:2] + "****" + id[len(id)-2:]
}
