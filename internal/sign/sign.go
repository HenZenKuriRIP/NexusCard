package sign

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// SignMD5 matches K2Board internal/payment.SignMD5:
// sort non-empty params (except signature) by key, join k=v&..., append secret, MD5 lower hex.
func SignMD5(params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "signature" || v == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+params[k])
	}
	raw := strings.Join(parts, "&") + secret
	sum := md5.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// BodySHA256 returns lower-hex SHA-256 of exact body bytes (empty body allowed).
func BodySHA256(body []byte) string {
	if body == nil {
		body = []byte{}
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// MerchantRequestSignature is B.8.1 header signature for K2 → platform.
func MerchantRequestSignature(appID, timestamp, nonce, bodySHA256, secret string) string {
	return SignMD5(map[string]string{
		"app_id":      appID,
		"timestamp":   timestamp,
		"nonce":       nonce,
		"body_sha256": bodySHA256,
	}, secret)
}
