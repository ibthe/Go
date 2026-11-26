package processor

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewEngineFromProxyConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "proxies.json")
	data := `{"proxies":[{"id":"test-edge","endpoint":"https://proxy.example.invalid:443"}]}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngineFromEnvironment(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Start(mtu); err != nil {
		t.Fatal(err)
	}
	if err := engine.Stop(); err != nil {
		t.Fatal(err)
	}
}
