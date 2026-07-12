package alipay

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	sdk "github.com/smartwalle/alipay/v3"

	"github.com/HenZenKuriRIP/NexusCard/internal/config"
)

// Client wraps smartwalle alipay SDK for giftcard-platform.
type Client struct {
	cfg    config.AlipayConfig
	inner  *sdk.Client
	public string // public_base_url for notify/return
}

func New(cfg config.AlipayConfig, publicBaseURL string) (*Client, error) {
	if !Configured(cfg) {
		return nil, fmt.Errorf("alipay: credentials not configured")
	}
	c := cfg
	c.AppID = strings.TrimSpace(c.AppID)
	c.PrivateKey = NormalizePEM(c.PrivateKey)
	c.AlipayPublicKey = NormalizePEM(c.AlipayPublicKey)
	c.Product = strings.ToLower(strings.TrimSpace(c.Product))
	if c.Product == "" {
		c.Product = "page"
	}
	if c.TimeoutExpress == "" {
		c.TimeoutExpress = "30m"
	}
	inner, err := sdk.New(c.AppID, c.PrivateKey, c.IsProduction)
	if err != nil {
		return nil, fmt.Errorf("alipay client: %w（检查私钥 PKCS1/PKCS8）", err)
	}
	if err := inner.LoadAliPayPublicKey(c.AlipayPublicKey); err != nil {
		return nil, fmt.Errorf("alipay public key: %w（须为支付宝公钥，非应用公钥）", err)
	}
	return &Client{cfg: c, inner: inner, public: strings.TrimRight(publicBaseURL, "/")}, nil
}

// Configured reports whether alipay credentials are present.
func Configured(cfg config.AlipayConfig) bool {
	return strings.TrimSpace(cfg.AppID) != "" &&
		strings.TrimSpace(cfg.PrivateKey) != "" &&
		strings.TrimSpace(cfg.AlipayPublicKey) != ""
}

func NormalizePEM(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\\n", "\n")
	return strings.TrimSpace(s)
}

// PayURL builds page/wap pay URL. outTradeNo must be platform_trade_no.
func (c *Client) PayURL(ctx context.Context, outTradeNo, subject string, amountCents int64, expireAt time.Time, productOverride, userAgent string) (string, error) {
	_ = ctx
	amount := FormatYuan(amountCents)
	// Prefer fixed neutral bill title from config; never pass raw plan/VPN names to Alipay.
	sub := BillSubject(c.cfg.BillSubject, subject)
	timeout := TimeoutExpress(c.cfg.TimeoutExpress, expireAt)
	notifyURL := c.public + "/alipay/notify"
	returnURL := c.public + "/c/return?out_trade_no=" + url.QueryEscape(outTradeNo)

	product := c.cfg.Product
	if productOverride != "" {
		product = productOverride
	}
	if product == "auto" {
		if isMobileUA(userAgent) {
			product = "wap"
		} else {
			product = "page"
		}
	}

	var payURL *url.URL
	var err error
	switch product {
	case "wap":
		p := sdk.TradeWapPay{}
		p.NotifyURL = notifyURL
		p.ReturnURL = returnURL
		p.Subject = sub
		p.OutTradeNo = outTradeNo
		p.TotalAmount = amount
		p.ProductCode = "QUICK_WAP_WAY"
		p.Body = sub
		p.GoodsType = "0"
		p.TimeoutExpress = timeout
		payURL, err = c.inner.TradeWapPay(p)
	default:
		p := sdk.TradePagePay{}
		p.NotifyURL = notifyURL
		p.ReturnURL = returnURL
		p.Subject = sub
		p.OutTradeNo = outTradeNo
		p.TotalAmount = amount
		p.ProductCode = "FAST_INSTANT_TRADE_PAY"
		p.Body = sub
		p.GoodsType = "0"
		p.TimeoutExpress = timeout
		payURL, err = c.inner.TradePagePay(p)
	}
	if err != nil {
		return "", fmt.Errorf("alipay create pay: %w", err)
	}
	if payURL == nil || payURL.String() == "" {
		return "", fmt.Errorf("alipay: empty pay url")
	}
	return payURL.String(), nil
}

// NotifyResult is a verified Alipay async notification.
type NotifyResult struct {
	OutTradeNo  string // platform_trade_no
	TradeNo     string // alipay trade no
	TradeStatus string
	TotalCents  int64
	AppID       string
	Success     bool // TRADE_SUCCESS / FINISHED
	Raw         string
}

