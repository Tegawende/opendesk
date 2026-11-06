import SwiftUI

/// Settings > AI agent > Procedures: steps Ollie follows for one kind of conversation. Ollie reads
/// a procedure only when a conversation matches its "Use this when" line.
struct ProceduresSettings: View {
    static let max = 50

    @Environment(AppEnvironment.self) private var env
    @State private var confirm: AccountConfirm?

    var body: some View {
        let store = env.agent
        let list = store.procedures.value
        AccountPage(title: "Procedures", spacing: 20) {
            AgentIntro(text: "Steps Ollie follows for one kind of conversation. Ollie reads a procedure only when a conversation matches its Use this when line.")
            if !store.procedures.loaded {
                Panel { SkeletonRows(count: 3) }
            } else if let error = store.procedures.error, list.isEmpty {
                Panel { ErrorState(message: error) { Task { await store.loadProcedures() } } }
            } else if list.isEmpty {
                Panel {
                    VStack(spacing: 10) {
                        EmptyState(symbol: "list.number", title: "No procedures yet", message: "Write the steps Ollie should follow for one kind of conversation, such as refund requests or bug reports.", actionTitle: "New procedure") {
                            env.router.openSetting(.procedures, id: "new")
                        }
                        VStack(alignment: .leading, spacing: 6) {
                            AgentTip(text: "Ollie reads one only when a conversation matches it")
                            AgentTip(text: "Copy the steps from a wiki page")
                        }
                        .padding(.bottom, 20)
                    }
                }
            } else {
                AccountSection(title: "\(list.count) of \(Self.max)", footer: list.count >= Self.max ? "A workspace can have up to 50 procedures." : nil) {
                    ForEach(Array(list.enumerated()), id: \.element.id) { index, procedure in
                        if index > 0 { AccountDivider(tiles: false) }
                        ProcedureRow(procedure: procedure)
                            .contextMenu {
                                Button("Edit", systemImage: "pencil") { env.router.openSetting(.procedures, id: procedure.id) }
                                Button("Delete", systemImage: "trash", role: .destructive) { confirm = ProcedureActions.delete(procedure, env: env) }
                            }
                    }
                }
                .motion(Motion.snappy, value: list.map(\.id))
            }
        }
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button { env.router.openSetting(.procedures, id: "new") } label: { Image(systemName: "plus") }
                    .disabled(list.count >= Self.max)
                    .accessibilityLabel("New procedure")
            }
        }
        .accountConfirm($confirm)
        .refreshable { await store.loadProcedures() }
        .task {
            await store.loadProcedures()
            if !store.bots.loaded { await store.loadBots() }
        }
    }
}

private struct ProcedureRow: View {
    let procedure: AgentProcedure

    @Environment(AppEnvironment.self) private var env

    var body: some View {
        let store = env.agent
        HStack(spacing: 10) {
            SettingsDetailLink(page: .procedures, id: procedure.id) {
                HStack {
                    VStack(alignment: .leading, spacing: 4) {
                        Text(procedure.name).font(ODFont.sans(15.5, weight: .medium)).foregroundStyle(procedure.enabled ? Theme.fg : Theme.fg3).lineLimit(1)
                        Text(procedure.when).font(ODFont.sans(13)).foregroundStyle(Theme.fg2).lineLimit(2)
                        HStack(spacing: 6) {
                            TonePill(text: procedure.channelsText, tone: procedure.channels.isEmpty ? .gray : .blue, height: 18)
                            Text(botsText(store)).font(ODFont.sans(12)).foregroundStyle(Theme.fg4).lineLimit(1)
                            if let updated = procedure.updatedAt {
                                Text("· \(Format.relShort(updated))\(procedure.updatedBy.map { " by \($0)" } ?? "")").font(ODFont.sans(12)).foregroundStyle(Theme.fg4).lineLimit(1)
                            }
                        }
                    }
                    Spacer(minLength: 4)
                }
                .padding(.leading, 14)
                .padding(.vertical, 11)
                .contentShape(Rectangle())
            }
            Toggle("Use \(procedure.name)", isOn: Binding(get: { procedure.enabled }, set: { on in Task { await ProcedureActions.setEnabled(procedure, on, env: env) } }))
                .labelsHidden()
                .tint(Tone.green.solid)
                .padding(.trailing, 14)
        }
    }

