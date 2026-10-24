import SwiftUI

/// Folders with live counts, the workspace's views, then the app's other areas.
struct ScopeSheet: View {
    @Environment(AppEnvironment.self) private var env
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 18) {
                    if let workspace = env.session.workspace?.workspace {
                        HStack(spacing: 10) {
                            WorkspaceAvatar(workspace: workspace, size: 28)
                            Text(workspace.name).font(ODFont.sans(16, weight: .semibold)).foregroundStyle(Theme.fg)
                            Spacer()
                            RealtimeDot(status: env.realtime.status)
                        }
                    }
                    Panel {
                        ForEach(Array(Folder.allCases.enumerated()), id: \.element) { index, folder in
                            if index > 0 { Hairline(inset: 46) }
                            row(scope: .folder(folder), title: folder.title, symbol: folder.symbol, count: folder.count(in: env.directory.counts))
                        }
                    }
                    if !env.directory.views.isEmpty {
                        SectionLabel("Views")
                        Panel {
                            ForEach(Array(env.directory.views.enumerated()), id: \.element.id) { index, view in
                                if index > 0 { Hairline(inset: 46) }
                                row(scope: .view(view.id), title: view.name, symbol: view.shared == true ? "square.stack" : "person.crop.square", count: env.directory.counts?.views[view.id])
                            }
                        }
                    }
                    SectionLabel("Workspace")
                    Panel {
                        ForEach(Array(areas.enumerated()), id: \.element) { index, area in
                            if index > 0 { Hairline(inset: 46) }
                            areaRow(area)
                        }
                    }
                }
                .padding(20)
            }
            .background(Theme.chrome.ignoresSafeArea())
            .navigationTitle("Inbox")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) { Button("Done") { dismiss() } }
            }
        }
    }

    private var areas: [AppArea] {
        AppArea.allCases.filter { $0 != .tickets && ($0 != .mail || env.directory.collab.enabled) }
    }

    private func areaRow(_ area: AppArea) -> some View {
        let selected = env.router.area == area
        return Button {
            Haptics.selection()
            env.router.show(area, [])
            dismiss()
        } label: {
            label(title: area.title, symbol: area.symbol, count: nil, selected: selected)
        }
        .buttonStyle(RowPressStyle())
        .accessibilityAddTraits(selected ? .isSelected : [])
    }

    private func row(scope: InboxScope, title: String, symbol: String, count: Int?) -> some View {
        let selected = env.router.area == .tickets && env.inbox.scope == scope
        return Button {
            Haptics.selection()
            env.inbox.setScope(scope)
            env.router.show(.tickets, [])
            dismiss()
        } label: {
            label(title: title, symbol: symbol, count: count, selected: selected)
        }
        .buttonStyle(RowPressStyle())
        .accessibilityAddTraits(selected ? .isSelected : [])
    }

    private func label(title: String, symbol: String, count: Int?, selected: Bool) -> some View {
        HStack(spacing: 12) {
            Image(systemName: symbol)
                .font(.system(size: 15))
                .foregroundStyle(selected ? Theme.fg : Theme.fg3)
                .frame(width: 20)
            Text(title)
                .font(ODFont.sans(15, weight: selected ? .semibold : .regular))
                .foregroundStyle(Theme.fg)
            Spacer()
            if let count, count > 0 {
                Text("\(count)")
                    .font(ODFont.mono(12.5))
                    .monospacedDigit()
                    .foregroundStyle(Theme.fg3)
                    .contentTransition(.numericText())
            }
            if selected {
                Image(systemName: "checkmark").font(.system(size: 13, weight: .semibold)).foregroundStyle(Theme.fg)
            }
        }
        .padding(.horizontal, 14)
        .frame(height: 46)
        .background(selected ? Theme.hover : .clear)
        .contentShape(Rectangle())
    }
}

/// The socket's state, as a small dot (green live, amber reconnecting).
struct RealtimeDot: View {
    let status: RealtimeService.Status

    var body: some View {
        let (color, label): (Color, String) = switch status {
        case .live: (Tone.green.solid, "Live")
        case .connecting, .reconnecting: (Tone.amber.solid, "Connecting")
        case .offline: (Tone.red.solid, "Offline")
        case .idle: (Theme.fg4, "Idle")
        }
        HStack(spacing: 5) {
            Circle().fill(color).frame(width: 7, height: 7)
            Text(label).font(ODFont.sans(12, weight: .medium)).foregroundStyle(Theme.fg3)
        }
        .accessibilityElement(children: .combine)
    }
}

