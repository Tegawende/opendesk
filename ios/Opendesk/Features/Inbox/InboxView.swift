import SwiftUI

struct InboxRootView: View {
    @Environment(AppEnvironment.self) private var env

    var body: some View {
        @Bindable var router = env.router
        NavigationStack(path: $router.inboxPath) {
            AreaRootView()
                .navigationDestination(for: AppRoute.self) { route in
                    AppRouteDestination(route: route)
                }
        }
    }
}

/// The ticket list for a folder or view: dense rows, swipe to solve, snooze
/// or take, pull to refresh, presence on the rows, all live.
struct InboxListView: View {
    @Environment(AppEnvironment.self) private var env
    @State private var query = ""
    @FocusState private var searchFocused: Bool
    @State private var showScopes = false
    @State private var showFilters = false
    @State private var snoozing: Ticket?
    @State private var assigning: Ticket?

    private var inbox: InboxStore { env.inbox }
    private var directory: DirectoryStore { env.directory }

    private var title: String {
        switch inbox.scope {
        case let .folder(folder): folder.title
        case let .view(id): directory.view(id)?.name ?? "View"
        }
    }

    private var subtitle: Text? {
        let tickets = inbox.tickets
        guard inbox.loadedOnce, !tickets.isEmpty else { return nil }
        let awaiting = tickets.filter(\.awaitingReply).count
        let urgent = tickets.filter { $0.priority == .urgent && !$0.status.isDone }.count
        var parts = [Format.plural(tickets.count, "ticket") + (inbox.nextCursor == nil ? "" : "+")]
        if awaiting > 0 { parts.append("\(awaiting) awaiting reply") }
        if urgent > 0 { parts.append("\(urgent) urgent") }
        return Text(parts.joined(separator: " · "))
    }

    private var waitingChats: [Ticket] {
        inbox.tickets.filter { ChatWait.of($0, agentOn: directory.ai.widgetAgentOn)?.state == .urgent && $0.assignee == nil }
    }

    var body: some View {
        VStack(spacing: 0) {
            GlassHeaderBar(
                title: title,
                subtitle: subtitle,
                query: $query,
                searchPrompt: "Search \(title.lowercased())",
                searchFocused: $searchFocused,
                menuLabel: "Folders and views",
                onMenu: { showScopes = true }
            ) {
                GlassCircleButton(symbol: "line.3.horizontal.decrease", label: "Filter and sort", badge: inbox.filters.count) {
                    showFilters = true
                }
            }
            list
        }
        .background(Theme.chrome.ignoresSafeArea())
        .toolbarVisibility(.hidden, for: .navigationBar)
        .onChange(of: query) { _, value in inbox.setSearch(value) }
        .onChange(of: inbox.tickets.map(\.id)) { _, ids in env.realtime.setWatched(ids) }
        .onAppear {
            env.router.visibleTicketID = nil
            env.realtime.setWatched(inbox.tickets.map(\.id))
        }
        .sheet(isPresented: $showScopes) {
            ScopeSheet()
                .presentationDetents([.medium, .large])
                .presentationCornerRadius(Theme.Radius.sheet)
        }
        .sheet(isPresented: $showFilters) {
            FilterSheet()
                .presentationDetents([.medium, .large])
                .presentationCornerRadius(Theme.Radius.sheet)
        }
        .sheet(item: $snoozing) { ticket in
            SnoozeSheet { date in
                Task { await apply(ticket, TicketPatch(snoozedUntil: .set(date)), done: "Snoozed #\(ticket.number)") }
            }
            .presentationDetents([.medium])
            .presentationCornerRadius(Theme.Radius.sheet)
        }
        .sheet(item: $assigning) { ticket in
            AssigneePicker(current: ticket.assignee?.id) { id in
                let change = TicketPatch(assigneeID: id.map { TicketPatch.Reference.set($0) } ?? .clear)
                Task { await apply(ticket, change, done: nil) }
            }
            .presentationDetents([.medium, .large])
            .presentationCornerRadius(Theme.Radius.sheet)
        }
    }

