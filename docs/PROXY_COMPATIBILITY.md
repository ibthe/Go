# Proxy compatibility probe

The first networking milestone validates a TLS-protected HTTP proxy endpoint.

Expected endpoint format:

`https://hostname:port`

The probe:
1. opens TCP to the endpoint;
2. performs TLS 1.2 or newer with normal certificate verification;
3. sends an HTTP CONNECT request for a caller-selected target;
4. treats only HTTP 200 as a successful tunnel negotiation.

It does **not** prove that the proxy can carry UDP, QUIC, DNS, arbitrary IP packets, or all device traffic. It also does not establish authorization to use a third-party endpoint. Credentials embedded in a URL are used for HTTP Basic proxy authentication and must never be committed to source control or logs.

Example:

```sh
go test ./...
```

A later platform tunnel will need a separate adapter that forwards TCP streams through a verified proxy transport. Full-device routing is not implemented by this probe.
