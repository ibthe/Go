# Supplied proxy endpoint audit

Audit date: 2026-11-26  
Runner: GitHub Actions, Ubuntu  
Probe target: `example.com:443`  
Method: DNS/TCP connect, TLS 1.2+ with normal certificate verification, then an unauthenticated HTTP CONNECT request. The probe closes the tunnel immediately after the CONNECT response and does not send application data through it.

## Results

| Intended role | Endpoint | Result |
|---|---|---|
| Primary | `23.237.103.44:8443` | TLS reached; CONNECT rejected with `400 Bad Request` |
| Primary | `ff.s1.redirtry.top:8443` | TLS reached; CONNECT rejected with `400 Bad Request` |
| Backup | `162.244.81.74:8443` | TLS reached; CONNECT rejected with `403 Forbidden` |
| Backup | `162.244.81.207:8443` | TLS reached; CONNECT rejected with `403 Forbidden` |
| YouTube | `23.237.103.46:8443` | TLS reached; CONNECT rejected with `400 Bad Request` |
| YouTube | `ff.y1.redirtry.top:8443` | TLS reached; CONNECT rejected with `400 Bad Request` |
| Google Play | `de26.staticvaultcdn.xyz:443` | TCP connect timed out; DNS resolved to `37.203.37.91` on this runner |

**Outcome: 0/7 passed the HTTPS CONNECT compatibility probe. None of these endpoints is approved for the active Go proxy pool.**

A 400 response shows that a TLS service answered, but it does not establish that the service is an HTTP CONNECT proxy. A 403 response shows that the service denied this unauthenticated CONNECT request; it may require an allowlist, credentials, a different policy, or a different protocol. A timeout is not evidence that the service is permanently offline.

The probe did not have credentials. If the operators issued credentials or documented a required protocol, retest only with those authorized details. Do not guess credentials, bypass access controls, or try to evade a 403.


## Extended protocol audit — 2026-11-27

Runner: GitHub Actions, Ubuntu 26.04.  \
Target: `https://example.com/`.  \
Methods: (1) TLS-wrapped HTTPS CONNECT using the Go transport, (2) plaintext
HTTP proxy using curl, and (3) unauthenticated SOCKS5 using curl with remote
DNS (`socks5h`). No credentials, protocol guessing beyond these documented
handshakes, or access-control bypass was attempted.

| Endpoint | HTTPS CONNECT | HTTP proxy | SOCKS5 |
|---|---|---|---|
| `23.237.103.44:8443` | 400 Bad Request | 400 Bad Request | Timeout (5 s) |
| `ff.s1.redirtry.top:8443` | 400 Bad Request | 400 Bad Request | Timeout (5 s) |
| `162.244.81.74:8443` | 403 Forbidden | 400 Bad Request | Invalid SOCKS5 initial response |
| `162.244.81.207:8443` | 403 Forbidden | 400 Bad Request | Invalid SOCKS5 initial response |
| `23.237.103.46:8443` | 400 Bad Request | 400 Bad Request | Timeout (5 s) |
| `ff.y1.redirtry.top:8443` | 400 Bad Request | 400 Bad Request | Timeout (5 s) |
| `de26.staticvaultcdn.xyz:443` | TCP timeout | TCP timeout | TCP timeout |

**Outcome: 0/7 endpoints demonstrated a working HTTP proxy or SOCKS5 service
in this audit.** None is eligible for activation in the Go proxy pool.

Interpretation:
- HTTP 400 means the service answered but did not establish the requested
  CONNECT tunnel. It does not identify the service's intended protocol.
- HTTP 403 means the CONNECT request was denied. It may be policy- or
  authentication-related; do not attempt to bypass it.
- SOCKS5 timeout or an invalid initial response is not a successful SOCKS5
  handshake.
- These tests did not try credentials, source-IP allowlisting, custom SNI,
  proprietary framing, TLS-wrapped SOCKS5, or other undocumented protocols.
  Those require configuration or documentation from the operator; they
  should not be guessed.

The FastProxy XPI's custom proxy selector contains exactly two PAC values:
`PROXY` (UI label `HTTP/S`) and `SOCKS5`. The HTTP proxy and SOCKS5 checks
above correspond to those advertised types. The XPI does not prove that the
seven supplied endpoints are FastProxy's own proxy servers or specify their
required authentication/protocol parameters.

The extended audit was run by
[GitHub Actions run 45951120543](https://github.com/ibthe/Go/actions/runs/45951120543).
Its report artifact is retained for seven days.

## Security review notes

Public reputation sources report elevated risk signals for the two backup IPs:

- `162.244.81.74` is reported by IPGeolocation as a detected proxy/VPN and spam-associated IP with a high threat score. This is third-party reputation data, not proof of malicious activity. See [IPGeolocation report](https://ipgeolocation.io/browse/ip/162.244.81.74).
- `162.244.81.207` has a historical spam report in CleanTalk's data. CleanTalk's page says it was not currently listed in its blacklist at its last update. See [CleanTalk report](https://cleantalk.org/blacklists/162.244.81.207).

These signals strengthen the case against putting the backup IPs into a production pool until the operator, authorization, and service purpose are independently confirmed.

## Configuration decision

No active `proxies.json` was generated from this list. Adding endpoints that failed the compatibility check would make failover unreliable and could send traffic to an unverified intermediary.

The audit workflow is repeatable at [Proxy endpoint audit](https://github.com/ibthe/Go/actions/runs/45750562357). Its artifact is retained for seven days. Re-run after obtaining valid operator documentation/credentials or after the endpoints are corrected.
