import SwiftUI

/// Settings › AI: whether the workspace uses AI, what Ollie does on its own, how drafts sound,
/// the provider key and models, and what each action costs (web `AiSettings.tsx`).
struct AISettings: View {
    @Environment(AppEnvironment.self) private var env
    @State private var store = AISettingsStore()
    @State private var editingKey = false
    @State private var confirm: AccountConfirm?

    private var admin: Bool { env.session.isAdmin }

    var body: some View {
        AccountPage(title: "AI") {
            if let config = store.config {
                if !admin { SettingsReadOnlyNote() }
                if !store.configured { notConfigured }
                master(config)
                if store.draft.enabled {
                    assistant(config)
                    automatic
                    writing
                }
                if admin, let provider = store.provider { providerSection(provider) }
                if store.draft.enabled { models(config) }
                if !config.costs.isEmpty && store.draft.enabled { costs(config) }
            } else if let error = store.error {
                ErrorState(message: error) { Task { await store.load(admin: admin) } }
            } else {
                WSLoading(rows: 5)
            }
        }
        .refreshable { await store.load(admin: admin) }
        .task {
            store.attach(env.session.api)
            await store.load(admin: admin)
        }
        .onChange(of: env.directory.ai) { _, _ in
            if !store.dirty { Task { await store.load(admin: admin) } }
        }
        .settingsSaveBar(dirty: store.dirty && admin, saving: store.saving, canSave: store.valid, save: save) { store.discard() }
        .sheet(isPresented: $editingKey) {
            WSProviderKeySheet(current: store.provider?.key) {
                await store.load(admin: admin)
            }
            .presentationDetents([.large])
            .presentationCornerRadius(Theme.Radius.sheet)
        }
        .accountConfirm($confirm)
    }

    // MARK: Sections

    private var notConfigured: some View {
        Notice(
            text: env.session.instance.selfHosted
                ? "No AI provider is set up on this instance. Add your own key below, or the instance's operator can set one for every workspace."
                : "No AI provider is set up for this workspace yet. Add your own key below.",
            tone: .amber, symbol: "exclamationmark.triangle"
        )
    }

    private func master(_ config: WSAIConfig) -> some View {
        AccountSection(footer: "Turn it off to hide every AI feature for the whole workspace, including \(config.assistantName).") {
            AccountToggleRow(symbol: "sparkles", tone: .violet, title: "Use AI in this workspace", isOn: $store.draft.enabled, disabled: !admin)
        }
    }

    private func assistant(_ config: WSAIConfig) -> some View {
        let name = config.assistantName
        return AccountSection(title: name, footer: "\(name) always asks before any other change, and never sends a reply to a customer on its own.") {
            AccountToggleRow(title: "\(name) the assistant", subtitle: "Teammates can ask \(name) about tickets, customers and the knowledge base, and have it make changes for them.", isOn: $store.draft.assistant, disabled: !admin)
            AccountDivider(tiles: false)
            AccountToggleRow(title: "Ask a teammate's \(name)", subtitle: "\(name) can ask a teammate's \(name) what they know about a ticket or customer, within what that teammate may see.", isOn: $store.draft.assistantPeer, disabled: !admin || !store.draft.assistant)
            if store.draft.assistant {
                AccountDivider(tiles: false)
                VStack(alignment: .leading, spacing: 4) {
                    Text("Changes \(name) makes without asking").font(ODFont.sans(13, weight: .medium)).foregroundStyle(Theme.fg2)
                    Text("Saved as soon as you switch them.").font(ODFont.footnote).foregroundStyle(Theme.fg3)
                }
                .padding(.horizontal, 14)
                .padding(.top, 12)
                ForEach(WSAutoTools.all, id: \.self) { tool in
                    AccountToggleRow(
                        title: WSAutoTools.title(tool), subtitle: WSAutoTools.hint(tool),
                        isOn: Binding(get: { store.auto.contains(tool) }, set: { on in toggleAuto(tool, on) }),
                        disabled: !admin || store.savingAuto
                    )
                }
            }
        }
    }

