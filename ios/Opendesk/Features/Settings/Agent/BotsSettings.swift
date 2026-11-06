import SwiftUI

/// Settings > AI agent > Bots: the bots customers talk to in chat and email. The default answers
/// wherever no other bot is picked.
struct BotsSettings: View {
    @Environment(AppEnvironment.self) private var env
    @State private var creating = false
    @State private var confirm: AccountConfirm?

    private static let freshNames = ["Pip", "Nova", "Juno", "Milo", "Iris", "Echo", "Kit", "Remy", "Skye", "Otto", "Wren", "Bea"]

    var body: some View {
        let store = env.agent
        AccountPage(title: "Bots", spacing: 20) {
            AgentIntro(text: "The bots your customers talk to in chat and email. Give each one a name, a face and a voice, then pick which widget or address it answers on.")
            if !store.bots.loaded {
                Panel { SkeletonRows(count: 3) }
            } else if let error = store.bots.error, store.bots.value.isEmpty {
                Panel { ErrorState(message: error) { Task { await store.loadBots() } } }
            } else {
                AccountSection(title: "Your bots", footer: "The default answers wherever no other bot is picked. Touch and hold a bot to reorder, duplicate or delete it.") {
                    ForEach(Array(store.bots.value.enumerated()), id: \.element.id) { index, bot in
                        if index > 0 { AccountDivider(tiles: true) }
                        SettingsDetailLink(page: .bots, id: bot.id) {
                            BotRow(bot: bot)
                        }
                        .contextMenu { menu(bot, index: index, store) }
                    }
                }
                .motion(Motion.snappy, value: store.bots.value.map(\.id))
                Text("In the app, the AI that drafts, triages and assists your team is always Ollie. Bots are what customers see.")
                    .font(ODFont.footnote)
                    .foregroundStyle(Theme.fg3)
                    .padding(.horizontal, 4)
            }
        }
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button { Task { await create(store) } } label: {
                    if creating { ProgressView() } else { Image(systemName: "plus") }
                }
                .disabled(creating)
                .accessibilityLabel("New bot")
            }
        }
        .accountConfirm($confirm)
        .refreshable { await store.loadBots() }
        .task { await store.loadBots() }
    }

    @ViewBuilder
    private func menu(_ bot: AgentBot, index: Int, _ store: AgentStore) -> some View {
        if !bot.isDefault {
            Button("Make default", systemImage: "star") { Task { await BotActions.makeDefault(bot, env: env) } }
        }
        Button("Duplicate", systemImage: "plus.square.on.square") { Task { await BotActions.duplicate(bot, env: env) } }
        if index > 0 {
            Button("Move up", systemImage: "arrow.up") { Task { await move(bot, by: -1, store) } }
        }
        if index < store.bots.value.count - 1 {
            Button("Move down", systemImage: "arrow.down") { Task { await move(bot, by: 1, store) } }
        }
        Button("Delete", systemImage: "trash", role: .destructive) { confirm = BotActions.delete(bot, env: env) }
    }

    private func create(_ store: AgentStore) async {
        guard let api = store.api else { return }
        creating = true
        defer { creating = false }
        let taken = Set(store.bots.value.map { $0.name.lowercased() })
        let name = Self.freshNames.first { !taken.contains($0.lowercased()) } ?? "Bot \(store.bots.value.count + 1)"
        let look = BotLook.random(seed: BotLook.randomSeed())
        do {
            let bot: AgentBot = try await api.post("v1/bots", body: JSONValue.object(["name": .string(name), "look": look.json]))
            store.upsert(bot)
            Haptics.success()
            env.router.openSetting(.bots, id: bot.id)
        } catch {
            env.toasts.error(error)
        }
    }

    private func move(_ bot: AgentBot, by step: Int, _ store: AgentStore) async {
        guard let api = store.api, let from = store.bots.value.firstIndex(where: { $0.id == bot.id }) else { return }
        let to = from + step
        guard store.bots.value.indices.contains(to) else { return }
        let before = store.bots.value
        var ids = before.map(\.id)
        ids.swapAt(from, to)
        withAnimation(Motion.snappy) {
            store.bots.value.swapAt(from, to)
            for index in store.bots.value.indices { store.bots.value[index].position = index }
        }
        Haptics.selection()
        do {
            let page: Page<AgentBot> = try await api.post("v1/bots/reorder", body: JSONValue.object(["ids": .array(ids.map(JSONValue.string))]))
            if !page.data.isEmpty { store.bots.value = page.data.sorted { $0.position < $1.position } }
        } catch {
            store.bots.value = before
            env.toasts.error(error)
        }
    }
}

