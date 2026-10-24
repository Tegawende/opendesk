#if DEBUG
import Foundation

// Ollie's routes beyond the core run and decision streams.

/// Ollie's past conversations and tips.
struct DemoAssistantExtras: Sendable {
    static func seed(_ ws: DemoWorkspace) -> DemoAssistantExtras { DemoAssistantExtras() }
}

extension DemoBackend {
    /// The answer (nil inside for 204), or nil when the route isn't this area's.
    func assistantRoutes(_ call: Call, in ws: inout DemoWorkspace) throws -> Data?? {
        nil
    }
}
#endif

#if DEBUG
extension DemoBackend {
    /// Streams (`text/event-stream`) beyond the assistant run and decision: nil when not one.
    func streamRoutes(_ call: Call) throws -> [Beat]? {
        nil
    }
}
#endif
