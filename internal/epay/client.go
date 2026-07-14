package epay

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Config for 彩虹易支付 / V1 易支付 (MD5).
type Config struct {
	APIURL  string // e.g. https://pay.example.com
	PID     string
	Key     string
	Types   string // comma-separated: alipay,wxpay,qqpay
	Enabled bool
	Name    string // default product name on bill
}

// Client talks to an epay-compatible gateway.
type Client struct {
	cfg    Config
	public string // public_base_url of this platform
	http   *http.Client
}

func New(cfg Config, publicBaseURL string) (*Client, error) {
	if !Configured(cfg) {
		return nil, fmt.Errorf("epay: credentials not configured")
	}
	cfg.APIURL = strings.TrimRight(strings.TrimSpace(cfg.APIURL), "/")
	cfg.PID = strings.TrimSpace(cfg.PID)
	cfg.Key = strings.TrimSpace(cfg.Key)
	if cfg.Types == "" {
		cfg.Types = "alipay"
	}
	return &Client{
		cfg:    cfg,
		public: strings.TrimRight(publicBaseURL, "/"),
		http:   &http.Client{Timeout: 20 * time.Second},
	}, nil
}

func Configured(cfg Config) bool {
	return strings.TrimSpace(cfg.APIURL) != "" &&
		strings.TrimSpace(cfg.PID) != "" &&
		strings.TrimSpace(cfg.Key) != ""
}

// TypeList returns configured pay types.
func (c *Client) TypeList() []string {
	return ParseTypes(c.cfg.Types)
}

func ParseTypes(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return []string{"alipay"}
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	if len(out) == 0 {
		return []string{"alipay"}
	}
	return out
}

// TypeAllowed reports whether t is in the configured type list.
func (c *Client) TypeAllowed(t string) bool {
	t = strings.ToLower(strings.TrimSpace(t))
	for _, x := range c.TypeList() {
		if x == t {
			return true
		}
	}
	return false
}

// SignMD5 is standard V1 易支付 signature:
// sort non-empty params (except sign/sign_type) by key, join k=v&..., append key, MD5 lower hex.
func SignMD5(params map[string]string, key string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "sign" || k == "sign_type" || v == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+params[k])
	}
	raw := strings.Join(parts, "&") + key
	sum := md5.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// VerifySign checks sign in params against key.
func VerifySign(params map[string]string, key string) bool {
	got := strings.ToLower(strings.TrimSpace(params["sign"]))
	if got == "" {
		return false
	}
	want := SignMD5(params, key)
	return got == want
}

// BuildSubmitURL builds page-jump pay URL (submit.php).
// outTradeNo must be platform_trade_no.
func (c *Client) BuildSubmitURL(outTradeNo, subject string, amountCents int64, payType string) (string, error) {
	payType = strings.ToLower(strings.TrimSpace(payType))
	if payType == "" {
		payType = c.TypeList()[0]
	}
	if !c.TypeAllowed(payType) {
		return "", fmt.Errorf("epay type not allowed: %s", payType)
	}
	name := strings.TrimSpace(subject)
	if n := strings.TrimSpace(c.cfg.Name); n != "" {
		name = n
	}
	if name == "" {
		name = "Digital Goods"
	}
	// epay name length limits vary; keep reasonable
	if utf8Len(name) > 64 {
		name = truncateRunes(name, 64)
	}
	params := map[string]string{
		"pid":          c.cfg.PID,
		"type":         payType,
		"out_trade_no": outTradeNo,
		"notify_url":   c.public + "/epay/notify",
		"return_url":   c.public + "/epay/return",
		"name":         name,
		"money":        FormatYuan(amountCents),
	}
	params["sign"] = SignMD5(params, c.cfg.Key)
	params["sign_type"] = "MD5"

	q := url.Values{}
	for k, v := range params {
		q.Set(k, v)
	}
	return c.cfg.APIURL + "/submit.php?" + q.Encode(), nil
}

// NotifyResult is a verified async notification.
type NotifyResult struct {
	OutTradeNo  string // platform_trade_no
	TradeNo     string // gateway trade no
	TradeStatus string
	Type        string
	TotalCents  int64
	Success     bool
}

// DecodeNotify verifies MD5 sign and parses payment result.
// Accepts both form values (POST) and query (GET).
func (c *Client) DecodeNotify(form url.Values) (*NotifyResult, error) {
	params := valuesToMap(form)
	if !VerifySign(params, c.cfg.Key) {
		return nil, fmt.Errorf("epay notify sign invalid")
	}
	// pid check when present
	if pid := strings.TrimSpace(params["pid"]); pid != "" && pid != c.cfg.PID {
		return nil, fmt.Errorf("epay pid mismatch")
	}
	status := strings.TrimSpace(params["trade_status"])
	if status == "" {
		// some panels use status=1
		if params["status"] == "1" || params["status"] == "success" {
			status = "TRADE_SUCCESS"
		}
	}
	res := &NotifyResult{
		OutTradeNo:  strings.TrimSpace(params["out_trade_no"]),
		TradeNo:     strings.TrimSpace(params["trade_no"]),
		TradeStatus: status,
		Type:        strings.TrimSpace(params["type"]),
		TotalCents:  ParseYuanToCents(params["money"]),
	}
	if res.OutTradeNo == "" {
		return nil, fmt.Errorf("epay notify missing out_trade_no")
	}
	switch strings.ToUpper(status) {
	case "TRADE_SUCCESS", "TRADE_FINISHED", "SUCCESS":
		res.Success = true
	default:
		// paid if money present and sign ok and status empty on some forks
		if status == "" && res.TotalCents > 0 && res.TradeNo != "" {
			res.Success = true
			res.TradeStatus = "TRADE_SUCCESS"
		}
	}
	return res, nil
}

// QueryOrder calls api.php?act=order (common 彩虹易支付).
func (c *Client) QueryOrder(ctx context.Context, outTradeNo string) (paid bool, paidCents int64, tradeNo string, err error) {
	u, err := url.Parse(c.cfg.APIURL + "/api.php")
	if err != nil {
		return false, 0, "", err
	}
	q := u.Query()
	q.Set("act", "order")
	q.Set("pid", c.cfg.PID)
	q.Set("key", c.cfg.Key)
	q.Set("out_trade_no", outTradeNo)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return false, 0, "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false, 0, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return false, 0, "", err
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return false, 0, "", fmt.Errorf("epay query parse: %w", err)
	}
	code := toInt(raw["code"])
	if code != 1 {
		// not found / unpaid — not an error for sync
		return false, 0, "", nil
	}
	// status: 1 paid, 0 unpaid
	st := toInt(raw["status"])
	tradeNo = toString(raw["trade_no"])
	paidCents = ParseYuanToCents(toString(raw["money"]))
	return st == 1, paidCents, tradeNo, nil
}

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
	return int64(f*100 + 0.5)
}

func valuesToMap(v url.Values) map[string]string {
	m := make(map[string]string, len(v))
	for k := range v {
		m[k] = strings.TrimSpace(v.Get(k))
	}
	return m
}

func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		// json numbers
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	default:
		if v == nil {
			return ""
		}
		return fmt.Sprint(v)
	}
}

func toInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	case json.Number:
		n, _ := t.Int64()
		return int(n)
	default:
		return 0
	}
}

func utf8Len(s string) int {
	return len([]rune(s))
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
