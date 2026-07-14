package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/HenZenKuriRIP/NexusCard/internal/service"
)

const cashierHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>Checkout · {{.Subject}}</title>
<link rel="stylesheet" href="/assets/app.css"/>
</head>
<body class="cashier">
<div class="cashier-card">
  <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px">
    <span class="muted" style="font-size:12px;letter-spacing:.08em;text-transform:uppercase">Checkout</span>
    <span class="badge {{.Status}}">{{.Status}}</span>
  </div>
  <h1>{{.Subject}}</h1>
  <div class="meta">Order {{.OutTradeNo}}</div>
  <div class="amt"><small style="font-size:18px;font-weight:600">¥</small> {{.AmountYuan}}</div>
  <div class="meta">
    Time left：<b id="cd">--</b><br/>
    {{if .AlipayReady}}Official Alipay{{end}}{{if and .AlipayReady .EpayReady}} · {{end}}{{if .EpayReady}}彩虹易支付{{end}}{{if .MockPay}} · Mock pay{{end}}
  </div>
  <div id="msg" class="status-msg" style="display:none;margin-top:14px;padding:10px 12px;border-radius:10px;font-size:13px"></div>
  <div id="deliver" style="display:none">
    <div class="meta" style="margin-top:14px">Delivery</div>
    <div class="deliver-box" id="deliverBody"></div>
  </div>
  {{if .CanPay}}
  {{if .AlipayReady}}
  <button class="btn btn-ali" id="payBtn" type="button">官方支付宝</button>
  {{end}}
  {{if .EpayReady}}
  {{range .EpayTypes}}
  <button class="btn btn-epay" type="button" data-epay-type="{{.}}">{{epayLabel .}}</button>
  {{end}}
  {{end}}
  {{if and (not .AlipayReady) (not .EpayReady) (not .MockPay)}}
  <button class="btn btn-ali" type="button" disabled title="configure payment">暂无可用支付方式</button>
  {{end}}
  {{if .MockPay}}
  <button class="btn btn-mock" id="mockBtn" type="button">Mock payment successful</button>
  {{end}}
  {{else if .Paid}}
  <a class="btn btn-ali" id="retBtn" href="{{.ReturnURL}}">Done — return</a>
  {{else}}
  <div class="meta" style="margin-top:16px">Order not payable（{{.Status}}）</div>
  {{end}}
  <p class="meta" style="margin-top:18px;margin-bottom:0"><a href="/shop/">← Back to store</a></p>
