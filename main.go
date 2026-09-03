package main

import (
	"context"
	"crypto/hmac"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/caarlos0/env/v11"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

func webhookURLs(template string, ids []string) []string {
	urls := make([]string, len(ids))
	for i, id := range ids {
		urls[i] = strings.ReplaceAll(template, "{id}", id)
	}
	return urls
}

const (
	listenAddr      = "0.0.0.0:8080"
	debounce        = 5 * time.Minute
	requestTimeout  = 15 * time.Second
	maxBodyBytes    = 1 << 20
	maxRetries      = 3
	retryBackoff    = 500 * time.Millisecond
	shutdownTimeout = 10 * time.Second
	rateLimit       = 10.0
	maxConcurrent   = 8
)

var (
	cfg  config
	urls []string
)

func main() {
	parsed, err := env.ParseAs[config]()
	if err != nil {
		slog.Error("invalid configuration", "err", err)
		os.Exit(1)
	}
	cfg = parsed
	urls = webhookURLs(cfg.TargetTemplate, cfg.IDs)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)
	slog.Info("webhook-fanout configured", "targets", len(urls))

	e := echo.NewWithConfig(echo.Config{
		Logger:      logger,
		IPExtractor: echo.ExtractIPFromXFFHeader(),
	})
	e.Use(middleware.Recover())
	e.Use(middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(rateLimit)))

	e.GET("/health", func(c *echo.Context) error {
		var pendingDeploy *string
		if deployAt, hasDeploy := nextDeployTime(); hasDeploy {
			formatted := deployAt.Format(time.DateTime)
			pendingDeploy = &formatted
		}
		return c.JSON(http.StatusOK, map[string]any{
			"status":         "ok",
			"remote":         c.RealIP(),
			"urls":           urls,
			"pending_deploy": pendingDeploy,
		})
	}, middleware.KeyAuth(func(c *echo.Context, key string, _ middleware.ExtractorSource) (bool, error) {
		return hmac.Equal([]byte(key), []byte(cfg.Secret)), nil
	}))
	e.POST("/api/git/stacks/webhook", handleWebhook,
		middleware.BodyLimit(maxBodyBytes),
		signatureMiddleware(cfg.Secret),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := (echo.StartConfig{
		Address:         listenAddr,
		HideBanner:      true,
		GracefulTimeout: shutdownTimeout,
	}).Start(ctx, e); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}

	dropPending()
	slog.Info("shutdown complete")
}
