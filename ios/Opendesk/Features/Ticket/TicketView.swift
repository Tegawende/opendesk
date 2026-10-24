import SwiftUI

/// One ticket: the thread, the composer, Ollie and the ticket's properties.
struct TicketView: View {
    let route: TicketRoute

    var body: some View { Text(route.id) }
}
