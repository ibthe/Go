// Package proxypool provides ordered failover across configured HTTPS CONNECT proxies.
package proxypool

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ibthe/Go/internal/proxyconfig"
	"github.com/ibthe/Go/internal/proxytransport"
)

// Pool rotates the first proxy attempted between calls and tries the remaining
// proxies if a connection fails. It never falls back to a direct connection.
type Pool struct {
	proxies []proxyconfig.ResolvedProxy
	timeout time.Duration
	next    atomic.Uint64
	dial    func(context.Context, *proxyconfig.ResolvedProxy, string, string, time.Duration) (net.Conn, error)
}

func New(proxies []proxyconfig.ResolvedProxy, timeout time.Duration) (*Pool, error) {
	if len(proxies) == 0 {
		return nil, errors.New("at least one proxy is required")
	}
	copyOfProxies := append([]proxyconfig.ResolvedProxy(nil), proxies...)
	for _, p := range copyOfProxies {
		if p.ID == "" || p.URL == nil {
			return nil, errors.New("each proxy must have an id and URL")
		}
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Pool{
		proxies: copyOfProxies,
		timeout: timeout,
		dial: func(ctx context.Context, p *proxyconfig.ResolvedProxy, network, address string, timeout time.Duration) (net.Conn, error) {
			return (&proxytransport.Dialer{ProxyURL: p.URL, Timeout: timeout}).DialContext(ctx, network, address)
		},
	}, nil
}

// DialContext connects through the first available configured proxy. Errors
// mention proxy IDs only; credentials and URL userinfo are not included.
func (p *Pool) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if p == nil || len(p.proxies) == 0 {
		return nil, errors.New("proxy pool is empty")
	}
	start := p.next.Add(1) - 1
	failures := make([]string, 0, len(p.proxies))
	for offset := 0; offset < len(p.proxies); offset++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		index := (int(start%uint64(len(p.proxies))) + offset) % len(p.proxies)
		proxy := &p.proxies[index]
		conn, err := p.dial(ctx, proxy, network, address, p.timeout)
		if err == nil {
			return conn, nil
		}
		failures = append(failures, proxy.ID)
	}
	return nil, fmt.Errorf("all configured proxies failed (%s); direct fallback is disabled", strings.Join(failures, ", "))
}
