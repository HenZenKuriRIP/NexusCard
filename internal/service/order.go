package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/HenZenKuriRIP/NexusCard/internal/config"
	"github.com/HenZenKuriRIP/NexusCard/internal/models"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrBadParam       = errors.New("bad param")
	ErrConflictPaid   = errors.New("already paid")
	ErrConflictClosed = errors.New("closed or expired")
	ErrMerchant       = errors.New("merchant")
)

var outTradeNoRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type OrderService struct {
	DB     *gorm.DB
	Cfg    *config.Config
	Alipay AlipayCloser // optional; set after construction for close/expire
}

// AlipayCloser is optional remote close for pending Alipay trades.
type AlipayCloser interface {
	CloseTrade(ctx context.Context, platformTradeNo string) error
	Configured() bool
}

func NewOrderService(db *gorm.DB, cfg *config.Config) *OrderService {
	return &OrderService{DB: db, Cfg: cfg}
}

type CreateOrderReq struct {
	OutTradeNo string `json:"out_trade_no"`
	Amount     int64  `json:"amount"`
	Currency   string `json:"currency"`
	Subject    string `json:"subject"`
	NotifyURL  string `json:"notify_url"`
	ReturnURL  string `json:"return_url"`
	ExpireAt   int64  `json:"expire_at"`
	UserRef    string `json:"user_ref"`
}

type OrderView struct {
	ID              uint   `json:"id,omitempty"`
	OutTradeNo      string `json:"out_trade_no"`
	PlatformTradeNo string `json:"platform_trade_no"`
	CashierToken    string `json:"cashier_token,omitempty"`
	CashierURL      string `json:"cashier_url,omitempty"`
	Amount          int64  `json:"amount"`
	PaidAmount      int64  `json:"paid_amount"`
	Currency        string `json:"currency"`
	Status          string `json:"status"`
	Subject         string `json:"subject,omitempty"`
	ExpireAt        int64  `json:"expire_at"`
	AlipayTradeNo   string `json:"alipay_trade_no,omitempty"`
	NotifyStatus    string `json:"notify_status,omitempty"`
	PaidAt          int64  `json:"paid_at,omitempty"`
	NeedManual      bool   `json:"need_manual,omitempty"`
	Source          string `json:"source,omitempty"`
	ProductID       *uint  `json:"product_id,omitempty"`
	BuyerEmail      string `json:"buyer_email,omitempty"`
	Delivered       string `json:"delivered,omitempty"`
	CreatedAt       int64  `json:"created_at,omitempty"`
}

func (s *OrderService) ToView(o *models.Order) OrderView {
	v := OrderView{
		ID:              o.ID,
		OutTradeNo:      o.OutTradeNo,
		PlatformTradeNo: o.PlatformTradeNo,
		CashierToken:    o.CashierToken,
		CashierURL:      s.cashierURL(o.CashierToken),
		Amount:          o.Amount,
		PaidAmount:      o.PaidAmount,
		Currency:        o.Currency,
		Status:          o.Status,
		Subject:         o.Subject,
		ExpireAt:        o.ExpireAt.Unix(),
		AlipayTradeNo:   o.AlipayTradeNo,
		NotifyStatus:    o.NotifyStatus,
		NeedManual:      o.NeedManual(),
		Source:          o.Source,
		ProductID:       o.ProductID,
		BuyerEmail:      o.BuyerEmail,
		Delivered:       o.Delivered,
		CreatedAt:       o.CreatedAt.Unix(),
	}
	if o.PaidAt != nil {
		v.PaidAt = o.PaidAt.Unix()
	}
	if v.Source == "" {
		v.Source = models.SourceK2
	}
	return v
}

func (s *OrderService) cashierURL(token string) string {
	return s.Cfg.Server.PublicBaseURL + "/c/" + token
}

