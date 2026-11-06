import SwiftUI

/// One chat widget (the web's `WidgetDetail`): every setting natively, saved together. The live
/// preview and the avatar and launcher pictures are the web app's.
struct WidgetDetailView: View {
    let widgetID: String

    @Environment(AppEnvironment.self) private var env
    @State private var store = BrandsStore()
    @State private var draft: WidgetDraft?
    @State private var base: WidgetDraft?
    @State private var saving = false
    @State private var typed: ChannelTypedConfirm?
    @State private var web: String?

    private var widget: ChatWidget? { store.widget(widgetID) }

    var body: some View {
        AccountPage(title: widget?.name ?? "Chat widget") {
            if !env.session.isAdmin {
                EmptyState(symbol: "lock", title: "Only admins can see chat widgets", message: "Ask a workspace admin to change this widget.")
                    .padding(.top, 20)
            } else {
                ChannelLoading(loaded: store.loaded, error: widget == nil ? store.error : nil, retry: reload) {
                    if let widget, let draft = Binding($draft) {
                        content(widget, draft)
                    } else if store.loaded {
                        EmptyState(symbol: "bubble.left", title: "This widget is gone", message: "It was deleted, or the link is wrong.")
                            .padding(.top, 30)
                    }
                }
            }
        }
        .settingsSaveBar(dirty: env.session.isAdmin && draft != base, saving: saving, canSave: !(draft?.invalid ?? true), save: save) { draft = base }
        .channelTypedConfirm($typed)
        .channelWebSheet($web) { reload() }
        .refreshable { await load() }
        .onAppear {
            store.attach(env.session.api)
            if env.session.isAdmin { reload() }
        }
    }

    private func reload() { Task { await load() } }

    private func load() async {
        await store.load()
        guard let widget else { return }
        let next = WidgetDraft(widget)
        if draft == nil || draft == base { draft = next }
        base = next
    }