    private var automatic: some View {
        AccountSection(title: "Automatic", footer: "These run on their own as tickets arrive and use the workspace's AI balance.") {
            AccountToggleRow(title: "Triage new tickets", subtitle: "Suggest a priority, labels, team and sentiment for each new ticket.", isOn: $store.draft.autoTriage, disabled: !admin)
            AccountDivider(tiles: false)
            AccountToggleRow(title: "Summarize long tickets", subtitle: "Keep a short summary at the top of tickets with long threads.", isOn: $store.draft.autoSummarize, disabled: !admin)
            AccountDivider(tiles: false)
            AccountToggleRow(title: "Answer in the chat widget", subtitle: "Answer visitors from published articles, and hand over to the team when it can't.", isOn: $store.draft.widgetAnswers, disabled: !admin)
        }
    }

    private var writing: some View {
        let custom = !WSAIValues.presetTones.contains(store.draft.tone)
        return AccountSection(title: "Writing", footer: "Used for drafts, rewrites and answers.") {
            VStack(alignment: .leading, spacing: 8) {
                Text("Tone").font(ODFont.sans(13, weight: .medium)).foregroundStyle(Theme.fg2)
                AccountSegmented(
                    options: WSAIValues.presetTones + ["custom"],
                    selection: Binding(get: { custom ? "custom" : store.draft.tone }, set: { pick in
                        if pick == "custom" {
                            if !custom { store.draft.tone = "" }
                        } else {
                            store.draft.tone = pick
                        }
                    })
                ) { $0 == "custom" ? "Custom" : $0.capitalized }
                    .disabled(!admin)
                    .opacity(admin ? 1 : 0.6)
            }
            .padding(14)
            if custom {
                AccountDivider(tiles: false)
                WSPanelField(label: "Describe the tone", text: $store.draft.tone, prompt: "Warm and plain, like a helpful colleague", disabled: !admin, counter: WSAIValues.toneMax)
            }
            AccountDivider(tiles: false)
            VStack(alignment: .leading, spacing: 8) {
                Text("Reply language").font(ODFont.sans(13, weight: .medium)).foregroundStyle(Theme.fg2)
                AccountSegmented(options: ["customer", "workspace"], selection: $store.draft.replyLanguage) {
                    $0 == "workspace" ? "Workspace language" : "Customer's language"
                }
                .disabled(!admin)
                .opacity(admin ? 1 : 0.6)
            }
            .padding(14)
            AccountDivider(tiles: false)
            WSPanelField(label: "Instructions", text: $store.draft.instructions, prompt: "Always sign off with the team name. Never promise refunds.", axis: .vertical, disabled: !admin, counter: WSAIValues.instructionsMax)
            AccountDivider(tiles: false)
            AccountToggleRow(title: "Look things up for drafts", subtitle: "Drafts can read from connected tools that run without approval, such as an order lookup.", isOn: $store.draft.draftTools, disabled: !admin)
        }
    }

    private func providerSection(_ provider: WSProvider) -> some View {
        AccountSection(title: "Provider", footer: provider.key != nil
            ? "AI runs on this workspace's own key. Its use is billed by the provider, not counted in the workspace's AI balance."
            : "Bring your own OpenRouter, Anthropic or OpenAI-compatible key to run the workspace's AI on it.") {
            if let key = provider.key {
                AccountRow(
                    symbol: "key", tone: key.status == "ok" ? .green : .amber,
                    title: "\(WSProviderKey.providerName(key.kind)) key ending \(key.last4)",
                    subtitle: keyLine(key)
                ) {
                    if key.status != "ok" { TonePill(text: Self.keyStatus(key.status), tone: .amber) }
                }
                AccountDivider()
                Button { editingKey = true } label: {
                    AccountRow(symbol: "arrow.triangle.2.circlepath", tone: .blue, title: "Replace key")
                }
                .buttonStyle(RowPressStyle())
                AccountDivider()
                Button { askRemoveKey() } label: {
                    AccountRow(symbol: "trash", tone: .red, title: "Remove key", subtitle: provider.instanceProvider.isEmpty ? "AI stops until a key is added again." : "AI goes back to the instance's provider.", titleColor: Tone.red.text)
                }
                .buttonStyle(RowPressStyle())
            } else {
                AccountRow(
                    symbol: "server.rack", tone: .gray,
                    title: provider.instanceProvider.isEmpty ? "No provider" : "The instance's provider",
                    subtitle: provider.instanceProvider.isEmpty ? nil : WSProviderKey.providerName(provider.instanceProvider)
                )
                AccountDivider()
                Button { editingKey = true } label: {
                    AccountRow(symbol: "key", tone: .violet, title: "Use your own key") { AccountDisclosure() }
                }
                .buttonStyle(RowPressStyle())
            }
        }
    }

