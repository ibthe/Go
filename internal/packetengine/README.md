# Packet engine boundary

This package is the platform-neutral Go contract for a native packet-tunnel
adapter. It deliberately does not depend on gVisor, cgo, Apple SDKs, or a
specific userspace IP stack.

## Contract

- The native bridge normalizes platform address-family constants to
  `FamilyIPv4` (4) and `FamilyIPv6` (6). Do not pass Darwin's raw
  `AF_INET6` value through as this package's family enum.
- Buffers contain complete IP packets without Ethernet headers.
- Input packets are processed synchronously, but output packets are emitted
  asynchronously. TCP acknowledgements, SYN-ACKs, retransmissions, and data
  returned by a proxy may be generated after the input call has returned.
- The processor emits complete IP packets through the callback supplied to
  `Start`. The engine validates and copies each output packet before queueing.
- The engine serializes packet processing and Start/Stop transitions.
- Input and output buffers are copied at the boundary to avoid shared mutable
  memory across cgo or native-language ownership boundaries.
- Invalid family/version/MTU combinations are rejected. Output queue overflow
  returns an explicit error; packets are never silently treated as delivered.

A concrete gVisor-backed Processor and a stable C ABI are still outstanding.
The isolated gVisor experiment is not wired into this package or the macOS
extension. This contract does not imply UDP, DNS, QUIC, or full-device VPN
support.
