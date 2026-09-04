package main

import (
	"context"
	"crypto/hmac"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	_ "time/tzdata"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"webhook-fanout/config"
	"webhook-fanout/fanout"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "err", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)
	slog.Info("webhook-fanout configured",
		"listen", cfg.ListenPath(),
		"template", cfg.TargetTemplate,
		"targets", len(cfg.IDs),
	)

	f := fanout.New(cfg, logger)

	e := echo.NewWithConfig(echo.Config{
		Logger:      logger,
		IPExtractor: echo.ExtractIPFromXFFHeader(),
	})
	e.Use(middleware.Recover())
	e.Use(middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(cfg.RateLimit)))

	e.GET("/health", f.HandleHealth, middleware.KeyAuth(func(c *echo.Context, key string, _ middleware.ExtractorSource) (bool, error) {
		return hmac.Equal([]byte(key), []byte(cfg.Secret)), nil
	}))
	e.POST(cfg.ListenPath(), f.HandleWebhook,
		middleware.BodyLimit(cfg.MaxBodyBytes),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := (echo.StartConfig{
		Address:         cfg.ListenAddr,
		HideBanner:      true,
		GracefulTimeout: cfg.ShutdownTimeout,
	}).Start(ctx, e); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}

	f.DropPending()
	slog.Info("shutdown complete")
}
