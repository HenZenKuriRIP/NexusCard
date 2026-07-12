package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/HenZenKuriRIP/NexusCard/internal/models"
	"github.com/HenZenKuriRIP/NexusCard/internal/service"
)

func (s *Server) registerAdmin(r *gin.Engine) {
	// Public login (rate limited)
	r.POST("/admin/api/v1/auth/login", rateLimitMiddleware(limAdminLogin, "admin_login"), s.adminLogin)

	ad := r.Group("/admin/api/v1")
	ad.Use(s.adminAuthMiddleware())
	{
		ad.GET("/auth/me", s.adminMe)
		ad.POST("/auth/password", s.adminChangePassword)
		ad.GET("/dashboard", s.adminDashboard)

		ad.GET("/orders", s.adminListOrders)
		ad.GET("/orders/:id", s.adminGetOrder)
		ad.POST("/orders/:id/renotify", s.adminRenotify)
		ad.POST("/orders/:id/close", s.adminClose)
		ad.POST("/orders/:id/sync-alipay", s.adminSyncAlipay)

		ad.GET("/products", s.adminListProducts)
		ad.POST("/products", s.adminCreateProduct)
		ad.GET("/products/:id", s.adminGetProduct)
		ad.PUT("/products/:id", s.adminUpdateProduct)
		ad.DELETE("/products/:id", s.adminDeleteProduct)
		ad.POST("/products/:id/cards", s.adminImportCards)
		ad.GET("/products/:id/cards", s.adminListCards)

		ad.GET("/merchants", s.adminListMerchants)
		ad.POST("/merchants", s.adminCreateMerchant)
		ad.PUT("/merchants/:id", s.adminUpdateMerchant)

		ad.GET("/settings", s.adminSettings)
		ad.GET("/settings/payment", s.adminGetPayment)
		ad.PUT("/settings/payment", s.adminSavePayment)
	}
}

func (s *Server) adminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		const p = "Bearer "
		if !strings.HasPrefix(h, p) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		tok := strings.TrimSpace(h[len(p):])
		// Legacy static token (scripts)
		if tok == s.Cfg.Server.AdminToken {
			c.Set("admin_user", &models.AdminUser{ID: 0, Username: "token", DisplayName: "API Token"})
			c.Next()
			return
		}
		u, err := s.Auth.ParseToken(tok)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Set("admin_user", u)
		c.Next()
	}
}

func adminUser(c *gin.Context) *models.AdminUser {
	v, _ := c.Get("admin_user")
	u, _ := v.(*models.AdminUser)
	return u
}

func (s *Server) adminLogin(c *gin.Context) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	tok, u, err := s.Auth.Login(body.Username, body.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid username or password"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token": tok,
		"user": gin.H{
			"id": u.ID, "username": u.Username, "display_name": u.DisplayName,
		},
	})
}

func (s *Server) adminMe(c *gin.Context) {
	u := adminUser(c)
	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{"id": u.ID, "username": u.Username, "display_name": u.DisplayName},
		"site_name": s.Cfg.Admin.SiteName,
	})
}

func (s *Server) adminChangePassword(c *gin.Context) {
	u := adminUser(c)
	if u.ID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "API token cannot change password; use account login"})
		return
	}
	var body struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := s.Auth.ChangePassword(u.ID, body.OldPassword, body.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) adminDashboard(c *gin.Context) {
	c.JSON(http.StatusOK, s.Orders.Stats())
}

func (s *Server) adminListOrders(c *gin.Context) {
	list, err := s.Orders.ListAdmin(c.Query("status"), c.Query("source"), 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	views := make([]service.OrderView, 0, len(list))
	for i := range list {
		views = append(views, s.Orders.ToView(&list[i]))
	}
	c.JSON(http.StatusOK, gin.H{"orders": views})
}

func (s *Server) adminGetOrder(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	o, err := s.Orders.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, s.Orders.ToView(o))
}

func (s *Server) adminRenotify(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := s.Orders.EnqueueRenotify(uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) adminClose(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	o, err := s.Orders.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	_, err = s.Orders.Close(o.AppID, o.OutTradeNo, "admin_close")
	if err != nil && !errors.Is(err, service.ErrConflictPaid) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	o2, _ := s.Orders.GetByID(uint(id))
	c.JSON(http.StatusOK, s.Orders.ToView(o2))
}

func (s *Server) adminListProducts(c *gin.Context) {
	list, err := s.Products.ListAdmin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// attach card stats
	type row struct {
		models.Product
		UnusedCards int64 `json:"unused_cards"`
		SoldCards   int64 `json:"sold_cards"`
	}
	out := make([]row, 0, len(list))
	for _, p := range list {
		u, sold, _ := s.Products.CardStats(p.ID)
		out = append(out, row{Product: p, UnusedCards: u, SoldCards: sold})
	}
	c.JSON(http.StatusOK, gin.H{"products": out})
}

func (s *Server) adminCreateProduct(c *gin.Context) {
	var in service.ProductInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	p, err := s.Products.Create(in)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, p)
}

