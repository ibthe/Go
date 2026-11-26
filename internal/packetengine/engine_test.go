package packetengine

import (
	"errors"
	"testing"
)

type fakeProcessor struct {
	startedMTU int
	startErr   error
	processErr error
	stopErr    error
	packet     []byte
	family     Family
	stops      int
}

func (p *fakeProcessor) Start(mtu int) error {
	p.startedMTU = mtu
	return p.startErr
}

func (p *fakeProcessor) ProcessPacket(packet []byte, family Family) ([][]byte, error) {
	p.packet = packet
	p.family = family
	return [][]byte{append([]byte(nil), packet...)}, p.processErr
}

func (p *fakeProcessor) Stop() error {
	p.stops++
	return p.stopErr
}

func TestEngineLifecycleAndPacketCopies(t *testing.T) {
	processor := &fakeProcessor{}
	engine, err := New(processor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Process([]byte{0x45}, FamilyIPv4); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Process before Start error = %v", err)
	}
	if err := engine.Start(1500); err != nil {
		t.Fatal(err)
	}
	if processor.startedMTU != 1500 {
		t.Fatalf("processor MTU = %d, want 1500", processor.startedMTU)
	}

	input := []byte{0x45, 0x00, 0x01}
	responses, err := engine.Process(input, FamilyIPv4)
	if err != nil {
		t.Fatal(err)
	}
	input[0] = 0
	if processor.packet[0] != 0x45 {
		t.Fatal("processor input aliases caller buffer")
	}
	responses[0][0] = 0
	if processor.packet[0] != 0x45 {
		t.Fatal("response aliases processor input buffer")
	}
	if err := engine.Stop(); err != nil {
		t.Fatal(err)
	}
	if processor.stops != 1 {
		t.Fatalf("Stop calls = %d, want 1", processor.stops)
	}
	if err := engine.Stop(); err != nil {
		t.Fatal(err)
	}
	if processor.stops != 1 {
		t.Fatalf("repeated Stop calls = %d, want 1", processor.stops)
	}
}

func TestEngineAcceptsIPv4AndIPv6(t *testing.T) {
	for _, tc := range []struct {
		name   string
		packet []byte
		family Family
	}{
		{"IPv4", []byte{0x45, 0, 0}, FamilyIPv4},
		{"IPv6", []byte{0x60, 0, 0}, FamilyIPv6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, _ := New(&fakeProcessor{})
			if err := engine.Start(1500); err != nil {
				t.Fatal(err)
			}
			if _, err := engine.Process(tc.packet, tc.family); err != nil {
				t.Fatal(err)
			}
			_ = engine.Stop()
		})
	}
}

func TestEngineRejectsInvalidPackets(t *testing.T) {
	tests := []struct {
		name   string
		packet []byte
		family Family
		mtu    int
		want   error
	}{
		{"unsupported family", []byte{0x45}, 1, 1500, ErrUnsupportedFamily},
		{"family mismatch", []byte{0x60}, FamilyIPv4, 1500, ErrPacketFamilyMismatch},
		{"empty", nil, FamilyIPv4, 1500, nil},
		{"over MTU", make([]byte, 1501), FamilyIPv4, 1500, ErrPacketExceedsMTU},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			engine, _ := New(&fakeProcessor{})
			if err := engine.Start(tc.mtu); err != nil {
				t.Fatal(err)
			}
			_, err := engine.Process(tc.packet, tc.family)
			if tc.want == nil {
				if err == nil {
					t.Fatal("expected validation error")
				}
			} else if !errors.Is(err, tc.want) {
				t.Fatalf("Process error = %v, want %v", err, tc.want)
			}
			_ = engine.Stop()
		})
	}
}

func TestEngineRejectsInvalidMTUAndProcessorStartFailure(t *testing.T) {
	for _, mtu := range []int{0, 1279, 65536} {
		engine, _ := New(&fakeProcessor{})
		if err := engine.Start(mtu); !errors.Is(err, ErrInvalidMTU) {
			t.Errorf("Start(%d) error = %v, want ErrInvalidMTU", mtu, err)
		}
	}

	startFailure := errors.New("stack init failed")
	engine, _ := New(&fakeProcessor{startErr: startFailure})
	if err := engine.Start(1500); !errors.Is(err, startFailure) {
		t.Fatalf("Start error = %v, want wrapped start failure", err)
	}
	if _, err := engine.Process([]byte{0x45}, FamilyIPv4); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Process after failed Start error = %v", err)
	}
}

func TestEngineRejectsMalformedProcessorOutput(t *testing.T) {
	processor := &malformedOutputProcessor{}
	engine, _ := New(processor)
	if err := engine.Start(1500); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Process([]byte{0x45}, FamilyIPv4); !errors.Is(err, ErrPacketFamilyMismatch) {
		t.Fatalf("Process error = %v, want family mismatch", err)
	}
	_ = engine.Stop()
}

type malformedOutputProcessor struct{}

func (*malformedOutputProcessor) Start(int) error { return nil }
func (*malformedOutputProcessor) ProcessPacket([]byte, Family) ([][]byte, error) {
	return [][]byte{{0x60}}, nil
}
func (*malformedOutputProcessor) Stop() error { return nil }
