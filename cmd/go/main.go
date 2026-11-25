package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ibthe/Go/internal/proxycheck"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "check-proxy" {
		fmt.Println("Go Smart Routing — development build")
		fmt.Println("Usage: go run ./cmd/go check-proxy -endpoint https://proxy.example:443 [-target example.com:443]")
		fmt.Println("This command only probes a proxy; it does not change system network settings.")
		return
	}
	fs := flag.NewFlagSet("check-proxy", flag.ExitOnError)
	endpoints := fs.String("endpoint", "", "HTTPS proxy endpoint; repeat by comma-separating endpoints")
	target := fs.String("target", "example.com:443", "CONNECT target used for the compatibility check")
	timeout := fs.Duration("timeout", 8*time.Second, "overall probe timeout")
	_ = fs.Parse(os.Args[2:])
	list := make([]string, 0)
	for _, e := range strings.Split(*endpoints, ",") {
		if strings.TrimSpace(e) != "" { list = append(list, strings.TrimSpace(e)) }
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	result, err := proxycheck.FirstHealthy(ctx, list, *target, &tls.Config{MinVersion: tls.VersionTLS12})
	if err != nil { fmt.Fprintln(os.Stderr, "Proxy check failed:", err); os.Exit(1) }
	fmt.Printf("Healthy proxy: %s; CONNECT %s; latency %s\n", result.Endpoint, result.Target, result.Latency.Round(time.Millisecond))
}
