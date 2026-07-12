package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HenZenKuriRIP/NexusCard/internal/models"
	"github.com/HenZenKuriRIP/NexusCard/internal/sign"
)

// NotifyWorker posts signed callbacks to K2 notify_url.
type NotifyWorker struct {
	Orders *OrderService
	Client *http.Client
	stop   chan struct{}
}

func NewNotifyWorker(orders *OrderService) *NotifyWorker {
	return &NotifyWorker{
		Orders: orders,
		Client: &http.Client{Timeout: 15 * time.Second},
		stop:   make(chan struct{}),
	}
}

func (w *NotifyWorker) Start() {
	go w.loop()
}

func (w *NotifyWorker) Stop() {
	select {
	case <-w.stop:
	default:
		close(w.stop)
	}
}

func (w *NotifyWorker) loop() {
	interval := w.Orders.Cfg.NotifyPollInterval()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-w.stop:
			return
		case <-t.C:
			w.tick()
		}
	}
}

func (w *NotifyWorker) tick() {
	cfg := w.Orders.Cfg
	now := time.Now()
	var list []models.Order
	err := w.Orders.DB.
		Where("notify_status = ? AND (notify_next_at IS NULL OR notify_next_at <= ?) AND status IN ?",
			models.NotifyPending, now, []string{models.StatusPaid, models.StatusPaidOrphan}).
		Order("notify_next_at ASC").
		Limit(20).
		Find(&list).Error
	if err != nil {
		slog.Error("notify worker query", "err", err)
		return
	}
	for i := range list {
		w.deliver(context.Background(), &list[i], cfg.NotifyWorker.MaxAttempts, cfg.NotifyWorker.BaseBackoffSec)
	}
}

func (w *NotifyWorker) deliver(ctx context.Context, o *models.Order, maxAttempts, baseBackoffSec int) {
	var m models.Merchant
	if err := w.Orders.DB.Where("app_id = ?", o.AppID).First(&m).Error; err != nil {
		w.fail(o, maxAttempts, baseBackoffSec, "merchant missing")
		return
	}
	// Outbound notify always signs with current api_secret only (never prev).
	secret := m.APISecret

	paidAt := int64(0)
	if o.PaidAt != nil {
		paidAt = o.PaidAt.Unix()
	}
	ts := time.Now().Unix()
	nonce := randomHex(12)
	bodyMap := map[string]any{
		"app_id":            o.AppID,
		"out_trade_no":      o.OutTradeNo,
		"platform_trade_no": o.PlatformTradeNo,
		"amount":            o.Amount,
		"paid_amount":       o.PaidAmount,
		"currency":          o.Currency,
		"status":            "paid", // always paid for success path (incl. orphan)
		"alipay_trade_no":   o.AlipayTradeNo,
		"paid_at":           paidAt,
		"timestamp":         ts,
		"nonce":             nonce,
	}
	signParams := map[string]string{
		"app_id":            o.AppID,
		"out_trade_no":      o.OutTradeNo,
		"platform_trade_no": o.PlatformTradeNo,
		"amount":            strconv.FormatInt(o.Amount, 10),
		"paid_amount":       strconv.FormatInt(o.PaidAmount, 10),
		"currency":          o.Currency,
		"status":            "paid",
		"alipay_trade_no":   o.AlipayTradeNo,
		"paid_at":           strconv.FormatInt(paidAt, 10),
		"timestamp":         strconv.FormatInt(ts, 10),
		"nonce":             nonce,
	}
	bodyMap["signature"] = sign.SignMD5(signParams, secret)
	raw, _ := json.Marshal(bodyMap)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.NotifyURL, bytes.NewReader(raw))
	if err != nil {
		w.fail(o, maxAttempts, baseBackoffSec, err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.Client.Do(req)
	if err != nil {
		w.fail(o, maxAttempts, baseBackoffSec, err.Error())
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	ack := isNotifyACK(resp.StatusCode, respBody)

	// Special: 404 after several attempts stop
	if resp.StatusCode == http.StatusNotFound && o.NotifyAttempts+1 >= 3 {
		_ = w.Orders.DB.Model(o).Updates(map[string]any{
			"notify_status":     models.NotifyFailed,
			"notify_attempts":   o.NotifyAttempts + 1,
			"notify_last_error": "k2 order not found",
		})
		slog.Warn("notify stopped: 404", "out_trade_no", o.OutTradeNo)
		return
	}

	if ack {
		_ = w.Orders.DB.Model(o).Updates(map[string]any{
			"notify_status":     models.NotifySuccess,
			"notify_attempts":   o.NotifyAttempts + 1,
			"notify_last_error": "",
			"notify_next_at":    nil,
		})
		slog.Info("k2 notify ok", "out_trade_no", o.OutTradeNo, "attempts", o.NotifyAttempts+1)
		return
	}
	w.fail(o, maxAttempts, baseBackoffSec, fmt.Sprintf("HTTP %d body=%q", resp.StatusCode, truncate(string(respBody), 200)))
}

func isNotifyACK(status int, body []byte) bool {
	if status < 200 || status >= 300 {
		return false
	}
	s := strings.ToLower(strings.TrimSpace(string(body)))
	return s == "ok" || s == "success"
}

func (w *NotifyWorker) fail(o *models.Order, maxAttempts, baseBackoffSec int, msg string) {
	attempts := o.NotifyAttempts + 1
	if attempts >= maxAttempts {
		_ = w.Orders.DB.Model(o).Updates(map[string]any{
			"notify_status":     models.NotifyFailed,
			"notify_attempts":   attempts,
			"notify_last_error": truncate(msg, 500),
		})
		slog.Error("notify exhausted", "out_trade_no", o.OutTradeNo, "err", msg)
		return
	}
	// Exponential backoff: base * 2^(attempts-1), cap 3600s
	backoff := baseBackoffSec << (attempts - 1)
	if backoff > 3600 {
		backoff = 3600
	}
	if backoff < baseBackoffSec {
		backoff = baseBackoffSec
	}
	next := time.Now().Add(time.Duration(backoff) * time.Second)
	_ = w.Orders.DB.Model(o).Updates(map[string]any{
		"notify_status":     models.NotifyPending,
		"notify_attempts":   attempts,
		"notify_next_at":    next,
		"notify_last_error": truncate(msg, 500),
	})
	slog.Warn("notify retry scheduled", "out_trade_no", o.OutTradeNo, "attempt", attempts, "next", next, "err", msg)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// ExpireWorker marks overdue pending orders as expired.
type ExpireWorker struct {
	Orders *OrderService
	stop   chan struct{}
}

func NewExpireWorker(orders *OrderService) *ExpireWorker {
	return &ExpireWorker{Orders: orders, stop: make(chan struct{})}
}

func (w *ExpireWorker) Start() {
	go w.loop()
}

func (w *ExpireWorker) Stop() {
	select {
	case <-w.stop:
	default:
		close(w.stop)
	}
}

func (w *ExpireWorker) loop() {
	t := time.NewTicker(w.Orders.Cfg.ExpireInterval())
	defer t.Stop()
	for {
		select {
		case <-w.stop:
			return
		case <-t.C:
			n, err := w.Orders.ExpireBatch(w.Orders.Cfg.ExpireWorker.BatchSize)
			if err != nil {
				slog.Error("expire worker", "err", err)
			} else if n > 0 {
				slog.Info("orders expired", "count", n)
			}
			// cleanup nonces older than 10 minutes
			cut := time.Now().Add(-10 * time.Minute)
			_ = w.Orders.DB.Where("created_at < ?", cut).Delete(&models.APINonce{})
		}
	}
}
