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

Together, these tests validate IPv4/IPv6 packet-to-stream handling and the IPv4 CONNECT integration in a controlled environment. They do **not** yet validate Apple's `NEPacketTunnelFlow`, a real TUN descriptor, UDP, DNS policy, system routing, or production performance.
