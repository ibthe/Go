# Roadmap

## Phase 0 — repository scaffold
- [x] Public repository initialized
- [x] Routing decision package and unit tests
- [x] CI workflow
- [ ] Select project license after dependency review

## Phase 1 — local macOS prototype
- [ ] Confirm proxy endpoint protocol and permission for independent use
- [ ] Implement a local-only proxy health checker
- [ ] Build a macOS Network Extension prototype
- [ ] Test DNS, IPv4/IPv6, TCP, UDP, and fail-closed behavior
- [ ] Package and sign a development build

## Phase 2 — Windows
- [ ] Select and integrate a supported tunnel adapter
- [ ] Implement service lifecycle and recovery
- [ ] Add route and DNS leak tests

## Phase 3 — Android
- [ ] Implement VpnService lifecycle
- [ ] Add foreground notification and consent flow
- [ ] Test Wi-Fi/mobile handoff, sleep, and battery behavior

## Phase 4 — release readiness
- [ ] Signed, versioned rule updates
- [ ] Reproducible CI builds and software bill of materials
- [ ] Security review and privacy documentation
- [ ] Platform-specific installation and recovery guides