    private func botsText(_ store: AgentStore) -> String {
        guard !procedure.botIDs.isEmpty else { return "Every bot" }
        let names = procedure.botIDs.compactMap { id in store.bots.value.first { $0.id == id }?.name }
        return names.isEmpty ? Format.plural(procedure.botIDs.count, "bot") : names.joined(separator: ", ")
    }
}

@MainActor
enum ProcedureActions {
    static func setEnabled(_ procedure: AgentProcedure, _ on: Bool, env: AppEnvironment) async {
        let store = env.agent
        guard let api = store.api else { return }
        var optimistic = procedure
        optimistic.enabled = on
        store.upsert(optimistic)
        do {
            let saved: AgentProcedure = try await api.patch("v1/ai/procedures/\(procedure.id)", body: JSONValue.object(["enabled": .bool(on)]))
            store.upsert(saved)
        } catch {
            store.upsert(procedure)
            env.toasts.error(error)
        }
    }

    static func delete(_ procedure: AgentProcedure, env: AppEnvironment, after: @escaping @MainActor () -> Void = {}) -> AccountConfirm {
        AccountConfirm(title: "Delete this procedure?", message: "Ollie stops following \(procedure.name), and tests that expect it will fail.", confirmTitle: "Delete") {
            guard let api = env.agent.api else { return }
            do {
                try await api.delete("v1/ai/procedures/\(procedure.id)")
                env.agent.procedures.value.removeAll { $0.id == procedure.id }
                Haptics.success()
                after()
            } catch {
                env.toasts.error(error)
            }
        }
    }
}

// MARK: - Editor

extension ProceduresSettings {
    /// A procedure's editor; "new" starts one.
    struct Detail: View {
        let id: String

        @Environment(AppEnvironment.self) private var env
        @State private var name = ""
        @State private var when = ""
        @State private var instructions = ""
        @State private var enabled = true
        @State private var channels: Set<String> = ["chat", "email"]
        @State private var botIDs: Set<String> = []
        @State private var base: Snapshot?
        @State private var tried = false
        @State private var saving = false
        @State private var copying = false
        @State private var confirm: AccountConfirm?

        private struct Snapshot: Equatable {
            var name: String
            var when: String
            var instructions: String
            var enabled: Bool
            var channels: Set<String>
            var botIDs: Set<String>
        }

        private var isNew: Bool { id == "new" }
        private var current: Snapshot { Snapshot(name: name, when: when, instructions: instructions, enabled: enabled, channels: channels, botIDs: botIDs) }
        private var dirty: Bool { base.map { $0 != current } ?? false }

        private var nameProblem: String? {
            let count = AgentText.trimmed(name).count
            return count < 1 || count > 80 ? "Give it a name of up to 80 characters." : nil
        }
        private var whenProblem: String? {
            let count = AgentText.trimmed(when).count
            return count < 3 || count > 300 ? "Describe it in 3 to 300 characters." : nil
        }
        private var instructionsProblem: String? {
            let count = AgentText.trimmed(instructions).count
            return count < 1 || count > 8000 ? "Write 1 to 8,000 characters." : nil
        }

        var body: some View {
            let store = env.agent
            let procedure = store.procedures.value.first { $0.id == id }
            AccountPage(title: isNew ? "New procedure" : (procedure?.name ?? "Procedure"), spacing: 22) {
                if !isNew, procedure == nil, store.procedures.loaded {
                    EmptyState(symbol: "questionmark.circle", title: "This procedure isn't here any more", message: "It may have been deleted.")
                        .padding(.top, 40)
                } else {
                    AgentField(label: "Name", text: $name, prompt: "Refund requests", max: 80, problem: tried ? nameProblem : nil)
                    AgentField(label: "Use this when", text: $when, prompt: "The customer asks for a refund or says they were charged twice", hint: "How Ollie recognises a conversation this is for. Until it picks the procedure, this line is all it sees.", max: 300, multiline: true, problem: tried ? whenProblem : nil)
                    VStack(alignment: .leading, spacing: 8) {
                        AgentField(label: "Instructions", text: $instructions, prompt: "1. Ask for the order number.\n2. Look the order up.\n3. Hand off refunds over $200.", hint: "Markdown. What to ask, what to check, what to say, and when to hand off.", max: 8000, multiline: true, mono: true, problem: tried ? instructionsProblem : nil)
                        Button { copying = true } label: { Label("Copy from a wiki page", systemImage: "doc.on.doc") }
                            .buttonStyle(ChipButtonStyle())
                    }
                    channelsSection
                    botsSection(store)
                    Panel {
                        AccountToggleRow(symbol: "power", tone: .green, title: "On", isOn: $enabled)
                    }
                    if !isNew, let procedure {
                        Button("Delete", role: .destructive) {
                            confirm = ProcedureActions.delete(procedure, env: env) { env.router.accountPath.removeLast() }
                        }
                        .buttonStyle(OutlineButtonStyle(tint: Tone.red.text))
                    }
                }
            }
            .settingsSaveBar(dirty: isNew || dirty, guarded: isNew ? !AgentText.trimmed(name + when + instructions).isEmpty : dirty, saveTitle: isNew ? "Create procedure" : "Save changes", saving: saving) {
                Task { await save() }
            } discard: {
                discard()
            }
            .sheet(isPresented: $copying) {
                WikiCopySheet { title, markdown in
                    instructions = String(markdown.prefix(8000))
                    if AgentText.trimmed(name).isEmpty { name = String(title.prefix(80)) }
                    env.toasts.show("Copied from \(title)", kind: .success)
                }
                .presentationDetents([.medium, .large])
                .presentationCornerRadius(Theme.Radius.sheet)
            }
            .accountConfirm($confirm)
            .task {
                if !store.procedures.loaded { await store.loadProcedures() }
                if !store.bots.loaded { await store.loadBots() }
                guard base == nil else { return }
                if let procedure = store.procedures.value.first(where: { $0.id == id }) { fill(procedure) }
                base = current
            }
        }