    @ViewBuilder
    private func content(_ widget: ChatWidget, _ draft: Binding<WidgetDraft>) -> some View {
        let brand = store.brand(widget.brandID)
        let s = draft.settings
        let team = draft.wrappedValue.teamID.flatMap { env.directory.team($0) }
        HStack(spacing: 12) {
            if let brand { ChannelBrandMark(name: brand.name, color: brand.color, size: 36) }
            VStack(alignment: .leading, spacing: 2) {
                Text(widget.name).font(ODFont.sans(18, weight: .semibold)).foregroundStyle(Theme.fg).lineLimit(1)
                Text("A widget of \(brand?.name ?? "this workspace").").font(ODFont.footnote).foregroundStyle(Theme.fg3)
            }
            Spacer(minLength: 0)
            TonePill(text: widget.seenAt == nil ? "Not installed" : (s.wrappedValue.enabled ? "Live" : "Hidden"), tone: widget.seenAt == nil ? .gray : (s.wrappedValue.enabled ? .green : .gray))
        }
        .padding(.horizontal, 4)

        AccountSection(title: "Preview and pictures") {
            ChannelWebRow(symbol: "rectangle.on.rectangle", title: "Open in the web app", subtitle: "See the real widget with your saved settings, and upload its avatar and launcher icon.", path: "/settings/brands/\(widget.brandID)/widgets/\(widget.id)") { reload() }
        }

        AccountSection(title: "Widget", footer: "Its name and where its conversations go.") {
            VStack(alignment: .leading, spacing: 14) {
                VStack(alignment: .leading, spacing: 6) {
                    ODTextField(label: "Name", text: Binding(get: { draft.wrappedValue.name }, set: { draft.wrappedValue.name = String($0.prefix(60)) }), prompt: "Marketing site")
                    if draft.wrappedValue.name.trimmingCharacters(in: .whitespaces).isEmpty { FormError(text: "Give the widget a name.") } else { ChannelHint("Usually the website, like Marketing site.") }
                }
                ChannelTeamMenu(label: "Team", teamID: draft.teamID, none: "No team", hint: "New conversations go to this team, unless a topic or rule says otherwise.")
                ChannelMenuField(label: "Bot", value: store.bots.first { $0.id == draft.wrappedValue.botID }?.name ?? "Ollie", hint: env.directory.ai.enabled ? "Who answers visitors here first. Manage bots in AI agent settings." : "The bot visitors see when the AI agent is on. Manage bots in AI agent settings.") {
                    Button("Ollie") { draft.wrappedValue.botID = nil }
                    ForEach(store.bots) { bot in Button(bot.name) { draft.wrappedValue.botID = bot.id } }
                }
            }
            .padding(14)
        }

        AccountSection(title: "Appearance", footer: "Colour, theme and where the launcher sits on your site.") {
            VStack(alignment: .leading, spacing: 14) {
                ChannelColorField(label: "Accent colour", hex: s.color, hint: "Leave it to use the brand colour.", fallback: brand?.color.isEmpty == false ? brand?.color : env.session.workspace?.workspace.settings?.brandColor)
                labelled("Theme", hint: "Auto follows the visitor's system.") {
                    AccountSegmented(options: ["auto", "light", "dark"], selection: s.theme) { ["auto": "Auto", "light": "Light", "dark": "Dark"][$0] ?? $0 }
                }
                labelled("Position", hint: "Which bottom corner the launcher sits in.") {
                    AccountSegmented(options: ["left", "right"], selection: s.position) { $0 == "left" ? "Left" : "Right" }
                }
                labelled("Corners", hint: "Of the panel, the bubbles and the launcher.") {
                    AccountSegmented(options: ["square", "soft", "round"], selection: s.radius) { $0.capitalized }
                }
                labelled("Launcher", hint: "A round button, or a button with a short label.") {
                    AccountSegmented(options: ["icon", "label"], selection: s.launcher) { $0 == "icon" ? "Icon" : "Icon and label" }
                }
                if s.wrappedValue.launcher == "label" {
                    WidgetTextField(label: "Launcher label", text: s.launcherLabel, prompt: "Chat with us", max: 24)
                }
            }
            .padding(14)
            AccountDivider(tiles: false)
            ChannelStepperRow(title: "Distance from the side", subtitle: "0 to 200 px", value: s.offsetX, range: 0 ... 200, step: 4, suffix: " px")
            AccountDivider(tiles: false)
            ChannelStepperRow(title: "Distance from the bottom", subtitle: "0 to 200 px", value: s.offsetY, range: 0 ... 200, step: 4, suffix: " px")
        }

        AccountSection(title: "Header", footer: "The top of the open widget.") {
            VStack(alignment: .leading, spacing: 14) {
                WidgetTextField(label: "Title", text: s.title, prompt: brand?.name ?? "", max: 40, hint: "Up to 40 characters. Empty shows the brand name.")
                WidgetTextField(label: "Subtitle", text: s.subtitle, prompt: "We usually reply in a few minutes", max: 80, hint: "Leave empty to show how fast you usually reply.")
            }
            .padding(14)
            AccountDivider(tiles: false)
            AccountToggleRow(title: "Show team faces", subtitle: team.map { "Shows people on \($0.name), available ones first." } ?? "Up to three teammates, first names only, available people first.", isOn: s.showTeam)
        }

        AccountSection(title: "Home screen", footer: "What visitors see before they start a conversation.") {
            VStack(alignment: .leading, spacing: 14) {
                WidgetTextField(label: "Greeting", text: s.greeting, prompt: "Hi there. How can we help?", max: 300, hint: "The first thing visitors read on the home screen.", area: true)
                WidgetTextField(label: "Chat intro", text: s.intro, prompt: "Ask us anything. We'll reply here, and by email if you leave one.", max: 200, hint: "Under the greeting when a visitor starts a new chat. Empty uses a translated default.", area: true)
            }
            .padding(14)
            AccountDivider(tiles: false)
            AccountToggleRow(title: "Welcome text in a new chat", subtitle: "Shows the greeting and intro at the top of a new chat. Off, the chat opens empty.", isOn: s.chatWelcome)
            AccountDivider(tiles: false)
            VStack(alignment: .leading, spacing: 6) {
                ChannelChipEditor(label: "Suggested questions", items: s.suggestions, placeholder: "Where is my order?", empty: "No suggestions. Visitors start with an empty message.", max: 4, mono: false, keyboard: .default) { text, items in
                    let value = WidgetDraft.tidy(text)
                    if value.count < 2 { return .failure(ChannelChipProblem("At least 2 characters")) }
                    if value.count > 60 { return .failure(ChannelChipProblem("Up to 60 characters")) }
                    if WidgetDraft.hasMarkup(value) { return .failure(ChannelChipProblem("Plain text only, without < or >")) }
                    if items.contains(where: { $0.caseInsensitiveCompare(value) == .orderedSame }) { return .failure(ChannelChipProblem("Already in the list")) }
                    return .success(value)
                }
                ChannelHint("Up to four. A tap starts a conversation with that question.")
            }
            .padding(14)
        }

        AccountSection(title: "Topics", footer: "When you add topics, the widget asks what the visitor needs before the first message, and the pick sends the conversation to its team.") {
            WidgetTopicsEditor(topics: s.topics)
                .padding(14)
        }

        AccountSection(title: "When you're closed", footer: "What visitors see outside business hours.") {
            VStack(alignment: .leading, spacing: 6) {
                WidgetTextField(label: "Offline message", text: s.offlineMessage, prompt: "We're away right now. Leave a message and we'll reply by email.", max: 300, hint: team.map { "Shown while \($0.name) is closed, by its own hours." } ?? "Shown while the workspace is closed. Empty uses a translated default.", area: true)
            }
            .padding(14)
            AccountDivider(tiles: false)
            AccountToggleRow(title: "Reply by email when they leave", subtitle: "A visitor who left an email and is no longer on the site gets your replies by email, from the brand's address for the team.", isOn: s.emailFollowUp)
            if s.wrappedValue.emailFollowUp, !hasFollowUpMailbox(widget) {
                AccountDivider(tiles: false)
                ChannelActionRow(title: "No sending address", subtitle: "Add an active sending mailbox to this brand to send follow-up emails.", button: "Add mailbox") {
                    env.router.openSetting(.mailboxes, id: "new/\(widget.brandID)")
                }
            }
        }

        AccountSection(title: "Behaviour", footer: "When the widget shows, what it asks for and where it may load.") {
            AccountToggleRow(title: "Show the widget", subtitle: "Off hides the launcher everywhere, even with the script installed.", isOn: s.enabled)
            AccountDivider(tiles: false)
            AccountToggleRow(title: "Ask for an email first", subtitle: "Visitors leave an address before chatting, so you can follow up by email.", isOn: s.requireEmail)
            AccountDivider(tiles: false)
            AccountToggleRow(title: "Let visitors send files", subtitle: "Visitors can attach, drop or paste files of up to 25 MB.", isOn: s.attachments)
            if s.wrappedValue.attachments {
                AccountDivider(tiles: false)
                WidgetFileTypes(limit: draft.limitTypes, types: s.attachmentTypes)
                    .padding(14)
            }
            AccountDivider(tiles: false)
            AccountToggleRow(title: "Let visitors send voice messages", subtitle: "Visitors record from their microphone, listen back, then send or delete the recording. Works with files turned off.", isOn: s.voiceMessages)
            if s.wrappedValue.voiceMessages {
                AccountDivider(tiles: false)
                ChannelStepperRow(title: "Longest voice message", subtitle: "In seconds, from 5 to 300. Recording stops on its own at this length.", value: s.voiceMaxSeconds, range: 5 ... 300, step: 5, suffix: " s")
            }
            AccountDivider(tiles: false)
            AccountToggleRow(title: "Sound for new replies", subtitle: "A soft chime when a reply arrives while the visitor's tab is in the background.", isOn: s.sound)
            AccountDivider(tiles: false)
            VStack(alignment: .leading, spacing: 6) {
                ChannelChipEditor(label: "Allowed sites", items: s.allowedOrigins, placeholder: "https://www.example.com", empty: "Loads on any site for now.", max: 20, keyboard: .URL) { text, items in
                    guard let origin = WidgetDraft.origin(text) else { return .failure(ChannelChipProblem("Enter a site address like https://example.com")) }
                    if items.contains(origin) { return .failure(ChannelChipProblem("Already in the list")) }
                    return .success(origin)
                }
                ChannelHint("The widget only loads on these origins. Leave empty to allow any site.")
            }
            .padding(14)
            if env.directory.ai.enabled {
                AccountDivider(tiles: false)
                Button { env.router.accountPath.append(.setting(.behaviour)) } label: {
                    AccountRow(symbol: "sparkles", tone: .violet, title: "Ollie answers new chats first", subtitle: "Name, instructions, tools and guardrails are in AI agent settings.") { Chevron() }
                }
                .buttonStyle(RowPressStyle())
            }
        }

        AccountSection(title: "Install", footer: "Paste this before </body> on every page of this site.") {
            VStack(alignment: .leading, spacing: 10) {
                ChannelCopyField(text: snippet(widget), copiedMessage: "Snippet copied")
                HStack(spacing: 6) {
                    Text("Widget key").font(ODFont.sans(12)).foregroundStyle(Theme.fg3)
                    Text(widget.key).font(ODFont.mono(12)).foregroundStyle(Theme.fg2).textSelection(.enabled)
                }
            }
            .padding(14)
        }

        AccountSection(title: "Identify signed-in users", footer: "Optional. Links chats to the right contact and keeps their history across devices.") {
            VStack(alignment: .leading, spacing: 10) {
                ChannelCopyField(text: WidgetDraft.identify, copiedMessage: "Snippet copied")
                Text("Compute user_hash on your server as the hex HMAC-SHA256 of the user_id, keyed with your widget secret. Never put the secret in browser code. Sessions without a valid hash still work, but are marked unverified.")
                    .font(ODFont.footnote).foregroundStyle(Theme.fg3)
                WidgetRotateSecret(store: store, widget: widget)
            }
            .padding(14)
        }

        AccountSection(title: "Delete") {
            ChannelActionRow(title: "Delete \(widget.name)", subtitle: "The widget stops loading on your site. Past conversations stay.", button: "Delete widget", buttonTone: .red) {
                typed = ChannelTypedConfirm(title: "Delete \(widget.name)", message: "\(widget.name) stops loading on your site right away. Past conversations stay in the inbox.", name: widget.name, confirmTitle: "Delete widget") {
                    do {
                        try await store.deleteWidget(widget.id)
                        env.toasts.show("\(widget.name) deleted", kind: .success)
                        env.router.accountPath.removeLast()
                    } catch { env.toasts.error(error) }
                }
            }
        }
    }

