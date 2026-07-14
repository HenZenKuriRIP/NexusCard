package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server       ServerConfig       `yaml:"server"`
	DB           DBConfig           `yaml:"db"`
	Alipay       AlipayConfig       `yaml:"alipay"`
	Epay         EpayConfig         `yaml:"epay"`
	NotifyWorker NotifyWorkerConfig `yaml:"notify_worker"`
	ExpireWorker ExpireWorkerConfig `yaml:"expire_worker"`
	Security     SecurityConfig     `yaml:"security"`
	SeedMerchant SeedMerchantConfig `yaml:"seed_merchant"`
	Admin        AdminConfig        `yaml:"admin"`
	Shop         ShopConfig         `yaml:"shop"`
}

type ServerConfig struct {
	Listen        string `yaml:"listen"`
	PublicBaseURL string `yaml:"public_base_url"`
	AdminToken    string `yaml:"admin_token"` // legacy bearer for scripts; UI uses JWT
}

type DBConfig struct {
	Driver string `yaml:"driver"`
	DSN    string `yaml:"dsn"`
}

type AlipayConfig struct {
	AppID           string `yaml:"app_id"`
	PrivateKey      string `yaml:"private_key"`
	AlipayPublicKey string `yaml:"alipay_public_key"`
	IsProduction    bool   `yaml:"is_production"`
	Product         string `yaml:"product"`
	TimeoutExpress  string `yaml:"timeout_express"`
	MockPay         bool   `yaml:"mock_pay"`
	// BillSubject is the Alipay payment title (subject/body). Empty → sanitize order subject or "Digital Goods".
	// Recommended fixed neutral text, e.g. "Digital Goods" — never put plan/VPN names here.
	BillSubject string `yaml:"bill_subject"`
}

// EpayConfig is 彩虹易支付 / V1 易支付 (optional; also editable in Admin).
type EpayConfig struct {
	APIURL  string `yaml:"api_url"`
	PID     string `yaml:"pid"`
	Key     string `yaml:"key"`
	Types   string `yaml:"types"` // alipay,wxpay,qqpay
	Name    string `yaml:"name"`
	Enabled bool   `yaml:"enabled"`
}

type NotifyWorkerConfig struct {
	MaxAttempts     int `yaml:"max_attempts"`
	BaseBackoffSec  int `yaml:"base_backoff_sec"`
	PollIntervalSec int `yaml:"poll_interval_sec"`
}

type ExpireWorkerConfig struct {
	IntervalSec int `yaml:"interval_sec"`
	BatchSize   int `yaml:"batch_size"`
}

type SecurityConfig struct {
	SignSkewSec        int  `yaml:"sign_skew_sec"`
	RateLimitRPS       int  `yaml:"rate_limit_rps"`
	HTTPSOnly          bool `yaml:"https_only"`
	SSRFBlockPrivateIP bool `yaml:"ssrf_block_private_ip"`
}

type SeedMerchantConfig struct {
	AppID                  string   `yaml:"app_id"`
	Name                   string   `yaml:"name"`
	APISecret              string   `yaml:"api_secret"`
	NotifyURLHostAllowlist []string `yaml:"notify_url_host_allowlist"`
}

type AdminConfig struct {
	Username  string `yaml:"username"`
	Password  string `yaml:"password"` // seed password only when no admin exists
	JWTSecret string `yaml:"jwt_secret"`
	SiteName  string `yaml:"site_name"`
}

