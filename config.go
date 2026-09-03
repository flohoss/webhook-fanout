package main

import (
	"log/slog"
	"slices"
	"strings"
	"time"
)

type IDs []string

func (s *IDs) UnmarshalText(text []byte) error {
	parts := strings.Split(string(text), ",")
	ids := make([]string, 0, len(parts))
	for _, p := range parts {
		ids = append(ids, strings.TrimSpace(p))
	}
	slices.Sort(ids)
	*s = ids
	return nil
}

type config struct {
	TZ             time.Location `env:"TZ,notEmpty" envDefault:"UTC"`
	Secret         string        `env:"WEBHOOK_SECRET,notEmpty,unset"`
	IDs            IDs           `env:"IDS,notEmpty"`
	TargetTemplate string        `env:"TARGET_TEMPLATE,notEmpty"`
	LogLevel       slog.Level    `env:"LOG_LEVEL,notEmpty" envDefault:"info"`
}