        private var channelsSection: some View {
            VStack(alignment: .leading, spacing: 8) {
                AgentGroupTitle(title: "Channels", hint: "Where Ollie may pick it by itself. With neither, only automations run it.")
                Panel {
                    AccountToggleRow(symbol: "bubble.left.and.bubble.right", tone: .violet, title: "Chat", isOn: member(of: $channels, "chat"))
                    AccountDivider()
                    AccountToggleRow(symbol: "envelope", tone: .blue, title: "Email", isOn: member(of: $channels, "email"))
                }
            }
        }

        private func botsSection(_ store: AgentStore) -> some View {
            VStack(alignment: .leading, spacing: 8) {
                AgentGroupTitle(title: "Bots", hint: "Only conversations these bots answer. Leave it empty for every bot.")
                Panel {
                    AccountChoiceRow(symbol: "person.3", tone: .violet, title: "Every bot", chosen: botIDs.isEmpty) { botIDs = [] }
                    ForEach(store.bots.value) { bot in
                        AccountDivider()
                        Button {
                            Haptics.selection()
                            if botIDs.contains(bot.id) { botIDs.remove(bot.id) } else { botIDs.insert(bot.id) }
                        } label: {
                            HStack(spacing: 12) {
                                BotFace(look: bot.look, image: bot.imageURL, size: 30)
                                Text(bot.name).font(ODFont.sans(15.5)).foregroundStyle(Theme.fg)
                                Spacer()
                                Image(systemName: botIDs.contains(bot.id) ? "checkmark.circle.fill" : "circle")
                                    .font(.system(size: 18))
                                    .foregroundStyle(botIDs.contains(bot.id) ? Theme.fg : Theme.fg4)
                            }
                            .padding(.horizontal, 14)
                            .padding(.vertical, 10)
                            .contentShape(Rectangle())
                        }
                        .buttonStyle(RowPressStyle())
                        .accessibilityAddTraits(botIDs.contains(bot.id) ? .isSelected : [])
                    }
                }
            }
        }

        private func member(of set: Binding<Set<String>>, _ value: String) -> Binding<Bool> {
            Binding(get: { set.wrappedValue.contains(value) }, set: { on in
                if on { set.wrappedValue.insert(value) } else { set.wrappedValue.remove(value) }
            })
        }

        private func fill(_ procedure: AgentProcedure) {
            name = procedure.name
            when = procedure.when
            instructions = procedure.instructions
            enabled = procedure.enabled
            channels = Set(procedure.channels)
            botIDs = Set(procedure.botIDs)
        }

        private func discard() {
            if isNew {
                env.router.accountPath.removeLast()
                return
            }
            guard let base else { return }
            withAnimation(Motion.snappy) {
                name = base.name
                when = base.when
                instructions = base.instructions
                enabled = base.enabled
                channels = base.channels
                botIDs = base.botIDs
            }
        }

