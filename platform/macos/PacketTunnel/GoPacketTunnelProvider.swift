import Foundation
import NetworkExtension

/// Lifecycle owner for the experimental macOS packet tunnel.
///
/// This provider deliberately installs no included routes. It exercises the
/// Network Extension lifecycle without redirecting ordinary device traffic
/// into the current TCP-only prototype.
final class GoPacketTunnelProvider: NEPacketTunnelProvider {
    private enum ConfigurationKey {
        static let proxyConfigPath = "proxyConfigPath"
        static let mtu = "mtu"
        static let connectTimeoutMillis = "connectTimeoutMillis"
    }

    private var packetEngine: GoPacketEngineCABI?
    private var flowAdapter: PacketTunnelFlowAdapter?

    override func startTunnel(
        options: [String: NSObject]?,
        completionHandler: @escaping (Error?) -> Void
    ) {
        guard
            let tunnelProtocol = protocolConfiguration as? NETunnelProviderProtocol,
            let configuration = tunnelProtocol.providerConfiguration,
            let configPath = configuration[ConfigurationKey.proxyConfigPath] as? String,
            configPath.hasPrefix("/"),
            FileManager.default.isReadableFile(atPath: configPath)
        else {
            completionHandler(GoPacketTunnelProviderError.invalidConfiguration)
            return
        }

        let mtuValue = configuration[ConfigurationKey.mtu] as? Int ?? 1500
        let timeoutValue = configuration[ConfigurationKey.connectTimeoutMillis] as? Int ?? 10_000
        guard (1280...65535).contains(mtuValue),
              timeoutValue > 0,
              timeoutValue <= Int(Int32.max) else {
            completionHandler(GoPacketTunnelProviderError.invalidConfiguration)
            return
        }

        guard let engine = GoPacketEngineCABI(
            proxyConfigPath: configPath,
            timeoutMillis: Int32(timeoutValue)
        ) else {
            completionHandler(GoPacketTunnelProviderError.engineInitializationFailed)
            return
        }

        do {
            // Initialize the userspace engine before asking the OS to activate
            // the tunnel interface. PacketTunnelFlowAdapter.start is idempotent.
            try engine.start(mtu: mtuValue)
        } catch {
            engine.stop()
            completionHandler(error)
            return
        }

        let settings = makeSmokeTestNetworkSettings(mtu: mtuValue)
        setTunnelNetworkSettings(settings) { [weak self] error in
            guard let self else {
                engine.stop()
                completionHandler(GoPacketTunnelProviderError.providerReleased)
                return
            }
            if let error {
                engine.stop()
                completionHandler(error)
                return
            }

            let adapter = PacketTunnelFlowAdapter(
                packetFlow: self.packetFlow,
                engine: engine,
                mtu: mtuValue
            ) { [weak self] failure in
                self?.cancelTunnelWithError(failure)
            }
            self.packetEngine = engine
            self.flowAdapter = adapter
            adapter.start { startError in
                if let startError {
                    adapter.stop()
                    self.packetEngine = nil
                    self.flowAdapter = nil
                }
                completionHandler(startError)
            }
        }
    }

    override func stopTunnel(
        with reason: NEProviderStopReason,
        completionHandler: @escaping () -> Void
    ) {
        let adapter = flowAdapter
        flowAdapter = nil
        packetEngine = nil
        if let adapter {
            adapter.stop(completion: completionHandler)
        } else {
            completionHandler()
        }
    }

    private func makeSmokeTestNetworkSettings(mtu: Int) -> NEPacketTunnelNetworkSettings {
        let settings = NEPacketTunnelNetworkSettings(tunnelRemoteAddress: "127.0.0.1")
        settings.mtu = NSNumber(value: mtu)

        let ipv4 = NEIPv4Settings(
            addresses: ["198.18.0.1"],
            subnetMasks: ["255.255.255.255"]
        )
        ipv4.includedRoutes = []
        settings.ipv4Settings = ipv4

        let ipv6 = NEIPv6Settings(
            addresses: ["fd00:676f:3a3a::1"],
            networkPrefixLengths: [NSNumber(value: 128)]
        )
        ipv6.includedRoutes = []
        settings.ipv6Settings = ipv6

        return settings
    }
}

private enum GoPacketTunnelProviderError: LocalizedError {
    case invalidConfiguration
    case engineInitializationFailed
    case providerReleased

    var errorDescription: String? {
        switch self {
        case .invalidConfiguration:
            return "The packet tunnel provider configuration is invalid"
        case .engineInitializationFailed:
            return "The Go packet engine could not be initialized"
        case .providerReleased:
            return "The packet tunnel provider stopped during startup"
        }
    }
}
