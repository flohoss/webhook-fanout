package config

import (
	"fmt"
	"regexp"
)

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
