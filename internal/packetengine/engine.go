// Package packetengine defines the platform-neutral contract between a packet
// tunnel adapter and a userspace IP stack.
package packetengine

import (
	"errors"
	"fmt"
	"sync"
)

type Family uint8

const (
	FamilyIPv4 Family = 4
	FamilyIPv6 Family = 6
)

const (
	minMTU = 1280
	maxMTU = 65535
)

var (
	ErrNotRunning          = errors.New("packet engine is not running")
	ErrInvalidMTU          = errors.New("MTU must be between 1280 and 65535")
	ErrUnsupportedFamily   = errors.New("unsupported IP address family")
	ErrPacketFamilyMismatch = errors.New("packet version does not match address family")
	ErrPacketExceedsMTU    = errors.New("packet exceeds configured MTU")
)

// Processor is implemented by the concrete userspace IP stack. ProcessPacket
// must return complete IP packets, without link-layer headers.
type Processor interface {
	Start(mtu int) error
	ProcessPacket(packet []byte, family Family) ([][]byte, error)
	Stop() error
}

// Engine serializes calls into Processor and owns its lifecycle. The concrete
// stack can be replaced without changing the Swift/C adapter contract.
type Engine struct {
	mu        sync.Mutex
	processor Processor
	mtu       int
	running   bool
}

func New(processor Processor) (*Engine, error) {
	if processor == nil {
		return nil, errors.New("packet processor is required")
	}
	return &Engine{processor: processor}, nil
}

func (e *Engine) Start(mtu int) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.running {
		return nil
	}
	if mtu < minMTU || mtu > maxMTU {
		return ErrInvalidMTU
	}
	if err := e.processor.Start(mtu); err != nil {
		return fmt.Errorf("start packet processor: %w", err)
	}
	e.mtu = mtu
	e.running = true
	return nil
}

// Process validates and copies packet buffers at the boundary. This prevents
// callers and stack implementations from accidentally sharing mutable memory.
func (e *Engine) Process(packet []byte, family Family) ([][]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running {
		return nil, ErrNotRunning
	}
	if err := validatePacket(packet, family, e.mtu); err != nil {
		return nil, err
	}

	responses, err := e.processor.ProcessPacket(append([]byte(nil), packet...), family)
	if err != nil {
		return nil, fmt.Errorf("process IP packet: %w", err)
	}
	out := make([][]byte, 0, len(responses))
	for _, response := range responses {
		if err := validatePacket(response, family, e.mtu); err != nil {
			return nil, fmt.Errorf("invalid response packet: %w", err)
		}
		out = append(out, append([]byte(nil), response...))
	}
	return out, nil
}

func (e *Engine) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running {
		return nil
	}
	e.running = false
	e.mtu = 0
	if err := e.processor.Stop(); err != nil {
		return fmt.Errorf("stop packet processor: %w", err)
	}
	return nil
}

func validatePacket(packet []byte, family Family, mtu int) error {
	if family != FamilyIPv4 && family != FamilyIPv6 {
		return ErrUnsupportedFamily
	}
	if len(packet) == 0 {
		return errors.New("empty IP packet")
	}
	if len(packet) > mtu {
		return ErrPacketExceedsMTU
	}
	version := packet[0] >> 4
	if (family == FamilyIPv4 && version != 4) || (family == FamilyIPv6 && version != 6) {
		return ErrPacketFamilyMismatch
	}
	return nil
}
