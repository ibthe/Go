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
        _ = try decodeEnvelope(data)

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
        let directoryAttributes = try FileManager.default.attributesOfItem(atPath: directory.path)
        let attributes = try FileManager.default.attributesOfItem(atPath: file.path)
        guard directoryAttributes[.type] as? FileAttributeType == .typeDirectory,
              let directoryPermissions = directoryAttributes[.posixPermissions] as? NSNumber,
              directoryPermissions.intValue & 0o077 == 0,
              attributes[.type] as? FileAttributeType == .typeRegular,
              let permissions = attributes[.posixPermissions] as? NSNumber,
              permissions.intValue & 0o077 == 0,
              let fileSize = attributes[.size] as? NSNumber,
              fileSize.intValue > 0, fileSize.intValue <= maximumSize else {
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
        let envelope = try decodeEnvelope(data)
        var values: [String: String] = [:]
        for proxy in envelope.proxies {
            guard let usernameKey = proxy.usernameEnv,
                  let passwordKey = proxy.passwordEnv else { continue }
            guard let credentials = try store.load(for: proxy.id) else {
                throw SharedConfigurationError.missingCredentials(proxyID: proxy.id)
            }
            values[usernameKey] = credentials.username
            values[passwordKey] = credentials.password
        }
        return values
    }

    private static func decodeEnvelope(_ data: Data) throws -> FileEnvelope {
        guard let root = try JSONSerialization.jsonObject(with: data) as? [String: Any],
              Set(root.keys) == Set(["proxies"]),
              let rawProxies = root["proxies"] as? [[String: Any]],
              !rawProxies.isEmpty else {
            throw SharedConfigurationError.invalidConfiguration
        }

        let allowedKeys: Set<String> = ["id", "endpoint", "username_env", "password_env"]
        var identifiers = Set<String>()
        var credentialReferences = Set<String>()

        for raw in rawProxies {
            guard Set(raw.keys).isSubset(of: allowedKeys),
                  let identifier = raw["id"] as? String,
                  !identifier.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
                  !identifier.contains("\\n"), !identifier.contains("\\r"),
                  identifiers.insert(identifier).inserted,
                  let endpoint = raw["endpoint"] as? String,
                  let components = URLComponents(string: endpoint),
                  components.scheme == "https",
                  let host = components.host, !host.isEmpty,
                  let port = components.port, (1...65535).contains(port),
                  components.user == nil, components.password == nil,
                  components.path.isEmpty || components.path == "/",
                  components.query == nil, components.fragment == nil else {
                throw SharedConfigurationError.invalidConfiguration
            }

            let hasUsername = raw.keys.contains("username_env")
            let hasPassword = raw.keys.contains("password_env")
            guard hasUsername == hasPassword else {
                throw SharedConfigurationError.invalidConfiguration
            }
            if hasUsername {
                guard let usernameKey = raw["username_env"] as? String,
                      let passwordKey = raw["password_env"] as? String,
                      usernameKey.range(of: "^[A-Za-z_][A-Za-z0-9_]*$", options: .regularExpression) != nil,
                      passwordKey.range(of: "^[A-Za-z_][A-Za-z0-9_]*$", options: .regularExpression) != nil,
                      credentialReferences.insert(usernameKey).inserted,
                      credentialReferences.insert(passwordKey).inserted else {
                    throw SharedConfigurationError.invalidConfiguration
                }
            }
        }

        return try JSONDecoder().decode(FileEnvelope.self, from: data)
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
    case missingCredentials(proxyID: String)

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
        case .missingCredentials(let proxyID):
            return "Credentials are missing from Keychain for proxy ID: \\(proxyID)."
        }
    }
}