    private func labelled<Content: View>(_ label: String, hint: String, @ViewBuilder content: () -> Content) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(label).font(ODFont.sans(13, weight: .medium)).foregroundStyle(Theme.fg2)
            content()
            ChannelHint(hint)
        }
    }

    /// A sending mailbox of the widget's brand, for follow-up emails.
    private func hasFollowUpMailbox(_ widget: ChatWidget) -> Bool {
        store.mailboxes.contains { m in
            (m.brandID == widget.brandID || m.aliasSettings.values.contains { $0.brandID == widget.brandID }) && m.status == "active" && m.sending != "system_off"
        }
    }

    /// The script tag, as the server answers it, or built for this instance's web app.
    private func snippet(_ widget: ChatWidget) -> String {
        if !widget.snippet.isEmpty { return widget.snippet }
        let instance = env.session.instance
        var origin = instance.appOrigin.absoluteString
        while origin.hasSuffix("/") { origin.removeLast() }
        let api = instance.apiURL.absoluteString.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        let inferred = instance.isCloud ? "https://api.opendesk.sh" : origin
        let apiAttribute = api == inferred ? "" : " data-api=\"\(api)\""
        return "<script src=\"\(origin)/widget/v1.js\" data-key=\"\(widget.key)\"\(apiAttribute) async></script>"
    }

    private func save() {
        guard let draft, let base, let widget, !draft.invalid else { return }
        let body = draft.patch(from: base)
        guard !body.isEmpty else { return }
        saving = true
        Task {
            do {
                let saved = try await store.patchWidget(widget.id, body)
                self.draft = WidgetDraft(saved)
                self.base = self.draft
                Haptics.success()
                env.toasts.show("Widget settings saved", kind: .success)
            } catch {
                Haptics.error()
                env.toasts.show("Couldn't save the widget settings. \(error.userMessage)", kind: .error)
            }
            saving = false
        }
    }
}