        private func save() async {
            tried = true
            guard nameProblem == nil, whenProblem == nil, instructionsProblem == nil, let api = env.agent.api else {
                Haptics.error()
                return
            }
            saving = true
            defer { saving = false }
            let body = JSONValue.object([
                "name": .string(AgentText.trimmed(name)),
                "when": .string(AgentText.trimmed(when)),
                "instructions": .string(AgentText.trimmed(instructions)),
                "enabled": .bool(enabled),
                "channels": .array(["chat", "email"].filter(channels.contains).map(JSONValue.string)),
                "bot_ids": .array(botIDs.sorted().map(JSONValue.string)),
            ])
            do {
                if isNew {
                    let saved: AgentProcedure = try await api.post("v1/ai/procedures", body: body)
                    env.agent.upsert(saved)
                    Haptics.success()
                    env.toasts.show("Procedure created", kind: .success)
                    env.router.accountPath.removeLast()
                } else {
                    let saved: AgentProcedure = try await api.patch("v1/ai/procedures/\(id)", body: body)
                    env.agent.upsert(saved)
                    fill(saved)
                    base = current
                    Haptics.success()
                    env.toasts.show("Procedure saved", kind: .success)
                }
            } catch {
                env.toasts.error(error)
            }
        }
    }
}

/// Picks a wiki page and copies its text once, as Markdown. Nothing stays linked to the page.
private struct WikiCopySheet: View {
    let copy: (_ title: String, _ markdown: String) -> Void

    @Environment(AppEnvironment.self) private var env
    @Environment(\.dismiss) private var dismiss
    @State private var query = ""
    @State private var pages: [WikiNode] = []
    @State private var hits: [WikiSearchHit] = []
    @State private var loaded = false
    @State private var busy: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            AccountSheetHeader(title: "Copy from a wiki page", subtitle: "The text is copied once. Nothing stays linked to the page.") { dismiss() }
            HStack(spacing: 8) {
                Image(systemName: "magnifyingglass").font(.system(size: 13)).foregroundStyle(Theme.fg3)
                TextField("Search the wiki", text: $query).font(ODFont.sans(15)).autocorrectionDisabled()
            }
            .padding(.horizontal, 12)
            .frame(height: 40)
            .background(Theme.sunken, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
            ScrollView {
                Panel {
                    let rows: [(id: String, title: String, detail: String?)] = query.isEmpty
                        ? pages.map { ($0.id, $0.title, nil) }
                        : hits.map { ($0.id, $0.title, $0.path.map(\.title).joined(separator: " / ")) }
                    if !loaded {
                        SkeletonRows(count: 4)
                    } else if rows.isEmpty {
                        Text("No pages match").font(ODFont.footnote).foregroundStyle(Theme.fg3).padding(14)
                    }
                    ForEach(Array(rows.enumerated()), id: \.element.id) { index, row in
                        if index > 0 { AccountDivider() }
                        Button { Task { await pick(row.id) } } label: {
                            AccountRow(symbol: "doc.text", tone: .gray, title: row.title.isEmpty ? "Untitled" : row.title, subtitle: row.detail) {
                                if busy == row.id { ProgressView() }
                            }
                        }
                        .buttonStyle(RowPressStyle())
                        .disabled(busy != nil)
                    }
                }
                .padding(.bottom, 24)
            }
            .scrollDismissesKeyboard(.interactively)
        }
        .padding(.horizontal, 20)
        .padding(.top, 22)
        .background(Theme.chrome)
        .task {
            guard let api = env.agent.api else { return }
            pages = (try? await api.all("v1/wiki/pages") as [WikiNode]) ?? []
            loaded = true
        }
        .task(id: query) {
            guard !query.isEmpty, let api = env.agent.api else { return }
            try? await Task.sleep(for: .milliseconds(200))
            guard !Task.isCancelled else { return }
            let page: Page<WikiSearchHit>? = try? await api.get("v1/wiki/search", query: [URLQueryItem(name: "q", value: query), URLQueryItem(name: "limit", value: "30")])
            hits = page?.data ?? []
        }
    }

    private func pick(_ id: String) async {
        guard let api = env.agent.api else { return }
        busy = id
        defer { busy = nil }
        do {
            let page: WikiPage = try await api.get("v1/wiki/pages/\(id)")
            copy(page.title.isEmpty ? "Untitled" : page.title, KBMarkdown.markdown(from: page.bodyHTML) ?? HTMLText.plain(from: page.bodyHTML))
            dismiss()
        } catch {
            env.toasts.show("Couldn't copy the page.", kind: .error)
        }
    }
}
