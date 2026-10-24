import SwiftUI

/// The Companies area's root list.
struct CompaniesListView: View {
    var body: some View { Text("Companies") }
}

/// One company.
struct CompanyDetailView: View {
    let id: String

    var body: some View { Text(id) }
}