private struct BotRow: View {
    let bot: AgentBot

    var body: some View {
        HStack(spacing: 12) {
            BotFace(look: bot.look, image: bot.imageURL, size: 38)
            VStack(alignment: .leading, spacing: 3) {
                HStack(spacing: 6) {
                    Text(bot.name).font(ODFont.sans(15.5, weight: .medium)).foregroundStyle(Theme.fg).lineLimit(1)
                    if bot.isDefault { TonePill(text: "Default", tone: .violet, height: 18) }
                }
                if !bot.role.isEmpty {
                    Text(bot.role).font(ODFont.sans(13)).foregroundStyle(Theme.fg2).lineLimit(1)
                }
                Text(bot.usageLine).font(ODFont.footnote).foregroundStyle(Theme.fg3).lineLimit(2)
            }
            Spacer(minLength: 8)
            Chevron()
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 11)
        .contentShape(Rectangle())
    }
}

/// What the list and the builder both do to a bot.
@MainActor
enum BotActions {
    static func makeDefault(_ bot: AgentBot, env: AppEnvironment) async {
        guard let api = env.agent.api else { return }
        do {
            let saved: AgentBot = try await api.post("v1/bots/\(bot.id)/default")
            env.agent.upsert(saved)
            Haptics.success()
            env.toasts.show("\(bot.name) is now the default bot", kind: .success)
        } catch {
            env.toasts.error(error)
        }
    }

    @discardableResult
    static func duplicate(_ bot: AgentBot, env: AppEnvironment) async -> AgentBot? {
        guard let api = env.agent.api else { return nil }
        do {
            let copy: AgentBot = try await api.post("v1/bots/\(bot.id)/duplicate")
            env.agent.upsert(copy)
            Haptics.success()
            env.toasts.show("Made a copy: \(copy.name)", kind: .success)
            return copy
        } catch {
            env.toasts.error(error)
            return nil
        }
    }

    /// The confirmation to delete `bot`, or nil after saying why it can't go yet.
    static func delete(_ bot: AgentBot, env: AppEnvironment, after: @escaping @MainActor () -> Void = {}) -> AccountConfirm? {
        if bot.isDefault {
            env.toasts.show("Make another bot the default to delete this one", kind: .error)
            return nil
        }
        if !bot.usage.isEmpty {
            env.toasts.show(Toast(message: "Can't delete \(bot.name) yet. \(bot.usageLine). Pick another bot there first.", kind: .error), seconds: 5)
            return nil
        }
        return AccountConfirm(title: "Delete \(bot.name)?", message: "\(bot.name) and its look, personality and picture will be removed. Past messages keep its name.", confirmTitle: "Delete bot") {
            guard let api = env.agent.api else { return }
            do {
                try await api.delete("v1/bots/\(bot.id)")
                env.agent.bots.value.removeAll { $0.id == bot.id }
                Haptics.success()
                env.toasts.show("Deleted \(bot.name)", kind: .success)
                after()
            } catch {
                env.toasts.error(error)
            }
        }
    }
}

// MARK: - Builder

extension BotsSettings {
    /// One bot's builder: a live preview, its name and role, and Look, Personality and Test tabs.
    struct Detail: View {
        let id: String

        @Environment(AppEnvironment.self) private var env
        @State private var missing = false

        var body: some View {
            let store = env.agent
            Group {
                if let bot = store.bots.value.first(where: { $0.id == id }) {
                    BotBuilder(bot: bot)
                } else if missing {
                    AccountPage(title: "Bot not found") {
                        EmptyState(symbol: "questionmark.circle", title: "Bot not found", message: "It may have been deleted. Your other bots are still on the list.", actionTitle: "Bots") {
                            env.router.accountPath.removeLast()
                        }
                        .padding(.top, 60)
                    }
                } else {
                    AccountPage(title: "Bot") { Panel { SkeletonRows(count: 4) } }
                }
            }
            .task {
                if !store.bots.loaded { await store.loadBots() }
                if store.bots.value.contains(where: { $0.id == id }) { return }
                if let api = store.api, let bot: AgentBot = try? await api.get("v1/bots/\(id)") {
                    store.upsert(bot)
                } else {
                    missing = true
                }
            }
        }
    }
}

