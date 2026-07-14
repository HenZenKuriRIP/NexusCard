package service

import (
	"strings"
	"testing"

	"github.com/HenZenKuriRIP/NexusCard/internal/models"
)

func TestGenerateSimCredential_Categories(t *testing.T) {
	cases := []string{
		models.CatAppleID, models.CatAppleGC, models.CatGoogle,
		models.CatNetflix, models.CatStreaming, models.CatData, models.CatOther,
	}
	for _, cat := range cases {
		p := &models.Product{Name: "test", Category: cat, AutoGenerate: true}
		s := GenerateSimCredential(p, "T123")
		if !strings.Contains(s, "模拟") && !strings.Contains(s, "SIM") && !strings.Contains(s, "sim") && !strings.Contains(s, "订单") {
			t.Fatalf("cat %s unexpected: %s", cat, s)
		}
		if strings.TrimSpace(s) == "" {
			t.Fatal("empty")
		}
	}
}