    private func keyLine(_ key: WSProviderKey) -> String {
        var parts: [String] = []
        if !key.model.isEmpty { parts.append(key.model) }
        if !key.baseURLHost.isEmpty { parts.append(key.baseURLHost) }
        if let by = key.createdBy?.name, !by.isEmpty { parts.append("Added by \(by)") }
        return parts.joined(separator: " \u{00B7} ")
    }

    static func keyStatus(_ status: String) -> String {
        switch status {
        case "invalid_key": "Key refused"
        case "no_funds": "Out of credit"
        case "unreachable": "Unreachable"
        default: "Check the key"
        }
    }

    @ViewBuilder
    private func models(_ config: WSAIConfig) -> some View {
        if let list = store.models, !list.data.isEmpty {
            let options = [""] + list.data.map(\.id)
            AccountSection(title: "Models", footer: "The fast model handles rewrites, translations and triage.") {
                WSMenuRow(
                    title: "Model", subtitle: "Drafts, answers and \(config.assistantName).",
                    options: options, selection: $store.draft.model,
                    label: { $0.isEmpty ? "Default (\(list.name(list.defaultID)))" : list.name($0) },
                    disabled: !admin
                )
                AccountDivider(tiles: false)
                WSMenuRow(
                    title: "Fast model",
                    options: options, selection: $store.draft.fastModel,
                    label: { $0.isEmpty ? "Default (\(list.name(list.fastID)))" : list.name($0) },
                    disabled: !admin
                )
                AccountDivider(tiles: false)
                AccountRow(title: "Search", subtitle: config.embeddings
                    ? "Articles and tickets are found by meaning as well as by words."
                    : "Articles and tickets are found by their words. Search by meaning needs an embeddings provider.") {
                    TonePill(text: config.embeddings ? "By meaning" : "By words", tone: config.embeddings ? .green : .gray)
                }
            }
        }
    }

    private func costs(_ config: WSAIConfig) -> some View {
        let keys = WSFeatureNames.costOrder.filter { config.costs[$0] != nil } + config.costs.keys.filter { !WSFeatureNames.costOrder.contains($0) }.sorted()
        return AccountSection(title: "What each action costs", footer: config.ownKey
            ? "Billed by your provider on your own key."
            : "Taken from the workspace's AI balance. Prices follow the model in use.") {
            ForEach(Array(keys.enumerated()), id: \.element) { index, key in
                if index > 0 { AccountDivider(tiles: false) }
                AccountRow(title: WSFeatureNames.action(key)) {
                    Text(config.costText(config.costs[key] ?? 0))
                        .font(ODFont.sans(14, weight: .medium))
                        .foregroundStyle(Theme.fg2)
                        .monospacedDigit()
                }
            }
        }
    }

    // MARK: Actions

    private func save() {
        Task {
            do {
                try await store.save()
                Haptics.success()
                env.toasts.show("AI settings saved", kind: .success)
            } catch {
                Haptics.error()
                env.toasts.error(error)
            }
        }
    }

    private func toggleAuto(_ tool: String, _ on: Bool) {
        Task {
            do {
                try await store.setAuto(tool, on)
            } catch {
                env.toasts.error(error)
            }
        }
    }

    private func askRemoveKey() {
        confirm = AccountConfirm(
            title: "Remove the key?",
            message: "The workspace's AI stops using this key right away.",
            confirmTitle: "Remove key"
        ) {
            do {
                try await store.removeKey()
                env.toasts.show("Key removed", kind: .success)
            } catch {
                env.toasts.error(error)
            }
        }
    }

    struct Detail: View {
        let id: String

        var body: some View {
            AISettings()
        }
    }
}

// MARK: - Store

