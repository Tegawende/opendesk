#if DEBUG
import Foundation

// Collab mail routes (contracts/collab.md).

/// Collab mail accounts, conversations, comments and drafts.
struct DemoMail: Sendable {
    static func seed(_ ws: DemoWorkspace) -> DemoMail { DemoMail() }
}

extension DemoBackend {
    /// The answer (nil inside for 204), or nil when the route isn't this area's.
    func mailRoutes(_ call: Call, in ws: inout DemoWorkspace) throws -> Data?? {
        nil
    }
}
#endif
