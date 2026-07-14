package database

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/HenZenKuriRIP/NexusCard/internal/config"
	"github.com/HenZenKuriRIP/NexusCard/internal/models"
)

func Open(cfg *config.Config) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch strings.ToLower(cfg.DB.Driver) {
	case "sqlite", "sqlite3", "":
		if dir := filepath.Dir(cfg.DB.DSN); dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("create db dir %s: %w", dir, err)
			}
		} else {
			if err := os.MkdirAll("data", 0o755); err != nil {
				return nil, fmt.Errorf("create data dir: %w", err)
			}
		}
		dialector = sqlite.Open(cfg.DB.DSN)
	case "mysql":
		dialector = mysql.Open(cfg.DB.DSN)
	default:
		return nil, fmt.Errorf("unsupported db driver %q", cfg.DB.Driver)
	}

	// IgnoreRecordNotFoundError: missing optional rows (e.g. site_settings) is normal.
	// Colorful + "\r\n" prefix causes blank/noisy lines under systemd journal.
	gormLog := logger.New(
		log.New(os.Stdout, "", log.LstdFlags),
		logger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: gormLog,
	})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(
		&models.Merchant{},
		&models.Order{},
		&models.APINonce{},
		&models.Product{},
		&models.CardCode{},
		&models.AdminUser{},
		&models.SiteSetting{},
	); err != nil {
		return nil, err
	}
	return db, nil
}
