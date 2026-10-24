import Foundation

/// Screens pushed on the Account tab's stack (`router.accountPath`). The raw value is the
/// demo path segment (`/account/security`).
enum AccountRoute: String, Hashable, CaseIterable {
    case profile, security, notifications, phone, savedReplies = "saved-replies", views, workspaces, appearance
}
