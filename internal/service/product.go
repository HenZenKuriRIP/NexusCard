package service

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/HenZenKuriRIP/NexusCard/internal/config"
	"github.com/HenZenKuriRIP/NexusCard/internal/models"
)

type ProductService struct {
	DB  *gorm.DB
	Cfg *config.Config
}

func NewProductService(db *gorm.DB, cfg *config.Config) *ProductService {
	return &ProductService{DB: db, Cfg: cfg}
}

var slugRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type ProductInput struct {
	Name            string `json:"name"`
	Slug            string `json:"slug"`
	Description     string `json:"description"`
	Category        string `json:"category"`
	Region          string `json:"region"`
	Badge           string `json:"badge"`
	Icon            string `json:"icon"`
	CoverURL        string `json:"cover_url"`
	Features        string `json:"features"`
	PriceCents      int64  `json:"price_cents"`
	Currency        string `json:"currency"`
	Stock           int    `json:"stock"`
	Enable          *bool  `json:"enable"`
	Sort            int    `json:"sort"`
	UseCardPool     *bool  `json:"use_card_pool"`
	AutoGenerate    *bool  `json:"auto_generate"`
	DeliverTemplate string `json:"deliver_template"`
}

func (s *ProductService) ListAdmin() ([]models.Product, error) {
	var list []models.Product
	err := s.DB.Order("sort desc, id desc").Find(&list).Error
	return list, err
}

func (s *ProductService) ListPublic(category string) ([]models.Product, error) {
	q := s.DB.Where("enable = ?", true)
	if category != "" && category != "all" {
		q = q.Where("category = ?", category)
	}
	var list []models.Product
	err := q.Order("sort desc, id desc").Find(&list).Error
	return list, err
}

func (s *ProductService) Get(id uint) (*models.Product, error) {
	var p models.Product
	if err := s.DB.First(&p, id).Error; err != nil {
		return nil, ErrNotFound
	}
	return &p, nil
}

func (s *ProductService) Create(in ProductInput) (*models.Product, error) {
	p, err := s.normalize(in, nil)
	if err != nil {
		return nil, err
	}
	if err := s.DB.Create(p).Error; err != nil {
		return nil, err
	}
	return p, nil
}

func (s *ProductService) Update(id uint, in ProductInput) (*models.Product, error) {
	var cur models.Product
	if err := s.DB.First(&cur, id).Error; err != nil {
		return nil, ErrNotFound
	}
	p, err := s.normalize(in, &cur)
	if err != nil {
		return nil, err
	}
	p.ID = id
	p.CreatedAt = cur.CreatedAt
	if err := s.DB.Save(p).Error; err != nil {
		return nil, err
	}
	return p, nil
}

func (s *ProductService) Delete(id uint) error {
	res := s.DB.Delete(&models.Product{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *ProductService) normalize(in ProductInput, cur *models.Product) (*models.Product, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name required", ErrBadParam)
	}
	if utf8.RuneCountInString(name) > 128 {
		return nil, fmt.Errorf("%w: name too long", ErrBadParam)
	}
	slug := strings.TrimSpace(strings.ToLower(in.Slug))
	if slug == "" {
		slug = slugify(name)
	}
	if !slugRe.MatchString(slug) {
		return nil, fmt.Errorf("%w: invalid slug", ErrBadParam)
	}
	if in.PriceCents <= 0 {
		return nil, fmt.Errorf("%w: price_cents must be > 0", ErrBadParam)
	}
	cury := strings.TrimSpace(in.Currency)
	if cury == "" {
		cury = "CNY"
	}
	enable := true
	if in.Enable != nil {
		enable = *in.Enable
	} else if cur != nil {
		enable = cur.Enable
	}
	usePool := false
	if in.UseCardPool != nil {
		usePool = *in.UseCardPool
	} else if cur != nil {
		usePool = cur.UseCardPool
	}
	autoGen := true // default on for new digital goods so shop is sellable out of box
	if in.AutoGenerate != nil {
		autoGen = *in.AutoGenerate
	} else if cur != nil {
		autoGen = cur.AutoGenerate
	}
	stock := in.Stock
	if cur == nil && autoGen && stock == 0 {
		stock = -1 // unlimited sim stock
	}
	if cur == nil && !usePool && !autoGen && stock == 0 {
		stock = -1
	}

	cat := strings.TrimSpace(in.Category)
	if cat == "" {
		if cur != nil && cur.Category != "" {
			cat = cur.Category
		} else {
			cat = models.CatOther
		}
	}
	return &models.Product{
		Name:            name,
		Slug:            slug,
		Description:     strings.TrimSpace(in.Description),
		Category:        cat,
		Region:          strings.TrimSpace(in.Region),
		Badge:           strings.TrimSpace(in.Badge),
		Icon:            strings.TrimSpace(in.Icon),
		CoverURL:        strings.TrimSpace(in.CoverURL),
		Features:        strings.TrimSpace(in.Features),
		PriceCents:      in.PriceCents,
		Currency:        cury,
		Stock:           stock,
		Enable:          enable,
		Sort:            in.Sort,
		UseCardPool:     usePool,
		AutoGenerate:    autoGen,
		DeliverTemplate: strings.TrimSpace(in.DeliverTemplate),
	}, nil
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "item-" + randomHex(4)
	}
	if len(out) > 48 {
		out = out[:48]
	}
	return out
}

