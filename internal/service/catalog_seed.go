package service

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/HenZenKuriRIP/NexusCard/internal/models"
)

// CategoryMeta is exposed to the shop frontend for filters.
type CategoryMeta struct {
	Code  string `json:"code"`
	Name  string `json:"name"`
	Icon  string `json:"icon"`
	Hint  string `json:"hint"`
	Color string `json:"color"`
}

func ShopCategories() []CategoryMeta {
	return []CategoryMeta{
		{Code: models.CatAppleID, Name: "美区 Apple ID", Icon: "", Hint: "成品账号", Color: "#111827"},
		{Code: models.CatAppleGC, Name: "App Store 礼品卡", Icon: "", Hint: "美区兑换码", Color: "#0071e3"},
		{Code: models.CatGoogle, Name: "Google 账号", Icon: "", Hint: "Gmail / Play", Color: "#ea4335"},
		{Code: models.CatNetflix, Name: "Netflix", Icon: "", Hint: "流媒体账号", Color: "#e50914"},
		{Code: models.CatStreaming, Name: "流媒体", Icon: "", Hint: "Disney+ / Spotify", Color: "#8b5cf6"},
		{Code: models.CatOther, Name: "软件 / 其他", Icon: "", Hint: "工具账号等", Color: "#64748b"},
	}
}

// seedSlugs are demo catalog keys; text fields are refreshed on boot.
var seedSlugs = map[string]bool{}

func init() {
	for _, p := range commercialProducts() {
		seedSlugs[p.Slug] = true
	}
}

// SeedCommercialCatalog upserts demo catalog by slug (safe for existing DBs).
func SeedCommercialCatalog(db *gorm.DB) error {
	// 下架历史「流量套餐」演示商品（分类与旧 slug）
	_ = db.Model(&models.Product{}).
		Where("category = ? OR slug IN ?", models.CatData, []string{"data-100g-month", "data-500g-month"}).
		Updates(map[string]any{
			"enable":      false,
			"description": "该演示商品已移除",
		}).Error

	items := commercialProducts()
	for i := range items {
		p := items[i]
		var cur models.Product
		err := db.Where("slug = ?", p.Slug).First(&cur).Error
		if err == nil {
			// 同步演示商品中文文案（不覆盖管理员可能改过的价格/库存，Shadowrocket 除外首次对齐价格）
			up := map[string]any{
				"name":             p.Name,
				"description":      p.Description,
				"features":         p.Features,
				"badge":            p.Badge,
				"region":           p.Region,
				"category":         p.Category,
				"icon":             p.Icon,
				"deliver_template": p.DeliverTemplate,
				"enable":           true,
			}
			if p.Slug == "shadowrocket-download-account" && cur.PriceCents != p.PriceCents {
				up["price_cents"] = p.PriceCents
			}
			if !cur.AutoGenerate {
				up["auto_generate"] = true
				if cur.Stock == 0 {
					up["stock"] = -1
				}
			}
			if err := db.Model(&cur).Updates(up).Error; err != nil {
				return err
			}
			continue
		}
		if err := db.Create(&p).Error; err != nil {
			return err
		}
		if p.UseCardPool {
			var created models.Product
			if db.Where("slug = ?", p.Slug).First(&created).Error == nil {
				for n := 1; n <= 3; n++ {
					_ = db.Create(&models.CardCode{
						ProductID: created.ID,
						Code:      fmt.Sprintf("DEMO-%s-%02d-%s", p.Slug, n, randomHex(3)),
						Status:    models.CardUnused,
					}).Error
				}
				_ = db.Model(&created).Update("stock", 3).Error
			}
		}
	}
	return nil
}

// SeedDemoProducts keeps the historical entrypoint used by main.
func SeedDemoProducts(db *gorm.DB) error {
	return SeedCommercialCatalog(db)
}

