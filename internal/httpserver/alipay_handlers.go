package httpserver

import (
	"io"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
)

// publicAlipayPay returns { pay_url } for cashier redirect to Alipay.
func (s *Server) publicAlipayPay(c *gin.Context) {
	var body struct {
		Token string `json:"token"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Token == "" {
		JSONErr(c, http.StatusBadRequest, 40001, "token required")
		return
	}
	if s.Alipay == nil || !s.Alipay.Configured() {
		JSONErr(c, http.StatusBadRequest, 40001, "Alipay is not configured: set app_id / private_key / alipay_public_key")
		return
	}
	payURL, err := s.Alipay.BuildPayURL(c.Request.Context(), body.Token, c.GetHeader("User-Agent"))
	if err != nil {
		JSONErr(c, http.StatusBadRequest, 40001, err.Error())
		return
	}
	JSONOK(c, gin.H{"pay_url": payURL})
}

func (s *Server) alipayNotify(c *gin.Context) {
	if s.Alipay == nil || !s.Alipay.Configured() {
		c.String(http.StatusServiceUnavailable, "fail")
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		c.String(http.StatusBadRequest, "fail")
		return
	}
	values, err := url.ParseQuery(string(body))
	if err != nil || len(values) == 0 {
		_ = c.Request.ParseForm()
		values = c.Request.Form
	}
	if len(values) == 0 {
		c.String(http.StatusBadRequest, "fail")
		return
	}
	if err := s.Alipay.HandleNotify(c.Request.Context(), values); err != nil {
		slog.Warn("alipay notify handle", "err", err)
		c.String(http.StatusBadRequest, "fail")
		return
	}
	c.String(http.StatusOK, "success")
}

// alipayBrowserReturn: Alipay return_url → sync + redirect to cashier.
func (s *Server) alipayBrowserReturn(c *gin.Context) {
	ptn := c.Query("out_trade_no")
	if ptn == "" {
		_ = c.Request.ParseForm()
		ptn = c.Request.FormValue("out_trade_no")
	}
	if ptn == "" {
		c.Redirect(http.StatusFound, "/shop/")
		return
	}
	o, err := s.Orders.FindByPlatformTradeNo(ptn)
	if err != nil {
		c.String(http.StatusOK, "Payment result is processing. Check the merchant order page.")
		return
	}
	if s.Alipay != nil && s.Alipay.Configured() {
		if o.Status == "pending" || o.Status == "expired" || o.Status == "closed" {
			if synced, err := s.Alipay.SyncFromAlipay(c.Request.Context(), o.ID); err == nil {
				o = synced
			}
		}
	}
	c.Redirect(http.StatusFound, "/c/"+o.CashierToken)
}
