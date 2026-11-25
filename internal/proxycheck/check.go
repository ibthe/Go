// Package proxycheck verifies that a TLS-protected HTTP proxy is reachable
// and accepts CONNECT requests. It never sends application traffic through it.
package proxycheck

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Result struct {
	Endpoint string
	Target   string
	Latency  time.Duration
}

func Check(ctx context.Context, rawEndpoint, target string, tlsConfig *tls.Config) (Result, error) {
	u, err := url.Parse(rawEndpoint)
	if err != nil { return Result{}, fmt.Errorf("parse proxy endpoint: %w", err) }
	if u.Scheme != "https" || u.Hostname() == "" || u.Port() == "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return Result{}, errors.New("proxy endpoint must be https://host:port with no path, query, or fragment")
	}
	if target == "" || strings.ContainsAny(target, "\r\n ") { return Result{}, errors.New("invalid CONNECT target") }

	start := time.Now()
	dialer := net.Dialer{Timeout: 8 * time.Second}
	rawConn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(u.Hostname(), u.Port()))
	if err != nil { return Result{}, fmt.Errorf("dial proxy: %w", err) }
	defer rawConn.Close()

	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if tlsConfig != nil { cfg = tlsConfig.Clone(); if cfg.MinVersion < tls.VersionTLS12 { cfg.MinVersion = tls.VersionTLS12 } }
	if cfg.ServerName == "" { cfg.ServerName = u.Hostname() }
	conn := tls.Client(rawConn, cfg)
	if deadline, ok := ctx.Deadline(); ok { _ = conn.SetDeadline(deadline) }
	if err := conn.HandshakeContext(ctx); err != nil { return Result{}, fmt.Errorf("TLS handshake: %w", err) }

	req, _ := http.NewRequest(http.MethodConnect, "https://"+target, nil)
	req.Host = target
	if u.User != nil {
		password, _ := u.User.Password()
		token := base64.StdEncoding.EncodeToString([]byte(u.User.Username()+":"+password))
		req.Header.Set("Proxy-Authorization", "Basic "+token)
	}
	if err := req.Write(conn); err != nil { return Result{}, fmt.Errorf("write CONNECT: %w", err) }
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil { return Result{}, fmt.Errorf("read CONNECT response: %w", err) }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("proxy rejected CONNECT: %s", resp.Status)
	}
	return Result{Endpoint: u.Hostname()+":"+u.Port(), Target: target, Latency: time.Since(start)}, nil
}

// FirstHealthy checks endpoints in order and returns the first one accepting CONNECT.
// Failed endpoints are reported without exposing embedded credentials.
func FirstHealthy(ctx context.Context, endpoints []string, target string, tlsConfig *tls.Config) (Result, error) {
	var failures []string
	for _, endpoint := range endpoints {
		result, err := Check(ctx, endpoint, target, tlsConfig)
		if err == nil { return result, nil }
		u, parseErr := url.Parse(endpoint)
		label := "invalid-endpoint"
		if parseErr == nil && u.Hostname() != "" { label = u.Hostname() }
		failures = append(failures, label+": "+err.Error())
		if ctx.Err() != nil { break }
	}
	if len(failures) == 0 { return Result{}, errors.New("no proxy endpoints configured") }
	return Result{}, fmt.Errorf("no healthy proxy endpoint: %s", strings.Join(failures, "; "))
}