func commercialProducts() []models.Product {
	return []models.Product{
		{
			Name: "美区 Apple ID · 成品号", Slug: "us-apple-id-ready",
			Category: models.CatAppleID, Region: "US", Badge: "热销", Icon: "🍎",
			Description: "已注册美区 Apple ID 演示商品，支持 App Store / iCloud 场景展示。支付成功后自动下发账号与密码。",
			Features:     "美区商店可用\n含安全设置说明\n支付后即时发货\n建议收货后立即改密",
			PriceCents: 1990, Currency: "CNY", Stock: -1, Enable: true, Sort: 200,
			UseCardPool: true, AutoGenerate: true,
			DeliverTemplate: "【Apple ID】\n订单：{trade_no}\n商品：{name}\n（卡密见下方交付内容）",
		},
		{
			Name: "美区 Apple ID · 教育场景", Slug: "us-apple-id-edu",
			Category: models.CatAppleID, Region: "US", Badge: "精选", Icon: "🍎",
			Description: "教育类场景演示用 Apple ID，支付后自动发货。",
			Features:     "教育场景演示\n含使用说明\n自动发货",
			PriceCents: 2990, Currency: "CNY", Stock: -1, Enable: true, Sort: 190,
			UseCardPool: false, AutoGenerate: true,
			DeliverTemplate: "【教育 Apple ID】订单 {trade_no}\n商品：{name}",
		},
		{
			Name: "App Store 美区礼品卡 $10", Slug: "us-itunes-10",
			Category: models.CatAppleGC, Region: "US", Badge: "秒发", Icon: "🎁",
			Description: "美区 App Store / iTunes $10 礼品卡演示商品，支付后下发兑换码。",
			Features:     "美区格式\n即时发货\n数字商品售出不退",
			PriceCents: 7800, Currency: "CNY", Stock: -1, Enable: true, Sort: 180,
			UseCardPool: true, AutoGenerate: true,
			DeliverTemplate: "【App Store $10】订单 {trade_no}",
		},
		{
			Name: "App Store 美区礼品卡 $50", Slug: "us-itunes-50",
			Category: models.CatAppleGC, Region: "US", Badge: "大额", Icon: "🎁",
			Description: "美区 $50 礼品卡演示商品，适合大额充值展示。",
			Features:     "仅美区格式\n自动发货",
			PriceCents: 36800, Currency: "CNY", Stock: -1, Enable: true, Sort: 170,
			UseCardPool: true, AutoGenerate: true,
			DeliverTemplate: "【App Store $50】订单 {trade_no}",
		},
		{
			Name: "Google 账号 · 成品号", Slug: "google-account",
			Category: models.CatGoogle, Region: "Global", Badge: "常用", Icon: "🔵",
			Description: "Google 账号演示商品，支付后下发 Gmail / YouTube / Play 风格登录信息。",
			Features:     "登录格式演示\n含初始密码\n建议收货后开启两步验证",
			PriceCents: 1500, Currency: "CNY", Stock: -1, Enable: true, Sort: 160,
			UseCardPool: true, AutoGenerate: true,
			DeliverTemplate: "【Google】订单 {trade_no}",
		},
		{
			Name: "Google Play 礼品卡 $25", Slug: "google-play-25",
			Category: models.CatGoogle, Region: "US", Badge: "充值", Icon: "🔵",
			Description: "美区 Google Play $25 兑换码风格演示商品。",
			Features:     "美区 Play 格式\n自动下发卡密",
			PriceCents: 18800, Currency: "CNY", Stock: -1, Enable: true, Sort: 150,
			UseCardPool: true, AutoGenerate: true,
			DeliverTemplate: "【Google Play $25】{trade_no}",
		},
		{
			Name: "Netflix · 共享席位（月）", Slug: "netflix-share-month",
			Category: models.CatNetflix, Region: "Global", Badge: "热销", Icon: "🎬",
			Description: "Netflix 共享席位月付演示商品，支付后下发账号字段。",
			Features:     "月周期演示\n流媒体账号格式\n模拟凭证",
			PriceCents: 2500, Currency: "CNY", Stock: -1, Enable: true, Sort: 140,
			UseCardPool: false, AutoGenerate: true,
			DeliverTemplate: "【Netflix 月付】订单 {trade_no}\n{name}",
		},
		{
			Name: "Netflix · 独享（季）", Slug: "netflix-solo-quarter",
			Category: models.CatNetflix, Region: "Global", Badge: "独享", Icon: "🎬",
			Description: "Netflix 独享账号季付演示商品。",
			Features:     "独享演示\n季付周期\n自动发货",
			PriceCents: 9800, Currency: "CNY", Stock: -1, Enable: true, Sort: 130,
			UseCardPool: true, AutoGenerate: true,
			DeliverTemplate: "【Netflix 独享】{trade_no}",
		},
		{
			Name: "Disney+ / 流媒体（月）", Slug: "disney-plus-month",
			Category: models.CatStreaming, Region: "Global", Badge: "组合", Icon: "🎧",
			Description: "流媒体会员演示（Disney+ 风格），可按需替换为 Spotify / YouTube Premium 等。",
			Features:     "流媒体会员\n自动发货\n可扩展 SKU",
			PriceCents: 2800, Currency: "CNY", Stock: -1, Enable: true, Sort: 120,
			UseCardPool: false, AutoGenerate: true,
			DeliverTemplate: "【流媒体】订单 {trade_no}\n{name}",
		},
		{
			Name: "Spotify Premium（月）", Slug: "spotify-premium-month",
			Category: models.CatStreaming, Region: "Global", Badge: "音乐", Icon: "🎧",
			Description: "Spotify Premium 风格数字商品演示。",
			Features:     "无广告风格演示\n自动发货",
			PriceCents: 1800, Currency: "CNY", Stock: -1, Enable: true, Sort: 110,
			UseCardPool: true, AutoGenerate: true,
			DeliverTemplate: "【Spotify】{trade_no}",
		},
		{
			Name: "Shadowrocket 下载账号", Slug: "shadowrocket-download-account",
			Category: models.CatOther, Region: "US", Badge: "热销", Icon: "🚀",
			Description: "Shadowrocket（小火箭）App Store 下载账号演示商品。支付成功后自动下发可用于下载的账号密码（模拟演示）。",
			Features:     "用于 App Store 下载 Shadowrocket\n支付后即时发货\n建议下载后立即退出账号\n数字商品售出不退",
			PriceCents: 2000, Currency: "CNY", Stock: -1, Enable: true, Sort: 210,
			UseCardPool: false, AutoGenerate: true,
			DeliverTemplate: "【Shadowrocket 下载账号】\n订单：{trade_no}\n商品：{name}\n（账号信息见下方）",
		},
	}
}
