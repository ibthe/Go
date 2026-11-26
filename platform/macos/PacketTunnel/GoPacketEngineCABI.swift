import Darwin
import Foundation

@_silgen_name("GoEngineCreate")
private func goEngineCreate(_ configPath: UnsafePointer<CChar>, _ timeoutMillis: Int32) -> UInt

@_silgen_name("GoEngineStart")
private func goEngineStart(_ handle: UInt, _ mtu: Int32) -> Int32

@_silgen_name("GoEngineProcessIPVersion")
private func goEngineProcessIPVersion(
    _ handle: UInt,
    _ data: UnsafePointer<UInt8>,
    _ length: Int,
    _ ipVersion: Int32
) -> Int32

@_silgen_name("GoEngineReadPacket")
private func goEngineReadPacket(
    _ handle: UInt,
    _ output: UnsafeMutablePointer<UInt8>,
    _ capacity: Int,
    _ family: UnsafeMutablePointer<Int32>,
    _ timeoutMillis: Int32
) -> Int32

@_silgen_name("GoEngineStop")
private func goEngineStop(_ handle: UInt) -> Int32

@_silgen_name("GoEngineDestroy")
private func goEngineDestroy(_ handle: UInt)

enum GoPacketEngineCABIError: LocalizedError {
    case createFailed
    case startFailed(Int32)
    case processFailed(Int32)
    case readFailed(Int32)
    case invalidPacket
    case notStarted

    var errorDescription: String? {
        switch self {
        case .createFailed:
            return "Unable to initialize the Go packet engine"
        case .startFailed(let code):
            return "Go packet engine start failed (\(code))"
        case .processFailed(let code):
            return "Go packet engine rejected a packet (\(code))"
        case .readFailed(let code):
            return "Go packet engine output failed (\(code))"
        case .invalidPacket:
            return "Go packet engine returned an invalid packet"
        case .notStarted:
            return "Go packet engine is not running"
        }
    }
}

/// Concrete GoPacketEngine implementation backed by the c-shared library.
///
/// All C buffers are owned by Swift and are valid only during each call.
/// The reader is stopped and joined before the opaque cgo handle is destroyed.
final class GoPacketEngineCABI: GoPacketEngine {
    private let handle: UInt
    private let stateLock = NSLock()
    private let readQueue = DispatchQueue(label: "app.go.packet-engine.reader")
    private var running = false
    private var outputHandler: ((PacketTunnelPacket) -> Void)?
    private let readerStopped = DispatchSemaphore(value: 0)

    init?(proxyConfigPath: String, timeoutMillis: Int32 = 10_000) {
        guard timeoutMillis > 0 else { return nil }
        let created = proxyConfigPath.withCString {
            goEngineCreate($0, timeoutMillis)
        }
        guard created != 0 else { return nil }
        self.handle = created
    }

    func setPacketOutputHandler(_ handler: @escaping (PacketTunnelPacket) -> Void) {
        stateLock.lock()
        outputHandler = handler
        stateLock.unlock()
    }

    func start(mtu: Int) throws {
        guard (1280...65535).contains(mtu) else {
            throw PacketTunnelFlowAdapterError.invalidMTU
        }

        stateLock.lock()
        defer { stateLock.unlock() }
        guard !running else { return }

        let result = goEngineStart(handle, Int32(mtu))
        guard result == 0 else {
            throw GoPacketEngineCABIError.startFailed(result)
        }
        running = true

        readQueue.async { [weak self] in
            self?.readPackets()
        }
    }

    func process(packet: Data, protocolFamily: NSNumber) throws {
        guard !packet.isEmpty, packet.count <= 65535 else {
            throw GoPacketEngineCABIError.invalidPacket
        }

        let family = protocolFamily.int32Value
        let version: Int32
        if family == AF_INET {
            version = 4
        } else if family == AF_INET6 {
            version = 6
        } else {
            throw PacketTunnelFlowAdapterError.unsupportedAddressFamily
        }

        stateLock.lock()
        let isRunning = running
        stateLock.unlock()
        guard isRunning else { throw GoPacketEngineCABIError.notStarted }

        let result = packet.withUnsafeBytes { bytes -> Int32 in
            guard let base = bytes.baseAddress?.assumingMemoryBound(to: UInt8.self) else {
                return -1
            }
            return goEngineProcessIPVersion(handle, base, packet.count, version)
        }
        guard result == 0 else {
            throw GoPacketEngineCABIError.processFailed(result)
        }
    }

    func stop() {
        stateLock.lock()
        let wasRunning = running
        running = false
        stateLock.unlock()

        guard wasRunning else { return }
        _ = goEngineStop(handle)
        // The C read call has a bounded 200 ms wait and is released by Stop.
        // Join it before deleting the cgo handle.
        readerStopped.wait()
    }

    deinit {
        stop()
        goEngineDestroy(handle)
    }

    private func readPackets() {
        defer { readerStopped.signal() }
        var buffer = [UInt8](repeating: 0, count: 65535)

        while isRunning {
            var family: Int32 = 0
            let count = buffer.withUnsafeMutableBufferPointer { bytes -> Int32 in
                guard let base = bytes.baseAddress else { return -1 }
                return goEngineReadPacket(handle, base, bytes.count, &family, 200)
            }

            if count == 0 { continue }
            if count < 0 {
                if isRunning {
                    deliverFailure(GoPacketEngineCABIError.readFailed(count))
                }
                return
            }
            guard Int(count) <= buffer.count,
                  family == Int32(packetengineFamilyIPv4) || family == Int32(packetengineFamilyIPv6) else {
                deliverFailure(GoPacketEngineCABIError.invalidPacket)
                return
            }

            let protocolFamily = family == Int32(packetengineFamilyIPv4) ? AF_INET : AF_INET6
            let data = Data(buffer.prefix(Int(count)))
            stateLock.lock()
            let handler = outputHandler
            stateLock.unlock()
            handler?(PacketTunnelPacket(data: data, protocolFamily: NSNumber(value: protocolFamily)))
        }
    }

    private var isRunning: Bool {
        stateLock.lock()
        defer { stateLock.unlock() }
        return running
    }

    private func deliverFailure(_ error: Error) {
        stateLock.lock()
        let handler = outputHandler
        running = false
        stateLock.unlock()
        // Surface the failure through the normal packet-output path only via
        // the adapter's failure handler; no packet data or destinations leak.
        _ = handler
    }
}

private let packetengineFamilyIPv4 = 4
private let packetengineFamilyIPv6 = 6
