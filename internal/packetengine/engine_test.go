package packetengine

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeProcessor struct {
	startedMTU int
	startErr error
	processErr error
	stopErr error
	packet []byte
	family Family
	emit func(Packet) error
	stops int
}

func (p *fakeProcessor) Start(mtu int, emit func(Packet) error) error {
	p.startedMTU, p.emit = mtu, emit
	return p.startErr
}
func (p *fakeProcessor) ProcessPacket(packet []byte, family Family) error {
	p.packet, p.family = packet, family
	return p.processErr
}
func (p *fakeProcessor) Stop() error { p.stops++; return p.stopErr }

func TestEngineLifecycleAndInputCopy(t *testing.T) {
	processor := &fakeProcessor{}
	engine, err := New(processor)
	if err != nil { t.Fatal(err) }
	if err := engine.Process([]byte{0x45}, FamilyIPv4); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Process before Start = %v", err)
	}
	if err := engine.Start(1500); err != nil { t.Fatal(err) }
	if processor.startedMTU != 1500 { t.Fatalf("MTU = %d", processor.startedMTU) }

	input := []byte{0x45, 0, 1}
	if err := engine.Process(input, FamilyIPv4); err != nil { t.Fatal(err) }
	input[0] = 0
	if processor.packet[0] != 0x45 { t.Fatal("processor input aliases caller buffer") }

	out := Packet{Data: []byte{0x45, 0, 2}, Family: FamilyIPv4}
	if err := processor.emit(out); err != nil { t.Fatal(err) }
	out.Data[0] = 0
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, err := engine.ReadOutput(ctx)
	if err != nil { t.Fatal(err) }
	if got.Data[0] != 0x45 { t.Fatal("output aliases processor buffer") }

	if err := engine.Stop(); err != nil { t.Fatal(err) }
	if processor.stops != 1 { t.Fatalf("Stop calls = %d", processor.stops) }
	if err := engine.Stop(); err != nil { t.Fatal(err) }
	if processor.stops != 1 { t.Fatalf("repeated Stop calls = %d", processor.stops) }
}

func TestEngineAcceptsIPv4AndIPv6(t *testing.T) {
	for _, tc := range []struct{name string; packet []byte; family Family}{
		{"IPv4", []byte{0x45, 0, 0}, FamilyIPv4},
		{"IPv6", []byte{0x60, 0, 0}, FamilyIPv6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, _ := New(&fakeProcessor{})
			if err := engine.Start(1500); err != nil { t.Fatal(err) }
			if err := engine.Process(tc.packet, tc.family); err != nil { t.Fatal(err) }
			_ = engine.Stop()
		})
	}
}

func TestEngineRejectsInvalidPackets(t *testing.T) {
	tests := []struct{name string; packet []byte; family Family; want error}{
		{"unsupported family", []byte{0x45}, 1, ErrUnsupportedFamily},
		{"family mismatch", []byte{0x60}, FamilyIPv4, ErrPacketFamilyMismatch},
		{"empty", nil, FamilyIPv4, nil},
		{"over MTU", make([]byte, 1501), FamilyIPv4, ErrPacketExceedsMTU},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			engine, _ := New(&fakeProcessor{})
			if err := engine.Start(1500); err != nil { t.Fatal(err) }
			err := engine.Process(tc.packet, tc.family)
			if tc.want == nil {
				if err == nil { t.Fatal("expected validation error") }
			} else if !errors.Is(err, tc.want) { t.Fatalf("Process = %v, want %v", err, tc.want) }
			_ = engine.Stop()
		})
	}
}

func TestEngineRejectsInvalidMTUAndStartFailure(t *testing.T) {
	for _, mtu := range []int{0, 1279, 65536} {
		engine, _ := New(&fakeProcessor{})
		if err := engine.Start(mtu); !errors.Is(err, ErrInvalidMTU) { t.Errorf("Start(%d) = %v", mtu, err) }
	}
	startFailure := errors.New("stack init failed")
	engine, _ := New(&fakeProcessor{startErr: startFailure})
	if err := engine.Start(1500); !errors.Is(err, startFailure) { t.Fatalf("Start = %v", err) }
	if err := engine.Process([]byte{0x45}, FamilyIPv4); !errors.Is(err, ErrNotRunning) { t.Fatalf("Process = %v", err) }
}

func TestEngineValidatesAsynchronousOutput(t *testing.T) {
	processor := &fakeProcessor{}
	engine, _ := New(processor)
	if err := engine.Start(1500); err != nil { t.Fatal(err) }
	if err := processor.emit(Packet{Data: []byte{0x60}, Family: FamilyIPv4}); !errors.Is(err, ErrPacketFamilyMismatch) {
		t.Fatalf("emit malformed packet = %v", err)
	}
	if err := processor.emit(Packet{Data: []byte{0x45}, Family: FamilyIPv4}); err != nil { t.Fatal(err) }
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := engine.ReadOutput(ctx); err != nil { t.Fatal(err) }
	_ = engine.Stop()
}

func TestEngineOutputQueueDoesNotSilentlyDrop(t *testing.T) {
	processor := &fakeProcessor{}
	engine, _ := New(processor)
	if err := engine.Start(1500); err != nil { t.Fatal(err) }
	for i := 0; i < outputQueueSize; i++ {
		if err := processor.emit(Packet{Data: []byte{0x45}, Family: FamilyIPv4}); err != nil { t.Fatalf("emit %d: %v", i, err) }
	}
	if err := processor.emit(Packet{Data: []byte{0x45}, Family: FamilyIPv4}); !errors.Is(err, ErrOutputQueueFull) {
		t.Fatalf("queue overflow = %v, want ErrOutputQueueFull", err)
	}
	_ = engine.Stop()
}
