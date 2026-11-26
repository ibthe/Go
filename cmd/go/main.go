package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ibthe/Go/internal/localproxy"
	"github.com/ibthe/Go/internal/proxycheck"
	"github.com/ibthe/Go/internal/proxyconfig"
	"github.com/ibthe/Go/internal/proxypool"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}
	switch os.Args[1] {
	case "check-proxy":
		checkProxy(os.Args[2:])
	case "serve-proxy":
		serveProxy(os.Args[2:])
	default:
		usage()
	}
}

func usage() {
	fmt.Println("Go — local proxy client (TCP over HTTPS CONNECT)")
	fmt.Println("  go check-proxy -endpoint https://proxy.example:443")
	fmt.Println("  go serve-proxy -config proxies.json [-listen 127.0.0.1:18080]")
	fmt.Println("The local proxy listens on loopback only; it does not change system settings.")
}

func checkProxy(args []string) {
	fs := flag.NewFlagSet("check-proxy", flag.ExitOnError)
	endpoints := fs.String("endpoint", "", "comma-separated HTTPS proxy endpoints")
	target := fs.String("target", "example.com:443", "CONNECT target")
	timeout := fs.Duration("timeout", 8*time.Second, "overall probe timeout")
	_ = fs.Parse(args)
	list := make([]string, 0)
	for _, e := range strings.Split(*endpoints, ",") {
		if strings.TrimSpace(e) != "" { list = append(list, strings.TrimSpace(e)) }
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	result, err := proxycheck.FirstHealthy(ctx, list, *target, &tls.Config{MinVersion: tls.VersionTLS12})
	if err != nil { fmt.Fprintln(os.Stderr, "Proxy check failed:", err); os.Exit(1) }
	fmt.Printf("Healthy proxy: %s; CONNECT %s; latency %s\\n", result.Endpoint, result.Target, result.Latency.Round(time.Millisecond))
}

func serveProxy(args []string) {
	fs := flag.NewFlagSet("serve-proxy", flag.ExitOnError)
	configPath := fs.String("config", "proxies.json", "proxy configuration JSON")
	listen := fs.String("listen", "127.0.0.1:18080", "loopback listen address")
	timeout := fs.Duration("timeout", 10*time.Second, "upstream connection timeout")
	_ = fs.Parse(args)
	ln, err := localproxy.NewListener(*listen)
	if err != nil { fmt.Fprintln(os.Stderr, "Listen failed:", err); os.Exit(1) }
	cfg, err := proxyconfig.Load(*configPath)
	if err != nil { _ = ln.Close(); fmt.Fprintln(os.Stderr, "Configuration failed:", err); os.Exit(1) }
	resolved, err := cfg.Resolve(os.LookupEnv)
	if err != nil { _ = ln.Close(); fmt.Fprintln(os.Stderr, "Credentials unavailable:", err); os.Exit(1) }
	pool, err := proxypool.New(resolved, *timeout)
	if err != nil { _ = ln.Close(); fmt.Fprintln(os.Stderr, "Proxy pool failed:", err); os.Exit(1) }
	server := &http.Server{Handler: localproxy.Handler{Dialer: pool}, ReadHeaderTimeout: 5*time.Second}
	fmt.Printf("Go local CONNECT proxy listening at http://%s\\n", ln.Addr())
	fmt.Println("TCP only. Upstream failure never falls back to direct networking. Press Ctrl+C to stop.")
	if err := server.Serve(ln); err != nil && err != http.ErrServerClosed { fmt.Fprintln(os.Stderr, "Proxy server failed:", err); os.Exit(1) }
}
