package fanout

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

	"webhook-fanout/config"
)

type Fanout struct {
	cfg    config.Config
	logger *slog.Logger
	client *http.Client

	mu             sync.Mutex
	timer          *time.Timer
	pendingPayload []byte
	pending        bool
	nextDeploy     time.Time
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
	if !ok || !hmac.Equal([]byte(got), []byte(f.hmacSHA256(body))) {
		f.logger.Warn("invalid webhook signature", "remote", c.RealIP())
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid signature")
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

func (f *Fanout) HandleHealth(c *echo.Context) error {
	var pendingDeploy *string
	if deployAt, hasDeploy := f.nextDeployTime(); hasDeploy {
		formatted := deployAt.Format(time.DateTime)
		pendingDeploy = &formatted
	}
	return c.JSON(http.StatusOK, map[string]any{
		"status":         "ok",
		"remote":         c.RealIP(),
		"targets":        len(f.cfg.IDs),
		"pending_deploy": pendingDeploy,
	})
}

func (f *Fanout) DropPending() {
	f.mu.Lock()
	if f.timer != nil {
		f.timer.Stop()
	}
	f.pendingPayload, f.pending = nil, false
	f.nextDeploy = time.Time{}
	f.mu.Unlock()
}

func (f *Fanout) nextDeployTime() (time.Time, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.nextDeploy, f.pending
}

func (f *Fanout) schedule(payload []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pendingPayload = payload
	f.nextDeploy = time.Now().Add(f.cfg.Debounce)
	if f.timer == nil {
		f.timer = time.AfterFunc(f.cfg.Debounce, f.firePending)
	} else {
		f.timer.Reset(f.cfg.Debounce)
		f.logger.Debug("debounce timer reset")
	}
	f.pending = true
}

func (f *Fanout) firePending() {
	f.mu.Lock()
	payload := f.pendingPayload
	f.pendingPayload, f.pending = nil, false
	f.nextDeploy = time.Time{}
	f.mu.Unlock()
	if payload != nil {
		f.deliver(payload)
	}
}

func (f *Fanout) deliver(payload []byte) {
	start := time.Now()
	var wg sync.WaitGroup
	var ok atomic.Int64
	sem := make(chan struct{}, f.cfg.MaxConcurrent)

	for _, url := range f.cfg.TargetURLs() {
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			if err := f.postWithRetry(url, payload); err != nil {
				f.logger.Error("webhook-fanout failed", "url", url, "err", err)
				return
			}
			ok.Add(1)
		})
	}
	wg.Wait()

	f.logger.Info("fan-out finished",
		"success", ok.Load(),
		"total", len(f.cfg.IDs),
		"duration", time.Since(start).Round(time.Millisecond),
	)
}

func (f *Fanout) postWithRetry(url string, payload []byte) error {
	delay := f.cfg.RetryBackoff
	for attempt := range f.cfg.MaxAttempts {
		if attempt > 0 {
			time.Sleep(delay)
			delay *= 2
		}

		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Hub-Signature-256", "sha256="+f.hmacSHA256(payload))

		resp, err := f.client.Do(req)
		if err != nil {
			f.logger.Error("webhook-fanout attempt network error", "url", url, "attempt", attempt+1, "err", err)
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		f.logger.Info("webhook-fanout delivery", "url", url, "status", resp.StatusCode, "attempt", attempt+1)
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return fmt.Errorf("definitive client error: HTTP %d", resp.StatusCode)
		}
		f.logger.Error("webhook-fanout attempt non-definitive status", "url", url, "status", resp.StatusCode, "attempt", attempt+1)
	}
	return fmt.Errorf("exhausted %d attempts for %s", f.cfg.MaxAttempts, url)
}

func (f *Fanout) hmacSHA256(payload []byte) string {
	mac := hmac.New(sha256.New, []byte(f.cfg.Secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}