    @ViewBuilder
    private var list: some View {
        if !inbox.loadedOnce && inbox.loading {
            ScrollView { SkeletonRows() }
                .scrollDisabled(true)
                .background(Theme.surface)
                .clipShape(UnevenRoundedRectangle(topLeadingRadius: Theme.Radius.card, topTrailingRadius: Theme.Radius.card, style: .continuous))
        } else if let error = inbox.error, inbox.tickets.isEmpty {
            ErrorState(title: "Couldn't load tickets", message: error) { inbox.reload() }
                .frame(maxHeight: .infinity)
        } else {
            List {
                if !waitingChats.isEmpty, inbox.search.isEmpty {
                    WaitingBanner(tickets: waitingChats) { env.router.openTicket($0) }
                        .listRowInsets(EdgeInsets(top: 10, leading: 12, bottom: 4, trailing: 12))
                        .listRowSeparator(.hidden)
                        .listRowBackground(Theme.surface)
                }
                ForEach(inbox.tickets) { ticket in
                    Button {
                        env.router.openTicket(ticket)
                    } label: {
                        TicketRow(
                            ticket: ticket,
                            viewers: env.realtime.presence[ticket.id] ?? [],
                            agentOn: directory.ai.widgetAgentOn,
                            hideAssignee: inbox.scope == .folder(.mine),
                            replyingBot: env.realtime.live[ticket.id]?.bot
                        )
                    }
                    .buttonStyle(RowPressStyle())
                    .listRowInsets(EdgeInsets())
                    .listRowSeparator(.hidden)
                    .listRowBackground(Theme.surface)
                    .swipeActions(edge: .trailing, allowsFullSwipe: true) {
                        if env.session.canWrite { trailingActions(ticket) }
                    }
                    .swipeActions(edge: .leading, allowsFullSwipe: true) {
                        if env.session.canWrite { leadingActions(ticket) }
                    }
                    .contextMenu { if env.session.canWrite { menu(ticket) } }
                    .onAppear {
                        if ticket.id == inbox.tickets.last?.id { Task { await inbox.loadMore() } }
                    }
                }
                if inbox.loadingMore {
                    ProgressView().frame(maxWidth: .infinity).padding(.vertical, 18)
                        .listRowSeparator(.hidden)
                        .listRowBackground(Theme.surface)
                }
                if inbox.tickets.isEmpty {
                    emptyState
                        .listRowSeparator(.hidden)
                        .listRowBackground(Theme.surface)
                }
                Color.clear.frame(height: 90)
                    .listRowSeparator(.hidden)
                    .listRowBackground(Theme.surface)
            }
            .listStyle(.plain)
            .scrollContentBackground(.hidden)
            .background(Theme.surface)
            .clipShape(UnevenRoundedRectangle(topLeadingRadius: Theme.Radius.card, topTrailingRadius: Theme.Radius.card, style: .continuous))
            .refreshable {
                await inbox.refresh()
                await directory.refreshCounts()
            }
            .motion(Motion.snappy, value: inbox.tickets.map(\.id))
        }
    }

    @ViewBuilder
    private var emptyState: some View {
        if !inbox.search.isEmpty {
            EmptyState(symbol: "magnifyingglass", title: "No results for \u{201C}\(inbox.search)\u{201D}", message: "Check the spelling or try fewer words.")
        } else if !inbox.filters.isEmpty {
            EmptyState(symbol: "line.3.horizontal.decrease", title: "No tickets match", message: "Try removing a filter.", actionTitle: "Clear filters") {
                inbox.filters = InboxFilters()
            }
        } else {
            switch inbox.scope {
            case .folder(.unassigned):
                EmptyState(symbol: "tray", title: "Inbox zero", message: "Every conversation has an owner. New ones land here until someone picks them up.")
            case .folder(.mine):
                EmptyState(symbol: "checkmark.circle", title: "You're all caught up", message: "Nothing assigned to you needs an answer. Tickets you own show up here.", actionTitle: "Pick up from Unassigned") {
                    inbox.setScope(.folder(.unassigned))
                }
            case .folder(.open):
                EmptyState(symbol: "tray", title: "Inbox zero", message: "No open conversations. When customers write in, they show up here.")
            case .folder(.pending):
                EmptyState(symbol: "clock", title: "Nothing pending", message: "Pending means you replied and are waiting on the customer. Their next message moves the ticket back to open.")
            case .folder(.solved):
                EmptyState(symbol: "checkmark.circle", title: "No solved tickets yet", message: "Solved conversations stay here for reference. A reply from the customer reopens them.")
            case .folder(.spam):
                EmptyState(symbol: "xmark.octagon", title: "No spam", message: "Messages marked as spam are kept here, out of the way.")
            case .view:
                EmptyState(symbol: "square.stack", title: "Nothing matches this view", message: "No tickets match this view's filters right now.")
            }
        }
    }

    // MARK: Swipes