/// A plain text setting with its limit, counted the way the server stores it.
private struct WidgetTextField: View {
    let label: String
    @Binding var text: String
    var prompt = ""
    let max: Int
    var hint: String?
    var area = false

    var body: some View {
        let count = WidgetDraft.tidy(text).count
        let problem = WidgetDraft.problem(text, max: max)
        VStack(alignment: .leading, spacing: 6) {
            if area {
                AccountTextEditor(label: label, text: $text, prompt: prompt, minHeight: 72)
            } else {
                ODTextField(label: label, text: $text, prompt: prompt)
            }
            HStack(alignment: .firstTextBaseline) {
                if let problem { FormError(text: problem) } else if let hint { ChannelHint(hint) }
                Spacer(minLength: 8)
                Text("\(count) / \(max)").font(ODFont.mono(11)).foregroundStyle(count > max ? Tone.red.text : Theme.fg4)
            }
        }
    }
}

/// Up to 8 topics, each a label and the team its conversations go to.
private struct WidgetTopicsEditor: View {
    @Binding var topics: [WidgetTopic]

    @Environment(AppEnvironment.self) private var env

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            ForEach(Array(topics.enumerated()), id: \.element.id) { index, topic in
                let id = topic.id
                VStack(alignment: .leading, spacing: 8) {
                    HStack(spacing: 8) {
                        TextField(text: Binding(get: { topic.label }, set: { value in update(id) { $0.label = String(value.prefix(50)) } })) {
                            Text("Billing and plans").foregroundStyle(Theme.fg4)
                        }
                        .font(ODFont.sans(15, weight: .medium))
                        .accessibilityLabel("Topic \(index + 1)")
                        Menu {
                            if index > 0 { Button("Move up", systemImage: "arrow.up") { move(index, -1) } }
                            if index < topics.count - 1 { Button("Move down", systemImage: "arrow.down") { move(index, 1) } }
                            Button("Remove \(topic.label.isEmpty ? "New topic" : topic.label)", systemImage: "trash", role: .destructive) {
                                topics.removeAll { $0.id == id }
                            }
                        } label: {
                            Image(systemName: "ellipsis").font(.system(size: 13, weight: .semibold)).foregroundStyle(Theme.fg3).frame(width: 30, height: 30).contentShape(Rectangle())
                        }
                    }
                    Menu {
                        Button("The widget's team") { update(id) { $0.teamID = nil } }
                        ForEach(env.directory.teams) { team in Button(team.name) { update(id) { $0.teamID = team.id } } }
                    } label: {
                        HStack(spacing: 4) {
                            Text("goes to").foregroundStyle(Theme.fg3)
                            Text(topic.teamID.flatMap { env.directory.team($0)?.name } ?? "The widget's team").foregroundStyle(Theme.fg)
                            Image(systemName: "chevron.up.chevron.down").font(.system(size: 9, weight: .semibold)).foregroundStyle(Theme.fg4)
                        }
                        .font(ODFont.sans(13))
                    }
                    if let problem = WidgetDraft.topicProblem(topic, in: topics) { FormError(text: problem) }
                }
                .padding(10)
                .background(Theme.sunken, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
            }
            HStack {
                Text(topics.isEmpty ? "No topics: visitors start typing right away." : "\(topics.count) of 8").font(ODFont.sans(12)).foregroundStyle(Theme.fg3)
                Spacer()
                Button {
                    topics.append(WidgetTopic(label: "", teamID: nil))
                    Haptics.tap()
                } label: {
                    Label("Add a topic", systemImage: "plus").font(ODFont.sans(14, weight: .medium))
                }
                .foregroundStyle(Theme.fg)
                .disabled(topics.count >= 8)
            }
        }
    }

    private func update(_ id: String, _ change: (inout WidgetTopic) -> Void) {
        guard let index = topics.firstIndex(where: { $0.id == id }) else { return }
        change(&topics[index])
    }

    private func move(_ index: Int, _ offset: Int) {
        let to = index + offset
        guard topics.indices.contains(to) else { return }
        topics.swapAt(index, to)
        Haptics.selection()
    }
}

