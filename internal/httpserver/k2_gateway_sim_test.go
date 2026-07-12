package httpserver_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HenZenKuriRIP/NexusCard/internal/config"
	"github.com/HenZenKuriRIP/NexusCard/internal/database"
	"github.com/HenZenKuriRIP/NexusCard/internal/httpserver"
	"github.com/HenZenKuriRIP/NexusCard/internal/service"
	"github.com/HenZenKuriRIP/NexusCard/internal/sign"
)

// Simulates K2Board giftcard Gateway client against a real platform instance.
// Mirrors internal/payment/gateways/giftcard.go behavior for contract confidence.
func TestK2GatewaySim_FullFlow(t *testing.T) {
	var notifyOK atomic.Bool
	k2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		// K2 HandleNotify-equivalent
		var n struct {
			AppID         string `json:"app_id"`
			OutTradeNo    string `json:"out_trade_no"`
			PlatformTN    string `json:"platform_trade_no"`
			Amount        int64  `json:"amount"`
			PaidAmount    int64  `json:"paid_amount"`
			Currency      string `json:"currency"`
			Status        string `json:"status"`
			AlipayTradeNo string `json:"alipay_trade_no"`
			PaidAt        int64  `json:"paid_at"`
			Timestamp     int64  `json:"timestamp"`
			Nonce         string `json:"nonce"`
			Signature     string `json:"signature"`
		}
		if err := json.Unmarshal(raw, &n); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		params := map[string]string{
			"app_id": n.AppID, "out_trade_no": n.OutTradeNo, "platform_trade_no": n.PlatformTN,
			"amount": strconv.FormatInt(n.Amount, 10), "paid_amount": strconv.FormatInt(n.PaidAmount, 10),
			"currency": n.Currency, "status": n.Status, "alipay_trade_no": n.AlipayTradeNo,
			"paid_at": strconv.FormatInt(n.PaidAt, 10), "timestamp": strconv.FormatInt(n.Timestamp, 10),
			"nonce": n.Nonce,
		}
		if !strings.EqualFold(sign.SignMD5(params, "test_secret"), n.Signature) {
			http.Error(w, "bad signature", 400)
			return
		}
		if n.AppID != "k2-main" || n.PaidAmount != 1999 || n.Status != "paid" {
			http.Error(w, "amount mismatch", 400)
			return
		}
		notifyOK.Store(true)
		w.Write([]byte("ok"))
	}))
	defer k2.Close()

	dir := t.TempDir()
	cfg := &config.Config{}
	cfg.Server.AdminToken = "a"
	cfg.Server.PublicBaseURL = "http://unused"
	cfg.DB.Driver = "sqlite"
	cfg.DB.DSN = filepath.Join(dir, "x.db")
	cfg.Alipay.MockPay = true
	cfg.NotifyWorker.PollIntervalSec = 1
	cfg.NotifyWorker.BaseBackoffSec = 1
	cfg.NotifyWorker.MaxAttempts = 8
	cfg.ExpireWorker.IntervalSec = 120
	cfg.Security.SignSkewSec = 300
	cfg.SeedMerchant.AppID = "k2-main"
	cfg.SeedMerchant.APISecret = "test_secret"
	cfg.SeedMerchant.Name = "k2"

	db, err := database.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_ = service.SeedMerchant(db, cfg)
	srv := httpserver.New(cfg, db)
	srv.StartWorkers()
	defer srv.StopWorkers()
	plat := httptest.NewServer(srv.Handler())
	defer plat.Close()
	cfg.Server.PublicBaseURL = plat.URL

	gw := k2GiftcardClient{
		BaseURL: plat.URL, AppID: "k2-main", Secret: "test_secret",
		HTTP: http.DefaultClient,
	}

	// CreatePayment
	intent, err := gw.CreatePayment(context.Background(), createReq{
		TradeNo: "K2SIM1999", Amount: 1999, Currency: "CNY", PlanName: "Pro",
		UserID: 7, ExpireAt: time.Now().Add(20 * time.Minute),
		NotifyURL: k2.URL + "/api/v1/payment/notify/giftcard",
		ReturnURL: "http://panel/#/user/order-result?trade_no=K2SIM1999",
	})
	if err != nil {
		t.Fatal(err)
	}
	if intent.Type != "redirect" || intent.URL == "" {
		t.Fatalf("%+v", intent)
	}
	token := intent.Extra["cashier_token"].(string)

	// mock pay via public API (user clicks)
	mb, _ := json.Marshal(map[string]string{"token": token})
	pr, err := http.Post(plat.URL+"/public/orders/mock-pay", "application/json", bytes.NewReader(mb))
	if err != nil {
		t.Fatal(err)
	}
	pr.Body.Close()
	if pr.StatusCode != 200 {
		t.Fatalf("mock pay status %d", pr.StatusCode)
	}

	// wait notify
	for i := 0; i < 50 && !notifyOK.Load(); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if !notifyOK.Load() {
		t.Fatal("K2 notify not received / not accepted")
	}

	// QueryPayment
	qr, err := gw.QueryPayment(context.Background(), "K2SIM1999")
	if err != nil {
		t.Fatal(err)
	}
	if !qr.Paid || qr.PaidAmount != 1999 {
		t.Fatalf("%+v", qr)
	}

	// Create again → already_paid
	_, err = gw.CreatePayment(context.Background(), createReq{
		TradeNo: "K2SIM1999", Amount: 1999, Currency: "CNY", PlanName: "Pro",
		UserID: 7, ExpireAt: time.Now().Add(20 * time.Minute),
		NotifyURL: k2.URL + "/api/v1/payment/notify/giftcard",
		ReturnURL: "http://panel/#/r",
	})
	if err == nil || !strings.Contains(err.Error(), "giftcard: already_paid:") {
		t.Fatalf("want already_paid, got %v", err)
	}

	// Close soft success
	if err := gw.ClosePayment(context.Background(), "K2SIM1999"); err != nil {
		t.Fatal(err)
	}

	// 40401 query
	qr2, err := gw.QueryPayment(context.Background(), "NOPE")
	if err != nil {
		t.Fatal(err)
	}
	if qr2.Paid {
		t.Fatal("404 should be unpaid")
	}
}