type ShopConfig struct {
	Title       string `yaml:"title"`
	Subtitle    string `yaml:"subtitle"`
	OrderTTLMin int    `yaml:"order_ttl_min"`
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	c.applyDefaults()
	c.applyEnvOverrides()
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// applyEnvOverrides allows secrets via env without putting PEM in yaml.
// GC_ALIPAY_APP_ID, GC_ALIPAY_PRIVATE_KEY, GC_ALIPAY_PUBLIC_KEY,
// GC_ALIPAY_IS_PRODUCTION=true|false, GC_ALIPAY_MOCK_PAY=true|false
func (c *Config) applyEnvOverrides() {
	if v := os.Getenv("GC_ALIPAY_APP_ID"); v != "" {
		c.Alipay.AppID = v
	}
	if v := os.Getenv("GC_ALIPAY_PRIVATE_KEY"); v != "" {
		c.Alipay.PrivateKey = v
	}
	if v := os.Getenv("GC_ALIPAY_PUBLIC_KEY"); v != "" {
		c.Alipay.AlipayPublicKey = v
	}
	if v := os.Getenv("GC_ALIPAY_PRODUCT"); v != "" {
		c.Alipay.Product = v
	}
	if v := os.Getenv("GC_PUBLIC_BASE_URL"); v != "" {
		c.Server.PublicBaseURL = strings.TrimRight(v, "/")
	}
	if v := os.Getenv("GC_ALIPAY_IS_PRODUCTION"); v != "" {
		c.Alipay.IsProduction = v == "1" || strings.EqualFold(v, "true")
	}
	if v := os.Getenv("GC_ALIPAY_MOCK_PAY"); v != "" {
		c.Alipay.MockPay = v == "1" || strings.EqualFold(v, "true")
	}
	if v := os.Getenv("GC_EPAY_API_URL"); v != "" {
		c.Epay.APIURL = v
	}
	if v := os.Getenv("GC_EPAY_PID"); v != "" {
		c.Epay.PID = v
	}
	if v := os.Getenv("GC_EPAY_KEY"); v != "" {
		c.Epay.Key = v
	}
	if v := os.Getenv("GC_EPAY_TYPES"); v != "" {
		c.Epay.Types = v
	}
	if v := os.Getenv("GC_EPAY_ENABLED"); v != "" {
		c.Epay.Enabled = v == "1" || strings.EqualFold(v, "true")
	}
}

func (c *Config) applyDefaults() {
	if c.Server.Listen == "" {
		c.Server.Listen = ":8088"
	}
	c.Server.PublicBaseURL = strings.TrimRight(strings.TrimSpace(c.Server.PublicBaseURL), "/")
	if c.Server.PublicBaseURL == "" {
		c.Server.PublicBaseURL = "http://127.0.0.1:8088"
	}
	if c.DB.Driver == "" {
		c.DB.Driver = "sqlite"
	}
	if c.DB.DSN == "" {
		c.DB.DSN = "data/giftcard.db"
	}
	if c.Alipay.Product == "" {
		c.Alipay.Product = "page"
	}
	if c.Alipay.TimeoutExpress == "" {
		c.Alipay.TimeoutExpress = "30m"
	}
	if strings.TrimSpace(c.Alipay.BillSubject) == "" {
		c.Alipay.BillSubject = "Digital Goods"
	}
	if c.NotifyWorker.MaxAttempts <= 0 {
		c.NotifyWorker.MaxAttempts = 12
	}
	if c.NotifyWorker.BaseBackoffSec <= 0 {
		c.NotifyWorker.BaseBackoffSec = 5
	}
	if c.NotifyWorker.PollIntervalSec <= 0 {
		c.NotifyWorker.PollIntervalSec = 2
	}
	if c.ExpireWorker.IntervalSec <= 0 {
		c.ExpireWorker.IntervalSec = 15
	}
	if c.ExpireWorker.BatchSize <= 0 {
		c.ExpireWorker.BatchSize = 100
	}
	if c.Security.SignSkewSec <= 0 {
		c.Security.SignSkewSec = 300
	}
	if c.Security.RateLimitRPS <= 0 {
		c.Security.RateLimitRPS = 50
	}
	if c.SeedMerchant.AppID == "" {
		c.SeedMerchant.AppID = "k2-main"
	}
	if c.SeedMerchant.Name == "" {
		c.SeedMerchant.Name = "K2Board"
	}
	if c.SeedMerchant.APISecret == "" {
		c.SeedMerchant.APISecret = "test_secret"
	}
	if c.Admin.Username == "" {
		c.Admin.Username = "admin"
	}
	if c.Admin.Password == "" {
		c.Admin.Password = "admin123"
	}
	if c.Admin.JWTSecret == "" {
		c.Admin.JWTSecret = c.Server.AdminToken
		if c.Admin.JWTSecret == "" {
			c.Admin.JWTSecret = "change-me-jwt-secret"
		}
	}
	if c.Admin.SiteName == "" {
		c.Admin.SiteName = "卡卡基地"
	}
	c.Admin.SiteName = normalizeBrandName(c.Admin.SiteName)
	if c.Shop.Title == "" {
		c.Shop.Title = c.Admin.SiteName
	}
	c.Shop.Title = normalizeBrandName(c.Shop.Title)
	if c.Shop.Subtitle == "" {
		c.Shop.Subtitle = "美区 Apple ID · 礼品卡 · Netflix / Google · 软件账号 · 自动发货"
	}
	if c.Shop.OrderTTLMin <= 0 {
		c.Shop.OrderTTLMin = 30
	}
	if c.Alipay.IsProduction {
		c.Alipay.MockPay = false
	}
	c.Epay.APIURL = strings.TrimRight(strings.TrimSpace(c.Epay.APIURL), "/")
	if c.Epay.Types == "" {
		c.Epay.Types = "alipay"
	}
	if strings.TrimSpace(c.Epay.Name) == "" {
		c.Epay.Name = "Digital Goods"
	}
}

func (c *Config) validate() error {
	if c.Server.AdminToken == "" && c.Admin.JWTSecret == "" {
		return fmt.Errorf("server.admin_token or admin.jwt_secret is required")
	}
	if c.Server.AdminToken == "" {
		c.Server.AdminToken = c.Admin.JWTSecret
	}
	if c.Alipay.IsProduction && c.Alipay.MockPay {
		return fmt.Errorf("alipay.mock_pay must be false when is_production=true")
	}
	return nil
}

// normalizeBrandName rewrites legacy English brand strings to 卡卡基地.
func normalizeBrandName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "卡卡基地"
	}
	low := strings.ToLower(s)
	switch {
	case low == "nexuscard", low == "nexuscard store", low == "giftcard", low == "giftcard store":
		return "卡卡基地"
	case strings.Contains(low, "nexuscard"):
		return "卡卡基地"
	default:
		return s
	}
}

func (c *Config) NotifyPollInterval() time.Duration {
	return time.Duration(c.NotifyWorker.PollIntervalSec) * time.Second
}

func (c *Config) ExpireInterval() time.Duration {
	return time.Duration(c.ExpireWorker.IntervalSec) * time.Second
}
