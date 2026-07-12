package alipay

import (
	"strings"
	"testing"
	"time"
)

func TestFormatYuan(t *testing.T) {
	if FormatYuan(2888) != "28.88" {
		t.Fatal(FormatYuan(2888))
	}
	if FormatYuan(100) != "1.00" {
		t.Fatal(FormatYuan(100))
	}
	if FormatYuan(0) != "0.00" {
		t.Fatal(FormatYuan(0))
	}
}

func TestParseYuanToCents(t *testing.T) {
	if ParseYuanToCents("28.88") != 2888 {
		t.Fatal(ParseYuanToCents("28.88"))
	}
	if ParseYuanToCents("1") != 100 {
		t.Fatal(ParseYuanToCents("1"))
	}
}

func TestTimeoutExpress(t *testing.T) {
	// remaining ~10m, config 30m → about 9–10m (floor minutes)
	exp := time.Now().Add(10*time.Minute + 30*time.Second)
	got := TimeoutExpress("30m", exp)
	if got != "10m" && got != "9m" {
		t.Fatalf("got %s want ~10m", got)
	}
	// remaining 2h, config 30m → 30m
	exp = time.Now().Add(2 * time.Hour)
	if got := TimeoutExpress("30m", exp); got != "30m" {
		t.Fatalf("got %s", got)
	}
}

func TestSanitizeSubject(t *testing.T) {
	s := SanitizeSubject("VIP/月付&特惠")
	if s == "" || containsAny(s, "/&") {
		t.Fatal(s)
	}
	s2 := SanitizeSubject("机场年付VPN套餐")
	if strings.Contains(s2, "机场") || strings.Contains(strings.ToLower(s2), "vpn") {
		t.Fatalf("sensitive left: %q", s2)
	}
	if BillSubject("Digital Goods", "机场VPN") != "Digital Goods" {
		t.Fatal(BillSubject("Digital Goods", "机场VPN"))
	}
}

func containsAny(s, chars string) bool {
	for _, c := range chars {
		for _, r := range s {
			if r == c {
				return true
			}
		}
	}
	return false
}
