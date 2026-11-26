// Package proxyconfig loads proxy endpoints without storing credentials in the file.
package proxyconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"strings"
)

const maxConfigSize = 1 << 20

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Config is the on-disk proxy configuration.
type Config struct {
	Proxies []Proxy `json:"proxies"`
}

// Proxy describes one HTTPS CONNECT proxy. Credentials are referenced by
// environment-variable name and are never written into the configuration.
type Proxy struct {
	ID          string `json:"id"`
	Endpoint    string `json:"endpoint"`
	UsernameEnv string `json:"username_env,omitempty"`
	PasswordEnv string `json:"password_env,omitempty"`
}

// ResolvedProxy contains credentials in memory for use by the transport layer.
// Do not log URL or serialize this value.
type ResolvedProxy struct {
	ID  string
	URL *url.URL
}

// Load reads and validates a JSON configuration file. On Unix-like systems,
// group/world-readable configuration files are rejected to reduce accidental
// exposure if a future format adds non-secret local settings.
func Load(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open proxy config: %w", err)
	}
	defer f.Close()

	if runtime.GOOS != "windows" {
		info, err := f.Stat()
		if err != nil {
			return Config{}, fmt.Errorf("stat proxy config: %w", err)
		}
		if info.Mode().Perm()&0077 != 0 {
			return Config{}, errors.New("proxy config permissions are too broad; use chmod 600")
		}
	}

	data, err := io.ReadAll(io.LimitReader(f, maxConfigSize+1))
	if err != nil {
		return Config{}, fmt.Errorf("read proxy config: %w", err)
	}
	if len(data) > maxConfigSize {
		return Config{}, errors.New("proxy config exceeds 1 MiB")
	}

	var cfg Config
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode proxy config: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return Config{}, errors.New("proxy config must contain one JSON value")
		}
		return Config{}, fmt.Errorf("decode trailing proxy config data: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate checks endpoint syntax and rejects embedded credentials.
func (c Config) Validate() error {
	if len(c.Proxies) == 0 {
		return errors.New("at least one proxy must be configured")
	}
	seen := make(map[string]struct{}, len(c.Proxies))
	for i, p := range c.Proxies {
		if strings.TrimSpace(p.ID) == "" || strings.ContainsAny(p.ID, "\r\n") {
			return fmt.Errorf("proxy %d: id is required", i)
		}
		if _, ok := seen[p.ID]; ok {
			return fmt.Errorf("proxy %d: duplicate id %q", i, p.ID)
		}
		seen[p.ID] = struct{}{}

		u, err := url.Parse(p.Endpoint)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.Port() == "" ||
			(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
			return fmt.Errorf("proxy %q: endpoint must be https://host:port without credentials, path, query, or fragment", p.ID)
		}
		if (p.UsernameEnv == "") != (p.PasswordEnv == "") {
			return fmt.Errorf("proxy %q: username_env and password_env must be set together", p.ID)
		}
		if p.UsernameEnv != "" && (!envName.MatchString(p.UsernameEnv) || !envName.MatchString(p.PasswordEnv)) {
			return fmt.Errorf("proxy %q: invalid credential environment variable name", p.ID)
		}
	}
	return nil
}

// Resolve expands credential references from the process environment. The
// returned URLs contain credentials only in memory and must never be logged.
func (c Config) Resolve(getenv func(string) (string, bool)) ([]ResolvedProxy, error) {
	if getenv == nil {
		return nil, errors.New("environment lookup is required")
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	out := make([]ResolvedProxy, 0, len(c.Proxies))
	for _, p := range c.Proxies {
		u, _ := url.Parse(p.Endpoint) // Validate has already checked this URL.
		if p.UsernameEnv != "" {
			username, okUser := getenv(p.UsernameEnv)
			password, okPass := getenv(p.PasswordEnv)
			if !okUser || !okPass {
				return nil, fmt.Errorf("proxy %q: credential environment variables are not set", p.ID)
			}
			u.User = url.UserPassword(username, password)
		}
		out = append(out, ResolvedProxy{ID: p.ID, URL: u})
	}
	return out, nil
}