/// Any file, or only the picked types.
private struct WidgetFileTypes: View {
    @Binding var limit: Bool
    @Binding var types: [String]
    @State private var picking = false

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Accepted files").font(ODFont.sans(13, weight: .medium)).foregroundStyle(Theme.fg2)
            AccountSegmented(options: [false, true], selection: Binding(get: { limit }, set: { on in
                limit = on
                if !on { types = [] }
            })) { $0 ? "Only these types" : "Any file" }
            if limit {
                Button { picking = true } label: {
                    HStack {
                        Text(summary).font(ODFont.sans(14)).foregroundStyle(types.isEmpty ? Theme.fg4 : Theme.fg).lineLimit(2)
                        Spacer()
                        Chevron()
                    }
                    .padding(.horizontal, 12)
                    .frame(minHeight: 40)
                    .background(Theme.surface, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
                    .overlay(RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous).strokeBorder(Theme.line, lineWidth: 1))
                }
                .buttonStyle(.plain)
                if types.isEmpty { FormError(text: "Pick at least one file type, or accept any file.") }
            }
            ChannelHint("Each file is checked by its contents, not only its name. Programs and scripts are always refused.")
        }
        .sheet(isPresented: $picking) {
            ChannelSheet(title: "Accepted files") {
                ForEach(WidgetDraft.fileGroups, id: \.0) { group in
                    VStack(alignment: .leading, spacing: 0) {
                        SectionLabel(group.1)
                        ForEach(WidgetDraft.fileTypes.filter { $0.group == group.0 }, id: \.id) { type in
                            AccountChoiceRow(title: type.extensions.joined(separator: " "), chosen: types.contains(type.id)) {
                                if types.contains(type.id) { types.removeAll { $0 == type.id } } else { types.append(type.id) }
                                types = WidgetDraft.fileTypes.map(\.id).filter(types.contains)
                            }
                        }
                    }
                }
            }
        }
    }

    private var summary: String {
        if types.count == WidgetDraft.fileTypes.count { return "All file types" }
        if types.isEmpty { return "Pick file types" }
        return WidgetDraft.fileTypes.filter { types.contains($0.id) }.map { $0.extensions[0] }.joined(separator: " ")
    }
}

