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
		name = "数字商品"
	}
	ts := time.Now().Format("2006-01-02 15:04:05")
	// Shadowrocket 等下载账号：按软件账号格式发货
	if strings.Contains(strings.ToLower(p.Slug), "shadowrocket") || strings.Contains(name, "Shadowrocket") {
		user := fmt.Sprintf("sr.%s@icloud.sim", randomHex(5))
		pass := "Sr" + randomHex(4) + "!" + randomHex(2)
		return fmt.Sprintf(`[模拟-Shadowrocket下载账号] %s
Apple ID：%s
密码：%s
地区：美区（模拟）
订单：%s
时间：%s
说明：仅演示发货格式，非真实可登录账号。下载后请立即退出账号。`, name, user, pass, tradeNo, ts)
	}
	switch cat {
	case models.CatAppleID:
		user := fmt.Sprintf("us.%s@icloud.sim", randomHex(5))
		pass := "Ap" + randomHex(4) + "!" + randomHex(2)
		return fmt.Sprintf(`[模拟-AppleID] %s
邮箱：%s
密码：%s
地区：美区（模拟）
订单：%s
时间：%s
说明：演示用模拟账号，非真实 App Store 登录。`, name, user, pass, tradeNo, ts)
	case models.CatAppleGC:
		code := formatGiftCode("X", 16)
		return fmt.Sprintf(`[模拟-AppStore礼品卡] %s
兑换码：%s
订单：%s
时间：%s
说明：演示兑换码，无法在真实 App Store 使用。`, name, code, tradeNo, ts)
	case models.CatGoogle:
		user := fmt.Sprintf("g.%s@gmail.sim", randomHex(5))
		pass := "Gg" + randomHex(5) + "#"
		return fmt.Sprintf(`[模拟-Google] %s
邮箱：%s
密码：%s
订单：%s
时间：%s
说明：仅演示发货格式。`, name, user, pass, tradeNo, ts)
	case models.CatNetflix:
		user := fmt.Sprintf("nf_%s@mail.sim", randomHex(4))
		pass := "Nf" + randomHex(6)
		return fmt.Sprintf(`[模拟-Netflix] %s
账号：%s
密码：%s
类型：模拟档案
订单：%s
时间：%s
说明：演示用凭证，非真实登录。`, name, user, pass, tradeNo, ts)
	case models.CatStreaming:
		user := fmt.Sprintf("stream_%s@mail.sim", randomHex(4))
		pass := "St" + randomHex(5)
		return fmt.Sprintf(`[模拟-流媒体] %s
账号：%s
密码：%s
订单：%s
时间：%s
说明：演示用流媒体凭证。`, name, user, pass, tradeNo, ts)
	case models.CatData:
		token := "sub://" + randomHex(24)
		return fmt.Sprintf(`[模拟-订阅] %s
订阅（模拟）：
%s
有效期：30 天（模拟）
订单：%s
时间：%s
说明：仅演示订阅载荷。`, name, token, tradeNo, ts)
	default:
		code := "SIM-" + strings.ToUpper(randomHex(8))
		return fmt.Sprintf(`[模拟卡密] %s
卡密：%s
订单：%s
时间：%s`, name, code, tradeNo, ts)
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
