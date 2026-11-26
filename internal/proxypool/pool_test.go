package proxypool

import (
	"context"
	"errors"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/ibthe/Go/internal/proxyconfig"
	"net/url"
)

func TestDialContextRotatesAndFallsBack(t *testing.T) {
	u, _ := url.Parse("https://proxy.example:443")
	proxies := []proxyconfig.ResolvedProxy{{ID: "one", URL: u}, {ID: "two", URL: u}, {ID: "three", URL: u}}
	p, err := New(proxies, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var attempts []string
	p.dial = func(_ context.Context, proxy *proxyconfig.ResolvedProxy, _, _ string, _ time.Duration) (net.Conn, error) {
		attempts = append(attempts, proxy.ID)
		if proxy.ID == "three" {
			left, right := net.Pipe()
			_ = right.Close()
			return left, nil
		}
		return nil, errors.New("offline")
	}
	conn, err := p.DialContext(context.Background(), "tcp", "example.com:443")
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if !reflect.DeepEqual(attempts, []string{"one", "two", "three"}) {
		t.Fatalf("attempt order = %v", attempts)
	}
}

func TestDialContextDoesNotFallBackDirect(t *testing.T) {
	u, _ := url.Parse("https://proxy.example:443")
	p, err := New([]proxyconfig.ResolvedProxy{{ID: "private-edge", URL: u}}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	p.dial = func(context.Context, *proxyconfig.ResolvedProxy, string, string, time.Duration) (net.Conn, error) {
		return nil, errors.New("secret failure detail")
	}
	_, err = p.DialContext(context.Background(), "tcp", "example.com:443")
	if err == nil || err.Error() != "all configured proxies failed (private-edge); direct fallback is disabled" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDialContextHonorsCancellation(t *testing.T) {
	u, _ := url.Parse("https://proxy.example:443")
	p, _ := New([]proxyconfig.ResolvedProxy{{ID: "one", URL: u}}, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := p.DialContext(ctx, "tcp", "example.com:443")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}