    @ViewBuilder
    private func trailingActions(_ ticket: Ticket) -> some View {
        if ticket.status.isDone {
            Button {
                Task { await apply(ticket, TicketPatch(status: .open), done: "Reopened #\(ticket.number)") }
            } label: { Label("Reopen", systemImage: "arrow.uturn.backward") }
                .tint(Tone.blue.solid)
        } else {
            Button {
                Task { await solve(ticket) }
            } label: { Label("Solve", systemImage: "checkmark") }
                .tint(Tone.green.solid)
        }
        Button {
            snoozing = ticket
        } label: { Label("Snooze", systemImage: "alarm") }
            .tint(Tone.violet.solid)
    }

    @ViewBuilder
    private func leadingActions(_ ticket: Ticket) -> some View {
        if ticket.assignee?.id != env.session.user?.id, let me = env.session.user {
            Button {
                Task { await apply(ticket, TicketPatch(assigneeID: .set(me.id)), done: "Assigned #\(ticket.number) to you") }
            } label: { Label("Assign to me", systemImage: "person.crop.circle.badge.plus") }
                .tint(Theme.inverse)
        } else {
            Button {
                assigning = ticket
            } label: { Label("Assign", systemImage: "person.2") }
                .tint(Theme.fg3)
        }
    }

    @ViewBuilder
    private func menu(_ ticket: Ticket) -> some View {
        if let me = env.session.user, ticket.assignee?.id != me.id {
            Button { Task { await apply(ticket, TicketPatch(assigneeID: .set(me.id)), done: nil) } } label: { Label("Assign to me", systemImage: "person.crop.circle.badge.plus") }
        }
        Button { assigning = ticket } label: { Label("Assign to\u{2026}", systemImage: "person.2") }
        Menu {
            ForEach(TicketStatus.allCases) { status in
                Button(status.title) { Task { await apply(ticket, TicketPatch(status: status), done: nil) } }
            }
        } label: { Label("Status", systemImage: "circle.dashed") }
        Menu {
            ForEach(Priority.allCases) { priority in
                Button(priority.title) { Task { await apply(ticket, TicketPatch(priority: priority), done: nil) } }
            }
        } label: { Label("Priority", systemImage: "chart.bar") }
        Button { snoozing = ticket } label: { Label("Snooze", systemImage: "alarm") }
        Divider()
        Button(role: ticket.spam ? nil : .destructive) {
            Task { await apply(ticket, TicketPatch(spam: !ticket.spam), done: ticket.spam ? "Moved out of spam" : "Marked as spam") }
        } label: { Label(ticket.spam ? "Not spam" : "Mark as spam", systemImage: "xmark.octagon") }
    }

    private func solve(_ ticket: Ticket) async {
        let previous = ticket.status
        do {
            _ = try await inbox.patch(ticket, TicketPatch(status: .solved), members: directory.members, meRef: env.session.userRef)
            Haptics.success()
            env.toasts.show(Toast(message: "Solved #\(ticket.number)", kind: .success, actionTitle: "Undo") {
                Task { await apply(ticket, TicketPatch(status: previous), done: nil) }
            })
        } catch {
            env.toasts.error(error)
        }
    }

    private func apply(_ ticket: Ticket, _ change: TicketPatch, done: String?) async {
        do {
            _ = try await inbox.patch(ticket, change, members: directory.members, meRef: env.session.userRef)
            Haptics.tap()
            if let done { env.toasts.show(done, kind: .success) }
        } catch {
            env.toasts.error(error)
        }
    }
}

/// "2 customers waiting in live chat": a red line above the list while someone waits for a person.
struct WaitingBanner: View {
    let tickets: [Ticket]
    let open: (Ticket) -> Void

    var body: some View {
        let longest = tickets.min { ($0.waitingSince ?? AppClock.now) < ($1.waitingSince ?? AppClock.now) }
        Button {
            if let longest { open(longest) }
        } label: {
            HStack(spacing: 10) {
                PingDot(color: Tone.red.solid, size: 7)
                Text(tickets.count == 1 ? "1 customer waiting in live chat" : "\(tickets.count) customers waiting in live chat")
                    .font(ODFont.sans(13.5, weight: .medium))
                    .foregroundStyle(Tone.red.text)
                Spacer()
                Text("Reply now").font(ODFont.sans(13, weight: .semibold)).foregroundStyle(Tone.red.text)
                Image(systemName: "arrow.right").font(.system(size: 11, weight: .bold)).foregroundStyle(Tone.red.text)
            }
            .padding(.horizontal, 12)
            .frame(height: 40)
            .background(Tone.red.soft, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous).strokeBorder(Tone.red.line, lineWidth: 1))
        }
        .buttonStyle(TapScaleStyle())
    }
}
