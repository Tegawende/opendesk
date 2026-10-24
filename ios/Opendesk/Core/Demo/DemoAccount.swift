#if DEBUG
import Foundation

// The account's routes beyond the core ones in DemoBackend.swift: profile, security
// (password, 2FA, passkeys), notification preferences, saved replies and views.

/// The signed-in user's account data that is not per workspace.
struct DemoAccount: Sendable {
    static func seed(user: User) -> DemoAccount { DemoAccount() }
}

/// The user's data in one workspace (member profile, preferences).
struct DemoAccountWorkspace: Sendable {
    static func seed(_ ws: DemoWorkspace) -> DemoAccountWorkspace { DemoAccountWorkspace() }
}

extension DemoBackend {
    /// Routes without a workspace. The answer (nil inside for 204), or nil when not this area's.
    func meRoutes(_ call: Call) throws -> Data?? {
        nil
    }

    /// Workspace routes. The answer (nil inside for 204), or nil when not this area's.
    func accountRoutes(_ call: Call, in ws: inout DemoWorkspace) throws -> Data?? {
        nil
    }
}
#endif
