package processor

import (
	"errors"
	"os"
	"time"

	"github.com/ibthe/Go/internal/packetengine"
	"github.com/ibthe/Go/internal/proxyconfig"
	"github.com/ibthe/Go/internal/proxypool"
)

// NewEngineFromProxyConfig loads the external proxy list, resolves credentials
// from the supplied environment lookup, and wires the proxy pool into the
// gVisor packet processor and platform-neutral packet engine.
func NewEngineFromProxyConfig(path string, getenv func(string) (string, bool), timeout time.Duration) (*packetengine.Engine, error) {
	if getenv == nil {
		return nil, errors.New("credential environment lookup is required")
	}
	cfg, err := proxyconfig.Load(path)
	if err != nil {
		return nil, err
	}
	resolved, err := cfg.Resolve(getenv)
	if err != nil {
		return nil, err
	}
	pool, err := proxypool.New(resolved, timeout)
	if err != nil {
		return nil, err
	}
	processor, err := NewGVisorProcessor(pool)
	if err != nil {
		return nil, err
	}
	return packetengine.New(processor)
}

// NewEngineFromEnvironment uses the current process environment for proxy
// credential references in the configuration file.
func NewEngineFromEnvironment(path string, timeout time.Duration) (*packetengine.Engine, error) {
	return NewEngineFromProxyConfig(path, os.LookupEnv, timeout)
}
