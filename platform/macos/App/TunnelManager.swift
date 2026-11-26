import Foundation
import NetworkExtension

@MainActor
final class TunnelManager: ObservableObject {
    @Published private(set) var status: NEVPNStatus = .disconnected
    @Published private(set) var isConfigured = false
    @Published private(set) var lastError: String?

    private let providerBundleIdentifier = "app.go.macos.PacketTunnel"
    private var manager: NETunnelProviderManager?
    private var statusObserver: NSObjectProtocol?

    init() {
        statusObserver = NotificationCenter.default.addObserver(
            forName: .NEVPNStatusDidChange,
            object: nil,
            queue: .main
        ) { [weak self] notification in
            Task { @MainActor in
                guard let self,
                      let connection = notification.object as? NEVPNConnection,
                      connection === self.manager?.connection else { return }
                self.status = connection.status
            }
        }
    }

    deinit {
        if let statusObserver {
            NotificationCenter.default.removeObserver(statusObserver)
        }
    }

    func load() async {
        do {
            let managers = try await NETunnelProviderManager.loadAllGoManagers()
            manager = managers.first {
                ($0.protocolConfiguration as? NETunnelProviderProtocol)?
                    .providerBundleIdentifier == providerBundleIdentifier
            }
            if let manager {
                status = manager.connection.status
                isConfigured = true
            } else {
                status = .disconnected
                isConfigured = false
            }
            lastError = nil
        } catch {
            lastError = error.localizedDescription
        }
    }

    /// Saves a non-secret config-file path. Credentials must never be placed in
    /// providerConfiguration; shared Keychain access is a separate signed-build task.
    func saveProfile(configPath: String) async throws {
        let path = configPath.trimmingCharacters(in: .whitespacesAndNewlines)
        guard path.hasPrefix("/"),
              FileManager.default.isReadableFile(atPath: path) else {
            throw TunnelManagerError.configurationFileNotReadable
        }

        _ = try GoSharedConfiguration.installConfiguration(from: URL(fileURLWithPath: path))

        let target = manager ?? NETunnelProviderManager()
        let tunnelProtocol = NETunnelProviderProtocol()
        tunnelProtocol.providerBundleIdentifier = providerBundleIdentifier
        tunnelProtocol.serverAddress = "Go local packet tunnel"
        tunnelProtocol.providerConfiguration = [
            "appGroupIdentifier": GoSharedConfiguration.appGroupIdentifier,
            "configFileName": GoSharedConfiguration.fileName,
            "mtu": 1500,
            "connectTimeoutMillis": 10_000
        ]
        target.protocolConfiguration = tunnelProtocol
        target.localizedDescription = "Go"
        target.isEnabled = true

        try await target.saveGoPreferences()
        try await target.reloadGoPreferences()
        manager = target
        status = target.connection.status
        isConfigured = true
        lastError = nil
    }

    func saveCredential(proxyID: String, username: String, password: String) throws {
        let identifier = proxyID.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !identifier.isEmpty, !username.isEmpty, !password.isEmpty else {
            throw TunnelManagerError.invalidCredential
        }
        try ProxyCredentialStore().save(
            ProxyCredentials(username: username, password: password),
            for: identifier
        )
        lastError = nil
    }

    func connect() async throws {
        guard let manager, isConfigured else {
            throw TunnelManagerError.profileNotConfigured
        }
        do {
            try manager.connection.startVPNTunnel()
            status = manager.connection.status
            lastError = nil
        } catch {
            lastError = error.localizedDescription
            throw error
        }
    }

    func disconnect() {
        manager?.connection.stopVPNTunnel()
        status = manager?.connection.status ?? .disconnected
    }
}

enum TunnelManagerError: LocalizedError {
    case configurationFileNotReadable
    case profileNotConfigured
    case invalidCredential

    var errorDescription: String? {
        switch self {
        case .configurationFileNotReadable:
            return "The proxy configuration file is not readable at this path."
        case .profileNotConfigured:
            return "Save a tunnel profile before connecting."
        case .invalidCredential:
            return "Proxy ID, username, and password are all required."
        }
    }
}

private extension NETunnelProviderManager {
    static func loadAllGoManagers() async throws -> [NETunnelProviderManager] {
        try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<[NETunnelProviderManager], Error>) in
            loadAllFromPreferences { managers, error in
                if let error {
                    continuation.resume(throwing: error)
                } else {
                    continuation.resume(returning: managers ?? [])
                }
            }
        }
    }

    func saveGoPreferences() async throws {
        try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, Error>) in
            saveToPreferences { error in
                if let error {
                    continuation.resume(throwing: error)
                } else {
                    continuation.resume(returning: ())
                }
            }
        }
    }

    func reloadGoPreferences() async throws {
        try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, Error>) in
            loadFromPreferences { error in
                if let error {
                    continuation.resume(throwing: error)
                } else {
                    continuation.resume(returning: ())
                }
            }
        }
    }
}
