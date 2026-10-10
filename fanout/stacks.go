package fanout

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

type stack struct {
	ID             int    `json:"id"`
	WebhookEnabled bool   `json:"webhookEnabled"`
	WebhookSecret  string `json:"webhookSecret"`
}

type target struct {
	id     string
	url    string
	secret string
}

func (f *Fanout) fetchStacks(ctx context.Context) ([]stack, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.cfg.Dockhand.StacksURL(), nil)
	if err != nil {
		return nil, fmt.Errorf("build stacks request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+f.cfg.DockhandAPIToken)
	req.Header.Set("Accept", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch stacks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("stacks API returned HTTP %d", resp.StatusCode)
	}

	var stacks []stack
	if err := json.NewDecoder(resp.Body).Decode(&stacks); err != nil {
		return nil, fmt.Errorf("decode stacks response: %w", err)
	}
	return stacks, nil
}

func (f *Fanout) fetchEnabledTargets(ctx context.Context) ([]target, error) {
	stacks, err := f.fetchStacks(ctx)
	if err != nil {
		return nil, err
	}

	targets := make([]target, 0, len(stacks))
	for _, s := range stacks {
		if !s.WebhookEnabled {
			continue
		}
		id := strconv.Itoa(s.ID)
		targets = append(targets, target{id: id, url: f.cfg.Dockhand.WebhookURL(id), secret: s.WebhookSecret})
	}

	f.logger.Info("stacks resolved", "enabled", len(targets), "total", len(stacks))
	return targets, nil
}