func (c *Client) DecodeNotify(ctx context.Context, form url.Values) (*NotifyResult, error) {
	_ = ctx
	noti, err := c.inner.DecodeNotification(form)
	if err != nil {
		return nil, fmt.Errorf("alipay verify notify: %w", err)
	}
	if noti.AppId != "" && !strings.EqualFold(noti.AppId, c.cfg.AppID) {
		return nil, fmt.Errorf("alipay app_id mismatch")
	}
	res := &NotifyResult{
		OutTradeNo:  noti.OutTradeNo,
		TradeNo:     noti.TradeNo,
		TradeStatus: string(noti.TradeStatus),
		TotalCents:  ParseYuanToCents(noti.TotalAmount),
		AppID:       noti.AppId,
		Raw:         form.Encode(),
	}
	switch noti.TradeStatus {
	case sdk.TradeStatusSuccess, sdk.TradeStatusFinished:
		res.Success = true
	}
	return res, nil
}

func (c *Client) Query(ctx context.Context, platformTradeNo string) (paid bool, paidCents int64, alipayTradeNo string, err error) {
	rsp, err := c.inner.TradeQuery(ctx, sdk.TradeQuery{OutTradeNo: platformTradeNo})
	if err != nil {
		return false, 0, "", err
	}
	if rsp == nil || !rsp.IsSuccess() {
		return false, 0, "", nil
	}
	paid = rsp.TradeStatus == sdk.TradeStatusSuccess || rsp.TradeStatus == sdk.TradeStatusFinished
	return paid, ParseYuanToCents(rsp.TotalAmount), rsp.TradeNo, nil
}

func (c *Client) Close(ctx context.Context, platformTradeNo string) error {
	rsp, err := c.inner.TradeClose(ctx, sdk.TradeClose{OutTradeNo: platformTradeNo})
	if err != nil {
		return err
	}
	if rsp != nil && !rsp.IsSuccess() {
		sub := rsp.SubCode
		if sub == "ACQ.TRADE_NOT_EXIST" || sub == "ACQ.TRADE_STATUS_ERROR" ||
			strings.Contains(rsp.SubMsg, "交易不存在") || strings.Contains(rsp.SubMsg, "Status不合法") {
			return nil
		}
		return fmt.Errorf("alipay trade.close: %s %s", rsp.SubCode, rsp.SubMsg)
	}
	return nil
}

// FormatYuan converts cents to Alipay major-unit string.
func FormatYuan(cents int64) string {
	if cents < 0 {
		cents = 0
	}
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

func ParseYuanToCents(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	if f < 0 {
		return 0
	}
	return int64(f*100 + 0.5)
}

// DefaultBillSubject is the neutral Alipay title when content is empty or scrubbed.
const DefaultBillSubject = "Digital Goods"

func SanitizeSubject(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return DefaultBillSubject
	}
	replacer := strings.NewReplacer(
		"/", " ", "\\", " ", "=", " ", "&", " ", "?", " ",
		"#", " ", "%", " ", "<", " ", ">", " ", "\"", " ", "'", " ",
		"@", " ", ":", " ",
	)
	s = replacer.Replace(s)

	lower := strings.ToLower(s)
	for _, w := range []string{
		"k2board", "anytls", "v2ray", "xray", "hysteria", "shadowsocks", "trojan",
		"wireguard", "openvpn", "clash", "sing-box", "singbox", "vpn", "proxy",
		"vmess", "vless", "ssr",
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
	for _, w := range []string{
		"机场", "科学上网", "翻墙", "翻牆", "加速器", "梯子", "节点订阅", "订阅链接",
		"代理节点", "流量套餐", "机场套餐", "翻墙套餐", "魔法上网",
	} {
		s = strings.ReplaceAll(s, w, " ")
	}

	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimSpace(s)
	if s == "" {
		return DefaultBillSubject
	}
	if utf8.RuneCountInString(s) > 256 {
		s = string([]rune(s)[:256])
	}
	return s
}

// BillSubject picks Alipay subject: config override > sanitized order subject > default.
func BillSubject(configured, orderSubject string) string {
	if t := strings.TrimSpace(configured); t != "" {
		return SanitizeSubject(t)
	}
	return SanitizeSubject(orderSubject)
}

// TimeoutExpress = min(config, remaining until expire_at), at least 1m.
func TimeoutExpress(cfgExpr string, expireAt time.Time) string {
	cfgMin := parseExpressMinutes(cfgExpr)
	if cfgMin <= 0 {
		cfgMin = 30
	}
	rem := time.Until(expireAt)
	remMin := int(rem.Minutes())
	if remMin < 1 {
		remMin = 1
	}
	if remMin < cfgMin {
		cfgMin = remMin
	}
	return fmt.Sprintf("%dm", cfgMin)
}

func parseExpressMinutes(s string) int {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 30
	}
	s = strings.TrimSuffix(s, "m")
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 30
	}
	return n
}

func isMobileUA(ua string) bool {
	ua = strings.ToLower(ua)
	for _, k := range []string{"mobile", "android", "iphone", "ipad", "micromessenger"} {
		if strings.Contains(ua, k) {
			return true
		}
	}
	return false
}
