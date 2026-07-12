package database

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
			_ = os.MkdirAll(dir, 0o755)
		} else {
			_ = os.MkdirAll("data", 0o755)
		}
		dialector = sqlite.Open(cfg.DB.DSN)
	case "mysql":
		dialector = mysql.Open(cfg.DB.DSN)
	default:
		return nil, fmt.Errorf("unsupported db driver %q", cfg.DB.Driver)
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
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