private struct BotDraft: Equatable {
    var name: String
    var role: String
    var language: String
    var look: BotLook
    var personality: BotPersonality

    init(_ bot: AgentBot) {
        name = bot.name
        role = bot.role
        language = bot.language
        look = bot.look
        personality = bot.personality
    }
}

private struct BotBuilder: View {
    let bot: AgentBot

    enum Tab: String, CaseIterable { case look, personality, test }

    @Environment(AppEnvironment.self) private var env
    @State private var draft: BotDraft
    @State private var base: BotDraft
    @State private var history: [BotLook] = []
    @State private var tab: Tab = .look
    @State private var mood: BotFace.Mood = .idle
    @State private var hopping = false
    @State private var saving = false
    @State private var uploading = false
    @State private var confirm: AccountConfirm?
    @State private var chat = AgentChat()

    init(bot: AgentBot) {
        self.bot = bot
        _draft = State(initialValue: BotDraft(bot))
        _base = State(initialValue: BotDraft(bot))
    }

    private var dirty: Bool { draft != base }
    private var nameProblem: String? {
        AgentText.trimmed(draft.name).isEmpty ? "Give the bot a name" : AgentText.problem(draft.name, max: BotLimits.name).map { _ in "Up to \(BotLimits.name) characters, plain text" }
    }
    private var roleProblem: String? { AgentText.problem(draft.role, max: BotLimits.role).map { _ in "Up to \(BotLimits.role) characters, plain text" } }
    private var needsPicture: Bool { draft.look.kind == "image" && bot.imageURL == nil }
    private var invalid: Bool { nameProblem != nil || roleProblem != nil || BotLimits.invalid(draft.personality) || needsPicture }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 22) {
                preview
                AccountSegmented(options: Tab.allCases, selection: $tab, label: { $0.rawValue.capitalized })
                switch tab {
                case .look:
                    BotLookEditor(look: lookBinding, name: draft.name, image: bot.imageURL, uploading: uploading, upload: { image in Task { await upload(image) } }, removeImage: { Task { await removeImage() } })
                case .personality:
                    BotPersonalityEditor(personality: $draft.personality, language: $draft.language)
                case .test:
                    testTab
                }
            }
            .padding(.horizontal, 16)
            .padding(.top, 8)
            .padding(.bottom, 140)
        }
        .scrollDismissesKeyboard(.interactively)
        .background(Theme.chrome.ignoresSafeArea())
        .accountTitle(bot.name)
        .navigationBarBackButtonHidden(dirty)
        .toolbar {
            if dirty {
                ToolbarItem(placement: .topBarLeading) {
                    Button {
                        confirm = AccountConfirm(title: "Discard your changes?", message: "Your edits to \(bot.name) aren't saved yet.", confirmTitle: "Discard") {
                            draft = base
                            env.router.accountPath.removeLast()
                        }
                    } label: {
                        Label("Bots", systemImage: "chevron.left").labelStyle(.titleAndIcon)
                    }
                }
            }
            ToolbarItem(placement: .topBarTrailing) {
                Menu {
                    if !bot.isDefault {
                        Button("Make default", systemImage: "star") { Task { await BotActions.makeDefault(bot, env: env) } }
                    }
                    Button("Duplicate", systemImage: "plus.square.on.square") {
                        Task {
                            if let copy = await BotActions.duplicate(bot, env: env) { env.router.openSetting(.bots, id: copy.id) }
                        }
                    }
                    Button("Delete", systemImage: "trash", role: .destructive) {
                        confirm = BotActions.delete(bot, env: env) { env.router.accountPath.removeLast() }
                    }
                } label: {
                    Image(systemName: "ellipsis.circle")
                }
                .accessibilityLabel("Actions for \(bot.name)")
            }
        }
        .settingsSaveBar(dirty: dirty, problem: needsPicture ? "Upload a picture or pick another look" : nil, saving: saving, canSave: !invalid) {
            Task { await save() }
        } discard: {
            withAnimation(Motion.snappy) { draft = base }
        }
        .accountConfirm($confirm)
        .onChange(of: BotDraft(bot)) { _, next in
            // Follow the server (realtime, another device) until the first edit.
            if draft == base { draft = next }
            base = next
        }
    }

    private var lookBinding: Binding<BotLook> {
        Binding(get: { draft.look }, set: { setLook($0) })
    }

    private func setLook(_ look: BotLook, remember: Bool = true) {
        guard look != draft.look else { return }
        if remember { history = Array((history + [draft.look]).suffix(20)) }
        draft.look = look
        hop()
    }

    private func hop() {
        guard mood == .idle else { return }
        hopping = true
        Task {
            try? await Task.sleep(for: .milliseconds(700))
            hopping = false
        }
    }

    private var preview: some View {
        VStack(spacing: 16) {
            BotFace(look: draft.look, image: bot.imageURL, size: 104, mood: mood == .idle && hopping ? .happy : mood, alive: true)
                .padding(.top, 10)
            AccountSegmented(options: [BotFace.Mood.idle, .thinking, .happy], selection: $mood, label: { $0 == .idle ? "Idle" : $0 == .thinking ? "Thinking" : "Happy" }, height: 30)
                .frame(maxWidth: 280)
            if tab == .look {
                HStack(spacing: 8) {
                    Button {
                        Haptics.tap()
                        var look = BotLook.random(seed: BotLook.randomSeed())
                        look.background = draft.look.background
                        setLook(look)
                    } label: {
                        Label("Shuffle", systemImage: "shuffle")
                    }
                    .buttonStyle(ChipButtonStyle(tone: .violet, filled: true))
                    .accessibilityHint("A new random look")
                    Button {
                        guard let previous = history.last else { return }
                        Haptics.tap()
                        history.removeLast()
                        setLook(previous, remember: false)
                    } label: {
                        Image(systemName: "arrow.uturn.backward")
                    }
                    .buttonStyle(ChipButtonStyle())
                    .disabled(history.isEmpty)
                    .accessibilityLabel("Undo look change")
                    Button {
                        Haptics.tap()
                        setLook(.ollie)
                    } label: {
                        Text("Ollie")
                    }
                    .buttonStyle(ChipButtonStyle())
                    .accessibilityHint("Start from Ollie's look")
                }
            }
            VStack(spacing: 12) {
                AgentField(label: "Name", text: $draft.name, prompt: "Pip", max: BotLimits.name, problem: nameProblem)
                AgentField(label: "Role", text: $draft.role, prompt: "Support at Acme", max: BotLimits.role, problem: roleProblem)
            }
            sample
        }
        .padding(16)
        .frame(maxWidth: .infinity)
        .background(Theme.surface, in: RoundedRectangle(cornerRadius: Theme.Radius.card, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: Theme.Radius.card, style: .continuous).strokeBorder(Theme.line, lineWidth: 1))
    }

    private var sample: some View {
        let p = draft.personality
        let text = AgentText.trimmed(p.greeting).isEmpty ? (BotPersonalityEditor.toneCopy[p.tone]?.sample ?? "") : p.greeting
        return VStack(alignment: .leading, spacing: 8) {
            Text("How it greets").font(ODFont.sans(11, weight: .medium)).foregroundStyle(Theme.fg4).textCase(.uppercase)
            HStack(alignment: .top, spacing: 8) {
                BotFace(look: draft.look, image: bot.imageURL, size: 26)
                VStack(alignment: .leading, spacing: 4) {
                    HStack(spacing: 6) {
                        Text(draft.name.isEmpty ? "Bot" : draft.name).font(ODFont.sans(12.5, weight: .semibold)).foregroundStyle(Theme.fg)
                        AITag()
                    }
                    Text(text)
                        .font(ODFont.sans(14))
                        .foregroundStyle(Theme.fg)
                        .padding(.horizontal, 12)
                        .padding(.vertical, 9)
                        .background(Theme.sunken, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
                }
                Spacer(minLength: 0)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var testTab: some View {
        VStack(alignment: .leading, spacing: 10) {
            AgentGroupTitle(title: "Ask it something", hint: "Answers use your knowledge and tools, as this bot. Test chats are not saved as tickets.")
            if dirty {
                Notice(text: "Testing your unsaved changes.", tone: .violet, symbol: "pencil")
            }
            AgentChatView(
                chat: chat,
                name: draft.name.isEmpty ? bot.name : draft.name,
                look: draft.look,
                image: bot.imageURL,
                suggestions: ["How do I reset my password?", "Can I get a refund?", "Can I talk to a person?"],
                send: { chat.send($0, api: env.agent.api, settings: testSettings, visitor: nil) }
            )
            .frame(height: 460)
            .background(Theme.surface, in: RoundedRectangle(cornerRadius: Theme.Radius.card, style: .continuous))
            .clipShape(RoundedRectangle(cornerRadius: Theme.Radius.card, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: Theme.Radius.card, style: .continuous).strokeBorder(Theme.line, lineWidth: 1))
            if !chat.turns.isEmpty {
                Button("Start over") { chat.reset() }
                    .buttonStyle(ChipButtonStyle())
            }
        }
    }

    /// The bot and its unsaved edits, for the test chat.
    private var testSettings: [String: JSONValue] {
        var out: [String: JSONValue] = ["bot_id": .string(bot.id)]
        if dirty {
            var edits: [String: JSONValue] = [:]
            if draft.name != base.name { edits["name"] = .string(AgentText.trimmed(draft.name)) }
            if draft.role != base.role { edits["role"] = .string(AgentText.trimmed(draft.role)) }
            if draft.language != base.language { edits["language"] = .string(draft.language) }
            let personality = draft.personality.patch(from: base.personality)
            if !personality.isEmpty { edits["personality"] = .object(personality) }
            if !edits.isEmpty { out["bot"] = .object(edits) }
        }
        return out
    }

    private func save() async {
        guard !invalid, let api = env.agent.api else { return }
        saving = true
        defer { saving = false }
        var body: [String: JSONValue] = [:]
        if draft.name != base.name { body["name"] = .string(AgentText.trimmed(draft.name)) }
        if draft.role != base.role { body["role"] = .string(AgentText.trimmed(draft.role)) }
        if draft.language != base.language { body["language"] = .string(draft.language) }
        if draft.look != base.look { body["look"] = draft.look.json }
        let personality = draft.personality.patch(from: base.personality)
        if !personality.isEmpty { body["personality"] = .object(personality) }
        do {
            let saved: AgentBot = try await api.patch("v1/bots/\(bot.id)", body: JSONValue.object(body))
            base = BotDraft(saved)
            draft = base
            env.agent.upsert(saved)
            Haptics.success()
            env.toasts.show("Saved \(saved.name)", kind: .success)
        } catch {
            env.toasts.error(error)
        }
    }

    private func upload(_ image: UIImage) async {
        guard let api = env.agent.api else { return }
        let square = Self.squared(image, side: 512)
        guard let data = square.jpegData(compressionQuality: 0.86) else { return }
        uploading = true
        defer { uploading = false }
        let boundary = "od-" + UUID().uuidString
        var body = Data()
        body.append(Data("--\(boundary)\r\n".utf8))
        body.append(Data("Content-Disposition: form-data; name=\"file\"; filename=\"bot.jpg\"\r\n".utf8))
        body.append(Data("Content-Type: image/jpeg\r\n\r\n".utf8))
        body.append(data)
        body.append(Data("\r\n--\(boundary)--\r\n".utf8))
        var request = APIRequest(method: "PUT", path: "v1/bots/\(bot.id)/image", body: body, contentType: "multipart/form-data; boundary=\(boundary)")
        request.timeout = 120
        do {
            let saved: AgentBot = try await api.send(request)
            env.agent.upsert(saved)
            Haptics.success()
        } catch {
            env.toasts.error(error)
        }
    }

    private func removeImage() async {
        guard let api = env.agent.api else { return }
        uploading = true
        defer { uploading = false }
        do {
            let saved: AgentBot = try await api.send(APIRequest(method: "DELETE", path: "v1/bots/\(bot.id)/image", body: nil, contentType: nil))
            env.agent.upsert(saved)
            if draft.look.kind == "image" { draft.look.kind = "built" }
        } catch {
            env.toasts.error(error)
        }
    }

    private static func squared(_ image: UIImage, side: CGFloat) -> UIImage {
        let size = image.size
        let edge = min(size.width, size.height)
        guard edge > 0 else { return image }
        let scale = side / edge
        let drawn = CGSize(width: size.width * scale, height: size.height * scale)
        let format = UIGraphicsImageRendererFormat()
        format.scale = 1
        return UIGraphicsImageRenderer(size: CGSize(width: side, height: side), format: format).image { _ in
            image.draw(in: CGRect(x: (side - drawn.width) / 2, y: (side - drawn.height) / 2, width: drawn.width, height: drawn.height))
        }
    }
}
