package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/HenZenKuriRIP/NexusCard/internal/epay"
	"github.com/HenZenKuriRIP/NexusCard/internal/models"
)

// EpayService owns the optional 彩虹易支付 client.
// Works for both K2 (source=k2) and shop orders — payment channel is independent of order source.
type EpayService struct {
	Orders *OrderService
	Client *epay.Client
}

func (e *EpayService) Configured() bool {
	return e != nil && e.Client != nil
}

// BuildPayURL returns epay submit redirect URL for a cashier token.
func (e *EpayService) BuildPayURL(ctx context.Context, cashierToken, payType string) (string, error) {
	_ = ctx
	if !e.Configured() {
		return "", fmt.Errorf("%w: epay not configured (fill api_url / pid / key)", ErrBadParam)
	}
	o, err := e.Orders.GetByCashierToken(cashierToken)
	if err != nil {
		return "", err
	}
	if o.Status != models.StatusPending {
		return "", fmt.Errorf("%w: order not payable", ErrConflictClosed)
	}
	if time.Now().After(o.ExpireAt) {
		return "", fmt.Errorf("%w: order expired", ErrConflictClosed)
	}
	return e.Client.BuildSubmitURL(o.PlatformTradeNo, o.Subject, o.Amount, payType)
}

// Types returns enabled pay type codes.
func (e *EpayService) Types() []string {
	if !e.Configured() {
		return nil
	}
	return e.Client.TypeList()
}

// HandleNotify processes verified epay notification; on success marks paid and enqueues K2 notify when applicable.
func (e *EpayService) HandleNotify(ctx context.Context, form url.Values) error {
	_ = ctx
	if !e.Configured() {
		return fmt.Errorf("epay not configured")
	}
	n, err := e.Client.DecodeNotify(form)
	if err != nil {
		return err
	}
	if !n.Success {
		slog.Info("epay notify non-success", "out_trade_no", n.OutTradeNo, "status", n.TradeStatus)
		return nil
	}
	o, err := e.Orders.FindByPlatformTradeNo(n.OutTradeNo)
	if err != nil {
		return err
	}
	if n.TotalCents > 0 && n.TotalCents != o.Amount {
		return fmt.Errorf("epay amount mismatch order=%d notify=%d", o.Amount, n.TotalCents)
	}
	cur, err := e.Orders.GetByID(o.ID)
	if err != nil {
		return err
	}
	_, err = e.Orders.acceptPayment(cur, firstNonEmpty(n.TradeNo, n.OutTradeNo), n.OutTradeNo)
	return err
}

// SyncFromEpay queries gateway and fulfills if paid (admin).
func (e *EpayService) SyncFromEpay(ctx context.Context, orderID uint) (*models.Order, error) {
	if !e.Configured() {
		return nil, fmt.Errorf("%w: epay not configured", ErrBadParam)
	}
	o, err := e.Orders.GetByID(orderID)
	if err != nil {
		return nil, err
	}
	if o.Status == models.StatusPaid || o.Status == models.StatusPaidOrphan {
		return o, nil
	}
	paid, cents, tradeNo, qerr := e.Client.QueryOrder(ctx, o.PlatformTradeNo)
	if qerr != nil {
		return nil, qerr
	}
	if !paid {
		return o, nil
	}
	if cents > 0 && cents != o.Amount {
		return nil, fmt.Errorf("epay query amount mismatch")
	}
	return e.Orders.acceptPayment(o, firstNonEmpty(tradeNo, o.PlatformTradeNo), o.PlatformTradeNo)
}