/// The identity verification secret: rotate it, see the new one once.
private struct WidgetRotateSecret: View {
    let store: BrandsStore
    let widget: ChatWidget

    @Environment(AppEnvironment.self) private var env
    @State private var confirm: AccountConfirm?
    @State private var secret: String?
    @State private var blocked: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            ChannelActionRow(symbol: "key", title: "Widget secret", subtitle: "Signs identify calls for \(widget.name). Each widget has its own.", button: "Rotate secret") {
                confirm = AccountConfirm(title: "Rotate the widget secret?", message: "The current secret for \(widget.name) stops working at once. Update your server before visitors sign in again.", confirmTitle: "Rotate secret") {
                    await rotate()
                }
            }
            .background(Theme.surface, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous).strokeBorder(Theme.line, lineWidth: 1))
            if let blocked { Notice(text: blocked, tone: .amber, symbol: "lock") }
        }
        .accountConfirm($confirm)
        .sheet(item: Binding(get: { secret.map { WidgetSecret(value: $0) } }, set: { secret = $0?.value })) { item in
            ChannelSheet(title: "New widget secret", subtitle: "Copy it now and store it on your server. You won't see it again.") {
                ChannelCopyField(text: item.value, copiedMessage: "Secret copied")
                Notice(text: "Treat it like a password. Anyone with it can impersonate your users in chat.", tone: .amber)
                Button("I've saved it") { secret = nil }.buttonStyle(InkButtonStyle())
            }
            .interactiveDismissDisabled()
        }
    }

    private struct WidgetSecret: Identifiable {
        let value: String
        var id: String { value }
    }

    private func rotate() async {
        blocked = nil
        do {
            let next = try await store.rotateSecret(widget.id)
            Haptics.success()
            secret = next
        } catch let error as APIError where error.code == "mfa_required" {
            blocked = "Sign in again with your two-factor code to do this."
        } catch let error as APIError where error.code == "reauth_required" {
            blocked = "This needs a sign-in from the last 12 hours. Sign in again to confirm it's you."
        } catch {
            env.toasts.show("Couldn't rotate the secret. \(error.userMessage)", kind: .error)
        }
    }
}