/// Sort, priority, channel, team and label.
struct FilterSheet: View {
    @Environment(AppEnvironment.self) private var env
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        @Bindable var inbox = env.inbox
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 18) {
                    SectionLabel("Sort by")
                    Panel {
                        ForEach(Array(TicketSort.allCases.enumerated()), id: \.element) { index, sort in
                            if index > 0 { Hairline(inset: 14) }
                            choiceRow(sort.title, selected: inbox.sort == sort) { inbox.sort = sort }
                        }
                    }

                    SectionLabel("Priority")
                    FlowChips(items: Priority.allCases.map { ($0.rawValue, $0.title) }, selected: Set(inbox.filters.priorities.map(\.rawValue))) { raw in
                        guard let priority = Priority(rawValue: raw) else { return }
                        if inbox.filters.priorities.contains(priority) {
                            inbox.filters.priorities.remove(priority)
                        } else {
                            inbox.filters.priorities.insert(priority)
                        }
                    }

                    SectionLabel("Channel")
                    FlowChips(items: Channel.allCases.map { ($0.rawValue, $0.title) }, selected: Set([inbox.filters.channel?.rawValue].compactMap { $0 })) { raw in
                        let channel = Channel(rawValue: raw)
                        inbox.filters.channel = inbox.filters.channel == channel ? nil : channel
                    }

                    if !env.directory.teams.isEmpty {
                        SectionLabel("Team")
                        FlowChips(items: env.directory.teams.map { ($0.id, $0.name) }, selected: Set([inbox.filters.teamID].compactMap { $0 })) { id in
                            inbox.filters.teamID = inbox.filters.teamID == id ? nil : id
                        }
                    }

                    if !env.directory.tags.isEmpty {
                        SectionLabel("Label")
                        FlowChips(items: env.directory.tags.map { ($0.name, $0.name) }, selected: Set([inbox.filters.tag].compactMap { $0 })) { name in
                            inbox.filters.tag = inbox.filters.tag == name ? nil : name
                        }
                    }
                }
                .padding(20)
            }
            .background(Theme.chrome.ignoresSafeArea())
            .navigationTitle("Filter and sort")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Clear") { inbox.filters = InboxFilters() }
                        .disabled(inbox.filters.isEmpty)
                }
                ToolbarItem(placement: .confirmationAction) { Button("Done") { dismiss() } }
            }
        }
    }

    private func choiceRow(_ title: String, selected: Bool, action: @escaping () -> Void) -> some View {
        Button {
            Haptics.selection()
            action()
        } label: {
            HStack {
                Text(title).font(ODFont.sans(15)).foregroundStyle(Theme.fg)
                Spacer()
                if selected { Image(systemName: "checkmark").font(.system(size: 13, weight: .semibold)).foregroundStyle(Theme.fg) }
            }
            .padding(.horizontal, 14)
            .frame(height: 44)
            .contentShape(Rectangle())
        }
        .buttonStyle(RowPressStyle())
    }
}

/// Toggle chips that wrap.
struct FlowChips: View {
    let items: [(String, String)]
    let selected: Set<String>
    let toggle: (String) -> Void

    var body: some View {
        FlowLayout(spacing: 7) {
            ForEach(items, id: \.0) { item in
                let on = selected.contains(item.0)
                Button {
                    Haptics.selection()
                    toggle(item.0)
                } label: {
                    Text(item.1)
                        .font(ODFont.sans(13.5, weight: .medium))
                        .foregroundStyle(on ? Theme.inverseFg : Theme.fg)
                        .padding(.horizontal, 12)
                        .frame(height: 32)
                        .background(on ? Theme.inverse : Theme.surface, in: RoundedRectangle(cornerRadius: 8, style: .continuous))
                        .overlay(RoundedRectangle(cornerRadius: 8, style: .continuous).strokeBorder(on ? .clear : Theme.line, lineWidth: 1))
                }
                .buttonStyle(TapScaleStyle())
                .accessibilityAddTraits(on ? .isSelected : [])
            }
        }
    }
}

/// A simple wrapping layout.
struct FlowLayout: Layout {
    var spacing: CGFloat = 6

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let width = proposal.width ?? .infinity
        var x: CGFloat = 0, y: CGFloat = 0, row: CGFloat = 0, widest: CGFloat = 0
        for view in subviews {
            let size = view.sizeThatFits(.unspecified)
            if x > 0, x + size.width > width {
                y += row + spacing
                x = 0
                row = 0
            }
            x += size.width + spacing
            row = max(row, size.height)
            widest = max(widest, x - spacing)
        }
        return CGSize(width: proposal.width ?? widest, height: y + row)
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        var x = bounds.minX, y = bounds.minY, row: CGFloat = 0
        for view in subviews {
            let size = view.sizeThatFits(.unspecified)
            if x > bounds.minX, x + size.width > bounds.maxX {
                y += row + spacing
                x = bounds.minX
                row = 0
            }
            view.place(at: CGPoint(x: x, y: y), proposal: ProposedViewSize(size))
            x += size.width + spacing
            row = max(row, size.height)
        }
    }
}

/// Snooze presets, or a date and time.
struct SnoozeSheet: View {
    @Environment(\.dismiss) private var dismiss
    let onPick: (Date) -> Void
    @State private var custom = Calendar.current.date(byAdding: .day, value: 1, to: AppClock.now) ?? AppClock.now
    @State private var showCustom = false

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 14) {
                    Panel {
                        ForEach(Array(SnoozePreset.allCases.enumerated()), id: \.element) { index, preset in
                            if index > 0 { Hairline(inset: 14) }
                            Button {
                                pick(preset.date())
                            } label: {
                                HStack {
                                    Text(preset.title).font(ODFont.sans(15)).foregroundStyle(Theme.fg)
                                    Spacer()
                                    Text(preset.subtitle()).font(ODFont.sans(13.5)).foregroundStyle(Theme.fg3)
                                }
                                .padding(.horizontal, 14)
                                .frame(height: 46)
                                .contentShape(Rectangle())
                            }
                            .buttonStyle(RowPressStyle())
                        }
                    }
                    Panel(padding: 14) {
                        DatePicker("Pick a date", selection: $custom, in: AppClock.now..., displayedComponents: [.date, .hourAndMinute])
                            .font(ODFont.sans(15))
                        Button("Snooze until then") { pick(custom) }
                            .buttonStyle(InkButtonStyle(height: 40))
                            .padding(.top, 10)
                    }
                    Text("The ticket goes on hold and comes back when the time comes or the customer replies.")
                        .font(ODFont.footnote)
                        .foregroundStyle(Theme.fg3)
                }
                .padding(20)
            }
            .background(Theme.chrome.ignoresSafeArea())
            .navigationTitle("Snooze until")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } }
            }
        }
    }

    private func pick(_ date: Date) {
        Haptics.tap()
        onPick(date)
        dismiss()
    }
}