type createReq struct {
	TradeNo, Currency, PlanName, NotifyURL, ReturnURL string
	Amount                                            int64
	UserID                                            uint
	ExpireAt                                          time.Time
}

type payIntent struct {
	Type   string
	URL    string
	Extra  map[string]any
	Amount int64
}

type queryRes struct {
	Paid       bool
	PaidAmount int64
}

type k2GiftcardClient struct {
	BaseURL, AppID, Secret string
	HTTP                   *http.Client
}

func (g k2GiftcardClient) CreatePayment(ctx context.Context, o createReq) (*payIntent, error) {
	bodyMap := map[string]any{
		"out_trade_no": o.TradeNo,
		"amount":       o.Amount,
		"currency":     o.Currency,
		"subject":      o.PlanName,
		"notify_url":   o.NotifyURL,
		"return_url":   o.ReturnURL,
		"expire_at":    o.ExpireAt.Unix(),
		"user_ref":     fmt.Sprintf("u%d", o.UserID),
	}
	raw, _ := json.Marshal(bodyMap)
	resp, status, err := g.do(ctx, "POST", g.BaseURL+"/api/v1/orders", raw)
	if err != nil {
		return nil, err
	}
	var env struct {
		Code    int            `json:"code"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(resp, &env); err != nil {
		return nil, fmt.Errorf("HTTP %d: %w", status, err)
	}
	if env.Code == 40901 {
		return nil, fmt.Errorf("giftcard: already_paid: out_trade_no=%s", o.TradeNo)
	}
	if env.Code != 0 {
		return nil, fmt.Errorf("giftcard: %s", env.Message)
	}
	urlStr, _ := env.Data["cashier_url"].(string)
	return &payIntent{
		Type: "redirect", URL: urlStr, Amount: o.Amount,
		Extra: map[string]any{
			"platform_trade_no": env.Data["platform_trade_no"],
			"cashier_token":     env.Data["cashier_token"],
			"cashier_url":       urlStr,
		},
	}, nil
}

func (g k2GiftcardClient) QueryPayment(ctx context.Context, tradeNo string) (*queryRes, error) {
	path := g.BaseURL + "/api/v1/orders/" + url.PathEscape(tradeNo)
	resp, status, err := g.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var env struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(resp, &env)
	if env.Code == 40401 || status == 404 {
		return &queryRes{Paid: false}, nil
	}
	if env.Code != 0 {
		return nil, fmt.Errorf("code=%d", env.Code)
	}
	st, _ := env.Data["status"].(string)
	paid := st == "paid" || st == "paid_orphan"
	pa, _ := env.Data["paid_amount"].(float64)
	return &queryRes{Paid: paid, PaidAmount: int64(pa)}, nil
}

func (g k2GiftcardClient) ClosePayment(ctx context.Context, tradeNo string) error {
	raw := []byte(`{"reason":"k2_cancel"}`)
	path := g.BaseURL + "/api/v1/orders/" + url.PathEscape(tradeNo) + "/close"
	resp, _, err := g.do(ctx, "POST", path, raw)
	if err != nil {
		return err
	}
	var env struct {
		Code int `json:"code"`
	}
	_ = json.Unmarshal(resp, &env)
	switch env.Code {
	case 0, 40401, 40901, 40902:
		return nil
	default:
		return fmt.Errorf("close code=%d", env.Code)
	}
}

func (g k2GiftcardClient) do(ctx context.Context, method, full string, body []byte) ([]byte, int, error) {
	if body == nil {
		body = []byte{}
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	nb := make([]byte, 16)
	_, _ = rand.Read(nb)
	nonce := hex.EncodeToString(nb)
	sum := sha256.Sum256(body)
	sig := sign.SignMD5(map[string]string{
		"app_id": g.AppID, "timestamp": ts, "nonce": nonce,
		"body_sha256": hex.EncodeToString(sum[:]),
	}, g.Secret)
	var rdr io.Reader
	if len(body) > 0 {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, full, rdr)
	if err != nil {
		return nil, 0, err
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-App-Id", g.AppID)
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Nonce", nonce)
	req.Header.Set("X-Signature", sig)
	resp, err := g.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return b, resp.StatusCode, nil
}
