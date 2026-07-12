package httpserver

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/HenZenKuriRIP/NexusCard/internal/service"
)

func (s *Server) registerMerchantAPI(r *gin.Engine) {
	api := r.Group("/api/v1")
	api.Use(rateLimitMiddleware(limMerchant, "merchant"), MerchantAuth(s.DB, s.Cfg))
	{
		api.POST("/orders", s.createOrder)
		api.GET("/orders/:out_trade_no", s.getOrder)
		api.POST("/orders/:out_trade_no/close", s.closeOrder)
	}
}

func (s *Server) createOrder(c *gin.Context) {
	m := merchantFrom(c)
	if m == nil {
		JSONErr(c, http.StatusUnauthorized, 40101, "no merchant")
		return
	}
	var req service.CreateOrderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONErr(c, http.StatusBadRequest, 40001, "invalid json: "+err.Error())
		return
	}
	o, err := s.Orders.Create(m.AppID, req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrConflictPaid):
			JSONErrData(c, http.StatusConflict, 40901, "already paid", s.Orders.ToView(o))
		case errors.Is(err, service.ErrConflictClosed):
			JSONErr(c, http.StatusConflict, 40902, "closed or expired")
		case errors.Is(err, service.ErrBadParam):
			JSONErr(c, http.StatusBadRequest, 40001, err.Error())
		case errors.Is(err, service.ErrMerchant):
			JSONErr(c, http.StatusForbidden, 40301, err.Error())
		default:
			JSONErr(c, http.StatusInternalServerError, 50000, err.Error())
		}
		return
	}
	JSONOK(c, s.Orders.ToView(o))
}

func (s *Server) getOrder(c *gin.Context) {
	m := merchantFrom(c)
	o, err := s.Orders.GetByOutTradeNo(m.AppID, c.Param("out_trade_no"))
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			JSONErr(c, http.StatusNotFound, 40401, "order not found")
			return
		}
		JSONErr(c, http.StatusInternalServerError, 50000, err.Error())
		return
	}
	JSONOK(c, s.Orders.ToView(o))
}

func (s *Server) closeOrder(c *gin.Context) {
	m := merchantFrom(c)
	var body struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&body)
	o, err := s.Orders.Close(m.AppID, c.Param("out_trade_no"), body.Reason)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrNotFound):
			JSONErr(c, http.StatusNotFound, 40401, "order not found")
		case errors.Is(err, service.ErrConflictPaid):
			JSONErrData(c, http.StatusConflict, 40901, "already paid", s.Orders.ToView(o))
		default:
			JSONErr(c, http.StatusInternalServerError, 50000, err.Error())
		}
		return
	}
	JSONOK(c, s.Orders.ToView(o))
}

func JSONErrData(c *gin.Context, httpStatus, code int, message string, data any) {
	c.JSON(httpStatus, gin.H{"code": code, "message": message, "data": data})
}
