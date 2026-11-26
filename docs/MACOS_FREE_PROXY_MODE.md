# macOS free local proxy mode

Until Apple Network Extension distribution is available, Go can run as a
loopback-only HTTP CONNECT proxy. This is a real TCP proxy path for individual
apps that support an HTTP proxy; it is **not** a whole-device VPN and does not
capture arbitrary app traffic.

## Run

Build the CLI from the repository:

```sh
go build -o go-client ./cmd/go
./go-client serve-proxy -config /path/to/proxies.json
```

The listener defaults to `127.0.0.1:18080`. It refuses non-loopback bind
addresses. Configure an application that supports an HTTP proxy to use
`127.0.0.1`, port `18080`, with no local proxy credentials.

The JSON file uses the format in [PROXY_CONFIGURATION.md](PROXY_CONFIGURATION.md).
Set the referenced credential environment variables before starting the
process. Keep the file private and use only upstream endpoints you are
authorized to use.

Stop with Ctrl+C. The client supports HTTP CONNECT/TCP only. UDP, DNS,
QUIC, transparent system routing, and whole-device protection are not
provided. If every configured upstream fails, connections fail closed; there
is no direct-connection fallback.

## macOS app packaging

The current SwiftUI app remains a development shell. This CLI mode is the
first usable no-Apple-membership path and is deliberately documented separately
rather than presenting the Packet Tunnel Connect button as a working VPN.
