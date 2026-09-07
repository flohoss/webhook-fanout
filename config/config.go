package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

type IDs []string

func (s *IDs) UnmarshalText(text []byte) error {
	parts := strings.Split(string(text), ",")
	ids := make([]string, 0, len(parts))
	for _, p := range parts {
		if id := strings.TrimSpace(p); id != "" {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	*s = ids
	return nil
}

type Template struct {
	raw string
	url *url.URL
}

func (t *Template) UnmarshalText(text []byte) error {
	raw := string(text)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("TARGET_TEMPLATE %q must be an absolute URL (e.g. https://host/api/git/stacks/{id}/webhook)", raw)
	}
	if strings.Count(u.Path, "{id}") != 1 {
		return fmt.Errorf("TARGET_TEMPLATE %q must contain exactly one {id} placeholder in its path", raw)
	}
	if !strings.Contains(u.Path, "/{id}") {
		return fmt.Errorf("TARGET_TEMPLATE %q must have {id} as a full path segment", raw)
	}
	t.raw, t.url = raw, u
	return nil
}

func (t Template) String() string { return t.raw }

func (t Template) listenPath() string {
	if p := strings.ReplaceAll(t.url.Path, "/{id}", ""); p != "" {
		return p
	}
	return "/"
}

func (t Template) targetURLs(ids []string) []string {
	urls := make([]string, len(ids))
	for i, id := range ids {
		urls[i] = strings.ReplaceAll(t.raw, "{id}", id)
	}
	return urls
}

type AcceptRegex struct {
	re *regexp.Regexp
}

func (a *AcceptRegex) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		a.re = nil
		return nil
	}
	re, err := regexp.Compile(string(text))
	if err != nil {
		return fmt.Errorf("ACCEPT_REGEX %q is not a valid regex: %w", text, err)
	}
	a.re = re
	return nil
}

func (a AcceptRegex) Matches(body []byte) bool {
	return a.re == nil || a.re.Match(body)
}

type Config struct {
	TZ              time.Location `env:"TZ,notEmpty" envDefault:"UTC"`
	Secret          string        `env:"WEBHOOK_SECRET,notEmpty,unset"`
	IDs             IDs           `env:"IDS,notEmpty"`
	TargetTemplate  Template      `env:"TARGET_TEMPLATE,notEmpty"`
	AcceptRegex     AcceptRegex   `env:"ACCEPT_REGEX"`
	LogLevel        slog.Level    `env:"LOG_LEVEL,notEmpty" envDefault:"info"`
	ListenAddr      string        `env:"LISTEN_ADDR,notEmpty" envDefault:"0.0.0.0:8080"`
	Debounce        time.Duration `env:"DEBOUNCE,notEmpty" envDefault:"5m"`
	RequestTimeout  time.Duration `env:"REQUEST_TIMEOUT,notEmpty" envDefault:"15s"`
	MaxBodyBytes    int64         `env:"MAX_BODY_BYTES,notEmpty" envDefault:"1048576"`
	MaxAttempts     int           `env:"MAX_ATTEMPTS,notEmpty" envDefault:"4"`
	RetryBackoff    time.Duration `env:"RETRY_BACKOFF,notEmpty" envDefault:"500ms"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT,notEmpty" envDefault:"10s"`
	RateLimit       float64       `env:"RATE_LIMIT,notEmpty" envDefault:"10"`
	MaxConcurrent   int           `env:"MAX_CONCURRENT,notEmpty" envDefault:"8"`
}

func (c Config) ListenPath() string {
	return c.TargetTemplate.listenPath()
}

func (c Config) TargetURLs() []string {
	return c.TargetTemplate.targetURLs(c.IDs)
}

func Load() (Config, error) {
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return Config{}, fmt.Errorf("load config: %w", err)
	}
	return cfg, cfg.validate()
}

func (c Config) validate() error {
	for name, v := range map[string]time.Duration{
		"DEBOUNCE":         c.Debounce,
		"REQUEST_TIMEOUT":  c.RequestTimeout,
		"RETRY_BACKOFF":    c.RetryBackoff,
		"SHUTDOWN_TIMEOUT": c.ShutdownTimeout,
	} {
		if v <= 0 {
			return fmt.Errorf("%s must be positive, got %s", name, v)
		}
	}
	if c.MaxAttempts < 1 {
		return fmt.Errorf("MAX_ATTEMPTS must be at least 1, got %d", c.MaxAttempts)
	}
	if c.MaxConcurrent < 1 {
		return fmt.Errorf("MAX_CONCURRENT must be at least 1 (0 would deadlock delivery), got %d", c.MaxConcurrent)
	}
	if c.MaxBodyBytes < 1 {
		return fmt.Errorf("MAX_BODY_BYTES must be positive, got %d", c.MaxBodyBytes)
	}
	if c.RateLimit <= 0 {
		return fmt.Errorf("RATE_LIMIT must be positive, got %g", c.RateLimit)
	}
	return nil
}
