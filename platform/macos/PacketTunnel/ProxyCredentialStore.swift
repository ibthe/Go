import Foundation
import Security

struct ProxyCredentials: Codable {
    let username: String
    let password: String
}

/// Stores proxy credentials as a generic-password item in the macOS Keychain.
/// The credential identifier is not a proxy URL and must not contain secrets.
final class ProxyCredentialStore {
    private let service: String
    private let accessGroup: String?

    init(service: String = "app.go.proxy-credentials") {
        self.service = service
        self.accessGroup = Bundle.main.object(forInfoDictionaryKey: "GoKeychainAccessGroup") as? String
    }

    private func query(for identifier: String) throws -> [String: Any] {
        guard !identifier.isEmpty else {
            throw ProxyCredentialStoreError.invalidIdentifier
        }
        guard let accessGroup, !accessGroup.isEmpty,
              !accessGroup.contains("$(") else {
            throw ProxyCredentialStoreError.sharedAccessGroupUnavailable
        }
        return [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: identifier,
            kSecAttrAccessGroup as String: accessGroup
        ]
    }

    func save(_ credentials: ProxyCredentials, for identifier: String) throws {
        let baseQuery = try query(for: identifier)
        let data = try JSONEncoder().encode(credentials)
        let attributes: [String: Any] = [
            kSecValueData as String: data,
            kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        ]

        let status = SecItemAdd(baseQuery.merging(attributes) { _, new in new } as CFDictionary, nil)
        if status == errSecDuplicateItem {
            let updateStatus = SecItemUpdate(baseQuery as CFDictionary, attributes as CFDictionary)
            guard updateStatus == errSecSuccess else {
                throw ProxyCredentialStoreError.keychainStatus(updateStatus)
            }
        } else if status != errSecSuccess {
            throw ProxyCredentialStoreError.keychainStatus(status)
        }
    }

    func load(for identifier: String) throws -> ProxyCredentials? {
        var query = try query(for: identifier)
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        var result: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        if status == errSecItemNotFound {
            return nil
        }
        guard status == errSecSuccess, let data = result as? Data else {
            throw ProxyCredentialStoreError.keychainStatus(status)
        }
        return try JSONDecoder().decode(ProxyCredentials.self, from: data)
    }

    func delete(for identifier: String) throws {
        let query = try query(for: identifier)
        let status = SecItemDelete(query as CFDictionary)
        guard status == errSecSuccess || status == errSecItemNotFound else {
            throw ProxyCredentialStoreError.keychainStatus(status)
        }
    }
}

enum ProxyCredentialStoreError: LocalizedError {
    case invalidIdentifier
    case sharedAccessGroupUnavailable
    case keychainStatus(OSStatus)

    var errorDescription: String? {
        switch self {
        case .invalidIdentifier:
            return "A proxy credential identifier is required."
        case .sharedAccessGroupUnavailable:
            return "The shared Keychain access group is unavailable. A correctly signed app and extension are required."
        case .keychainStatus(let status):
            return "Keychain operation failed (\(status))."
        }
    }
}