@MainActor
@Observable
final class AISettingsStore {
    private(set) var config: WSAIConfig?
    private(set) var provider: WSProvider?
    private(set) var models: WSModelList?
    var draft = WSAIValues()
    private(set) var base = WSAIValues()
    private(set) var auto: [String] = []
    private(set) var error: String?
    private(set) var saving = false
    private(set) var savingAuto = false

    @ObservationIgnored private var api: APIClient?

    func attach(_ api: APIClient) { self.api = api }

    var dirty: Bool { config != nil && draft != base }
    var valid: Bool { draft.tone.count <= WSAIValues.toneMax && draft.instructions.count <= WSAIValues.instructionsMax && !draft.tone.trimmingCharacters(in: .whitespaces).isEmpty }
    var configured: Bool { (config?.provider.isEmpty == false) || provider?.key != nil }

    func load(admin: Bool) async {
        guard let api else { return }
        do {
            let fresh: WSAIConfig = try await api.get("v1/ai/config")
            config = fresh
            let keep = dirty
            base = fresh.features
            if !keep { draft = base }
            auto = fresh.assistantAuto
            error = nil
        } catch {
            if !error.isCancellation, config == nil { self.error = error.userMessage }
        }
        models = try? await api.get("v1/ai/models")
        if admin { provider = try? await api.get("v1/ai/provider") }
    }

    func discard() { draft = base }

    func save() async throws {
        guard let api else { return }
        saving = true
        defer { saving = false }
        var body: [String: JSONValue] = [:]
        let d = draft
        if d.enabled != base.enabled { body["enabled"] = .bool(d.enabled) }
        if d.assistant != base.assistant { body["assistant"] = .bool(d.assistant) }
        if d.assistantPeer != base.assistantPeer { body["assistant_peer"] = .bool(d.assistantPeer) }
        if d.autoTriage != base.autoTriage { body["auto_triage"] = .bool(d.autoTriage) }
        if d.autoSummarize != base.autoSummarize { body["auto_summarize"] = .bool(d.autoSummarize) }
        if d.widgetAnswers != base.widgetAnswers { body["widget_answers"] = .bool(d.widgetAnswers) }
        if d.draftTools != base.draftTools { body["draft_tools"] = .bool(d.draftTools) }
        if d.tone != base.tone { body["tone"] = .string(d.tone.trimmingCharacters(in: .whitespacesAndNewlines)) }
        if d.replyLanguage != base.replyLanguage { body["reply_language"] = .string(d.replyLanguage) }
        if d.instructions != base.instructions { body["instructions"] = .string(d.instructions.trimmingCharacters(in: .whitespacesAndNewlines)) }
        if d.model != base.model { body["model"] = .string(d.model) }
        if d.fastModel != base.fastModel { body["fast_model"] = .string(d.fastModel) }
        guard !body.isEmpty else { return }
        let _: EmptyBody = try await api.patch("v1/ai/settings", body: JSONValue.object(body))
        base = d
        await reloadConfig()
    }

    /// Ollie's no-ask changes save at once, apart from the rest of the page.
    func setAuto(_ tool: String, _ on: Bool) async throws {
        guard let api else { return }
        let before = auto
        var next = auto.filter { $0 != tool }
        if on { next.append(tool) }
        auto = next
        savingAuto = true
        defer { savingAuto = false }
        do {
            let _: EmptyBody = try await api.patch("v1/ai/settings", body: ["assistant_auto": next])
        } catch {
            auto = before
            throw error
        }
    }

    func removeKey() async throws {
        guard let api else { return }
        try await api.delete("v1/ai/provider")
        provider = try? await api.get("v1/ai/provider")
        await reloadConfig()
    }

    private func reloadConfig() async {
        guard let api, let fresh: WSAIConfig = try? await api.get("v1/ai/config") else { return }
        config = fresh
        if !dirty { base = fresh.features; draft = base }
        auto = fresh.assistantAuto
    }
}

// MARK: - Provider key

/// Adding or replacing the workspace's own provider key: test it, then save it.
struct WSProviderKeySheet: View {
    let current: WSProviderKey?
    let saved: () async -> Void

