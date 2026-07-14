package epay

import (
	"net/url"
	"testing"
)

func TestSignMD5(t *testing.T) {
	// Classic epay sample: sorted join + key
	params := map[string]string{
		"money":        "1.00",
		"name":         "VIP",
		"notify_url":   "https://a.com/n",
		"out_trade_no": "T1",
		"pid":          "1000",
		"return_url":   "https://a.com/r",
		"type":         "alipay",
		"sign":         "should_be_ignored",
		"sign_type":    "MD5",
	}
	sig := SignMD5(params, "secretkey")
	if sig == "" || len(sig) != 32 {
		t.Fatalf("bad sig %q", sig)
	}
	// re-sign without sign fields should match
	params2 := map[string]string{
		"money": "1.00", "name": "VIP", "notify_url": "https://a.com/n",
		"out_trade_no": "T1", "pid": "1000", "return_url": "https://a.com/r", "type": "alipay",
	}
	if SignMD5(params2, "secretkey") != sig {
		t.Fatal("sign unstable")
	}
	params2["sign"] = sig
	if !VerifySign(params2, "secretkey") {
		t.Fatal("verify failed")
	}
	if VerifySign(params2, "wrong") {
		t.Fatal("verify should fail with wrong key")
	}
}

func TestBuildSubmitURL(t *testing.T) {
	cli, err := New(Config{
		APIURL:  "https://pay.example.com/",
		PID:     "1001",
		Key:     "k",
		Types:   "alipay,wxpay",
		Enabled: true,
		Name:    "Digital Goods",
	}, "https://shop.example.com")
	if err != nil {
		t.Fatal(err)
	}
	u, err := cli.BuildSubmitURL("PTN001", "order subject ignored", 199, "alipay")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/submit.php" {
		t.Fatalf("path %s", parsed.Path)
	}
	q := parsed.Query()
	if q.Get("pid") != "1001" || q.Get("type") != "alipay" || q.Get("money") != "1.99" {
		t.Fatalf("query %#v", q)
	}
	if q.Get("notify_url") != "https://shop.example.com/epay/notify" {
		t.Fatalf("notify %s", q.Get("notify_url"))
	}
	if q.Get("name") != "Digital Goods" {
		t.Fatalf("name %s", q.Get("name"))
	}
	params := map[string]string{}
	for k := range q {
		params[k] = q.Get(k)
	}
	if !VerifySign(params, "k") {
		t.Fatal("built url sign invalid")
	}
	if _, err := cli.BuildSubmitURL("PTN001", "x", 100, "bank"); err == nil {
		t.Fatal("expected type not allowed")
	}
}

func TestDecodeNotify(t *testing.T) {
	cli, _ := New(Config{APIURL: "https://p.com", PID: "9", Key: "kk"}, "https://s.com")
	params := map[string]string{
		"pid":          "9",
		"trade_no":     "E123",
		"out_trade_no": "PTN9",
		"type":         "alipay",
		"name":         "Goods",
		"money":        "10.50",
		"trade_status": "TRADE_SUCCESS",
	}
	params["sign"] = SignMD5(params, "kk")
	params["sign_type"] = "MD5"
	v := url.Values{}
	for k, val := range params {
		v.Set(k, val)
	}
	n, err := cli.DecodeNotify(v)
	if err != nil {
		t.Fatal(err)
	}
	if !n.Success || n.OutTradeNo != "PTN9" || n.TradeNo != "E123" || n.TotalCents != 1050 {
		t.Fatalf("%+v", n)
	}
}

func TestParseTypes(t *testing.T) {
	if got := ParseTypes("alipay, wxpay, alipay"); len(got) != 2 || got[0] != "alipay" || got[1] != "wxpay" {
		t.Fatalf("%v", got)
	}
}

func TestFormatYuan(t *testing.T) {
	if FormatYuan(1) != "0.01" || FormatYuan(100) != "1.00" || FormatYuan(199) != "1.99" {
		t.Fatal(FormatYuan(1), FormatYuan(100), FormatYuan(199))
	}
}
