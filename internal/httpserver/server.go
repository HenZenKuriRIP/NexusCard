package httpserver

import (
	"log/slog"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/HenZenKuriRIP/NexusCard/internal/alipay"
	"github.com/HenZenKuriRIP/NexusCard/internal/config"
	"github.com/HenZenKuriRIP/NexusCard/internal/epay"
	"github.com/HenZenKuriRIP/NexusCard/internal/service"
)

type Server struct {
	Cfg      *config.Config
	DB       *gorm.DB
	Orders   *service.OrderService
	Products *service.ProductService
	Auth     *service.AuthService
	Settings *service.SettingsService
	Alipay   *service.AlipayService
	Epay     *service.EpayService
	Notify   *service.NotifyWorker
	Expire   *service.ExpireWorker
	engine   *gin.Engine
	aliMu    sync.Mutex
	epayMu   sync.Mutex
}

func New(cfg *config.Config, db *gorm.DB) *Server {
	gin.SetMode(gin.ReleaseMode)
	orders := service.NewOrderService(db, cfg)
	settings := service.NewSettingsService(db, cfg)
	s := &Server{
		Cfg:      cfg,
		DB:       db,
		Orders:   orders,
		Products: service.NewProductService(db, cfg),
		Auth:     service.NewAuthService(db, cfg),
		Settings: settings,
		Notify:   service.NewNotifyWorker(orders),
		Expire:   service.NewExpireWorker(orders),
		engine:   gin.New(),
	}
	s.reloadAlipayLocked()
	s.reloadEpayLocked()

	s.engine.Use(gin.Recovery(), gin.Logger())
	s.registerCashier(s.engine)
	s.registerMerchantAPI(s.engine)
	s.registerShopAPI(s.engine)
	s.registerAdmin(s.engine)
	s.registerWeb(s.engine)
	return s
}

// reloadAlipay rebuilds Alipay client from DB+yaml settings.
func (s *Server) reloadAlipay() {
	s.aliMu.Lock()
	defer s.aliMu.Unlock()
	s.reloadAlipayLocked()
}

func (s *Server) reloadAlipayLocked() {
	acfg := s.Settings.ToConfigAlipay()
	// sync mock_pay onto runtime Cfg for cashier template
	s.Cfg.Alipay.MockPay = acfg.MockPay
	s.Cfg.Alipay.BillSubject = acfg.BillSubject
	s.Cfg.Alipay.IsProduction = acfg.IsProduction
	s.Cfg.Alipay.Product = acfg.Product
	s.Cfg.Alipay.AppID = acfg.AppID
	s.Cfg.Alipay.PrivateKey = acfg.PrivateKey
	s.Cfg.Alipay.AlipayPublicKey = acfg.AlipayPublicKey
	s.Cfg.Alipay.TimeoutExpress = acfg.TimeoutExpress

	view := s.Settings.AlipayPublicView()
	if !view["effective_enabled"].(bool) {
		s.Alipay = nil
		s.Orders.Alipay = nil
		// Expected on fresh install; summary also printed at listen line.
		if acfg.MockPay {
			slog.Debug("alipay real-pay off; mock_pay on")
		} else {
			slog.Debug("alipay not ready — configure in Admin -> Payment")
		}
		return
	}
	cli, err := alipay.New(acfg, s.Cfg.Server.PublicBaseURL)
	if err != nil {
		slog.Error("alipay client init failed", "err", err)
		s.Alipay = nil
		s.Orders.Alipay = nil
		return
	}
	svc := &service.AlipayService{Orders: s.Orders, Client: cli}
	s.Alipay = svc
	s.Orders.Alipay = svc
	slog.Info("alipay enabled",
		"app_id", maskAppID(acfg.AppID),
		"production", acfg.IsProduction,
		"mock_pay", acfg.MockPay,
		"notify", s.Cfg.Server.PublicBaseURL+"/alipay/notify",
	)
}

// reloadEpay rebuilds epay client from DB+yaml settings.
func (s *Server) reloadEpay() {
	s.epayMu.Lock()
	defer s.epayMu.Unlock()
	s.reloadEpayLocked()
}

func (s *Server) reloadEpayLocked() {
	ecfg := s.Settings.ToConfigEpay()
	s.Cfg.Epay = ecfg

	view := s.Settings.EpayPublicView()
	if !view["effective_enabled"].(bool) {
		s.Epay = nil
		// Expected until Admin enables 彩虹易支付; avoid noisy start logs.
		if ecfg.Enabled {
			slog.Debug("epay not ready — configure api_url / pid / key in Admin -> Payment")
		} else {
			slog.Debug("epay disabled")
		}
		return
	}
	cli, err := epay.New(epay.Config{
		APIURL:  ecfg.APIURL,
		PID:     ecfg.PID,
		Key:     ecfg.Key,
		Types:   ecfg.Types,
		Enabled: ecfg.Enabled,
		Name:    ecfg.Name,
	}, s.Cfg.Server.PublicBaseURL)
	if err != nil {
		slog.Error("epay client init failed", "err", err)
		s.Epay = nil
		return
	}
	s.Epay = &service.EpayService{Orders: s.Orders, Client: cli}
	slog.Info("epay enabled",
		"api_url", ecfg.APIURL,
		"pid", maskAppID(ecfg.PID),
		"types", ecfg.Types,
		"notify", s.Cfg.Server.PublicBaseURL+"/epay/notify",
	)
}

func (s *Server) StartWorkers() {
	s.Notify.Start()
	s.Expire.Start()
	slog.Info("workers started", "notify_poll", s.Cfg.NotifyPollInterval(), "expire", s.Cfg.ExpireInterval())
}

func (s *Server) StopWorkers() {
	s.Notify.Stop()
	s.Expire.Stop()
}

func (s *Server) Handler() http.Handler { return s.engine }

func (s *Server) Run() error {
	s.StartWorkers()
	view := s.Settings.AlipayPublicView()
	ev := s.Settings.EpayPublicView()
	slog.Info("giftcard-platform listening",
		"addr", s.Cfg.Server.Listen,
		"public", s.Cfg.Server.PublicBaseURL,
		"shop", s.Cfg.Server.PublicBaseURL+"/shop/",
		"admin", s.Cfg.Server.PublicBaseURL+"/admin/",
		"alipay", view["effective_enabled"],
		"epay", ev["effective_enabled"],
		"mock_pay", view["mock_pay"],
	)
	return s.engine.Run(s.Cfg.Server.Listen)
}
