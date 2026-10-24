#if DEBUG
import Foundation

// Contacts and companies routes.

/// Contacts and companies.
struct DemoDirectory: Sendable {
    static func seed(_ ws: DemoWorkspace) -> DemoDirectory { DemoDirectory() }
}

extension DemoBackend {
    /// The answer (nil inside for 204), or nil when the route isn't this area's.
    func directoryRoutes(_ call: Call, in ws: inout DemoWorkspace) throws -> Data?? {
        nil
    }
}
#endif
