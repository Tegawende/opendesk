import SwiftUI

/// The Mail area (Collab): boxes and the conversation list.
struct MailListView: View {
    var body: some View { Text("Mail") }
}

/// One Collab conversation.
struct MailThreadView: View {
    let id: String

    var body: some View { Text(id) }
}
