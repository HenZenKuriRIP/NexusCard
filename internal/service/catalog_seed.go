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
		{Code: models.CatAppleID, Name: "US Apple ID", Icon: "", Hint: "Ready accounts", Color: "#111827"},
		{Code: models.CatAppleGC, Name: "App Store Cards", Icon: "", Hint: "US gift codes", Color: "#0071e3"},
		{Code: models.CatGoogle, Name: "Google Accounts", Icon: "", Hint: "Gmail / Play", Color: "#ea4335"},
		{Code: models.CatNetflix, Name: "Netflix", Icon: "", Hint: "Streaming accounts", Color: "#e50914"},
		{Code: models.CatStreaming, Name: "Streaming", Icon: "", Hint: "Disney+ / Spotify", Color: "#8b5cf6"},
		{Code: models.CatData, Name: "Data plans", Icon: "", Hint: "Subscription tokens", Color: "#0ea5e9"},
		{Code: models.CatOther, Name: "Other", Icon: "", Hint: "More digital goods", Color: "#64748b"},
	}
}

// SeedCommercialCatalog upserts demo catalog by slug (safe for existing DBs).
func SeedCommercialCatalog(db *gorm.DB) error {
	items := commercialProducts()
	for i := range items {
		p := items[i]
		var cur models.Product
		err := db.Where("slug = ?", p.Slug).First(&cur).Error
		if err == nil {
			up := map[string]any{}
			if cur.Category == "" || cur.Category == "other" {
				up["category"] = p.Category
				up["region"] = p.Region
				up["badge"] = p.Badge
				up["icon"] = p.Icon
				up["features"] = p.Features
			}
			if !cur.AutoGenerate {
				up["auto_generate"] = true
				if cur.Stock == 0 {
					up["stock"] = -1
				}
			}
			if len(up) > 0 {
				_ = db.Model(&cur).Updates(up).Error
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
			Name: "US Apple ID · Ready Account", Slug: "us-apple-id-ready",
			Category: models.CatAppleID, Region: "US", Badge: "Hot", Icon: "",
			Description: "Pre-registered US Apple ID for App Store / iCloud style demos. Auto-delivers account + password after payment.",
			Features: "US storefront ready\nSecurity setup notes included\nInstant auto delivery\nChange password after delivery",
			PriceCents: 1990, Currency: "CNY", Stock: -1, Enable: true, Sort: 200,
			UseCardPool: true, AutoGenerate: true,
			DeliverTemplate: "[Apple ID]\nOrder: {trade_no}\nSee delivered secret below.",
		},
		{
			Name: "US Apple ID · Edu Style", Slug: "us-apple-id-edu",
			Category: models.CatAppleID, Region: "US", Badge: "Pick", Icon: "",
			Description: "Demo SKU for education-style Apple ID scenarios. Instant delivery after payment.",
			Features: "Demo education scenario\nUsage notes included\nAuto delivery",
			PriceCents: 2990, Currency: "CNY", Stock: -1, Enable: true, Sort: 190,
			UseCardPool: false, AutoGenerate: true,
			DeliverTemplate: "[Edu Apple ID] Order {trade_no}\nProduct: {name}",
		},
		{
			Name: "App Store US Gift Card $10", Slug: "us-itunes-10",
			Category: models.CatAppleGC, Region: "US", Badge: "Instant", Icon: "",
			Description: "US App Store / iTunes style $10 gift code demo. Code delivered after payment.",
			Features: "US region format\nInstant delivery\nNo refunds on digital goods",
			PriceCents: 7800, Currency: "CNY", Stock: -1, Enable: true, Sort: 180,
			UseCardPool: true, AutoGenerate: true,
			DeliverTemplate: "[App Store $10] Order {trade_no}",
		},
		{
			Name: "App Store US Gift Card $50", Slug: "us-itunes-50",
			Category: models.CatAppleGC, Region: "US", Badge: "High", Icon: "",
			Description: "US $50 gift card demo SKU for larger top-ups.",
			Features: "US only format\nAuto delivery",
			PriceCents: 36800, Currency: "CNY", Stock: -1, Enable: true, Sort: 170,
			UseCardPool: true, AutoGenerate: true,
			DeliverTemplate: "[App Store $50] Order {trade_no}",
		},
		{
			Name: "Google Account · Ready", Slug: "google-account",
			Category: models.CatGoogle, Region: "Global", Badge: "Common", Icon: "",
			Description: "Demo Google account credential pack for Gmail / YouTube / Play style delivery.",
			Features: "Login format demo\nInitial password included\nEnable 2FA after delivery",
			PriceCents: 1500, Currency: "CNY", Stock: -1, Enable: true, Sort: 160,
			UseCardPool: true, AutoGenerate: true,
			DeliverTemplate: "[Google] Order {trade_no}",
		},
		{
			Name: "Google Play Gift Card $25", Slug: "google-play-25",
			Category: models.CatGoogle, Region: "US", Badge: "Top-up", Icon: "",
			Description: "US Google Play $25 redeem-code style demo.",
			Features: "US Play format\nAuto secret",
			PriceCents: 18800, Currency: "CNY", Stock: -1, Enable: true, Sort: 150,
			UseCardPool: true, AutoGenerate: true,
			DeliverTemplate: "[Google Play $25] {trade_no}",
		},
		{
			Name: "Netflix · Shared Seat (Month)", Slug: "netflix-share-month",
			Category: models.CatNetflix, Region: "Global", Badge: "Hot", Icon: "",
			Description: "Demo Netflix shared-seat monthly product. Delivers account fields after payment.",
			Features: "Monthly cycle demo\nStreaming account format\nSim credentials",
			PriceCents: 2500, Currency: "CNY", Stock: -1, Enable: true, Sort: 140,
			UseCardPool: false, AutoGenerate: true,
			DeliverTemplate: "[Netflix monthly] Order {trade_no}\n{name}",
		},
		{
			Name: "Netflix · Solo (Quarter)", Slug: "netflix-solo-quarter",
			Category: models.CatNetflix, Region: "Global", Badge: "Solo", Icon: "",
			Description: "Demo solo Netflix account for a quarter cycle.",
			Features: "Solo use demo\nQuarter cycle\nAuto delivery",
			PriceCents: 9800, Currency: "CNY", Stock: -1, Enable: true, Sort: 130,
			UseCardPool: true, AutoGenerate: true,
			DeliverTemplate: "[Netflix solo] {trade_no}",
		},
		{
			Name: "Disney+ / Streaming Month", Slug: "disney-plus-month",
			Category: models.CatStreaming, Region: "Global", Badge: "Bundle", Icon: "",
			Description: "Streaming membership demo (Disney+ style). Swap for Spotify / YouTube Premium as needed.",
			Features: "Streaming membership\nAuto delivery\nExtensible SKUs",
			PriceCents: 2800, Currency: "CNY", Stock: -1, Enable: true, Sort: 120,
			UseCardPool: false, AutoGenerate: true,
			DeliverTemplate: "[Streaming] Order {trade_no}\n{name}",
		},
		{
			Name: "Spotify Premium Month", Slug: "spotify-premium-month",
			Category: models.CatStreaming, Region: "Global", Badge: "Music", Icon: "",
			Description: "Spotify Premium style digital good demo.",
			Features: "Ad-free style demo\nAuto delivery",
			PriceCents: 1800, Currency: "CNY", Stock: -1, Enable: true, Sort: 110,
			UseCardPool: true, AutoGenerate: true,
			DeliverTemplate: "[Spotify] {trade_no}",
		},
		{
			Name: "Data Plan · Light 100G/mo", Slug: "data-100g-month",
			Category: models.CatData, Region: "Global", Badge: "Data", Icon: "",
			Description: "Light data/subscription plan demo. Delivers a simulated subscription token.",
			Features: "~100G/mo demo\nSubscription link format\nLight usage",
			PriceCents: 1200, Currency: "CNY", Stock: -1, Enable: true, Sort: 100,
			UseCardPool: false, AutoGenerate: true,
			DeliverTemplate: "[Data 100G] Order {trade_no}\n{name}",
		},
		{
			Name: "Data Plan · Pro 500G/mo", Slug: "data-500g-month",
			Category: models.CatData, Region: "Global", Badge: "Pro", Icon: "",
			Description: "Higher data plan demo for multi-device scenarios.",
			Features: "~500G/mo demo\nMulti-device\nAuto delivery",
			PriceCents: 3800, Currency: "CNY", Stock: -1, Enable: true, Sort: 90,
			UseCardPool: false, AutoGenerate: true,
			DeliverTemplate: "[Data 500G] {trade_no} · {name}",
		},
	}
}
