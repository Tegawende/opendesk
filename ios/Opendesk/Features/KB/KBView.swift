import SwiftUI

/// The knowledge base: collections and articles.
struct KBListView: View {
    var body: some View { Text("Knowledge base") }
}

/// One article, or a new one when `id` is `"new"`.
struct ArticleView: View {
    let id: String

    var body: some View { Text(id) }
}