func (s *Server) adminGetProduct(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	p, err := s.Products.Get(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	u, sold, _ := s.Products.CardStats(p.ID)
	c.JSON(http.StatusOK, gin.H{"product": p, "unused_cards": u, "sold_cards": sold})
}

func (s *Server) adminUpdateProduct(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var in service.ProductInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	p, err := s.Products.Update(uint(id), in)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, p)
}

func (s *Server) adminDeleteProduct(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := s.Products.Delete(uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) adminImportCards(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var body struct {
		Codes string `json:"codes"` // newline or comma separated
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	raw := strings.ReplaceAll(body.Codes, ",", "\n")
	parts := strings.Split(raw, "\n")
	n, err := s.Products.ImportCards(uint(id), parts)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"imported": n})
}

func (s *Server) adminListCards(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	list, err := s.Products.ListCards(uint(id), c.Query("status"), 200)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"cards": list})
}

func (s *Server) adminListMerchants(c *gin.Context) {
	var list []models.Merchant
	_ = s.DB.Find(&list).Error
	type view struct {
		ID      uint   `json:"id"`
		AppID   string `json:"app_id"`
		Name    string `json:"name"`
		Enable  bool   `json:"enable"`
	}
	out := make([]view, 0, len(list))
	for _, m := range list {
		out = append(out, view{ID: m.ID, AppID: m.AppID, Name: m.Name, Enable: m.Enable})
	}
	c.JSON(http.StatusOK, gin.H{"merchants": out})
}

func (s *Server) adminCreateMerchant(c *gin.Context) {
	var body struct {
		AppID     string `json:"app_id"`
		Name      string `json:"name"`
		APISecret string `json:"api_secret"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.AppID == "" || body.APISecret == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "app_id and api_secret required"})
		return
	}
	m := models.Merchant{
		AppID: body.AppID, Name: body.Name, APISecret: body.APISecret, Enable: true,
		NotifyURLHostAllowlist: "[]",
	}
	if err := s.DB.Create(&m).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": m.ID, "app_id": m.AppID, "name": m.Name, "enable": m.Enable})
}

func (s *Server) adminUpdateMerchant(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var m models.Merchant
	if err := s.DB.First(&m, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var body struct {
		Name      *string `json:"name"`
		Enable    *bool   `json:"enable"`
		APISecret *string `json:"api_secret"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Name != nil {
		m.Name = *body.Name
	}
	if body.Enable != nil {
		m.Enable = *body.Enable
	}
	if body.APISecret != nil && *body.APISecret != "" {
		m.APISecretPrev = m.APISecret
		m.APISecret = *body.APISecret
	}
	if err := s.DB.Save(&m).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": m.ID, "app_id": m.AppID, "name": m.Name, "enable": m.Enable})
}

func (s *Server) adminSyncAlipay(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if s.Alipay == nil || !s.Alipay.Configured() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "alipay not configured"})
		return
	}
	o, err := s.Alipay.SyncFromAlipay(c.Request.Context(), uint(id))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, s.Orders.ToView(o))
}

func (s *Server) adminSettings(c *gin.Context) {
	pay := s.Settings.AlipayPublicView()
	c.JSON(http.StatusOK, gin.H{
		"site_name":         s.Cfg.Admin.SiteName,
		"public_base_url":   s.Cfg.Server.PublicBaseURL,
		"mock_pay":          pay["mock_pay"],
		"alipay_configured": pay["effective_enabled"],
		"alipay_app_id":     pay["app_id_masked"],
		"alipay_production": pay["is_production"],
		"alipay_product":    pay["product"],
		"alipay_notify_url": s.Cfg.Server.PublicBaseURL + "/alipay/notify",
		"shop_title":        s.Cfg.Shop.Title,
		"shop_subtitle":     s.Cfg.Shop.Subtitle,
		"seed_merchant": gin.H{
			"app_id": s.Cfg.SeedMerchant.AppID,
		},
		"payment": pay,
	})
}

func (s *Server) adminGetPayment(c *gin.Context) {
	a := s.Settings.GetAlipay()
	// Never return full secrets — only whether set + masked app id
	c.JSON(http.StatusOK, gin.H{
		"app_id":            a.AppID,
		"private_key":       "", // blank for edit; leave empty to keep
		"alipay_public_key": "",
		"has_private_key":   a.PrivateKey != "",
		"has_public_key":    a.AlipayPublicKey != "",
		"is_production":     a.IsProduction,
		"product":           a.Product,
		"timeout_express":   a.TimeoutExpress,
		"mock_pay":          a.MockPay,
		"bill_subject":      a.BillSubject,
		"enabled":           a.Enabled,
		"notify_url":        s.Cfg.Server.PublicBaseURL + "/alipay/notify",
		"public_base_url":   s.Cfg.Server.PublicBaseURL,
	})
}

func (s *Server) adminSavePayment(c *gin.Context) {
	var in service.AlipaySettings
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.Settings.SaveAlipay(in); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.reloadAlipay()
	c.JSON(http.StatusOK, gin.H{"ok": true, "payment": s.Settings.AlipayPublicView()})
}

func maskAppID(id string) string {
	if len(id) <= 4 {
		return id
	}
	return id[:2] + "****" + id[len(id)-2:]
}
