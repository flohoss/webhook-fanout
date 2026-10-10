package config

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/caarlos0/env/v11"
)

const ListenPath = "/api/git/stacks/webhook"

type Config struct {
	TZ               time.Location `env:"TZ,notEmpty" envDefault:"UTC"`
	Secret           string        `env:"FANOUT_SECRET,notEmpty,unset"`
	TrustedProxy     TrustedProxy  `env:"TRUSTED_PROXY,notEmpty"`
	Dockhand         Dockhand      `env:"DOCKHAND_URL,notEmpty"`
	DockhandAPIToken string        `env:"DOCKHAND_API_TOKEN,notEmpty,unset"`
	AcceptRegex      AcceptRegex   `env:"ACCEPT_REGEX"`
	LogLevel         slog.Level    `env:"LOG_LEVEL,notEmpty" envDefault:"info"`
	ListenAddr       string        `env:"LISTEN_ADDR,notEmpty" envDefault:"0.0.0.0:8080"`
	Debounce         time.Duration `env:"DEBOUNCE,notEmpty" envDefault:"5m"`
	RequestTimeout   time.Duration `env:"REQUEST_TIMEOUT,notEmpty" envDefault:"30s"`
	MaxBodyBytes     int64         `env:"MAX_BODY_BYTES,notEmpty" envDefault:"1048576"`
	MaxAttempts      int           `env:"MAX_ATTEMPTS,notEmpty" envDefault:"4"`
	RetryBackoff     time.Duration `env:"RETRY_BACKOFF,notEmpty" envDefault:"500ms"`
	ShutdownTimeout  time.Duration `env:"SHUTDOWN_TIMEOUT,notEmpty" envDefault:"10s"`
	RateLimit        float64       `env:"RATE_LIMIT,notEmpty" envDefault:"10"`
	MaxConcurrent    int           `env:"MAX_CONCURRENT,notEmpty" envDefault:"4"`
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
