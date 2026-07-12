package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/HenZenKuriRIP/NexusCard/internal/alipay"
	"github.com/HenZenKuriRIP/NexusCard/internal/models"
)

// AlipayService owns the optional real Alipay client.
type AlipayService struct {
	Orders *OrderService
	Client *alipay.Client
}

func (a *AlipayService) Configured() bool {
	return a != nil && a.Client != nil
}

func (a *AlipayService) CloseTrade(ctx context.Context, platformTradeNo string) error {
	if !a.Configured() {
		return nil
	}
	return a.Client.Close(ctx, platformTradeNo)
}

// BuildPayURL returns Alipay redirect URL for a cashier token.
func (a *AlipayService) BuildPayURL(ctx context.Context, cashierToken, userAgent string) (string, error) {
	if !a.Configured() {
		return "", fmt.Errorf("%w: alipay not configured (fill alipay.app_id/private_key/alipay_public_key)", ErrBadParam)
	}
	o, err := a.Orders.GetByCashierToken(cashierToken)
	if err != nil {
		return "", err
	}
	if o.Status != models.StatusPending {
		return "", fmt.Errorf("%w: order not payable", ErrConflictClosed)
	}
	if time.Now().After(o.ExpireAt) {
		return "", fmt.Errorf("%w: order expired", ErrConflictClosed)
	}
	return a.Client.PayURL(ctx, o.PlatformTradeNo, o.Subject, o.Amount, o.ExpireAt, "", userAgent)
}

// HandleNotify processes verified Alipay form notification.
func (a *AlipayService) HandleNotify(ctx context.Context, form url.Values) error {
	if !a.Configured() {
		return fmt.Errorf("alipay not configured")
	}
	n, err := a.Client.DecodeNotify(ctx, form)
	if err != nil {
		return err
	}
	if !n.Success {
		slog.Info("alipay notify non-success", "out_trade_no", n.OutTradeNo, "status", n.TradeStatus)
		return nil
	}
	o, err := a.Orders.FindByPlatformTradeNo(n.OutTradeNo)
	if err != nil {
		return err
	}
	if n.TotalCents != o.Amount {
		return fmt.Errorf("alipay amount mismatch order=%d notify=%d", o.Amount, n.TotalCents)
	}
	// reload fresh before accept (Find may return stale for history match)
	cur, err := a.Orders.GetByID(o.ID)
	if err != nil {
		return err
	}
	out, err := a.Orders.acceptPayment(cur, firstNonEmpty(n.TradeNo, n.OutTradeNo), n.OutTradeNo)
	if err != nil {
		return err
	}
	// After historical SUCCESS, close current unpaid PT if different
	if out.PlatformTradeNo != n.OutTradeNo {
		if cerr := a.Client.Close(ctx, out.PlatformTradeNo); cerr != nil {
			slog.Warn("alipay close current after historical pay", "err", cerr, "ptn", out.PlatformTradeNo)
		}
	}
	return nil
}

// SyncFromAlipay queries Alipay and fulfills if paid (admin).
func (a *AlipayService) SyncFromAlipay(ctx context.Context, orderID uint) (*models.Order, error) {
	if !a.Configured() {
		return nil, fmt.Errorf("%w: alipay not configured", ErrBadParam)
	}
	o, err := a.Orders.GetByID(orderID)
	if err != nil {
		return nil, err
	}
	if o.Status == models.StatusPaid || o.Status == models.StatusPaidOrphan {
		return o, nil
	}
	candidates := []string{o.PlatformTradeNo}
	meta := map[string]any{}
	_ = json.Unmarshal([]byte(o.Meta), &meta)
	if hist, ok := meta["platform_trade_no_history"].([]any); ok {
		for _, h := range hist {
			if s, ok := h.(string); ok && s != "" {
				candidates = append(candidates, s)
			}
		}
	}
	for _, ptn := range candidates {
		paid, cents, tradeNo, qerr := a.Client.Query(ctx, ptn)
		if qerr != nil || !paid {
			continue
		}
		if cents > 0 && cents != o.Amount {
			return nil, fmt.Errorf("alipay query amount mismatch")
		}
		return a.Orders.acceptPayment(o, firstNonEmpty(tradeNo, ptn), ptn)
	}
	return o, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