</div>
<script>
const token = {{.TokenJSON}};
const expireAt = {{.ExpireAt}};
const returnURL = {{.ReturnURLJSON}};
const initialDelivered = {{.DeliveredJSON}};
function fmt(sec){
  if(sec<=0) return "expired";
  const m=Math.floor(sec/60), s=sec%60;
  return m+"m "+String(s).padStart(2,"0")+"s";
}
function tick(){
  const left = expireAt - Math.floor(Date.now()/1000);
  const el=document.getElementById("cd");
  if(el) el.textContent = fmt(left);
}
tick(); setInterval(tick,1000);
function show(t, ok){
  const m=document.getElementById("msg");
  m.style.display="block";
  m.style.background = ok ? "#ecfdf5" : "#fef2f2";
  m.style.color = ok ? "#047857" : "#b91c1c";
  m.textContent=t;
}
function showDeliver(text){
  if(!text) return;
  document.getElementById("deliver").style.display="block";
  document.getElementById("deliverBody").textContent=text;
}
if(initialDelivered) showDeliver(initialDelivered);
async function poll(){
  try{
    const r = await fetch("/public/orders/status?token="+encodeURIComponent(token));
    const j = await r.json();
    if(j.code===0 && j.data){
      if(j.data.status==="paid" || j.data.status==="paid_orphan"){
        show("Payment successful","ok");
        if(j.data.delivered) showDeliver(j.data.delivered);
        setTimeout(()=>{ if(returnURL) location.href=returnURL; }, 1600);
      }
    }
  }catch(e){}
}
setInterval(poll, 2500);
const payBtn=document.getElementById("payBtn");
if(payBtn && !payBtn.disabled){
  payBtn.onclick = async ()=>{
    payBtn.disabled=true;
    try{
      const r = await fetch("/public/orders/alipay-pay", {
        method:"POST", headers:{"Content-Type":"application/json"},
        body: JSON.stringify({token})
      });
      const j = await r.json();
      if(j.code===0 && j.data && j.data.pay_url){
        location.href = j.data.pay_url;
        return;
      }
      show(j.message||"Failed to start Alipay");
      payBtn.disabled=false;
    }catch(e){ show(String(e)); payBtn.disabled=false; }
  };
}
document.querySelectorAll("[data-epay-type]").forEach(btn=>{
  btn.onclick = async ()=>{
    const type = btn.getAttribute("data-epay-type");
    btn.disabled=true;
    try{
      const r = await fetch("/public/orders/epay-pay", {
        method:"POST", headers:{"Content-Type":"application/json"},
        body: JSON.stringify({token, type})
      });
      const j = await r.json();
      if(j.code===0 && j.data && j.data.pay_url){
        location.href = j.data.pay_url;
        return;
      }
      show(j.message||"Failed to start Epay");
      btn.disabled=false;
    }catch(e){ show(String(e)); btn.disabled=false; }
  };
});
const mockBtn=document.getElementById("mockBtn");
if(mockBtn){
  mockBtn.onclick = async ()=>{
    mockBtn.disabled=true;
    try{
      const r = await fetch("/public/orders/mock-pay", {
        method:"POST", headers:{"Content-Type":"application/json"},
        body: JSON.stringify({token})
      });
      const j = await r.json();
      if(j.code===0){
        show("Mock payment successful","ok");
        if(j.data && j.data.delivered) showDeliver(j.data.delivered);
        poll();
      } else { show(j.message||"failed"); mockBtn.disabled=false; }
    }catch(e){ show(String(e)); mockBtn.disabled=false; }
  };
}
</script>
</body>
</html>`

var cashierTmpl = template.Must(template.New("cashier").Funcs(template.FuncMap{
	"epayLabel": epayTypeLabel,
}).Parse(cashierHTML))

func epayTypeLabel(t string) string {
	switch t {
	case "alipay":
		return "支付宝（易支付）"
	case "wxpay":
		return "微信支付（易支付）"
	case "qqpay":
		return "QQ钱包（易支付）"
	case "bank":
		return "网银（易支付）"
	default:
		return t + "（易支付）"
	}
}

func (s *Server) registerCashier(r *gin.Engine) {
	// Static path before /c/:token
	r.GET("/c/return", s.alipayBrowserReturn)
	r.GET("/c/:token", rateLimitMiddleware(limPublicPay, "cashier"), s.cashierPage)
	r.GET("/c/:token/return", s.cashierReturn)
	r.GET("/public/orders/status", rateLimitMiddleware(limPublicPay, "status"), s.publicStatus)
	r.POST("/public/orders/mock-pay", rateLimitMiddleware(limPublicPay, "mock_pay"), s.publicMockPay)
	r.POST("/public/orders/alipay-pay", rateLimitMiddleware(limPublicPay, "alipay_pay"), s.publicAlipayPay)
	r.POST("/public/orders/epay-pay", rateLimitMiddleware(limPublicPay, "epay_pay"), s.publicEpayPay)
	r.POST("/alipay/notify", rateLimitMiddleware(limNotify, "alipay_notify"), s.alipayNotify)
	r.GET("/epay/notify", rateLimitMiddleware(limNotify, "epay_notify"), s.epayNotify)
	r.POST("/epay/notify", rateLimitMiddleware(limNotify, "epay_notify"), s.epayNotify)
	r.GET("/epay/return", rateLimitMiddleware(limPublicPay, "epay_return"), s.epayBrowserReturn)
	r.POST("/epay/return", rateLimitMiddleware(limPublicPay, "epay_return"), s.epayBrowserReturn)
	r.GET("/healthz", func(c *gin.Context) {
		view := s.Settings.AlipayPublicView()
		ev := s.Settings.EpayPublicView()
		c.JSON(http.StatusOK, gin.H{
			"ok":       true,
			"service":  "giftcard-platform",
			"alipay":   view["effective_enabled"],
			"epay":     ev["effective_enabled"],
			"mock_pay": view["mock_pay"],
		})
	})
}

func (s *Server) cashierPage(c *gin.Context) {
	token := c.Param("token")
	o, err := s.Orders.GetByCashierToken(token)
	if err != nil {
		c.String(http.StatusNotFound, "Order not found or link invalid")
		return
	}
	canPay := o.Status == "pending" && time.Now().Before(o.ExpireAt)
	paid := o.Status == "paid" || o.Status == "paid_orphan"
	yuan := fmt.Sprintf("%d.%02d", o.Amount/100, o.Amount%100)
	ret := o.ReturnURL
	if ret == "" {
		ret = "/shop/"
	}
	epayReady := s.Epay != nil && s.Epay.Configured() && canPay
	var epayTypes []string
	if epayReady {
		epayTypes = s.Epay.Types()
	}
	data := map[string]any{
		"Subject":       o.Subject,
		"OutTradeNo":    o.OutTradeNo,
		"AmountYuan":    yuan,
		"Status":        o.Status,
		"CanPay":        canPay,
		"Paid":          paid,
		"MockPay":       s.Settings.GetAlipay().MockPay && canPay,
		"AlipayReady":   s.Alipay != nil && s.Alipay.Configured() && canPay,
		"EpayReady":     epayReady,
		"EpayTypes":     epayTypes,
		"ExpireAt":      o.ExpireAt.Unix(),
		"TokenJSON":     template.JS(mustJSON(token)),
		"ReturnURLJSON": template.JS(mustJSON(ret)),
		"ReturnURL":     ret,
		"DeliveredJSON": template.JS(mustJSON(o.Delivered)),
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := cashierTmpl.Execute(c.Writer, data); err != nil {
		c.String(http.StatusInternalServerError, "template error")
	}
}

func (s *Server) cashierReturn(c *gin.Context) {
	o, err := s.Orders.GetByCashierToken(c.Param("token"))
	if err != nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	if o.ReturnURL == "" {
		c.Redirect(http.StatusFound, "/shop/")
		return
	}
	c.Redirect(http.StatusFound, o.ReturnURL)
}

func (s *Server) publicStatus(c *gin.Context) {
	token := c.Query("token")
	o, err := s.Orders.GetByCashierToken(token)
	if err != nil {
		JSONErr(c, http.StatusNotFound, 40401, "not found")
		return
	}
	JSONOK(c, gin.H{
		"status":       o.Status,
		"out_trade_no": o.OutTradeNo,
		"paid_amount":  o.PaidAmount,
		"expire_at":    o.ExpireAt.Unix(),
		"delivered":    o.Delivered,
	})
}

func (s *Server) publicMockPay(c *gin.Context) {
	var body struct {
		Token string `json:"token"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Token == "" {
		JSONErr(c, http.StatusBadRequest, 40001, "token required")
		return
	}
	o, err := s.Orders.MarkPaidMock(body.Token)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrNotFound):
			JSONErr(c, http.StatusNotFound, 40401, "not found")
		case errors.Is(err, service.ErrConflictClosed):
			JSONErr(c, http.StatusConflict, 40902, err.Error())
		case errors.Is(err, service.ErrBadParam):
			JSONErr(c, http.StatusBadRequest, 40001, err.Error())
		default:
			JSONErr(c, http.StatusInternalServerError, 50000, err.Error())
		}
		return
	}
	JSONOK(c, s.Orders.ToView(o))
}

func mustJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
