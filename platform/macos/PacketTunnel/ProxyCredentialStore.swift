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

    init(service: String = "app.go.proxy-credentials") {
        self.service = service
    }

    func save(_ credentials: ProxyCredentials, for identifier: String) throws {
        guard !identifier.isEmpty else {
            throw ProxyCredentialStoreError.invalidIdentifier
        }
        let data = try JSONEncoder().encode(credentials)
        let baseQuery: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: identifier
        ]
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
        guard !identifier.isEmpty else {
            throw ProxyCredentialStoreError.invalidIdentifier
        }
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: identifier,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne
        ]
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
        guard !identifier.isEmpty else {
            throw ProxyCredentialStoreError.invalidIdentifier
        }
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: identifier
        ]
        let status = SecItemDelete(query as CFDictionary)
        guard status == errSecSuccess || status == errSecItemNotFound else {
            throw ProxyCredentialStoreError.keychainStatus(status)
        }
    }
}

enum ProxyCredentialStoreError: Error {
    case invalidIdentifier
    case keychainStatus(OSStatus)
}