// MARK: - Draft

struct WidgetDraft: Equatable {
    var name: String
    var teamID: String?
    var botID: String?
    /// Files are held to `attachment_types`; off, any file but a program is taken.
    var limitTypes: Bool
    var settings: WidgetSettings

    init(_ widget: ChatWidget) {
        name = widget.name
        teamID = widget.teamID
        botID = widget.botID
        settings = widget.settings
        limitTypes = !widget.settings.attachmentTypes.isEmpty
    }

    var invalid: Bool {
        let s = settings
        if name.trimmingCharacters(in: .whitespaces).isEmpty { return true }
        let texts: [(String, Int)] = [(s.greeting, 300), (s.intro, 200), (s.launcherLabel, 24), (s.title, 40), (s.subtitle, 80), (s.offlineMessage, 300)]
        if texts.contains(where: { Self.problem($0.0, max: $0.1) != nil }) { return true }
        if s.topics.contains(where: { Self.topicProblem($0, in: s.topics) != nil }) { return true }
        if s.attachments, limitTypes, s.attachmentTypes.isEmpty { return true }
        if !s.color.isEmpty, !ChannelColor.valid(s.color) { return true }
        return false
    }

    /// Only what changed: PATCH merges, and settings merge key by key.
    func patch(from base: WidgetDraft) -> ChannelBody {
        var body: ChannelBody = [:]
        if name != base.name { body["name"] = .string(name.trimmingCharacters(in: .whitespaces)) }
        if teamID != base.teamID { body["team_id"] = .chString(teamID) }
        if botID != base.botID { body["bot_id"] = .chString(botID) }
        var tidied = settings
        tidied.launcherLabel = Self.tidy(tidied.launcherLabel)
        tidied.title = Self.tidy(tidied.title)
        tidied.subtitle = Self.tidy(tidied.subtitle)
        tidied.offlineMessage = Self.tidy(tidied.offlineMessage)
        tidied.suggestions = tidied.suggestions.map(Self.tidy)
        tidied.topics = tidied.topics.map { topic in
            var out = topic
            out.label = Self.tidy(topic.label)
            return out
        }
        var settingsPatch = tidied.changes(from: base.settings)
        if settings.topics == base.settings.topics { settingsPatch["topics"] = nil }
        if !settingsPatch.isEmpty { body["settings"] = .object(settingsPatch) }
        return body
    }

    // MARK: Rules, as the web's (`components/widget/fields`)

    static func tidy(_ text: String) -> String {
        text.trimmingCharacters(in: .whitespacesAndNewlines).split(whereSeparator: \.isWhitespace).joined(separator: " ")
    }