// Create implements C.5.2 idempotent create + revive.
// On already paid returns (*Order, ErrConflictPaid) with the existing order for 40901 data.
func (s *OrderService) Create(appID string, req CreateOrderReq) (*models.Order, error) {
	if !outTradeNoRe.MatchString(req.OutTradeNo) {
		return nil, fmt.Errorf("%w: invalid out_trade_no", ErrBadParam)
	}
	if req.Amount <= 0 {
		return nil, fmt.Errorf("%w: amount must be positive cents", ErrBadParam)
	}
	currency := strings.TrimSpace(req.Currency)
	if currency == "" {
		currency = "CNY"
	}
	subject := sanitizeOrderSubject(req.Subject)
	if utf8.RuneCountInString(subject) > 256 {
		r := []rune(subject)
		subject = string(r[:256])
	}
	if strings.TrimSpace(req.NotifyURL) == "" || strings.TrimSpace(req.ReturnURL) == "" {
		return nil, fmt.Errorf("%w: notify_url and return_url required", ErrBadParam)
	}
	if err := s.validateNotifyURL(appID, req.NotifyURL); err != nil {
		return nil, err
	}

	now := time.Now()
	if req.ExpireAt <= now.Unix() {
		return nil, fmt.Errorf("%w: expire_at must be in the future", ErrBadParam)
	}
	hardMax := now.Add(24 * time.Hour).Unix()
	expUnix := req.ExpireAt
	if expUnix > hardMax {
		expUnix = hardMax
	}
	expireAt := time.Unix(expUnix, 0)

	var result *models.Order
	var conflictPaid bool

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var existing models.Order
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("app_id = ? AND out_trade_no = ?", appID, req.OutTradeNo).
			First(&existing).Error

		if errors.Is(err, gorm.ErrRecordNotFound) {
			o := &models.Order{
				PlatformTradeNo: newPlatformTradeNo(),
				OutTradeNo:      req.OutTradeNo,
				AppID:           appID,
				Amount:          req.Amount,
				Currency:        currency,
				Subject:         subject,
				Status:          models.StatusPending,
				CashierToken:    newCashierToken(),
				NotifyURL:       req.NotifyURL,
				ReturnURL:       req.ReturnURL,
				UserRef:         strings.TrimSpace(req.UserRef),
				ExpireAt:        expireAt,
				NotifyStatus:    models.NotifyNone,
				Meta:            "{}",
				Source:          models.SourceK2,
			}
			if err := tx.Create(o).Error; err != nil {
				return err
			}
			result = o
			return nil
		}
		if err != nil {
			return err
		}

		if existing.Amount != req.Amount || !strings.EqualFold(existing.Currency, currency) {
			return fmt.Errorf("%w: amount/currency mismatch on existing order", ErrBadParam)
		}

		switch existing.Status {
		case models.StatusPaid, models.StatusPaidOrphan:
			cp := existing
			result = &cp
			conflictPaid = true
			return nil
		case models.StatusClosed:
			return ErrConflictClosed
		case models.StatusPending:
			if now.Before(existing.ExpireAt) {
				existing.NotifyURL = req.NotifyURL
				existing.ReturnURL = req.ReturnURL
				if err := tx.Save(&existing).Error; err != nil {
					return err
				}
				result = &existing
				return nil
			}
			// Locally past expire: mark expired then revive
			existing.Status = models.StatusExpired
			existing.CloseReason = "auto_expired"
			if err := tx.Save(&existing).Error; err != nil {
				return err
			}
			revived, err := reviveOrder(tx, &existing, expireAt, req)
			if err != nil {
				return err
			}
			result = revived
			return nil
		case models.StatusExpired:
			if expUnix <= now.Unix() {
				return ErrConflictClosed
			}
			revived, err := reviveOrder(tx, &existing, expireAt, req)
			if err != nil {
				return err
			}
			result = revived
			return nil
		default:
			return fmt.Errorf("%w: unknown status %s", ErrBadParam, existing.Status)
		}
	})
	if err != nil {
		return nil, err
	}
	if conflictPaid {
		return result, ErrConflictPaid
	}
	return result, nil
}

func reviveOrder(tx *gorm.DB, o *models.Order, expireAt time.Time, req CreateOrderReq) (*models.Order, error) {
	oldPTN := o.PlatformTradeNo
	meta := map[string]any{}
	_ = json.Unmarshal([]byte(o.Meta), &meta)
	if meta == nil {
		meta = map[string]any{}
	}
	var hist []any
	switch h := meta["platform_trade_no_history"].(type) {
	case []any:
		hist = h
	case []string:
		for _, x := range h {
			hist = append(hist, x)
		}
	}
	hist = append(hist, oldPTN)
	meta["platform_trade_no_history"] = hist
	meta["revived_at"] = time.Now().Unix()
	newPTN := newPlatformTradeNo()
	meta["latest_platform_trade_no"] = newPTN
	mb, _ := json.Marshal(meta)

	o.PlatformTradeNo = newPTN
	o.CashierToken = newCashierToken()
	o.Status = models.StatusPending
	o.ExpireAt = expireAt
	o.CloseReason = ""
	o.AlipayTradeNo = ""
	o.PaidAmount = 0
	o.PaidAt = nil
	o.NotifyStatus = models.NotifyNone
	o.NotifyAttempts = 0
	o.NotifyNextAt = nil
	o.NotifyLastError = ""
	o.NotifyURL = req.NotifyURL
	o.ReturnURL = req.ReturnURL
	o.Meta = string(mb)
	if err := tx.Save(o).Error; err != nil {
		return nil, err
	}
	return o, nil
}