// ImportCards adds unused card codes for a product; updates stock if stock>=0.
func (s *ProductService) ImportCards(productID uint, codes []string) (int, error) {
	var p models.Product
	if err := s.DB.First(&p, productID).Error; err != nil {
		return 0, ErrNotFound
	}
	n := 0
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		for _, raw := range codes {
			code := strings.TrimSpace(raw)
			if code == "" {
				continue
			}
			c := models.CardCode{ProductID: productID, Code: code, Status: models.CardUnused}
			if err := tx.Create(&c).Error; err != nil {
				continue // skip dup
			}
			n++
		}
		if n > 0 && p.Stock >= 0 {
			return tx.Model(&p).Update("stock", gorm.Expr("stock + ?", n)).Error
		}
		if n > 0 && p.Stock < 0 {
			// keep unlimited
		}
		return nil
	})
	return n, err
}

func (s *ProductService) ListCards(productID uint, status string, limit int) ([]models.CardCode, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := s.DB.Where("product_id = ?", productID).Order("id desc").Limit(limit)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var list []models.CardCode
	err := q.Find(&list).Error
	return list, err
}

func (s *ProductService) CardStats(productID uint) (unused, sold int64, err error) {
	s.DB.Model(&models.CardCode{}).Where("product_id = ? AND status = ?", productID, models.CardUnused).Count(&unused)
	s.DB.Model(&models.CardCode{}).Where("product_id = ? AND status = ?", productID, models.CardSold).Count(&sold)
	return
}

// CreateShopOrder builds a shop-sourced pending order for a product.
func (s *ProductService) CreateShopOrder(orders *OrderService, productID uint, email, returnPath string) (*models.Order, error) {
	var p models.Product
	if err := s.DB.First(&p, productID).Error; err != nil || !p.Enable {
		return nil, fmt.Errorf("%w: product unavailable", ErrNotFound)
	}
	if !p.AutoGenerate {
		if p.UseCardPool {
			var unused int64
			s.DB.Model(&models.CardCode{}).Where("product_id = ? AND status = ?", p.ID, models.CardUnused).Count(&unused)
			if unused <= 0 && p.Stock == 0 {
				return nil, fmt.Errorf("%w: out of stock", ErrConflictClosed)
			}
			if p.Stock == 0 {
				return nil, fmt.Errorf("%w: out of stock", ErrConflictClosed)
			}
		} else if p.Stock == 0 {
			return nil, fmt.Errorf("%w: out of stock", ErrConflictClosed)
		}
	}

	ttl := time.Duration(s.Cfg.Shop.OrderTTLMin) * time.Minute
	expireAt := time.Now().Add(ttl)
	outNo := "SHOP" + randomHex(10)
	_ = returnPath
	o := &models.Order{
		PlatformTradeNo: newPlatformTradeNo(),
		OutTradeNo:      outNo,
		AppID:           "shop",
		Amount:          p.PriceCents,
		Currency:        p.Currency,
		Subject:         p.Name,
		Status:          models.StatusPending,
		CashierToken:    newCashierToken(),
		NotifyURL:       "", // shop: no K2 callback
		ReturnURL:       "", // set below
		UserRef:         strings.TrimSpace(email),
		BuyerEmail:      strings.TrimSpace(email),
		ExpireAt:        expireAt,
		NotifyStatus:    models.NotifyNone,
		Meta:            "{}",
		Source:          models.SourceShop,
		ProductID:       &p.ID,
	}
	o.ReturnURL = s.Cfg.Server.PublicBaseURL + "/shop/#/order/" + o.CashierToken
	if err := s.DB.Create(o).Error; err != nil {
		return nil, err
	}
	_ = orders // reserved
	return o, nil
}

// DeliverOnPaid assigns card code, auto-generates sim credentials, or template content.
func DeliverOnPaid(tx *gorm.DB, o *models.Order) (string, error) {
	if o.ProductID == nil {
		return o.Delivered, nil
	}
	var p models.Product
	if err := tx.First(&p, *o.ProductID).Error; err != nil {
		return "", nil
	}
	// 1) Prefer unused card from pool
	if p.UseCardPool {
		var card models.CardCode
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("product_id = ? AND status = ?", p.ID, models.CardUnused).
			Order("id asc").First(&card).Error
		if err == nil {
			now := time.Now()
			oid := o.ID
			if err := tx.Model(&card).Updates(map[string]any{
				"status": models.CardSold, "order_id": oid, "sold_at": now,
			}).Error; err != nil {
				return "", err
			}
			if p.Stock > 0 {
				_ = tx.Model(&p).Update("stock", gorm.Expr("CASE WHEN stock > 0 THEN stock - 1 ELSE 0 END")).Error
			}
			return card.Code, nil
		}
	}
	// 2) Auto-generate simulated digital goods (Apple ID / Netflix / etc.)
	if p.AutoGenerate {
		content := GenerateSimCredential(&p, o.OutTradeNo)
		// Persist as sold card for audit trail
		now := time.Now()
		oid := o.ID
		_ = tx.Create(&models.CardCode{
			ProductID: p.ID,
			Code:      content,
			Status:    models.CardSold,
			OrderID:   &oid,
			SoldAt:    &now,
		}).Error
		return content, nil
	}
	// 3) Template fallback
	content := fillTemplate(p.DeliverTemplate, p.Name, o.OutTradeNo)
	if content == "" {
		content = "Paid, but no stock left. Contact support. Order: " + o.OutTradeNo
	}
	if p.Stock > 0 {
		_ = tx.Model(&p).Update("stock", gorm.Expr("CASE WHEN stock > 0 THEN stock - 1 ELSE 0 END")).Error
	}
	return content, nil
}

func fillTemplate(tpl, name, trade string) string {
	if tpl == "" {
		return ""
	}
	r := strings.ReplaceAll(tpl, "{name}", name)
	r = strings.ReplaceAll(r, "{trade_no}", trade)
	return r
}


