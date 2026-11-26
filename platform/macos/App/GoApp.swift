import NetworkExtension
import SwiftUI

struct GoApp: App {
    var body: some Scene {
        WindowGroup {
            TunnelDashboard()
        }
    }
}

private struct TunnelDashboard: View {
    @StateObject private var tunnel = TunnelManager()
    @State private var configPath = ""
    @State private var isSaving = false

    var body: some View {
        VStack(spacing: 16) {
            Image(systemName: "network")
                .font(.system(size: 36))
            Text("Go")
                .font(.largeTitle)
            HStack(spacing: 8) {
                Circle()
                    .fill(statusColor)
                    .frame(width: 9, height: 9)
                Text(statusDescription)
                    .font(.headline)
            }

            VStack(alignment: .leading, spacing: 8) {
                Text("Proxy configuration file")
                    .font(.callout.weight(.medium))
                TextField("/absolute/path/to/proxies.json", text: $configPath)
                    .textFieldStyle(.roundedBorder)
                Button(isSaving ? "Saving…" : "Save tunnel profile") {
                    Task {
                        isSaving = true
                        defer { isSaving = false }
                        do {
                            try await tunnel.saveProfile(configPath: configPath)
                        } catch {
                            // The error is displayed below; no secrets or paths are logged.
                            errorMessage = error.localizedDescription
                        }
                    }
                }
                .disabled(isSaving || configPath.isEmpty)
            }
            .frame(maxWidth: 380)

            HStack {
                Button("Connect") {
                    Task {
                        do {
                            try await tunnel.connect()
                            errorMessage = nil
                        } catch {
                            errorMessage = error.localizedDescription
                        }
                    }
                }
                .disabled(!tunnel.isConfigured || isActive)

                Button("Disconnect") {
                    tunnel.disconnect()
                }
                .disabled(!isActive)
            }

            if let error = errorMessage ?? tunnel.lastError {
                Text(error)
                    .foregroundStyle(.red)
                    .font(.caption)
                    .frame(maxWidth: 380)
            }

            Text("Experimental TCP-only engine. The provider currently installs no traffic routes; connecting only exercises the Network Extension lifecycle.")
                .font(.caption)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
                .frame(maxWidth: 390)
        }
        .padding(28)
        .frame(minWidth: 480, minHeight: 390)
        .task {
            await tunnel.load()
        }
    }

    @State private var errorMessage: String?

    private var isActive: Bool {
        switch tunnel.status {
        case .connecting, .connected, .reasserting, .disconnecting:
            return true
        default:
            return false
        }
    }

    private var statusDescription: String {
        switch tunnel.status {
        case .invalid: return "Not configured"
        case .disconnected: return tunnel.isConfigured ? "Disconnected" : "No profile"
        case .connecting: return "Connecting"
        case .connected: return "Connected (smoke test)"
        case .reasserting: return "Reconnecting"
        case .disconnecting: return "Disconnecting"
        @unknown default: return "Unknown"
        }
    }

    private var statusColor: Color {
        switch tunnel.status {
        case .connected: return .green
        case .connecting, .reasserting, .disconnecting: return .orange
        default: return .secondary
        }
    }
}
