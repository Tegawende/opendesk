#if DEBUG
import Foundation

// The ticket view's routes beyond the core ones in DemoBackend.swift.

/// Ticket data beyond the core seed: custom field values, side conversations, drafts, merges.
struct DemoTicketExtras: Sendable {
    static func seed(_ ws: DemoWorkspace) -> DemoTicketExtras { DemoTicketExtras() }
}

extension DemoBackend {
    /// The answer (nil inside for 204), or nil when the route isn't this area's.
    func ticketRoutes(_ call: Call, in ws: inout DemoWorkspace) throws -> Data?? {
        nil
    }
}
#endif
