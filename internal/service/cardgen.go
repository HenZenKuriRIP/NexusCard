package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/HenZenKuriRIP/NexusCard/internal/models"
)

// GenerateSimCredential creates demo digital goods content by product category.
// Marked clearly as SIM so operators know these are simulated assets.
func GenerateSimCredential(p *models.Product, tradeNo string) string {
	cat := p.Category
	name := p.Name
	if name == "" {
		name = "Digital Goods"
	}
	ts := time.Now().Format("2006-01-02 15:04:05")
	switch cat {
	case models.CatAppleID:
		user := fmt.Sprintf("us.%s@icloud.sim", randomHex(5))
		pass := "Ap" + randomHex(4) + "!" + randomHex(2)
		return fmt.Sprintf(`[SIM-AppleID] %s
Email: %s
Password: %s
Region: US (simulated)
Order: %s
Time: %s
Note: Simulated account for demo only. Not a real App Store login.`, name, user, pass, tradeNo, ts)
	case models.CatAppleGC:
		code := formatGiftCode("X", 16)
		return fmt.Sprintf(`[SIM-AppStore-Card] %s
Code: %s
Order: %s
Time: %s
Note: Simulated redeem code. Cannot be used on real App Store.`, name, code, tradeNo, ts)
	case models.CatGoogle:
		user := fmt.Sprintf("g.%s@gmail.sim", randomHex(5))
		pass := "Gg" + randomHex(5) + "#"
		return fmt.Sprintf(`[SIM-Google] %s
Email: %s
Password: %s
Order: %s
Time: %s
Note: Simulated account for demo delivery format only.`, name, user, pass, tradeNo, ts)
	case models.CatNetflix:
		user := fmt.Sprintf("nf_%s@mail.sim", randomHex(4))
		pass := "Nf" + randomHex(6)
		return fmt.Sprintf(`[SIM-Netflix] %s
Account: %s
Password: %s
Type: simulated profile
Order: %s
Time: %s
Note: Simulated Netflix credential. Not a real login.`, name, user, pass, tradeNo, ts)
	case models.CatStreaming:
		user := fmt.Sprintf("stream_%s@mail.sim", randomHex(4))
		pass := "St" + randomHex(5)
		return fmt.Sprintf(`[SIM-Streaming] %s
Account: %s
Password: %s
Order: %s
Time: %s
Note: Simulated streaming credential.`, name, user, pass, tradeNo, ts)
	case models.CatData:
		token := "sub://" + randomHex(24)
		return fmt.Sprintf(`[SIM-Data-Plan] %s
Subscription (simulated):
%s
Validity: 30 days (simulated)
Order: %s
Time: %s
Note: Simulated subscription payload only.`, name, token, tradeNo, ts)
	default:
		code := "SIM-" + strings.ToUpper(randomHex(8))
		return fmt.Sprintf(`[SIM-Code] %s
Code: %s
Order: %s
Time: %s`, name, code, tradeNo, ts)
	}
}

func formatGiftCode(prefix string, n int) string {
	_ = prefix
	raw := strings.ToUpper(randomHex((n + 1) / 2))
	if len(raw) < n {
		raw = raw + randomHex(8)
	}
	raw = raw[:n]
	var parts []string
	for i := 0; i < len(raw); i += 4 {
		end := i + 4
		if end > len(raw) {
			end = len(raw)
		}
		parts = append(parts, raw[i:end])
	}
	return strings.Join(parts, "-")
}
