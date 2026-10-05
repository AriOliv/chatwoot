// Command whatsmeow-bridge exposes WhatsApp Web multi-device sessions (via whatsmeow)
// over a small HTTP API and forwards incoming events to Chatwoot as webhooks.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	waLog "go.mau.fi/whatsmeow/util/log"
)

type Config struct {
	ListenAddr  string
	DatabaseURL string
	APIToken    string
	HistoryDays int
	LogLevel    string
}

func loadConfig() Config {
	historyDays, err := strconv.Atoi(getenv("HISTORY_DAYS", "30"))
	if err != nil {
		historyDays = 30
	}
	return Config{
		ListenAddr:  getenv("LISTEN_ADDR", ":8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		APIToken:    os.Getenv("WHATSMEOW_BRIDGE_TOKEN"),
		HistoryDays: historyDays,
		LogLevel:    getenv("LOG_LEVEL", "INFO"),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	cfg := loadConfig()
	log := waLog.Stdout("bridge", cfg.LogLevel, false)
	if cfg.DatabaseURL == "" || cfg.APIToken == "" {
		log.Errorf("DATABASE_URL and WHATSMEOW_BRIDGE_TOKEN are required")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, err := OpenStore(ctx, cfg.DatabaseURL, log)
	if err != nil {
		log.Errorf("failed to open store: %v", err)
		os.Exit(1)
	}
	defer store.Close()

	manager := NewSessionManager(store, cfg, log)
	if err := manager.RestoreAll(ctx); err != nil {
		log.Errorf("failed to restore sessions: %v", err)
	}

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           NewAPI(manager, cfg.APIToken).Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Infof("listening on %s", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("http server: %v", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	manager.DisconnectAll()
}
