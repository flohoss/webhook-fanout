package fanout

import (
	"bytes"
	"context"
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

	"webhook-fanout/config"
)

type Fanout struct {
	cfg    config.Config
	logger *slog.Logger
	client *http.Client

	mu             sync.Mutex
	timer          *time.Timer
	pendingPayload []byte
}

func New(cfg config.Config, logger *slog.Logger) *Fanout {
	return &Fanout{
		cfg:    cfg,
		logger: logger,
		client: &http.Client{
			Timeout: cfg.RequestTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     30 * time.Second,
			},
		},
	}
}

func (f *Fanout) HandleWebhook(c *echo.Context) error {
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "failed to read body")
	}

	got, ok := strings.CutPrefix(strings.ToLower(c.Request().Header.Get("X-Hub-Signature-256")), "sha256=")
	if !ok || !hmac.Equal([]byte(got), []byte(f.hmacSHA256(f.cfg.Secret, body))) {
		f.logger.Warn("invalid webhook signature", "remote", c.RealIP())
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid signature")
	}

	if !f.cfg.AcceptRegex.Matches(body) {
		f.logger.Info("webhook ignored", "remote", c.RealIP())
		return c.JSON(http.StatusAccepted, map[string]any{"ignored": true})
	}

	deployAt := time.Now().Add(f.cfg.Debounce)
	f.schedule(body)

	f.logger.Info("webhook accepted",
		"remote", c.RealIP(),
		"deploy", deployAt.Format(time.DateTime),
	)
	return c.JSON(http.StatusAccepted, map[string]any{
		"deploy": deployAt.Format(time.DateTime),
	})
}

func (f *Fanout) DropPending() {
	f.mu.Lock()
	if f.timer != nil {
		f.timer.Stop()
	}
	f.pendingPayload = nil
	f.mu.Unlock()
}

func (f *Fanout) schedule(payload []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pendingPayload = payload
	if f.timer == nil {
		f.timer = time.AfterFunc(f.cfg.Debounce, f.firePending)
	} else {
		f.timer.Reset(f.cfg.Debounce)
		f.logger.Debug("debounce timer reset")
	}
}

func (f *Fanout) firePending() {
	f.mu.Lock()
	payload := f.pendingPayload
	f.pendingPayload = nil
	f.mu.Unlock()
	if payload != nil {
		f.deliver(payload)
	}
}

func (f *Fanout) deliver(payload []byte) {
	start := time.Now()

	urls, err := f.fetchEnabledTargets(context.Background())
	if err != nil {
		f.logger.Error("stacks lookup failed, skipping fan-out", "err", err)
		return
	}
	if len(urls) == 0 {
		f.logger.Warn("no enabled stacks, skipping fan-out")
		return
	}

	var wg sync.WaitGroup
	var ok atomic.Int64
	sem := make(chan struct{}, f.cfg.MaxConcurrent)

	for _, t := range urls {
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			if err := f.postWithRetry(t, payload); err != nil {
				f.logger.Error("webhook-fanout failed", "url", t.url, "err", err)
				return
			}
			ok.Add(1)
		})
	}
	wg.Wait()

	f.logger.Info("fan-out finished",
		"success", ok.Load(),
		"total", len(urls),
		"duration", time.Since(start).Round(time.Millisecond),
	)
}

func (f *Fanout) postWithRetry(t target, payload []byte) error {
	delay := f.cfg.RetryBackoff
	for attempt := range f.cfg.MaxAttempts {
		if attempt > 0 {
			time.Sleep(delay)
			delay *= 2
		}

		req, err := http.NewRequest(http.MethodPost, t.url, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Hub-Signature-256", "sha256="+f.hmacSHA256(t.secret, payload))

		resp, err := f.client.Do(req)
		if err != nil {
			f.logger.Error("webhook-fanout attempt network error", "url", t.url, "attempt", attempt+1, "err", err)
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		f.logger.Info("webhook-fanout delivery", "url", t.url, "status", resp.StatusCode, "attempt", attempt+1)
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return fmt.Errorf("definitive client error: HTTP %d", resp.StatusCode)
		}
		f.logger.Error("webhook-fanout attempt non-definitive status", "url", t.url, "status", resp.StatusCode, "attempt", attempt+1)
	}
	return fmt.Errorf("exhausted %d attempts for %s", f.cfg.MaxAttempts, t.url)
}

func (f *Fanout) hmacSHA256(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}
