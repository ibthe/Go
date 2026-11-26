# Go

**Go** is a personal, cross-platform smart-routing client for macOS, Windows, and Android.

The intended user experience is simple: the user turns the connection on; Go evaluates routing rules, checks configured proxy outbounds, and selects a working route automatically.

> **Development status:** early scaffold. This repository does not yet contain a working VPN tunnel, platform integration, proxy credentials, or production routing rules.

## Project goals

- One-button connection with automatic route selection.
- Route only destinations that match explicit proxy rules; use direct routing for the rest.
- Retry configured proxy outbounds in a predictable order.
- Fail closed for destinations that require a proxy when no configured proxy is healthy.
- Keep browsing history and destination telemetry on-device; no analytics by default.
- Use signed, versioned rule updates. Never execute remotely downloaded scripts.
- Share routing logic across macOS, Windows, and Android while using each platform's native VPN/tunnel APIs.

## Repository layout

- `cmd/go`: development CLI.
- `internal/routing`: platform-independent domain routing decisions.
- `internal/proxyconfig`: external proxy configuration and credential references.\n- `internal/proxypool`: rotating HTTPS CONNECT proxy pool with failover.\n- `internal/proxytransport`: HTTPS CONNECT TCP transport.
- `platforms/`: native integration plans and future platform adapters.
- `rules/`: schemas and local rule format; no third-party server list is bundled.
- `.github/workflows/`: continuous integration.

## Build and test

Requires Go 1.23 or newer.

```sh
go test ./...
go vet ./...
go run ./cmd/go
```

The current CLI is only a development placeholder. It does not alter system networking.

## Proxy infrastructure

The repository intentionally contains **no FastProxy server addresses, PAC dump, downloaded domain database, credentials, or FastProxy control-plane URLs**.

The PAC file inspected during research is a third-party configuration. Its presence on a user's device does not establish permission to reuse the associated proxy infrastructure. Before any such endpoint is added, we must establish that independent use is permitted and technically supported. Until then, development and tests use local fixtures only.

## Platform plan

- **macOS:** native Network Extension packet-tunnel provider; requires the appropriate Apple entitlement and signing/provisioning.
- **Windows:** native tunnel/packet-capture adapter and a Windows service; evaluate Wintun and Windows Filtering Platform.
- **Android:** `VpnService`-based tunnel and foreground-service lifecycle.

A shared Go routing core is the initial design. Native adapters and packaging are not implemented yet.

## Licensing

No project license has been selected. Until one is added, the repository is public for development visibility but is **not** licensed for third-party reuse. Third-party components will be documented separately.

## Security reporting

Do not publish proxy credentials, private configuration, user traffic captures, or personal browsing data in issues. See [SECURITY.md](SECURITY.md).
