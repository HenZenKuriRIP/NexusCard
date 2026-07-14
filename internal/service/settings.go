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
const settingKeyEpay = "epay"

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

// EpaySettings is 彩虹易支付 / V1 易支付 config (DB overrides yaml).
type EpaySettings struct {
	APIURL  string `json:"api_url"` // e.g. https://pay.example.com
	PID     string `json:"pid"`
	Key     string `json:"key"`
	Types   string `json:"types"` // comma-separated: alipay,wxpay,qqpay
	Name    string `json:"name"`  // bill product name
	Enabled bool   `json:"enabled"`
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
	// Find (not First): missing row is normal before Admin saves payment config.
	if err := s.DB.Where("key = ?", settingKeyAlipay).Limit(1).Find(&row).Error; err != nil || row.Key == "" {
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

func (s *SettingsService) GetEpay() EpaySettings {
	out := EpaySettings{
		APIURL:  s.Cfg.Epay.APIURL,
		PID:     s.Cfg.Epay.PID,
		Key:     s.Cfg.Epay.Key,
		Types:   s.Cfg.Epay.Types,
		Name:    s.Cfg.Epay.Name,
		Enabled: s.Cfg.Epay.Enabled,
	}
	if out.Types == "" {
		out.Types = "alipay"
	}
	if out.Name == "" {
		out.Name = "Digital Goods"
	}
	var row models.SiteSetting
	// Find (not First): missing row is normal before Admin saves epay config.
	if err := s.DB.Where("key = ?", settingKeyEpay).Limit(1).Find(&row).Error; err != nil || row.Key == "" {
		return out
	}
	var dbv EpaySettings
	if json.Unmarshal([]byte(row.Value), &dbv) != nil {
		return out
	}
	if strings.TrimSpace(dbv.APIURL) != "" {
		out.APIURL = dbv.APIURL
	}
	if strings.TrimSpace(dbv.PID) != "" {
		out.PID = dbv.PID
	}
	if strings.TrimSpace(dbv.Key) != "" {
		out.Key = dbv.Key
	}
	if strings.TrimSpace(dbv.Types) != "" {
		out.Types = dbv.Types
	}
	if strings.TrimSpace(dbv.Name) != "" {
		out.Name = dbv.Name
	}
	// Enabled always from DB when row exists
	out.Enabled = dbv.Enabled
	return out
}

func (s *SettingsService) SaveEpay(in EpaySettings) error {
	if strings.TrimSpace(in.Types) == "" {
		in.Types = "alipay"
	}
	if strings.TrimSpace(in.Name) == "" {
		in.Name = "Digital Goods"
	}
	cur := s.GetEpay()
	if strings.TrimSpace(in.Key) == "" {
		in.Key = cur.Key
	}
	b, _ := json.Marshal(in)
	row := models.SiteSetting{Key: settingKeyEpay, Value: string(b), UpdatedAt: time.Now()}
	return s.DB.Save(&row).Error
}

func (s *SettingsService) ToConfigEpay() config.EpayConfig {
	e := s.GetEpay()
	return config.EpayConfig{
		APIURL:  e.APIURL,
		PID:     e.PID,
		Key:     e.Key,
		Types:   e.Types,
		Name:    e.Name,
		Enabled: e.Enabled,
	}
}

func (s *SettingsService) EpayPublicView() map[string]any {
	e := s.GetEpay()
	configured := strings.TrimSpace(e.APIURL) != "" &&
		strings.TrimSpace(e.PID) != "" &&
		strings.TrimSpace(e.Key) != ""
	return map[string]any{
		"api_url":           strings.TrimSpace(e.APIURL),
		"pid_masked":        maskMid(e.PID),
		"has_key":           strings.TrimSpace(e.Key) != "",
		"types":             e.Types,
		"name":              e.Name,
		"enabled":           e.Enabled,
		"configured":        configured,
		"effective_enabled": e.Enabled && configured,
	}
}
