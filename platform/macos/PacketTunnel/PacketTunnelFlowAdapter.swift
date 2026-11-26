import Darwin
import Foundation
import NetworkExtension

/// The narrow boundary implemented by the Go userspace network stack.
///
/// Packets are complete IP packets (no Ethernet header). The protocol family
/// is AF_INET or AF_INET6, as supplied by NEPacketTunnelFlow. Implementations
/// must not log packet contents or destination addresses.
protocol GoPacketEngine: AnyObject {
    func start(mtu: Int) throws
    func setPacketOutputHandler(_ handler: @escaping (PacketTunnelPacket) -> Void)
    func setFailureHandler(_ handler: @escaping (Error) -> Void)
    func process(packet: Data, protocolFamily: NSNumber) throws
    func stop()
}

struct PacketTunnelPacket {
    let data: Data
    let protocolFamily: NSNumber
}

enum PacketTunnelFlowAdapterError: Error {
    case invalidMTU
    case mismatchedPacketMetadata
    case unsupportedAddressFamily
    case packetExceedsMTU
    case packetWriteFailed
}

/// Pumps packets between Apple's Network Extension and the Go packet engine.
///
/// The containing NEPacketTunnelProvider must apply network settings before
/// calling start(), and must call stop() during stopTunnel(with:).
final class PacketTunnelFlowAdapter {
    private let packetFlow: NEPacketTunnelFlow
    private let engine: GoPacketEngine
    private let mtu: Int
    private let queue = DispatchQueue(label: "app.go.packet-tunnel.flow")
    private let failureHandler: (Error) -> Void

    private var running = false

    init(
        packetFlow: NEPacketTunnelFlow,
        engine: GoPacketEngine,
        mtu: Int,
        failureHandler: @escaping (Error) -> Void = { _ in }
    ) {
        self.packetFlow = packetFlow
        self.engine = engine
        self.mtu = mtu
        self.failureHandler = failureHandler
    }

    func start(completion: @escaping (Error?) -> Void) {
        queue.async {
            guard !self.running else {
                completion(nil)
                return
            }

            guard (1280...65535).contains(self.mtu) else {
                completion(PacketTunnelFlowAdapterError.invalidMTU)
                return
            }

            do {
                self.engine.setPacketOutputHandler { [weak self] packet in
                    self?.writeOutbound(packet)
                }
                self.engine.setFailureHandler { [weak self] error in
                    self?.queue.async { self?.fail(error) }
                }
                try self.engine.start(mtu: self.mtu)
                self.running = true
                completion(nil)
                self.readNextBatch()
            } catch {
                self.engine.stop()
                completion(error)
            }
        }
    }

    func stop(completion: (() -> Void)? = nil) {
        queue.async {
            self.running = false
            self.engine.stop()
            completion?()
        }
    }

    private func fail(_ error: Error) {
        guard running else { return }
        running = false
        engine.stop()
        failureHandler(error)
    }

    private func isSupportedFamily(_ family: NSNumber) -> Bool {
        let value = family.int32Value
        return value == AF_INET || value == AF_INET6
    }

    private func writeOutbound(_ packet: PacketTunnelPacket) {
        queue.async {
            guard self.running else { return }
            guard self.isSupportedFamily(packet.protocolFamily) else {
                self.fail(PacketTunnelFlowAdapterError.unsupportedAddressFamily)
                return
            }
            guard !packet.data.isEmpty, packet.data.count <= self.mtu else {
                self.fail(PacketTunnelFlowAdapterError.packetExceedsMTU)
                return
            }
            if !self.packetFlow.writePackets([packet.data], withProtocols: [packet.protocolFamily]) {
                self.fail(PacketTunnelFlowAdapterError.packetWriteFailed)
            }
        }
    }

    private func readNextBatch() {
        guard running else { return }

        packetFlow.readPackets { [weak self] packets, protocolFamilies in
            guard let self else { return }

            self.queue.async {
                guard self.running else { return }

                guard packets.count == protocolFamilies.count else {
                    self.fail(PacketTunnelFlowAdapterError.mismatchedPacketMetadata)
                    return
                }

                for (packet, family) in zip(packets, protocolFamilies) {
                    guard self.isSupportedFamily(family) else {
                        self.fail(PacketTunnelFlowAdapterError.unsupportedAddressFamily)
                        return
                    }
                    guard packet.count <= self.mtu else {
                        self.fail(PacketTunnelFlowAdapterError.packetExceedsMTU)
                        return
                    }

                    do {
                        try self.engine.process(packet: packet, protocolFamily: family)
                    } catch {
                        // Fail closed for this packet: do not bypass the tunnel.
                        // Intentionally avoid logging packet or destination data.
                    }
                }

                self.readNextBatch()
            }
        }
    }
}
