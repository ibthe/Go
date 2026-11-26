import SwiftUI

@main
struct GoApp: App {
    var body: some Scene {
        WindowGroup {
            VStack(spacing: 12) {
                Image(systemName: "network")
                    .font(.system(size: 36))
                Text("Go")
                    .font(.title)
                Text("macOS host shell")
                    .foregroundStyle(.secondary)
                Text("The packet tunnel extension is an experimental, unsigned development component.")
                    .multilineTextAlignment(.center)
                    .frame(maxWidth: 320)
                    .font(.callout)
            }
            .padding(32)
            .frame(minWidth: 420, minHeight: 260)
        }
    }
}
