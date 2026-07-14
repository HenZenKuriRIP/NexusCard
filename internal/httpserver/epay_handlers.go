package httpserver

import (
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// publicEpayPay returns { pay_url } for cashier redirect to 彩虹易支付.
// Body: { "token": "...", "type": "alipay|wxpay|..." }
func (s *Server) publicEpayPay(c *gin.Context) {
	var body struct {
		Token string `json:"token"`
		Type  string `json:"type"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Token == "" {
		JSONErr(c, http.StatusBadRequest, 40001, "token required")
		return
	}
	if s.Epay == nil || !s.Epay.Configured() {
		JSONErr(c, http.StatusBadRequest, 40001, "Epay is not configured: set api_url / pid / key")
		return
	}
	payURL, err := s.Epay.BuildPayURL(c.Request.Context(), body.Token, body.Type)
	if err != nil {
		JSONErr(c, http.StatusBadRequest, 40001, err.Error())
		return
	}
	JSONOK(c, gin.H{"pay_url": payURL})
}

// epayNotify handles async notify (GET or POST form). Must respond plain "success".
func (s *Server) epayNotify(c *gin.Context) {
	if s.Epay == nil || !s.Epay.Configured() {
		c.String(http.StatusServiceUnavailable, "fail")
		return
	}
	values, err := parseNotifyValues(c)
	if err != nil || len(values) == 0 {
		c.String(http.StatusBadRequest, "fail")
		return
	}
	if err := s.Epay.HandleNotify(c.Request.Context(), values); err != nil {
		slog.Warn("epay notify handle", "err", err)
		c.String(http.StatusBadRequest, "fail")
		return
	}
	c.String(http.StatusOK, "success")
}

// epayBrowserReturn: epay return_url → redirect to cashier (notify is source of truth).
func (s *Server) epayBrowserReturn(c *gin.Context) {
	_ = c.Request.ParseForm()
	ptn := firstQuery(c, "out_trade_no")
	if ptn == "" {
		c.Redirect(http.StatusFound, "/shop/")
		return
	}
	// Best-effort: if signed GET return includes success fields, try accept (many panels only sign notify).
	if s.Epay != nil && s.Epay.Configured() {
		values := c.Request.URL.Query()
		if len(values) == 0 {
			values = c.Request.Form
		}
		if values.Get("sign") != "" {
			if err := s.Epay.HandleNotify(c.Request.Context(), values); err != nil {
				slog.Debug("epay return sync", "err", err)
			}
		}
	}
	o, err := s.Orders.FindByPlatformTradeNo(ptn)
	if err != nil {
		c.String(http.StatusOK, "Payment result is processing. Check the merchant order page.")
		return
	}
	// Optional active query if still pending
	if s.Epay != nil && s.Epay.Configured() && (o.Status == "pending" || o.Status == "expired" || o.Status == "closed") {
		if synced, err := s.Epay.SyncFromEpay(c.Request.Context(), o.ID); err == nil {
			o = synced
		}
	}
	c.Redirect(http.StatusFound, "/c/"+o.CashierToken)
}

func parseNotifyValues(c *gin.Context) (url.Values, error) {
	if c.Request.Method == http.MethodGet {
		return c.Request.URL.Query(), nil
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if len(body) > 0 {
		if values, err := url.ParseQuery(string(body)); err == nil && len(values) > 0 {
			return values, nil
		}
	}
	_ = c.Request.ParseForm()
	if len(c.Request.Form) > 0 {
		return c.Request.Form, nil
	}
	// fallback query on POST
	return c.Request.URL.Query(), nil
}

func firstQuery(c *gin.Context, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(c.Query(k)); v != "" {
			return v
		}
		if v := strings.TrimSpace(c.Request.FormValue(k)); v != "" {
			return v
		}
	}
	return ""
}
