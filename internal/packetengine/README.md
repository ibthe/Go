# Packet engine boundary

This package is the platform-neutral Go contract for a native packet-tunnel
adapter. It deliberately does not depend on gVisor, cgo, Apple SDKs, or a
specific userspace IP stack.

## Contract

- The native bridge normalizes platform address-family constants to
  `FamilyIPv4` (4) and `FamilyIPv6` (6). Do not pass Darwin's raw
  `AF_INET6` value through as this package's family enum.
- Buffers contain complete IP packets without Ethernet headers.
- The processor returns zero or more complete IP packets for each input.
- The engine serializes processor calls and owns Start/Stop transitions.
- Input and output buffers are copied at the boundary to avoid shared mutable
  memory across cgo or native-language ownership boundaries.
- Invalid family/version/MTU combinations are rejected before packets reach
  the stack. Processor output is validated before it is returned to Swift.

A concrete gVisor-backed Processor and a stable C ABI are still outstanding.
The isolated gVisor experiment is not wired into this package or the macOS
extension. This contract does not imply UDP, DNS, QUIC, or full-device VPN
support.