    static func hasMarkup(_ text: String) -> Bool {
        text.unicodeScalars.contains { $0 == "<" || $0 == ">" || ($0.value < 0x20 && $0 != "\n" && $0 != "\t") || $0.value == 0x7F }
    }

    static func problem(_ text: String, max: Int) -> String? {
        if tidy(text).count > max { return "Up to \(max) characters" }
        if hasMarkup(text) { return "Plain text only, without < or >" }
        return nil
    }

    static func topicProblem(_ topic: WidgetTopic, in all: [WidgetTopic]) -> String? {
        let label = tidy(topic.label)
        if label.count < 2 || label.count > 40 { return "Use 2 to 40 characters." }
        if hasMarkup(label) { return "Plain text only." }
        if all.contains(where: { $0.id != topic.id && tidy($0.label).lowercased() == label.lowercased() }) { return "Each topic needs a different label." }
        return nil
    }

    /// A site's origin from what was typed, `https://` when no scheme.
    static func origin(_ text: String) -> String? {
        let trimmed = text.trimmingCharacters(in: .whitespaces)
        guard !trimmed.isEmpty, let url = URL(string: trimmed.contains("://") ? trimmed : "https://\(trimmed)"),
              let scheme = url.scheme?.lowercased(), scheme == "https" || scheme == "http", let host = url.host(), host.contains(".") || host == "localhost"
        else { return nil }
        return "\(scheme)://\(host.lowercased())" + (url.port.map { ":\($0)" } ?? "")
    }

    static let identify = """
    window.opendesk('identify', {
      user_id: "42",
      user_hash: "<hmac from your server>",
      email: "ada@example.com",
      name: "Ada Lovelace",
    });
    """

    struct FileType {
        let id: String
        let group: String
        let extensions: [String]
    }

    static let fileGroups = [("image", "Images"), ("document", "Documents"), ("spreadsheet", "Spreadsheets"), ("presentation", "Presentations"), ("media", "Audio and video"), ("archive", "Archives")]

    static let fileTypes: [FileType] = [
        FileType(id: "png", group: "image", extensions: [".png"]), FileType(id: "jpg", group: "image", extensions: [".jpg", ".jpeg"]),
        FileType(id: "gif", group: "image", extensions: [".gif"]), FileType(id: "webp", group: "image", extensions: [".webp"]),
        FileType(id: "heic", group: "image", extensions: [".heic", ".heif"]), FileType(id: "pdf", group: "document", extensions: [".pdf"]),
        FileType(id: "txt", group: "document", extensions: [".txt"]), FileType(id: "csv", group: "document", extensions: [".csv"]),
        FileType(id: "rtf", group: "document", extensions: [".rtf"]), FileType(id: "doc", group: "document", extensions: [".doc"]),
        FileType(id: "docx", group: "document", extensions: [".docx"]), FileType(id: "odt", group: "document", extensions: [".odt"]),
        FileType(id: "xls", group: "spreadsheet", extensions: [".xls"]), FileType(id: "xlsx", group: "spreadsheet", extensions: [".xlsx"]),
        FileType(id: "ods", group: "spreadsheet", extensions: [".ods"]), FileType(id: "ppt", group: "presentation", extensions: [".ppt"]),
        FileType(id: "pptx", group: "presentation", extensions: [".pptx"]), FileType(id: "odp", group: "presentation", extensions: [".odp"]),
        FileType(id: "mp4", group: "media", extensions: [".mp4", ".m4v"]), FileType(id: "mov", group: "media", extensions: [".mov"]),
        FileType(id: "mp3", group: "media", extensions: [".mp3"]), FileType(id: "m4a", group: "media", extensions: [".m4a"]),
        FileType(id: "wav", group: "media", extensions: [".wav"]), FileType(id: "webm", group: "media", extensions: [".webm"]),
        FileType(id: "ogg", group: "media", extensions: [".ogg", ".oga", ".opus"]), FileType(id: "zip", group: "archive", extensions: [".zip"]),
    ]
}
