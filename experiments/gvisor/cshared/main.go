package main

/*
#include <stdint.h>
#include <stddef.h>
*/
import "C"

import (
	"context"
	"encoding/json"
	"errors"
	"runtime/cgo"
	"strings"
	"time"
	"unsafe"

	processor "github.com/ibthe/Go/experiments/gvisor"
	"github.com/ibthe/Go/internal/packetengine"
)

type bridge struct {
	engine *packetengine.Engine
}

//export GoEngineCreate
func GoEngineCreate(configPath *C.char, timeoutMillis C.int) C.uintptr_t {
	if configPath == nil || timeoutMillis < 1 {
		return 0
	}
	engine, err := processor.NewEngineFromEnvironment(C.GoString(configPath), time.Duration(timeoutMillis)*time.Millisecond)
	if err != nil {
		return 0
	}
	return C.uintptr_t(cgo.NewHandle(&bridge{engine: engine}))
}

//export GoEngineCreateWithCredentials
func GoEngineCreateWithCredentials(configPath *C.char, credentialsJSON *C.char, timeoutMillis C.int) C.uintptr_t {
	if configPath == nil || credentialsJSON == nil || timeoutMillis < 1 {
		return 0
	}
	raw := C.GoString(credentialsJSON)
	if len(raw) > 64*1024 {
		return 0
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var credentials map[string]string
	if err := decoder.Decode(&credentials); err != nil || credentials == nil {
		return 0
	}
	if decoder.Decode(new(any)) == nil {
		return 0
	}
	lookup := func(name string) (string, bool) {
		value, ok := credentials[name]
		return value, ok
	}
	engine, err := processor.NewEngineFromProxyConfig(C.GoString(configPath), lookup, time.Duration(timeoutMillis)*time.Millisecond)
	if err != nil {
		return 0
	}
	return C.uintptr_t(cgo.NewHandle(&bridge{engine: engine}))
}

//export GoEngineStart
func GoEngineStart(raw C.uintptr_t, mtu C.int) C.int {
	b, ok := getBridge(raw)
	if !ok {
		return -1
	}
	if err := b.engine.Start(int(mtu)); err != nil {
		return -2
	}
	return 0
}

//export GoEngineProcess
func GoEngineProcess(raw C.uintptr_t, data *C.uint8_t, length C.size_t, family C.int) C.int {
	b, ok := getBridge(raw)
	if !ok || data == nil || length == 0 || uint64(length) > 65535 {
		return -1
	}
	var f packetengine.Family
	switch int(family) {
	case 2: // AF_INET
		f = packetengine.FamilyIPv4
	case 30: // AF_INET6 on Darwin; callers should use GoEngineProcessIPVersion below on other platforms.
		f = packetengine.FamilyIPv6
	case 10: // AF_INET6 on Linux
		f = packetengine.FamilyIPv6
	default:
		return -2
	}
	packet := C.GoBytes(unsafe.Pointer(data), C.int(length))
	if err := b.engine.Process(packet, f); err != nil {
		return -3
	}
	return 0
}

//export GoEngineProcessIPVersion
func GoEngineProcessIPVersion(raw C.uintptr_t, data *C.uint8_t, length C.size_t, ipVersion C.int) C.int {
	b, ok := getBridge(raw)
	if !ok || data == nil || length == 0 || uint64(length) > 65535 {
		return -1
	}
	var family packetengine.Family
	switch int(ipVersion) {
	case 4:
		family = packetengine.FamilyIPv4
	case 6:
		family = packetengine.FamilyIPv6
	default:
		return -2
	}
	if err := b.engine.Process(C.GoBytes(unsafe.Pointer(data), C.int(length)), family); err != nil {
		return -3
	}
	return 0
}

// GoEngineReadPacket returns packet length, 0 on timeout, and a negative value
// on error. If capacity is too small, it returns the required length without
// copying. The caller owns the C buffer for the duration of this call.
//
//export GoEngineReadPacket
func GoEngineReadPacket(raw C.uintptr_t, output *C.uint8_t, capacity C.size_t, family *C.int, timeoutMillis C.int) C.int {
	b, ok := getBridge(raw)
	if !ok || output == nil || family == nil || timeoutMillis < 1 {
		return -1
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMillis)*time.Millisecond)
	defer cancel()
	packet, err := b.engine.ReadOutput(ctx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return 0
		}
		return -2
	}
	*family = C.int(packet.Family)
	if uint64(capacity) < uint64(len(packet.Data)) {
		return C.int(len(packet.Data))
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(output)), int(capacity))
	copy(dst, packet.Data)
	return C.int(len(packet.Data))
}

//export GoEngineStop
func GoEngineStop(raw C.uintptr_t) C.int {
	b, ok := getBridge(raw)
	if !ok {
		return -1
	}
	if err := b.engine.Stop(); err != nil {
		return -2
	}
	return 0
}

//export GoEngineDestroy
func GoEngineDestroy(raw C.uintptr_t) {
	if raw == 0 {
		return
	}
	handle := cgo.Handle(raw)
	if value, ok := safeValue(handle); ok {
		_ = value.engine.Stop()
		handle.Delete()
	}
}

func getBridge(raw C.uintptr_t) (*bridge, bool) {
	if raw == 0 {
		return nil, false
	}
	value, ok := safeValue(cgo.Handle(raw))
	return value, ok
}

func safeValue(handle cgo.Handle) (value *bridge, ok bool) {
	defer func() {
		if recover() != nil {
			value, ok = nil, false
		}
	}()
	value, ok = handle.Value().(*bridge)
	return
}

func main() {
}
