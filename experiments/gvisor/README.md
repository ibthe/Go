# gVisor netstack spike

This is an isolated compatibility experiment, not part of the production build.

## Why it is isolated

The main application currently targets Go 1.23. The pinned gVisor Go-compatible branch requires Go 1.27.3. The nested module keeps that toolchain and gVisor's dependency graph out of the production module.

The dependency is pinned to gVisor Go-branch commit:

- Repository: https://github.com/gvisor/gvisor
- Branch: `go`
- Commit: `71fa0a6d436cebce326882814e66962494be351c`
- Go toolchain: `1.27.3`

The upstream project documents that its `go` branch is the standard-Go-compatible import branch; `master` itself is not intended to be consumed with ordinary `go get`.

## Run

From this directory:

```sh
go mod tidy
go test -race -count=1 ./...
```

## What the tests prove

- `TestIPv4TCPFlowBridgesIntoGoRelay` creates two independent in-memory gVisor IPv4/TCP stacks. Their channel link endpoints exchange actual IP packets. A TCP connection is established through that packet exchange; the server accepts it as a `net.Conn`; the Go tunnel relay maps the accepted flow's local destination address to an HTTPS CONNECT request; and a local TLS CONNECT fixture forwards the stream to a TCP echo backend.
- `TestIPv6TCPFlowBridgesIntoGoRelay` repeats the packet-to-stream path over IPv6 and verifies that the relay preserves the original bracketed IPv6 destination (`[fd77::2]:8443`). It uses an in-memory stream dialer to isolate and test the IPv6 destination mapping.

`GVisorProcessor` now implements the root `packetengine.Processor` contract. It creates a gVisor stack with IPv4/IPv6 routes, spoofing/promiscuous ingress, a TCP SYN forwarder, and an asynchronous packet-output loop. Accepted TCP streams are passed to the supplied `tunnel.ContextDialer`; the intended production dialer is `proxypool.Pool`, so proxy failure never falls back to a direct socket. `TestGVisorProcessorConnectsPacketEngineToTCPDialer` drives a synthetic client stack through the root packet engine and verifies a TCP echo round trip and the original destination address.

Together, the tests validate IPv4/IPv6 packet-to-stream handling, IPv4 CONNECT transport, and the packet-engine-to-dialer integration in a controlled environment. They do **not** yet validate Apple's `NEPacketTunnelFlow`, a real TUN descriptor, UDP, DNS policy, system routing, or production performance. The processor is still an isolated prototype and must not be treated as a production VPN.


## Wiring the proxy pool into the packet engine

The exported `processor.NewEngineFromEnvironment` factory composes the external
JSON configuration, environment-based credential lookup, rotating
`proxypool.Pool`, gVisor TCP processor, and root `packetengine.Engine`:

```go
engine, err := processor.NewEngineFromEnvironment("proxies.json", 10*time.Second)
if err != nil {
    return err
}
if err := engine.Start(1500); err != nil {
    return err
}
defer engine.Stop()
```

The endpoint list remains external to the repository. This module still needs
the Go 1.27.3 toolchain and is not yet exposed to Swift through a C ABI.

## C ABI for native platform adapters

The `cshared` package exposes a deliberately small C ABI for Swift and other
native hosts. Build it from this directory with:

```sh
go build -buildmode=c-shared -o libgo_packet_engine.dylib ./cshared # macOS
# Linux CI uses .so; Windows packaging will need a separate DLL build.
```

The build emits the matching `libgo_packet_engine.h` header. The exported API:

- `GoEngineCreate(configPath, timeoutMillis)` is the development/environment-lookup constructor.
- `GoEngineCreateWithCredentials(configPath, credentialsJSON, timeoutMillis)` accepts a bounded JSON map of credential-reference names to secret values, resolves them in memory, and returns an opaque `uintptr_t` handle. The native caller's string buffer is copied synchronously and is not retained.
- `GoEngineStart(handle, mtu)` starts the userspace stack.
- `GoEngineProcessIPVersion(handle, bytes, length, 4|6)` copies one complete IP packet into Go.
- `GoEngineReadPacket(handle, buffer, capacity, &family, timeoutMillis)` polls for one outbound packet; positive means bytes copied, zero means timeout, negative means error.
- `GoEngineStop(handle)` stops the engine; `GoEngineDestroy(handle)` releases the opaque handle.

The native caller owns all buffers and must keep them valid only for the duration
of each call. The opaque handle is backed by `runtime/cgo.Handle`; no Go pointer
is retained by native code. Calls on one handle must be serialized by the host,
and the host must stop polling before destroying it. For now, allocate a
65,535-byte output buffer: a too-small buffer returns the required size after
consuming that packet. The API returns coarse status codes and intentionally
does not expose packet contents, destinations, or credentials in diagnostics.

The macOS Swift wrapper now calls
`GoEngineCreateWithCredentials`: the host app copies non-secret configuration
into a shared App Group container, the extension reads proxy secrets from a
shared Keychain access group, and the credential map is passed synchronously
into Go. App Group and Keychain entitlement sharing still require Apple
registration, signed provisioning profiles, and real-Mac runtime validation.
The environment-based constructor remains for development tooling only.
UDP, DNS interception, QUIC behavior, and full-device routing remain
unsupported/unverified.
