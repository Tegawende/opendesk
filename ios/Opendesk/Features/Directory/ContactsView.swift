import SwiftUI

/// The Contacts area's root list.
struct ContactsListView: View {
    var body: some View { Text("Contacts") }
}

/// One contact.
struct ContactDetailView: View {
    let id: String

    var body: some View { Text(id) }
}
