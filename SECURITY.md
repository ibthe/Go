# Security policy

## Scope

This is an early development scaffold, not a production VPN or proxy client. Do not rely on it to protect traffic.

## Reporting

Please do not publish credentials, private proxy configuration, browsing history, packet captures containing personal data, or undisclosed vulnerabilities in public issues.

For a sensitive report, contact the repository owner privately through GitHub.

## Privacy principles

- No analytics or browsing-history collection by default.
- No remote executable configuration.
- Rule updates must be authenticated, versioned, schema-validated, and rollback-protected.
- Proxy failure must not silently downgrade proxy-required traffic to direct.
- macOS proxy configuration is copied into a dedicated App Group container with restrictive POSIX permissions; the system VPN profile stores only a fixed filename and non-secret settings.
- macOS proxy usernames/passwords are stored in a shared Keychain access group, indexed by proxy ID. Secrets are resolved only in the packet-tunnel extension and passed synchronously into Go memory; they are not placed in JSON configuration, provider preferences, command-line arguments, or logs.
- App Group and Keychain sharing are not considered verified until the identifiers are registered with Apple, both targets are signed with matching provisioning profiles, and runtime access is tested on a Mac.
- The current macOS packet tunnel is still a TCP-only development scaffold with no included traffic routes. It must not be treated as a production VPN.