    @Environment(AppEnvironment.self) private var env
    @Environment(\.dismiss) private var dismiss
    @State private var kind = "openrouter"
    @State private var key = ""
    @State private var baseURL = ""
    @State private var model = ""
    @State private var fastModel = ""
    @State private var testing = false
    @State private var saving = false
    @State private var result: WSProviderTest?
    @State private var error: String?

    private var body_: [String: JSONValue] {
        var body: [String: JSONValue] = ["kind": .string(kind), "api_key": .string(key.trimmingCharacters(in: .whitespacesAndNewlines))]
        let base = baseURL.trimmingCharacters(in: .whitespaces)
        if kind == "openai", !base.isEmpty { body["base_url"] = .string(base) }
        let main = model.trimmingCharacters(in: .whitespaces)
        if !main.isEmpty { body["model"] = .string(main) }
        let fast = fastModel.trimmingCharacters(in: .whitespaces)
        if !fast.isEmpty { body["fast_model"] = .string(fast) }
        return body
    }

    private var ready: Bool {
        key.trimmingCharacters(in: .whitespaces).count >= 8
            && (kind != "openai" || (baseURL.hasPrefix("https://") && !model.trimmingCharacters(in: .whitespaces).isEmpty))
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                AccountSheetHeader(
                    title: current == nil ? "Use your own key" : "Replace key",
                    subtitle: "The key is stored encrypted and never shown again. Test it before saving."
                ) { dismiss() }
                AccountSegmented(options: ["openrouter", "anthropic", "openai"], selection: $kind) {
                    $0 == "openai" ? "OpenAI-compatible" : WSProviderKey.providerName($0)
                }
                ODTextField(label: "API key", text: $key, prompt: "Paste the key", secure: true, submitLabel: .done)
                if kind == "openai" {
                    ODTextField(label: "Base URL", text: $baseURL, prompt: "https://api.openai.com/v1", contentType: .URL, keyboard: .URL, submitLabel: .next)
                }
                ODTextField(label: kind == "openai" ? "Model" : "Model (optional)", text: $model, prompt: kind == "anthropic" ? "claude-sonnet" : "gpt-5", submitLabel: .next)
                ODTextField(label: "Fast model (optional)", text: $fastModel, prompt: "Uses the model when empty", submitLabel: .done)
                if let result {
                    Notice(
                        text: result.ok
                            ? "The key works.\(result.models.isEmpty ? "" : " \(wsPlural(result.models.count, "1 model", "\(result.models.count) models")) available.")"
                            : (result.message ?? "The provider refused the key."),
                        tone: result.ok ? .green : .red,
                        symbol: result.ok ? "checkmark.circle" : "xmark.octagon"
                    )
                }
                FormError(text: error)
                HStack(spacing: 10) {
                    Button(action: test) { ButtonLabel(title: "Test", busy: testing) }
                        .buttonStyle(OutlineButtonStyle())
                        .disabled(!ready || testing || saving)
                    Button(action: save) { ButtonLabel(title: "Save key", busy: saving) }
                        .buttonStyle(InkButtonStyle())
                        .disabled(!ready || testing || saving)
                }
            }
            .padding(22)
            .motion(Motion.snappy, value: result?.ok)
        }
        .scrollDismissesKeyboard(.interactively)
        .background(Theme.surface)
        .onAppear {
            if let current {
                kind = current.kind
                model = current.model
            }
        }
        .onChange(of: kind) { _, _ in result = nil }
        .onChange(of: key) { _, _ in result = nil }
    }

    private func test() {
        testing = true
        error = nil
        Task {
            defer { testing = false }
            do {
                result = try await env.session.api.post("v1/ai/provider/test", body: JSONValue.object(body_))
            } catch {
                self.error = error.userMessage
            }
        }
    }

    private func save() {
        saving = true
        error = nil
        Task {
            defer { saving = false }
            do {
                let _: EmptyBody = try await env.session.api.send(APIRequest(method: "PUT", path: "v1/ai/provider", body: try JSONEncoder().encode(JSONValue.object(body_))))
                key = ""
                Haptics.success()
                env.toasts.show("Key saved", kind: .success)
                await saved()
                dismiss()
            } catch {
                Haptics.error()
                self.error = error.userMessage
            }
        }
    }
}
