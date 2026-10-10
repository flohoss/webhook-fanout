package config

import (
	"fmt"
	"net"
	"strings"
)

type TrustedProxy struct {
	ipNet *net.IPNet
}

func (t *TrustedProxy) UnmarshalText(text []byte) error {
	raw := strings.TrimSpace(string(text))
	cidr := raw
	if !strings.Contains(raw, "/") {
		ip := net.ParseIP(raw)
		if ip == nil {
			return fmt.Errorf("TRUSTED_PROXY %q is not a valid IP or CIDR", raw)
		}
		bits := 128
		if ip.To4() != nil {
			bits = 32
		}
		cidr = fmt.Sprintf("%s/%d", ip.String(), bits)
	}
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("TRUSTED_PROXY %q is not a valid CIDR: %w", raw, err)
	}
	t.ipNet = ipNet
	return nil
}

func (t TrustedProxy) IPNet() *net.IPNet { return t.ipNet }

func (t TrustedProxy) String() string { return t.ipNet.String() }
