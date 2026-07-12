package models

import (
	"time"
)

// Order status values (platform payment order).
const (
	StatusPending    = "pending"
	StatusPaid       = "paid"
	StatusClosed     = "closed"
	StatusExpired    = "expired"
	StatusPaidOrphan = "paid_orphan"
)

// Notify status for K2 callback worker.
const (
	NotifyNone    = "none"
	NotifyPending = "pending"
	NotifySuccess = "success"
	NotifyFailed  = "failed"
)

// Order sources.
const (
	SourceK2   = "k2"
	SourceShop = "shop"
)

// Merchant is a K2 panel client of this platform.
type Merchant struct {
	ID                     uint      `gorm:"primaryKey" json:"id"`
	AppID                  string    `gorm:"uniqueIndex;size:64;not null" json:"app_id"`
	Name                   string    `gorm:"size:128" json:"name"`
	APISecret              string    `gorm:"size:128;not null" json:"-"`
	APISecretPrev          string    `gorm:"size:128" json:"-"`
	Enable                 bool      `gorm:"default:true" json:"enable"`
	NotifyURLHostAllowlist string    `gorm:"type:text" json:"notify_url_host_allowlist"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

func (Merchant) TableName() string { return "merchants" }

// Order is a platform payment order; out_trade_no is K2 trade_no or shop order id.
type Order struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	PlatformTradeNo string     `gorm:"uniqueIndex;size:64;not null" json:"platform_trade_no"`
	OutTradeNo      string     `gorm:"size:64;not null;uniqueIndex:idx_app_out" json:"out_trade_no"`
	AppID           string     `gorm:"size:64;not null;uniqueIndex:idx_app_out;index" json:"app_id"`
	Amount          int64      `gorm:"not null" json:"amount"` // cents
	Currency        string     `gorm:"size:8;default:CNY" json:"currency"`
	Subject         string     `gorm:"size:256" json:"subject"`
	Status          string     `gorm:"size:32;index;default:pending" json:"status"`
	CashierToken    string     `gorm:"uniqueIndex;size:64;not null" json:"cashier_token"`
	NotifyURL       string     `gorm:"size:512" json:"notify_url"`
	ReturnURL       string     `gorm:"size:512" json:"return_url"`
	UserRef         string     `gorm:"size:64" json:"user_ref"`
	AlipayTradeNo   string     `gorm:"size:64" json:"alipay_trade_no"`
	PaidAmount      int64      `gorm:"default:0" json:"paid_amount"`
	PaidAt          *time.Time `json:"paid_at"`
	ExpireAt        time.Time  `gorm:"index" json:"expire_at"`
	CloseReason     string     `gorm:"size:64" json:"close_reason"`
	NotifyStatus    string     `gorm:"size:16;default:none;index" json:"notify_status"`
	NotifyAttempts  int        `gorm:"default:0" json:"notify_attempts"`
	NotifyNextAt    *time.Time `gorm:"index" json:"notify_next_at"`
	NotifyLastError string     `gorm:"size:512" json:"notify_last_error"`
	Meta            string     `gorm:"type:text" json:"meta"`
	// Shop / catalog
	Source      string `gorm:"size:16;index;default:k2" json:"source"`
	ProductID   *uint  `gorm:"index" json:"product_id,omitempty"`
	BuyerEmail  string `gorm:"size:128" json:"buyer_email,omitempty"`
	Delivered   string `gorm:"type:text" json:"delivered,omitempty"` // card codes / content after paid
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Order) TableName() string { return "orders" }

func (o *Order) NeedManual() bool { return o.Status == StatusPaidOrphan }

// Product category codes for shop filters.
const (
	CatAppleID    = "apple_id"
	CatAppleGC    = "apple_gc"
	CatGoogle     = "google"
	CatNetflix    = "netflix"
	CatStreaming  = "streaming"
	CatData       = "data"
	CatOther      = "other"
)

// Product is a sellable virtual SKU on the shop.
type Product struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	Name        string `gorm:"size:128;not null" json:"name"`
	Slug        string `gorm:"uniqueIndex;size:64" json:"slug"`
	Description string `gorm:"type:text" json:"description"`
	// Category: apple_id | apple_gc | google | netflix | streaming | data | other
	Category string `gorm:"size:32;index;default:other" json:"category"`
	// Region label e.g. US / Global
	Region string `gorm:"size:32" json:"region"`
	// Badge e.g. Hot / Instant / OfficialTop-up
	Badge string `gorm:"size:32" json:"badge"`
	// Icon emoji or short mark for cards without cover art
	Icon     string `gorm:"size:16" json:"icon"`
	CoverURL string `gorm:"size:512" json:"cover_url"`
	// Features: multi-line bullet points for product page
	Features   string `gorm:"type:text" json:"features"`
	PriceCents int64  `gorm:"not null" json:"price_cents"`
	Currency   string `gorm:"size:8;default:CNY" json:"currency"`
	// Stock: -1 = unlimited; >=0 counts inventory
	Stock           int       `gorm:"default:-1" json:"stock"`
	Enable          bool      `gorm:"default:true;index" json:"enable"`
	Sort            int       `gorm:"default:0;index" json:"sort"`
	UseCardPool     bool      `gorm:"default:true" json:"use_card_pool"`
	// AutoGenerate: if true, mint simulated credentials when pool empty (demo Apple ID / Netflix etc.)
	AutoGenerate    bool      `gorm:"default:false" json:"auto_generate"`
	DeliverTemplate string    `gorm:"type:text" json:"deliver_template"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// SiteSetting is a key-value store for runtime config (e.g. alipay) editable in admin.
type SiteSetting struct {
	Key       string    `gorm:"primaryKey;size:64" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (SiteSetting) TableName() string { return "site_settings" }

func (Product) TableName() string { return "products" }

// CardCode is inventory for virtual goods.
type CardCode struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	ProductID uint       `gorm:"index;not null" json:"product_id"`
	Code      string     `gorm:"size:256;not null" json:"code"`
	Status    string     `gorm:"size:16;index;default:unused" json:"status"` // unused | sold
	OrderID   *uint      `gorm:"index" json:"order_id,omitempty"`
	SoldAt    *time.Time `json:"sold_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func (CardCode) TableName() string { return "card_codes" }

const (
	CardUnused = "unused"
	CardSold   = "sold"
)

// AdminUser for console login.
type AdminUser struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	Username     string     `gorm:"uniqueIndex;size:64;not null" json:"username"`
	PasswordHash string     `gorm:"size:128;not null" json:"-"`
	DisplayName  string     `gorm:"size:64" json:"display_name"`
	Enable       bool       `gorm:"default:true" json:"enable"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (AdminUser) TableName() string { return "admin_users" }

// APINonce prevents replay of merchant signed requests.
type APINonce struct {
	AppID     string    `gorm:"primaryKey;size:64"`
	Nonce     string    `gorm:"primaryKey;size:64"`
	CreatedAt time.Time `gorm:"index"`
}

func (APINonce) TableName() string { return "api_nonces" }
