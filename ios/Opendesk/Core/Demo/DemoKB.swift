#if DEBUG
import Foundation

// Knowledge base and reports routes.

/// Knowledge base collections and articles, and the numbers reports draw.
struct DemoKB: Sendable {
    static func seed(_ ws: DemoWorkspace) -> DemoKB { DemoKB() }
}

extension DemoBackend {
    /// The answer (nil inside for 204), or nil when the route isn't this area's.
    func kbRoutes(_ call: Call, in ws: inout DemoWorkspace) throws -> Data?? {
        nil
    }
}
#endif
