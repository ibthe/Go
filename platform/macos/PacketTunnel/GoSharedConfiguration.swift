import Darwin
import Foundation

enum GoSharedConfiguration {
    static let appGroupIdentifier = "group.app.go.macos"
    static let directoryName = "Library/Application Support/Go"
    static let fileName = "proxies.json"
    private static let maximumSize = 1 << 20

    private struct FileEnvelope: Decodable {
        let proxies: [ProxyReference]
    }

    private struct ProxyReference: Decodable {
        let id: String
        let usernameEnv: String?
        let passwordEnv: String?

        enum CodingKeys: String, CodingKey {
            case id
            case usernameEnv = "username_env"
            case passwordEnv = "password_env"
        }
    }

    static func installConfiguration(from sourceURL: URL) throws -> URL {
        let data = try Data(contentsOf: sourceURL, options: [.mappedIfSafe])
        guard !data.isEmpty, data.count <= maximumSize else {
            throw SharedConfigurationError.invalidConfigurationSize
        }
        let envelope = try JSONDecoder().decode(FileEnvelope.self, from: data)
        guard !envelope.proxies.isEmpty else {
            throw SharedConfigurationError.invalidConfiguration
        }
        for proxy in envelope.proxies {
            guard !proxy.id.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
                  (proxy.usernameEnv == nil) == (proxy.passwordEnv == nil) else {
                throw SharedConfigurationError.invalidConfiguration
            }
        }

        let directory = try containerURL().appendingPathComponent(directoryName, isDirectory: true)
        try FileManager.default.createDirectory(
            at: directory,
            withIntermediateDirectories: true,
            attributes: [.posixPermissions: 0o700]
        )
        guard chmod(directory.path, mode_t(0o700)) == 0 else {
            throw SharedConfigurationError.fileProtectionFailed
        }

        let destination = directory.appendingPathComponent(fileName, isDirectory: false)
        try data.write(to: destination, options: [.atomic])
        guard chmod(destination.path, mode_t(0o600)) == 0 else {
            throw SharedConfigurationError.fileProtectionFailed
        }
        return destination
    }

    static func configurationURL() throws -> URL {
        let directory = try containerURL().appendingPathComponent(directoryName, isDirectory: true)
        let file = directory.appendingPathComponent(fileName, isDirectory: false)
        let attributes = try FileManager.default.attributesOfItem(atPath: file.path)
        guard attributes[.type] as? FileAttributeType == .typeRegular,
              let permissions = attributes[.posixPermissions] as? NSNumber,
              permissions.intValue & 0o077 == 0 else {
            throw SharedConfigurationError.fileProtectionFailed
        }
        return file
    }

    static func loadCredentialValues(
        configURL: URL,
        store: ProxyCredentialStore
    ) throws -> [String: String] {
        let data = try Data(contentsOf: configURL, options: [.mappedIfSafe])
        guard data.count <= maximumSize else {
            throw SharedConfigurationError.invalidConfigurationSize
        }
        let envelope = try JSONDecoder().decode(FileEnvelope.self, from: data)
        var values: [String: String] = [:]
        for proxy in envelope.proxies {
            guard let usernameKey = proxy.usernameEnv,
                  let passwordKey = proxy.passwordEnv else { continue }
            guard let credentials = try store.load(for: proxy.id) else { continue }
            values[usernameKey] = credentials.username
            values[passwordKey] = credentials.password
        }
        return values
    }

    private static func containerURL() throws -> URL {
        guard let url = FileManager.default.containerURL(
            forSecurityApplicationGroupIdentifier: appGroupIdentifier
        ) else {
            throw SharedConfigurationError.appGroupUnavailable
        }
        return url
    }
}

enum SharedConfigurationError: LocalizedError {
    case appGroupUnavailable
    case invalidConfiguration
    case invalidConfigurationSize
    case fileProtectionFailed

    var errorDescription: String? {
        switch self {
        case .appGroupUnavailable:
            return "The Go shared application container is unavailable. A signed App Group entitlement is required."
        case .invalidConfiguration:
            return "The proxy configuration file is invalid."
        case .invalidConfigurationSize:
            return "The proxy configuration file is empty or exceeds 1 MiB."
        case .fileProtectionFailed:
            return "The shared configuration file permissions are not sufficiently restrictive."
        }
    }
}
