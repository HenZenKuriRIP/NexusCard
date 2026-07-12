package sign

import "testing"

func TestAppendixC_GoldenVectors(t *testing.T) {
	secret := "test_secret"
	appID := "k2-main"
	ts := "1720770000"
	nonce := "n1n2n3n4n5n6n7n8"

	// C.1 empty body
	empty := BodySHA256(nil)
	if empty != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatal(empty)
	}
	if got := MerchantRequestSignature(appID, ts, nonce, empty, secret); got != "15906004c50c79f16dca9d067124e4c3" {
		t.Fatalf("C.1 %s", got)
	}

	// C.2 create body
	createBody := []byte(`{"out_trade_no":"T1","amount":100,"currency":"CNY","subject":"VIP","notify_url":"https://panel.example.com/api/v1/payment/notify/giftcard","return_url":"https://panel.example.com/#/user/order-result?trade_no=T1"}`)
	if got := BodySHA256(createBody); got != "c4f97996d3633582e36ae6f438e9c563b77cc3c265bad47f868a8b9ddad12a85" {
		t.Fatalf("C.2 sha %s", got)
	}
	if got := MerchantRequestSignature(appID, ts, nonce, BodySHA256(createBody), secret); got != "0d39b855a6a1e46bba9f78e5839fd15d" {
		t.Fatalf("C.2 sig %s", got)
	}

	// C.3 close
	closeBody := []byte(`{"reason":"k2_cancel"}`)
	if got := MerchantRequestSignature(appID, ts, nonce, BodySHA256(closeBody), secret); got != "a97ed3842d6bc2906bd0c4228315c28d" {
		t.Fatalf("C.3 %s", got)
	}

	// C.4 notify
	sig := SignMD5(map[string]string{
		"app_id": appID, "out_trade_no": "T1", "platform_trade_no": "GC1",
		"amount": "100", "paid_amount": "100", "currency": "CNY", "status": "paid",
		"alipay_trade_no": "ALI1", "paid_at": "1720770000", "timestamp": "1720770001",
		"nonce": "notifynonce0001",
	}, secret)
	if sig != "f0acd306f758fc3062a40effee6b99b8" {
		t.Fatalf("C.4 %s", sig)
	}
}
