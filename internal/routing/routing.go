// Package routing contains platform-independent routing decisions.
// It deliberately does not perform DNS lookups or open network connections.
package routing

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

type Action string

const (
	Proxy  Action = "proxy"
	Direct Action = "direct"
	Block  Action = "block"
)

type Rule struct {
	// Domain is an exact hostname or a suffix beginning with a dot,
	// e.g. "example.com" or ".example.com".
	Domain string
	Action Action
}

type Decision struct {
	Action Action
	Reason string
}

// NormalizeHost strips a trailing dot and an optional port. It does not
// resolve the hostname and does not inspect user traffic.
func NormalizeHost(host string) (string, error) {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return "", errors.New("host is empty")
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.TrimSuffix(host, ".")
	if host == "" || strings.ContainsAny(host, "/\\ ") {
		return "", fmt.Errorf("invalid hostname %q", host)
	}
	return host, nil
}

// Decide applies the first matching explicit rule. Unknown destinations
// default to direct routing; callers must separately enforce fail-closed
// behavior for destinations that have been classified as proxy-required.
func Decide(host string, rules []Rule) (Decision, error) {
	normalized, err := NormalizeHost(host)
	if err != nil {
		return Decision{}, err
	}
	for _, rule := range rules {
		if rule.Action != Proxy && rule.Action != Direct && rule.Action != Block {
			return Decision{}, fmt.Errorf("unsupported action %q", rule.Action)
		}
		domain := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(rule.Domain)), ".")
		if domain == "" {
			return Decision{}, errors.New("rule domain is empty")
		}
		if normalized == domain || strings.HasSuffix(normalized, "."+domain) {
			return Decision{Action: rule.Action, Reason: "matched domain rule: " + rule.Domain}, nil
		}
	}
	return Decision{Action: Direct, Reason: "no matching rule"}, nil
}
