package httpserver

import (
	"bytes"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/HenZenKuriRIP/NexusCard/internal/config"
	"github.com/HenZenKuriRIP/NexusCard/internal/models"
	"github.com/HenZenKuriRIP/NexusCard/internal/sign"
)

const ctxMerchantKey = "merchant"

// MerchantAuth verifies B.8.1 header signature and anti-replay nonce.
func MerchantAuth(db *gorm.DB, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		appID := strings.TrimSpace(c.GetHeader("X-App-Id"))
		tsStr := strings.TrimSpace(c.GetHeader("X-Timestamp"))
		nonce := strings.TrimSpace(c.GetHeader("X-Nonce"))
		sig := strings.TrimSpace(c.GetHeader("X-Signature"))
		if appID == "" || tsStr == "" || nonce == "" || sig == "" {
			JSONErr(c, http.StatusUnauthorized, 40101, "missing auth headers")
			c.Abort()
			return
		}
		ts, err := strconv.ParseInt(tsStr, 10, 64)
		if err != nil {
			JSONErr(c, http.StatusUnauthorized, 40101, "invalid timestamp")
			c.Abort()
			return
		}
		skew := int64(cfg.Security.SignSkewSec)
		now := time.Now().Unix()
		if math.Abs(float64(now-ts)) > float64(skew) {
			JSONErr(c, http.StatusUnauthorized, 40102, "timestamp skew")
			c.Abort()
			return
		}

		var m models.Merchant
		if err := db.Where("app_id = ?", appID).First(&m).Error; err != nil {
			JSONErr(c, http.StatusUnauthorized, 40101, "unknown app_id")
			c.Abort()
			return
		}
		if !m.Enable {
			JSONErr(c, http.StatusForbidden, 40301, "merchant disabled")
			c.Abort()
			return
		}

		// Read exact body bytes for body_sha256
		raw, err := io.ReadAll(c.Request.Body)
		if err != nil {
			JSONErr(c, http.StatusBadRequest, 40001, "read body")
			c.Abort()
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(raw))

		bodySHA := sign.BodySHA256(raw)
		expect1 := sign.MerchantRequestSignature(appID, tsStr, nonce, bodySHA, m.APISecret)
		ok := strings.EqualFold(expect1, sig)
		if !ok && m.APISecretPrev != "" {
			expect2 := sign.MerchantRequestSignature(appID, tsStr, nonce, bodySHA, m.APISecretPrev)
			ok = strings.EqualFold(expect2, sig)
		}
		if !ok {
			JSONErr(c, http.StatusUnauthorized, 40101, "bad signature")
			c.Abort()
			return
		}

		// Nonce unique for 10 minutes
		nn := models.APINonce{AppID: appID, Nonce: nonce, CreatedAt: time.Now()}
		if err := db.Create(&nn).Error; err != nil {
			// unique conflict
			JSONErr(c, http.StatusUnauthorized, 40102, "nonce replay")
			c.Abort()
			return
		}

		c.Set(ctxMerchantKey, &m)
		c.Next()
	}
}

func merchantFrom(c *gin.Context) *models.Merchant {
	v, _ := c.Get(ctxMerchantKey)
	m, _ := v.(*models.Merchant)
	return m
}

func AdminAuth(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		const p = "Bearer "
		if !strings.HasPrefix(h, p) || strings.TrimSpace(h[len(p):]) != cfg.Server.AdminToken {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}
