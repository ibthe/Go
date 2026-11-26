package proxyconfig

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadAndResolve(t *testing.T) {
	path := filepath.Join(t.TempDir(), "proxies.json")
	data := `{"proxies":[{"id":"edge-a","endpoint":"https://proxy.example:443","username_env":"GO_PROXY_USER","password_env":"GO_PROXY_PASS"}]}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{"GO_PROXY_USER": "user:name", "GO_PROXY_PASS": "p@ss/word"}
	resolved, err := cfg.Resolve(func(key string) (string, bool) {
		v, ok := values[key]
		return v, ok
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := resolved[0].URL.User.Username(); got != "user:name" {
		t.Fatalf("username = %q", got)
	}
	if got, ok := resolved[0].URL.User.Password(); !ok || got != "p@ss/word" {
		t.Fatalf("password = %q, present = %v", got, ok)
	}
	if cfg.Proxies[0].Endpoint != "https://proxy.example:443" {
		t.Fatal("credentials unexpectedly changed on-disk config")
	}
}

func TestValidateRejectsEmbeddedCredentialsAndDuplicateIDs(t *testing.T) {
	tests := []Config{
		{Proxies: []Proxy{{ID: "a", Endpoint: "https://user:pass@proxy.example:443"}}},
		{Proxies: []Proxy{{ID: "a", Endpoint: "https://proxy.example:443"}, {ID: "a", Endpoint: "https://other.example:443"}}},
		{Proxies: []Proxy{{ID: "a", Endpoint: "http://proxy.example:443"}}},
		{Proxies: []Proxy{{ID: "a", Endpoint: "https://proxy.example:443", UsernameEnv: "USER"}}},
	}
	for i, cfg := range tests {
		if err := cfg.Validate(); err == nil {
			t.Errorf("case %d: expected validation error", i)
		}
	}
}

func TestResolveRequiresCredentials(t *testing.T) {
	cfg := Config{Proxies: []Proxy{{ID: "a", Endpoint: "https://proxy.example:443", UsernameEnv: "USER", PasswordEnv: "PASS"}}}
	_, err := cfg.Resolve(func(string) (string, bool) { return "", false })
	if err == nil {
		t.Fatal("expected missing credential error")
	}
}

func TestResolveWithLookupUsesInjectedValues(t *testing.T) {
	cfg := Config{Proxies: []Proxy{{
		ID: "keychain-proxy", Endpoint: "https://proxy.example:443",
		UsernameEnv: "KEYCHAIN_USER", PasswordEnv: "KEYCHAIN_PASS",
	}}}
	values := map[string]string{"KEYCHAIN_USER": "secure-user", "KEYCHAIN_PASS": "secure-pass"}
	resolved, err := cfg.ResolveWithLookup(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := resolved[0].URL.User.Username(); got != "secure-user" {
		t.Fatalf("username = %q", got)
	}
	if got, ok := resolved[0].URL.User.Password(); !ok || got != "secure-pass" {
		t.Fatalf("password = %q, present = %v", got, ok)
	}
}

func TestValidateRejectsReusedCredentialReferences(t *testing.T) {
	cfg := Config{Proxies: []Proxy{
		{ID: "a", Endpoint: "https://a.example:443", UsernameEnv: "SHARED_USER", PasswordEnv: "A_PASS"},
		{ID: "b", Endpoint: "https://b.example:443", UsernameEnv: "SHARED_USER", PasswordEnv: "B_PASS"},
	}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected reused credential reference to be rejected")
	}
}

func TestLoadRejectsBroadPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permission semantics")
	}
	path := filepath.Join(t.TempDir(), "proxies.json")
	if err := os.WriteFile(path, []byte(`{"proxies":[{"id":"a","endpoint":"https://proxy.example:443"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected broad permissions to be rejected")
	}
}
