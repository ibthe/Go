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

## Security review notes

Public reputation sources report elevated risk signals for the two backup IPs:

- `162.244.81.74` is reported by IPGeolocation as a detected proxy/VPN and spam-associated IP with a high threat score. This is third-party reputation data, not proof of malicious activity. See [IPGeolocation report](https://ipgeolocation.io/browse/ip/162.244.81.74).
- `162.244.81.207` has a historical spam report in CleanTalk's data. CleanTalk's page says it was not currently listed in its blacklist at its last update. See [CleanTalk report](https://cleantalk.org/blacklists/162.244.81.207).

These signals strengthen the case against putting the backup IPs into a production pool until the operator, authorization, and service purpose are independently confirmed.

## Configuration decision

No active `proxies.json` was generated from this list. Adding endpoints that failed the compatibility check would make failover unreliable and could send traffic to an unverified intermediary.

The audit workflow is repeatable at [Proxy endpoint audit](https://github.com/ibthe/Go/actions/runs/45750562357). Its artifact is retained for seven days. Re-run after obtaining valid operator documentation/credentials or after the endpoints are corrected.
