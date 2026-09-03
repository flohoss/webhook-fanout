package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v5"
)

var client = &http.Client{
	Timeout: requestTimeout,
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     30 * time.Second,
	},
}

var (
	mu             sync.Mutex
	timer          *time.Timer
	pendingPayload []byte
	pending        bool
	nextDeploy     time.Time
)

func signatureMiddleware(secret string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			body, err := io.ReadAll(c.Request().Body)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, "failed to read body")
			}
			_ = c.Request().Body.Close()
			c.Request().Body = io.NopCloser(bytes.NewReader(body))

			got, ok := strings.CutPrefix(strings.ToLower(c.Request().Header.Get("X-Hub-Signature-256")), "sha256=")
			if !ok || !hmac.Equal([]byte(got), []byte(hmacSHA256(secret, body))) {
				slog.Warn("invalid webhook signature", "remote", c.RealIP())
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid signature")
			}
			return next(c)
		}
	}
}

func handleWebhook(c *echo.Context) error {
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "failed to read body")
	}

	deployAt := time.Now().Add(debounce)
	schedule(body)

	slog.Info("webhook accepted",
		"remote", c.RealIP(),
		"deploy", deployAt.Format(time.DateTime),
	)
	return c.JSON(http.StatusAccepted, map[string]any{
		"deploy": deployAt.Format(time.DateTime),
	})
}

func nextDeployTime() (time.Time, bool) {
	mu.Lock()
	defer mu.Unlock()
	return nextDeploy, pending
}

func schedule(payload []byte) {
	mu.Lock()
	defer mu.Unlock()
	pendingPayload = payload
	nextDeploy = time.Now().Add(debounce)
	if timer != nil {
		timer.Stop()
		slog.Debug("debounce timer reset")
	}
	timer = time.AfterFunc(debounce, func() {
		mu.Lock()
		payload := pendingPayload
		pendingPayload, pending = nil, false
		nextDeploy = time.Time{}
		mu.Unlock()
		if payload != nil {
			deliver(payload)
		}
	})
	pending = true
}

func dropPending() {
	mu.Lock()
	if timer != nil {
		timer.Stop()
	}
	pendingPayload, pending = nil, false
	nextDeploy = time.Time{}
	mu.Unlock()
}

func deliver(payload []byte) {
	start := time.Now()
	var wg sync.WaitGroup
	var ok atomic.Int64
	sem := make(chan struct{}, maxConcurrent)

	for _, url := range urls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := postWithRetry(url, payload); err != nil {
				slog.Error("webhook-fanout failed", "url", url, "err", err)
				return
			}
			ok.Add(1)
		}()
	}
	wg.Wait()

	slog.Info("fan-out finished",
		"success", ok.Load(),
		"total", len(urls),
		"duration", time.Since(start).Round(time.Millisecond),
	)
}

func postWithRetry(url string, payload []byte) error {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", "sha256="+hmacSHA256(cfg.Secret, payload))

	delay := retryBackoff
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(delay)
			delay *= 2
			if req.GetBody != nil {
				if body, err := req.GetBody(); err == nil {
					req.Body = body
				}
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			slog.Error("webhook-fanout attempt network error", "url", url, "attempt", attempt+1, "err", err)
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		slog.Info("webhook-fanout delivery", "url", url, "status", resp.StatusCode, "attempt", attempt+1)
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return fmt.Errorf("definitive client error: HTTP %d", resp.StatusCode)
		}
		slog.Error("webhook-fanout attempt non-definitive status", "url", url, "status", resp.StatusCode, "attempt", attempt+1)
	}
	return fmt.Errorf("exhausted %d retries for %s", maxRetries, url)
}

func hmacSHA256(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}