func (s *OrderService) GetByOutTradeNo(appID, outTradeNo string) (*models.Order, error) {
	var o models.Order
	err := s.DB.Where("app_id = ? AND out_trade_no = ?", appID, outTradeNo).First(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &o, err
}

func (s *OrderService) GetByCashierToken(token string) (*models.Order, error) {
	var o models.Order
	err := s.DB.Where("cashier_token = ?", token).First(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &o, err
}

// FindByPlatformTradeNo looks up by current platform_trade_no or meta history.
func (s *OrderService) FindByPlatformTradeNo(platformTradeNo string) (*models.Order, error) {
	var o models.Order
	err := s.DB.Where("platform_trade_no = ?", platformTradeNo).First(&o).Error
	if err == nil {
		return &o, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	// Fallback: scan recent orders for history (v1; OK for low volume)
	var list []models.Order
	if err := s.DB.Order("id desc").Limit(500).Find(&list).Error; err != nil {
		return nil, err
	}
	for i := range list {
		meta := map[string]any{}
		_ = json.Unmarshal([]byte(list[i].Meta), &meta)
		if hist, ok := meta["platform_trade_no_history"].([]any); ok {
			for _, h := range hist {
				if s, ok := h.(string); ok && s == platformTradeNo {
					return &list[i], nil
				}
			}
		}
	}
	return nil, ErrNotFound
}

func (s *OrderService) GetByID(id uint) (*models.Order, error) {
	var o models.Order
	err := s.DB.First(&o, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &o, err
}

func (s *OrderService) Close(appID, outTradeNo, reason string) (*models.Order, error) {
	var o models.Order
	err := s.DB.Where("app_id = ? AND out_trade_no = ?", appID, outTradeNo).First(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if reason == "" {
		reason = "k2_cancel"
	}
	switch o.Status {
	case models.StatusPaid, models.StatusPaidOrphan:
		return &o, ErrConflictPaid
	case models.StatusClosed, models.StatusExpired:
		return &o, nil
	case models.StatusPending:
		res := s.DB.Model(&models.Order{}).
			Where("id = ? AND status = ?", o.ID, models.StatusPending).
			Updates(map[string]any{
				"status":       models.StatusClosed,
				"close_reason": reason,
			})
		if res.Error != nil {
			return nil, res.Error
		}
		_ = s.DB.First(&o, o.ID)
		// Best-effort Alipay trade.close
		if s.Alipay != nil && s.Alipay.Configured() {
			if err := s.Alipay.CloseTrade(context.Background(), o.PlatformTradeNo); err != nil {
				slog.Warn("alipay close on order close", "err", err, "ptn", o.PlatformTradeNo)
			}
		}
		return &o, nil
	default:
		return &o, nil
	}
}

// MarkPaidMock simulates successful payment (dev / alipay.mock_pay only).
func (s *OrderService) MarkPaidMock(token string) (*models.Order, error) {
	if !s.Cfg.Alipay.MockPay {
		return nil, fmt.Errorf("%w: mock_pay disabled", ErrBadParam)
	}
	var o models.Order
	err := s.DB.Where("cashier_token = ?", token).First(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if o.Status != models.StatusPending {
		if o.Status == models.StatusPaid || o.Status == models.StatusPaidOrphan {
			return &o, nil
		}
		return nil, fmt.Errorf("%w: order not payable", ErrConflictClosed)
	}
	if time.Now().After(o.ExpireAt) {
		return nil, fmt.Errorf("%w: order expired", ErrConflictClosed)
	}
	return s.acceptPayment(&o, "MOCK"+randomHex(8), o.PlatformTradeNo)
}

// acceptPayment marks paid (or paid_orphan) and enqueues K2 notify.
func (s *OrderService) acceptPayment(o *models.Order, alipayTradeNo, platformTradeNoPaid string) (*models.Order, error) {
	now := time.Now()
	var out models.Order

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var cur models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cur, o.ID).Error; err != nil {
			return err
		}
		switch cur.Status {
		case models.StatusPaid, models.StatusPaidOrphan:
			out = cur
			return nil
		case models.StatusPending, models.StatusClosed, models.StatusExpired:
			// proceed
		default:
			return fmt.Errorf("cannot pay status %s", cur.Status)
		}

		newStatus := models.StatusPaid
		if cur.Status == models.StatusClosed || cur.Status == models.StatusExpired {
			newStatus = models.StatusPaidOrphan
		}

		meta := map[string]any{}
		_ = json.Unmarshal([]byte(cur.Meta), &meta)
		if meta == nil {
			meta = map[string]any{}
		}
		meta["latest_paid_platform_trade_no"] = platformTradeNoPaid
		mb, _ := json.Marshal(meta)

		// Fulfill virtual goods for shop / product-linked orders
		delivered, derr := DeliverOnPaid(tx, &cur)
		if derr != nil {
			return derr
		}

		notifyStatus := models.NotifyPending
		var next *time.Time
		if strings.TrimSpace(cur.NotifyURL) == "" {
			// Shop orders: no external merchant callback
			notifyStatus = models.NotifySuccess
		} else {
			t := now
			next = &t
		}

		updates := map[string]any{
			"status":          newStatus,
			"alipay_trade_no": alipayTradeNo,
			"paid_amount":     cur.Amount,
			"paid_at":         now,
			"notify_status":   notifyStatus,
			"notify_attempts": 0,
			"meta":            string(mb),
			"delivered":       delivered,
		}
		if next != nil {
			updates["notify_next_at"] = next
		} else {
			updates["notify_next_at"] = nil
		}
		res := tx.Model(&models.Order{}).
			Where("id = ? AND status = ?", cur.ID, cur.Status).
			Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		return tx.First(&out, cur.ID).Error
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ListAdmin returns recent orders with optional filters.
func (s *OrderService) ListAdmin(status, source string, limit int) ([]models.Order, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := s.DB.Model(&models.Order{}).Order("id desc").Limit(limit)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if source != "" {
		q = q.Where("source = ?", source)
	}
	var list []models.Order
	err := q.Find(&list).Error
	return list, err
}

// Stats for dashboard.
func (s *OrderService) Stats() map[string]any {
	var pending, paid, orphan, todayPaid int64
	var todayAmount int64
	s.DB.Model(&models.Order{}).Where("status = ?", models.StatusPending).Count(&pending)
	s.DB.Model(&models.Order{}).Where("status = ?", models.StatusPaid).Count(&paid)
	s.DB.Model(&models.Order{}).Where("status = ?", models.StatusPaidOrphan).Count(&orphan)
	start := time.Now().Truncate(24 * time.Hour)
	s.DB.Model(&models.Order{}).Where("status IN ? AND paid_at >= ?", []string{models.StatusPaid, models.StatusPaidOrphan}, start).Count(&todayPaid)
	s.DB.Model(&models.Order{}).Where("status IN ? AND paid_at >= ?", []string{models.StatusPaid, models.StatusPaidOrphan}, start).
		Select("COALESCE(SUM(paid_amount),0)").Scan(&todayAmount)
	var products, cards int64
	s.DB.Model(&models.Product{}).Count(&products)
	s.DB.Model(&models.CardCode{}).Where("status = ?", models.CardUnused).Count(&cards)
	return map[string]any{
		"pending_orders":  pending,
		"paid_orders":     paid,
		"orphan_orders":   orphan,
		"today_paid":      todayPaid,
		"today_amount":    todayAmount,
		"products":        products,
		"unused_cards":    cards,
		"mock_pay":        s.Cfg.Alipay.MockPay,
	}
}

func (s *OrderService) EnqueueRenotify(id uint) error {
	now := time.Now()
	res := s.DB.Model(&models.Order{}).
		Where("id = ? AND status IN ?", id, []string{models.StatusPaid, models.StatusPaidOrphan}).
		Updates(map[string]any{
			"notify_status":     models.NotifyPending,
			"notify_attempts":   0,
			"notify_next_at":    now,
			"notify_last_error": "",
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *OrderService) ExpireBatch(limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	var list []models.Order
	now := time.Now()
	if err := s.DB.Where("status = ? AND expire_at < ?", models.StatusPending, now).
		Limit(limit).Find(&list).Error; err != nil {
		return 0, err
	}
	n := 0
	for _, o := range list {
		res := s.DB.Model(&models.Order{}).
			Where("id = ? AND status = ?", o.ID, models.StatusPending).
			Updates(map[string]any{
				"status":       models.StatusExpired,
				"close_reason": "auto_expired",
			})
		if res.Error == nil && res.RowsAffected > 0 {
			n++
			if s.Alipay != nil && s.Alipay.Configured() {
				if err := s.Alipay.CloseTrade(context.Background(), o.PlatformTradeNo); err != nil {
					slog.Warn("alipay close on expire", "err", err, "ptn", o.PlatformTradeNo)
				}
			}
		}
	}
	return n, nil
}

func (s *OrderService) validateNotifyURL(appID, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("%w: invalid notify_url", ErrBadParam)
	}
	if s.Cfg.Security.HTTPSOnly && u.Scheme != "https" {
		return fmt.Errorf("%w: notify_url must be https", ErrBadParam)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%w: notify_url scheme", ErrBadParam)
	}

	var m models.Merchant
	if err := s.DB.Where("app_id = ?", appID).First(&m).Error; err != nil {
		return fmt.Errorf("%w: merchant not found", ErrMerchant)
	}
	if !m.Enable {
		return fmt.Errorf("%w: merchant disabled", ErrMerchant)
	}
	allow := parseAllowlist(m.NotifyURLHostAllowlist)
	if len(allow) > 0 {
		host := strings.ToLower(u.Hostname())
		ok := false
		for _, h := range allow {
			if strings.EqualFold(h, host) {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("%w: notify_url host not in allowlist", ErrBadParam)
		}
	}
	if s.Cfg.Security.SSRFBlockPrivateIP {
		h := strings.ToLower(u.Hostname())
		if h == "localhost" || h == "127.0.0.1" || h == "0.0.0.0" || strings.HasPrefix(h, "10.") ||
			strings.HasPrefix(h, "192.168.") || strings.HasPrefix(h, "169.254.") {
			return fmt.Errorf("%w: notify_url private host blocked", ErrBadParam)
		}
	}
	return nil
}

func parseAllowlist(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil
	}
	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil
	}
	return list
}

// SeedMerchant creates default merchant if missing.
// sanitizeOrderSubject neutralizes sensitive bill titles for K2 / Alipay display.
func sanitizeOrderSubject(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "Digital Goods"
	}
	replacer := strings.NewReplacer(
		"/", " ", "\\", " ", "=", " ", "&", " ", "?", " ",
		"#", " ", "%", " ", "<", " ", ">", " ", "\"", " ", "'", " ",
	)
	s = replacer.Replace(s)
	lower := strings.ToLower(s)
	for _, w := range []string{
		"k2board", "anytls", "v2ray", "xray", "hysteria", "shadowsocks", "trojan",
		"wireguard", "openvpn", "clash", "vpn", "proxy", "vmess", "vless",
	} {
		for {
			i := strings.Index(lower, w)
			if i < 0 {
				break
			}
			s = s[:i] + " " + s[i+len(w):]
			lower = strings.ToLower(s)
		}
	}
	for _, w := range []string{"机场", "科学上网", "翻墙", "加速器", "梯子", "流量套餐", "机场套餐", "节点订阅"} {
		s = strings.ReplaceAll(s, w, " ")
	}
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "Digital Goods"
	}
	return s
}

func SeedMerchant(db *gorm.DB, cfg *config.Config) error {
	var n int64
	db.Model(&models.Merchant{}).Where("app_id = ?", cfg.SeedMerchant.AppID).Count(&n)
	if n > 0 {
		return nil
	}
	allow, _ := json.Marshal(cfg.SeedMerchant.NotifyURLHostAllowlist)
	m := models.Merchant{
		AppID:                  cfg.SeedMerchant.AppID,
		Name:                   cfg.SeedMerchant.Name,
		APISecret:              cfg.SeedMerchant.APISecret,
		Enable:                 true,
		NotifyURLHostAllowlist: string(allow),
	}
	return db.Create(&m).Error
}
