package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/HenZenKuriRIP/NexusCard/internal/config"
	"github.com/HenZenKuriRIP/NexusCard/internal/database"
	"github.com/HenZenKuriRIP/NexusCard/internal/httpserver"
	"github.com/HenZenKuriRIP/NexusCard/internal/service"
)

// Set via -ldflags "-X main.version=..."
var version = "dev"

func main() {
	cfgPath := flag.String("config", "configs/config.example.yaml", "path to config yaml")
	showVersion := flag.Bool("version", false, "print version and exit")
	checkConfig := flag.Bool("check-config", false, "load config and exit 0 if valid")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}
	if *checkConfig {
		fmt.Println("ok")
		os.Exit(0)
	}

	db, err := database.Open(cfg)
	if err != nil {
		slog.Error("open db", "err", err)
		os.Exit(1)
	}

	if err := service.SeedMerchant(db, cfg); err != nil {
		slog.Error("seed merchant", "err", err)
		os.Exit(1)
	}
	auth := service.NewAuthService(db, cfg)
	if err := auth.SeedAdmin(); err != nil {
		slog.Error("seed admin", "err", err)
		os.Exit(1)
	}
	if err := service.SeedDemoProducts(db); err != nil {
		slog.Error("seed products", "err", err)
		os.Exit(1)
	}
	slog.Info("bootstrap ready",
		"merchant", cfg.SeedMerchant.AppID,
		"admin", cfg.Admin.Username,
		"site", cfg.Admin.SiteName,
	)

	srv := httpserver.New(cfg, db)
	if err := srv.Run(); err != nil {
		slog.Error("server exit", "err", err)
		os.Exit(1)
	}
}
