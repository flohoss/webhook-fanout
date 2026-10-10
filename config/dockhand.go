package config

import (
	"fmt"
	"net/url"
	"strings"
)

type Dockhand struct {
	url *url.URL
}

func (d *Dockhand) UnmarshalText(text []byte) error {
	raw := string(text)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("DOCKHAND_URL %q must be an absolute URL (e.g. http://dockhand.example.com:3000)", raw)
	}
	d.url = u
	return nil
}

func (d Dockhand) String() string { return d.url.String() }

func (d Dockhand) StacksURL() string { return d.base() + "/api/git/stacks" }

func (d Dockhand) WebhookURL(id string) string {
	return d.base() + "/api/git/stacks/" + id + "/webhook"
}

func (d Dockhand) base() string { return strings.TrimSuffix(d.url.String(), "/") }
