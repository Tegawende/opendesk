#if DEBUG
import Foundation

// The inbox's extra routes: new ticket, bulk, search.

/// Data for new tickets, bulk changes and search.
struct DemoInboxExtras: Sendable {
    static func seed(_ ws: DemoWorkspace) -> DemoInboxExtras { DemoInboxExtras() }
}

extension DemoBackend {
    /// The answer (nil inside for 204), or nil when the route isn't this area's.
    func inboxRoutes(_ call: Call, in ws: inout DemoWorkspace) throws -> Data?? {
        nil
    }
}
#endif
