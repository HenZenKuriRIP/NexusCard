package httpserver

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/HenZenKuriRIP/NexusCard/internal/models"
	"github.com/HenZenKuriRIP/NexusCard/internal/service"
)

func (s *Server) registerShopAPI(r *gin.Engine) {
	api := r.Group("/api/shop/v1")
	api.Use(rateLimitMiddleware(limShopRead, "shop_read"))
	{
		api.GET("/config", s.shopConfig)
		api.GET("/categories", s.shopCategories)
		api.GET("/products", s.shopListProducts)
		api.GET("/products/:id", s.shopGetProduct)
		api.GET("/orders/by-token", rateLimitMiddleware(limPublicPay, "shop_order"), s.shopOrderByToken)
		api.POST("/checkout", rateLimitMiddleware(limShopWrite, "shop_checkout"), s.shopCheckout)
	}
}

func (s *Server) shopConfig(c *gin.Context) {
	view := s.Settings.AlipayPublicView()
	ev := s.Settings.EpayPublicView()
	c.JSON(http.StatusOK, gin.H{
		"title":             s.Cfg.Shop.Title,
		"subtitle":          s.Cfg.Shop.Subtitle,
		"mock_pay":          view["mock_pay"],
		"alipay_configured": view["effective_enabled"],
		"epay_configured":   ev["effective_enabled"],
		"site_name":         s.Cfg.Admin.SiteName,
		"features": []string{
			"PaidAuto delivery", "US ID / gift cards", "Netflix & streaming", "DataData", "Orderslookup",
		},
	})
}

func (s *Server) shopCategories(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"categories": service.ShopCategories()})
}

func (s *Server) shopListProducts(c *gin.Context) {
	list, err := s.Products.ListPublic(c.Query("category"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	type item struct {
		ID          uint     `json:"id"`
		Name        string   `json:"name"`
		Slug        string   `json:"slug"`
		Description string   `json:"description"`
		Category    string   `json:"category"`
		Region      string   `json:"region"`
		Badge       string   `json:"badge"`
		Icon        string   `json:"icon"`
		CoverURL    string   `json:"cover_url"`
		Features    []string `json:"features"`
		PriceCents  int64    `json:"price_cents"`
		Currency    string   `json:"currency"`
		Stock       int      `json:"stock"`
		InStock     bool     `json:"in_stock"`
		UseCardPool bool     `json:"use_card_pool"`
	}
	out := make([]item, 0, len(list))
	for i := range list {
		p := &list[i]
		out = append(out, item{
			ID: p.ID, Name: p.Name, Slug: p.Slug, Description: p.Description,
			Category: p.Category, Region: p.Region, Badge: p.Badge, Icon: p.Icon,
			CoverURL: p.CoverURL, Features: splitFeatures(p.Features),
			PriceCents: p.PriceCents, Currency: p.Currency, Stock: p.Stock,
			InStock: s.productInStock(p), UseCardPool: p.UseCardPool,
		})
	}
	c.JSON(http.StatusOK, gin.H{"products": out})
}

func (s *Server) shopGetProduct(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	p, err := s.Products.Get(uint(id))
	if err != nil || !p.Enable {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id": p.ID, "name": p.Name, "slug": p.Slug, "description": p.Description,
		"category": p.Category, "region": p.Region, "badge": p.Badge, "icon": p.Icon,
		"cover_url": p.CoverURL, "features": splitFeatures(p.Features),
		"price_cents": p.PriceCents, "currency": p.Currency, "stock": p.Stock,
		"in_stock": s.productInStock(p), "use_card_pool": p.UseCardPool,
	})
}

func (s *Server) shopCheckout(c *gin.Context) {
	var body struct {
		ProductID uint   `json:"product_id"`
		Email     string `json:"email"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.ProductID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "product_id required"})
		return
	}
	o, err := s.Products.CreateShopOrder(s.Orders, body.ProductID, body.Email, "")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	v := s.Orders.ToView(o)
	c.JSON(http.StatusOK, gin.H{"order": v, "cashier_url": v.CashierURL})
}

func (s *Server) shopOrderByToken(c *gin.Context) {
	token := c.Query("token")
	o, err := s.Orders.GetByCashierToken(token)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, s.Orders.ToView(o))
}

func (s *Server) productInStock(p *models.Product) bool {
	if p.AutoGenerate {
		return true
	}
	if p.UseCardPool {
		u, _, _ := s.Products.CardStats(p.ID)
		if u > 0 {
			return true
		}
		return p.Stock < 0
	}
	return p.Stock != 0
}

func splitFeatures(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == '|' || r == ';' })
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
