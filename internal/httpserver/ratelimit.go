package httpserver

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type visitor struct {
	count int
	reset time.Time
}

// ipRateLimiter is a simple fixed-window limiter per IP+bucket.
type ipRateLimiter struct {
	mu      sync.Mutex
	visitors map[string]*visitor
	limit   int
	window  time.Duration
}

func newIPRateLimiter(limit int, window time.Duration) *ipRateLimiter {
	l := &ipRateLimiter{
		visitors: make(map[string]*visitor),
		limit:    limit,
		window:   window,
	}
	go func() {
		t := time.NewTicker(time.Minute)
		for range t.C {
			l.mu.Lock()
			now := time.Now()
			for k, v := range l.visitors {
				if now.After(v.reset) {
					delete(l.visitors, k)
				}
			}
			l.mu.Unlock()
		}
	}()
	return l
}

func (l *ipRateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	v, ok := l.visitors[key]
	if !ok || now.After(v.reset) {
		l.visitors[key] = &visitor{count: 1, reset: now.Add(l.window)}
		return true
	}
	if v.count >= l.limit {
		return false
	}
	v.count++
	return true
}

func rateLimitMiddleware(l *ipRateLimiter, bucket string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !l.allow(bucket + ":" + ip) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code": 42900, "message": "Too many requests, try again later", "error": "rate limited",
			})
			return
		}
		c.Next()
	}
}

// shared limiters
var (
	limShopRead   = newIPRateLimiter(120, time.Minute) // list products
	limShopWrite  = newIPRateLimiter(20, time.Minute)  // checkout
	limPublicPay  = newIPRateLimiter(30, time.Minute)  // mock/alipay pay / status
	limAdminLogin = newIPRateLimiter(15, time.Minute)
	limMerchant   = newIPRateLimiter(120, time.Minute) // K2 create/query — higher
	limNotify     = newIPRateLimiter(300, time.Minute) // alipay notify
)
