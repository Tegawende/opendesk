import SwiftUI

/// Settings › Routing (contracts/teams.md "Routing"): how a new ticket gets its team. The
/// address or widget decides first, then the first enabled rule that matches, then the team's
/// assignment. Rules reorder from their menus; the test panel shows where a sample would go.
struct RoutingSettings: View {
    @Environment(AppEnvironment.self) private var env
    @State private var store = RoutingStore()
    @State private var confirm: AccountConfirm?

    var body: some View {
        AccountPage(title: "Routing") {
            Text("How new tickets find their team: the address or widget first, then rules, triage and automations.")
                .font(ODFont.callout)
                .foregroundStyle(Theme.fg3)
                .padding(.horizontal, 4)
            if !env.session.isAdmin {
                EmptyState(symbol: "lock", title: "Only admins can see routing", message: "Ask a workspace admin to change how new tickets find their team.")
                    .padding(.top, 20)
            } else {
                RoutingFlow()
                ChannelLoading(loaded: store.loaded, error: store.rules.isEmpty ? store.error : nil, retry: reload) {
                    rules
                    RoutingTestPanel(store: store)
                }
            }
        }
        .toolbar {
            if env.session.isAdmin {
                ToolbarItem(placement: .topBarTrailing) {
                    Button { env.router.openSetting(.routing, id: "new") } label: { Image(systemName: "plus") }
                        .accessibilityLabel("New rule")
                }
            }
        }
        .accountConfirm($confirm)
        .refreshable { await store.load() }
        .onAppear {
            store.attach(env.session.api)
            if env.session.isAdmin { reload() }
        }
    }

    private func reload() { Task { await store.load() } }

    @ViewBuilder
    private var rules: some View {
        if store.rules.isEmpty {
            AccountSection(title: "Rules") {
                VStack(spacing: 14) {
                    EmptyState(
                        symbol: "arrow.triangle.branch",
                        title: "Route by what a ticket says",
                        message: "Addresses and widgets already send tickets to their team. Add a rule to send, say, every invoice question to Billing whatever address it came in on.",
                        actionTitle: "New rule",
                        action: { env.router.openSetting(.routing, id: "new") }
                    )
                    VStack(alignment: .leading, spacing: 6) {
                        Label("Without rules, each ticket goes to its address's or widget's team.", systemImage: "tray")
                        Label("Rules run top to bottom, and the first match decides.", systemImage: "arrow.triangle.branch")
                    }
                    .font(ODFont.sans(12.5))
                    .foregroundStyle(Theme.fg3)
                }
                .padding(.vertical, 18)
                .padding(.horizontal, 12)
            }
        } else {
            AccountSection(title: "Rules · first match wins", footer: "Checked from the top for every new ticket. Use Move up and Move down to change the order.") {
                ForEach(Array(store.rules.enumerated()), id: \.element.id) { index, rule in
                    if index > 0 { AccountDivider(tiles: false) }
                    RoutingRuleRow(rule: rule, index: index, summary: store.summary(rule, env: env)) { on in
                        toggle(rule, on)
                    }
                    .contextMenu { menu(rule, index) }
                }
            }
        }
    }

    @ViewBuilder
    private func menu(_ rule: RoutingRule, _ index: Int) -> some View {
        Button("Edit rule", systemImage: "arrow.triangle.branch") { env.router.openSetting(.routing, id: "rule/\(rule.id)") }
        if index > 0 { Button("Move up", systemImage: "arrow.up") { move(index, by: -1) } }
        if index < store.rules.count - 1 { Button("Move down", systemImage: "arrow.down") { move(index, by: 1) } }
        Divider()
        Button("Delete rule", systemImage: "trash", role: .destructive) {
            confirm = AccountConfirm(title: "Delete this rule?", message: "\(rule.name) stops routing new tickets. Tickets it already routed stay where they are.", confirmTitle: "Delete rule") {
                do {
                    try await store.remove(rule.id)
                    env.toasts.show("\(rule.name) deleted", kind: .success)
                } catch { env.toasts.error(error) }
            }
        }
    }

    private func toggle(_ rule: RoutingRule, _ on: Bool) {
        Task {
            do { try await store.setEnabled(rule.id, on) } catch { env.toasts.error(error) }
        }
    }

    private func move(_ index: Int, by offset: Int) {
        Haptics.selection()
        Task {
            do { try await store.move(index, by: offset) } catch { env.toasts.error(error) }
        }
    }

    struct Detail: View {
        let id: String

        var body: some View {
            RoutingRuleEditor(ruleID: id == "new" ? nil : String(id.hasPrefix("rule/") ? id.dropFirst(5) : Substring(id)))
        }
    }
}

/// The four steps a new ticket goes through, in order.
private struct RoutingFlow: View {
    @Environment(AppEnvironment.self) private var env

    private struct Step: Identifiable {
        let symbol: String
        let tone: Tone
        let title: String
        let hint: String
        let links: [(String, SettingsPage)]
        var id: String { title }
    }

    private var steps: [Step] {
        [
            Step(symbol: "tray", tone: .blue, title: "Address, widget or API", hint: "Email goes to its address's team, chat to the topic's team, else the widget's. The API can name one.", links: [("Mailboxes", .mailboxes)]),
            Step(symbol: "arrow.triangle.branch", tone: .violet, title: "Rules, first match wins", hint: "A matching rule moves the ticket to its team, whatever step 1 chose.", links: []),
            Step(symbol: "flowchart", tone: .amber, title: "Triage and automations", hint: "With no team yet, triage can pick one. Automations run last and can still change it.", links: [("Triage", .triage), ("Automations", .automations)]),
            Step(symbol: "shuffle", tone: .teal, title: "Assignment", hint: "The team's method picks a person, else the address's, else the workspace's.", links: [("Assignment", .assignment)]),
        ]
    }

    var body: some View {
        LazyVGrid(columns: [GridItem(.flexible(), spacing: 8), GridItem(.flexible(), spacing: 8)], spacing: 8) {
            ForEach(Array(steps.enumerated()), id: \.element.id) { index, step in
                VStack(alignment: .leading, spacing: 6) {
                    HStack(spacing: 6) {
                        Image(systemName: step.symbol)
                            .font(.system(size: 11, weight: .semibold))
                            .foregroundStyle(step.tone.text)
                            .frame(width: 22, height: 22)
                            .background(step.tone.soft, in: RoundedRectangle(cornerRadius: 6, style: .continuous))
                        Text("\(index + 1)").font(ODFont.mono(10.5)).foregroundStyle(Theme.fg4)
                    }
                    Text(step.title).font(ODFont.sans(13, weight: .medium)).foregroundStyle(Theme.fg).lineLimit(2)
                    Text(step.hint).font(ODFont.sans(11.5)).foregroundStyle(Theme.fg3).fixedSize(horizontal: false, vertical: true)
                    Spacer(minLength: 0)
                    if !step.links.isEmpty {
                        HStack(spacing: 10) {
                            ForEach(step.links, id: \.0) { link in
                                Button(link.0) { env.router.accountPath.append(.setting(link.1)) }
                                    .font(ODFont.sans(12, weight: .medium))
                                    .foregroundStyle(Theme.fg2)
                                    .underline()
                            }
                        }
                    }
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
                .padding(11)
                .background(Theme.surface, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
                .overlay(RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous).strokeBorder(Theme.line, lineWidth: 1))
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityLabel("How a new ticket gets its team")
    }
}

private struct RoutingRuleRow: View {
    let rule: RoutingRule
    let index: Int
    let summary: String
    let toggle: (Bool) -> Void

    @Environment(AppEnvironment.self) private var env

    var body: some View {
        HStack(spacing: 10) {
            SettingsDetailLink(page: .routing, id: "rule/\(rule.id)") {
                HStack(spacing: 10) {
                    Text("\(index + 1)").font(ODFont.mono(11)).foregroundStyle(Theme.fg4).frame(width: 18, alignment: .trailing)
                    VStack(alignment: .leading, spacing: 3) {
                        Text(rule.name).font(ODFont.sans(15, weight: .medium)).foregroundStyle(Theme.fg).lineLimit(1)
                        Text("\(summary) · \(ChannelFormat.plural(rule.runCount, "run"))").font(ODFont.footnote).foregroundStyle(Theme.fg3).lineLimit(2)
                        HStack(spacing: 4) {
                            Image(systemName: "arrow.right").font(.system(size: 9, weight: .semibold)).foregroundStyle(Theme.fg4)
                            if let team = rule.teamID.flatMap({ env.directory.team($0) }) {
                                Text(team.name).font(ODFont.sans(12.5, weight: .medium)).foregroundStyle(Theme.fg2)
                            } else {
                                Text("No team").font(ODFont.sans(12.5, weight: .medium)).foregroundStyle(Tone.red.text)
                            }
                        }
                    }
                    Spacer(minLength: 4)
                }
                .contentShape(Rectangle())
            }
            Toggle("", isOn: Binding(get: { rule.enabled }, set: toggle))
                .labelsHidden()
                .tint(Tone.green.solid)
                .accessibilityLabel("Turn \(rule.name) on or off")
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 11)
        .opacity(rule.enabled ? 1 : 0.6)
    }
}

// MARK: - Test

private struct RoutingTestPanel: View {
    let store: RoutingStore

    @Environment(AppEnvironment.self) private var env
    @State private var channel = "email"
    @State private var to = ""
    @State private var from = ""
    @State private var subject = ""
    @State private var message = ""
    @State private var widgetID = ""
    @State private var topic = ""
    @State private var page = ""
    @State private var running = false
    @State private var result: RoutingTestResult?

    var body: some View {
        let addresses = store.mailboxes.flatMap { m in [m.email] + m.aliases }
        let address = to.isEmpty ? (addresses.first ?? "") : to
        let widget = store.widgets.first { $0.id == (widgetID.isEmpty ? store.widgets.first?.id : widgetID) }
        AccountSection(title: "Test routing", footer: "Write a sample email or chat and see where it would go. Nothing is created and no rule counts a run.") {
            VStack(alignment: .leading, spacing: 12) {
                AccountSegmented(options: ["email", "chat"], selection: Binding(get: { channel }, set: { channel = $0; result = nil })) { $0 == "email" ? "Email" : "Chat" }
                if channel == "email" {
                    ChannelMenuField(label: "Sent to", value: address.isEmpty ? "Choose an address" : address) {
                        ForEach(store.mailboxes) { mailbox in
                            Section(mailbox.name) {
                                Button(mailbox.email) { to = mailbox.email; result = nil }
                                ForEach(mailbox.aliases, id: \.self) { alias in Button("\(alias) (alias)") { to = alias; result = nil } }
                            }
                        }
                    }
                } else {
                    ChannelMenuField(label: "Widget", value: widget?.name ?? "Choose a widget") {
                        ForEach(store.widgets) { item in Button(item.name) { widgetID = item.id; topic = ""; result = nil } }
                    }
                    if let widget, !widget.settings.topics.isEmpty {
                        ChannelMenuField(label: "Topic", value: widget.settings.topics.first { $0.id == topic }?.label ?? "No topic") {
                            Button("No topic") { topic = ""; result = nil }
                            ForEach(widget.settings.topics) { item in Button(item.label) { topic = item.id; result = nil } }
                        }
                    }
                    ODTextField(label: "Page", text: $page, prompt: "https://margin.app/pricing", keyboard: .URL)
                }
                ODTextField(label: "From", text: $from, prompt: "ada@example.com", contentType: .emailAddress, keyboard: .emailAddress)
                if channel == "email" {
                    ODTextField(label: "Subject", text: $subject, prompt: "Question about my invoice")
                }
                AccountTextEditor(label: channel == "email" ? "Message" : "First message", text: $message, prompt: "Hi, I was charged twice this month.", minHeight: 80)
                Button { run(address: address, widgetID: widget?.id ?? "") } label: { ButtonLabel(title: "Test", busy: running) }
                    .buttonStyle(InkButtonStyle())
                    .disabled(running || (channel == "email" ? address.isEmpty : widget == nil))
            }
            .padding(14)
            if let result {
                AccountDivider(tiles: false)
                RoutingTestResultView(result: result, store: store)
            }
        }
        .onChange(of: from) { result = nil }
        .onChange(of: subject) { result = nil }
        .onChange(of: message) { result = nil }
        .onChange(of: page) { result = nil }
    }

    private func run(address: String, widgetID: String) {
        var body: ChannelBody = ["channel": .string(channel), "body": .string(message)]
        let sender = from.trimmingCharacters(in: .whitespaces)
        if !sender.isEmpty { body["from"] = .string(sender) }
        if channel == "email" {
            body["to"] = .string(address)
            body["subject"] = .string(subject)
        } else {
            body["widget_id"] = .string(widgetID)
            if !topic.isEmpty { body["topic"] = .string(topic) }
            let url = page.trimmingCharacters(in: .whitespaces)
            if !url.isEmpty { body["page_url"] = .string(url) }
        }
        running = true
        Task {
            do {
                result = try await store.test(body)
                Haptics.success()
            } catch { env.toasts.error(error) }
            running = false
        }
    }
}

private struct RoutingTestResultView: View {
    let result: RoutingTestResult
    let store: RoutingStore

    @Environment(AppEnvironment.self) private var env

    var body: some View {
        let decided = result.steps.firstIndex { $0.matched == "true" }
        let via = result.via
        let mailbox = store.mailboxes.first { $0.id == result.mailboxID }
        VStack(alignment: .leading, spacing: 10) {
            SectionLabel("Steps")
            VStack(alignment: .leading, spacing: 6) {
                step(via == "address" || via == "widget" || via == "topic" ? "match" : "skip",
                     via == "address" || (via == "rule" && mailbox != nil) ? "The address's team" : via == "topic" ? "The topic's team" : via == "widget" ? "The widget's team" : "No team from the address or widget")
                ForEach(Array(result.steps.enumerated()), id: \.element.id) { index, item in
                    let skipped = decided.map { index > $0 } ?? false
                    let state = item.matched == "true" ? "match" : item.matched == "unknown" ? "unknown" : skipped ? "skip" : "miss"
                    step(state, item.name + (item.matched == "unknown" ? " · needs Ollie, so no match" : skipped ? " · not checked" : ""))
                }
            }
            VStack(spacing: 0) {
                line("Team", result.teamID.flatMap { env.directory.team($0)?.name } ?? "No team")
                if store.brands.count > 1, let brand = store.brands.first(where: { $0.id == result.brandID }) {
                    line("Brand", brand.name)
                }
                line("Replies from", mailbox?.email ?? "No address", mono: mailbox != nil)
                line("Assignment", ["round_robin": "Round robin", "load_balanced": "Load balanced"][result.assignMode] ?? "Off")
                if !result.skills.isEmpty { line("Required skills", result.skills.joined(separator: ", ")) }
                line("Decided by", why, last: true)
            }
            .background(Theme.surface, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous).strokeBorder(Theme.line, lineWidth: 1))
        }
        .padding(14)
        .background(Theme.sunken.opacity(0.5))
    }

    private var why: String {
        guard let via = result.via else { return "Nothing, it stays without a team" }
        if via == "rule" { return "Rule: \(result.ruleName ?? "")" }
        return [
            "address": "The address's team", "widget": "The widget's team", "topic": "The topic's team", "triage": "Triage",
            "automation": "An automation", "agent": "Moved by a teammate", "api": "Set through the API",
        ][via] ?? via
    }

    private func step(_ state: String, _ text: String) -> some View {
        HStack(spacing: 8) {
            Image(systemName: state == "match" ? "checkmark" : state == "unknown" ? "circle.dashed" : state == "skip" ? "minus" : "xmark")
                .font(.system(size: 8, weight: .bold))
                .foregroundStyle(state == "match" ? Tone.green.text : state == "unknown" ? Tone.violet.text : Theme.fg4)
                .frame(width: 16, height: 16)
                .background(state == "match" ? Tone.green.soft : state == "unknown" ? Tone.violet.soft : Theme.active, in: Circle())
            Text(text).font(ODFont.sans(13)).foregroundStyle(state == "skip" ? Theme.fg4 : state == "miss" ? Theme.fg3 : Theme.fg).lineLimit(2)
        }
    }

    private func line(_ label: String, _ value: String, mono: Bool = false, last: Bool = false) -> some View {
        VStack(spacing: 0) {
            HStack(alignment: .firstTextBaseline) {
                Text(label).font(ODFont.sans(13)).foregroundStyle(Theme.fg3)
                Spacer(minLength: 10)
                Text(value).font(mono ? ODFont.mono(12) : ODFont.sans(13, weight: .medium)).foregroundStyle(Theme.fg).multilineTextAlignment(.trailing)
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 9)
            if !last { Hairline(inset: 12) }
        }
    }
}

// MARK: - Rule editor

/// One rule: a name, on or off, all or any of up to 20 conditions, the team and required skills.
struct RoutingRuleEditor: View {
    let ruleID: String?

    @Environment(AppEnvironment.self) private var env
    @State private var store = RoutingStore()
    @State private var draft = RoutingDraft()
    @State private var base = RoutingDraft()
    @State private var started = false
    @State private var saving = false
    @State private var tried = false
    @State private var confirm: AccountConfirm?

    private var rule: RoutingRule? { ruleID.flatMap { id in store.rules.first { $0.id == id } } }

    var body: some View {
        AccountPage(title: rule?.name ?? (ruleID == nil ? "New rule" : "Rule")) {
            if !env.session.isAdmin {
                EmptyState(symbol: "lock", title: "Only admins can see routing", message: "Ask a workspace admin to change how new tickets find their team.")
                    .padding(.top, 20)
            } else if ruleID != nil, store.loaded, rule == nil {
                EmptyState(symbol: "arrow.triangle.branch", title: "This rule is gone", message: "It was deleted, or the link is wrong.")
                    .padding(.top, 30)
            } else if ruleID != nil, !store.loaded {
                SkeletonRows(count: 5)
            } else {
                form
            }
        }
        .settingsSaveBar(
            dirty: env.session.isAdmin && (ruleID == nil || draft != base),
            guarded: draft != base,
            saveTitle: ruleID == nil ? "Create rule" : "Save changes",
            saving: saving,
            save: save
        ) {
            if ruleID == nil { env.router.accountPath.removeLast() } else { draft = base }
        }
        .accountConfirm($confirm)
        .onAppear {
            store.attach(env.session.api)
            Task {
                await store.load()
                await store.loadSkills()
                guard !started else { return }
                started = true
                let next = rule.map { RoutingDraft($0, skills: store.skills) } ?? RoutingDraft()
                draft = next
                base = next
            }
        }
    }

    @ViewBuilder
    private var form: some View {
        let problems = draft.problems
        Text(rule.map { "Rule \($0.position + 1), matched \(ChannelFormat.plural($0.runCount, "ticket"))." } ?? "When a new ticket matches, it goes to the team you pick.")
            .font(ODFont.callout)
            .foregroundStyle(Theme.fg3)
            .padding(.horizontal, 4)

        AccountSection {
            VStack(alignment: .leading, spacing: 8) {
                ODTextField(label: "Name", text: Binding(get: { draft.name }, set: { draft.name = String($0.prefix(80)) }), prompt: "Invoices to Billing")
                if tried, problems.name { FormError(text: "Give the rule a name.") }
            }
            .padding(14)
            AccountDivider(tiles: false)
            AccountToggleRow(title: "On", subtitle: draft.enabled ? "Checked for every new ticket." : "Skipped until you turn it on.", isOn: $draft.enabled)
        }

        AccountSection(title: "When", footer: "The conditions a new ticket is checked against.") {
            VStack(alignment: .leading, spacing: 12) {
                AccountSegmented(options: ["any", "all"], selection: $draft.match) { $0 == "any" ? "Any matches" : "All match" }
                ChannelConditionsEditor(
                    conditions: $draft.conditions,
                    fields: store.fields(env: env),
                    joiner: draft.match == "all" ? "and" : "or",
                    max: 20,
                    minimum: 1,
                    addLabel: "Add a condition"
                )
                if draft.conditions.contains(where: { $0.field == "ai_decide" }) {
                    Notice(text: "Ollie can't answer while a ticket is being routed, so this condition never matches here and the test shows it as unknown. To route on Ollie's answer, use a new ticket automation that sets the team.", tone: .violet, symbol: "sparkles")
                }
                if problems.ai { FormError(text: "Ask Ollie at most 3 questions in one rule.") }
                if tried, problems.conditions { FormError(text: "Fill in every condition.") }
            }
            .padding(14)
        }

        AccountSection(title: "Then", footer: "Where matching tickets go.") {
            VStack(alignment: .leading, spacing: 14) {
                ChannelTeamMenu(label: "Send to team", teamID: $draft.teamID, none: "Choose a team", hint: "The team's assignment method then picks a person.")
                if tried, problems.team { FormError(text: "Choose the team this rule sends tickets to.") }
                VStack(alignment: .leading, spacing: 8) {
                    Text("Required skills").font(ODFont.sans(13, weight: .medium)).foregroundStyle(Theme.fg2)
                    let skills = store.skills.filter { !$0.archived }
                    if skills.isEmpty {
                        Text("No skills yet").font(ODFont.footnote).foregroundStyle(Theme.fg4)
                    } else {
                        FlowLayout(spacing: 6) {
                            ForEach(skills) { skill in
                                let on = draft.skillIDs.contains(skill.id)
                                Button {
                                    if on { draft.skillIDs.removeAll { $0 == skill.id } } else { draft.skillIDs.append(skill.id) }
                                    Haptics.selection()
                                } label: {
                                    Label(skill.name, systemImage: on ? "checkmark" : "sparkles")
                                        .font(ODFont.sans(13, weight: .medium))
                                        .foregroundStyle(on ? Tone.violet.text : Theme.fg2)
                                        .padding(.horizontal, 10)
                                        .frame(height: 30)
                                        .background(on ? Tone.violet.soft : Theme.sunken, in: Capsule())
                                        .overlay(Capsule().strokeBorder(on ? Tone.violet.line : Theme.line, lineWidth: 1))
                                }
                                .buttonStyle(TapScaleStyle())
                                .accessibilityAddTraits(on ? .isSelected : [])
                            }
                        }
                    }
                    ChannelHint("Only teammates with every selected skill can receive this ticket automatically.")
                }
            }
            .padding(14)
        }

        if let rule {
            AccountSection(title: "Delete") {
                ChannelActionRow(title: "Delete this rule?", subtitle: "New tickets stop being routed by it.", button: "Delete rule", buttonTone: .red) {
                    confirm = AccountConfirm(title: "Delete this rule?", message: "\(rule.name) stops routing new tickets. Tickets it already routed stay where they are.", confirmTitle: "Delete rule") {
                        do {
                            try await store.remove(rule.id)
                            env.toasts.show("\(rule.name) deleted", kind: .success)
                            env.router.accountPath.removeLast()
                        } catch { env.toasts.error(error) }
                    }
                }
            }
        }
    }

    private func save() {
        tried = true
        let problems = draft.problems
        if problems.any {
            Haptics.error()
            env.toasts.show(problems.team ? "Choose the team this rule sends tickets to." : problems.name ? "Give the rule a name." : problems.ai ? "Ask Ollie at most 3 questions in one rule." : "Fill in every condition.", kind: .error)
            return
        }
        saving = true
        let body = draft.body(skills: store.skills)
        Task {
            do {
                let saved = try await store.save(ruleID, body)
                let next = RoutingDraft(saved, skills: store.skills)
                draft = next
                base = next
                Haptics.success()
                if ruleID == nil {
                    env.toasts.show("\(saved.name) created", kind: .success)
                    env.router.accountPath.removeLast()
                    env.router.openSetting(.routing, id: "rule/\(saved.id)")
                } else {
                    env.toasts.show("Rule saved", kind: .success)
                }
            } catch {
                Haptics.error()
                env.toasts.show("Couldn't save the rule. \(error.userMessage)", kind: .error)
            }
            saving = false
        }
    }
}

struct RoutingDraft: Equatable {
    var name = ""
    var enabled = true
    var match = "any"
    var conditions = [ChannelCondition(field: "subject", op: "contains")]
    var teamID: String?
    var skillIDs: [String] = []

    init() {}

    init(_ rule: RoutingRule, skills: [RoutingSkill]) {
        name = rule.name
        enabled = rule.enabled
        match = rule.match
        conditions = rule.conditions
        teamID = rule.teamID
        // Archived skills cannot be picked, so they leave the draft rather than ride along.
        skillIDs = skills.isEmpty ? rule.skillIDs : rule.skillIDs.filter { id in skills.contains { $0.id == id && !$0.archived } }
    }

    struct Problems {
        var name: Bool
        var team: Bool
        var conditions: Bool
        var ai: Bool
        var any: Bool { name || team || conditions || ai }
    }

    var problems: Problems {
        let bad = conditions.contains { c in
            c.field == "ai_decide" ? (c.question ?? "").trimmingCharacters(in: .whitespaces).count < 3 : c.value.trimmingCharacters(in: .whitespaces).isEmpty
        }
        return Problems(
            name: name.trimmingCharacters(in: .whitespaces).isEmpty,
            team: teamID == nil,
            conditions: bad || conditions.isEmpty,
            ai: conditions.filter { $0.field == "ai_decide" }.count > 3
        )
    }

    func body(skills: [RoutingSkill]) -> ChannelBody {
        let conditionsJSON = (try? JSONValue.from(conditions)) ?? .array([])
        return [
            "name": .string(name.trimmingCharacters(in: .whitespaces)),
            "enabled": .bool(enabled),
            "match": .string(match),
            "team_id": teamID.map { .string($0) } ?? .null,
            "skill_ids": .chStrings(skillIDs.filter { id in skills.contains { $0.id == id && !$0.archived } }),
            "conditions": conditionsJSON,
        ]
    }
}

// MARK: - Store

@MainActor
@Observable
final class RoutingStore {
    private(set) var rules: [RoutingRule] = []
    private(set) var skills: [RoutingSkill] = []
    private(set) var mailboxes: [SettingsMailbox] = []
    private(set) var brands: [SettingsBrand] = []
    private(set) var widgets: [ChatWidget] = []
    private(set) var loaded = false
    private(set) var error: String?

    @ObservationIgnored private var api: APIClient?

    func attach(_ api: APIClient) { self.api = api }

    func load() async {
        guard let api else { return }
        do {
            let page: ChannelList<RoutingRule> = try await api.get("v1/routing-rules")
            rules = page.data.sorted { $0.position < $1.position }
            let limit = [URLQueryItem(name: "limit", value: "100")]
            async let boxes: ChannelList<SettingsMailbox>? = try? api.get("v1/mailboxes", query: limit)
            async let brandPage: ChannelList<SettingsBrand>? = try? api.get("v1/brands", query: limit)
            async let widgetPage: ChannelList<ChatWidget>? = try? api.get("v1/widgets", query: limit)
            if let page = await boxes { mailboxes = page.data }
            if let page = await brandPage { brands = page.data }
            if let page = await widgetPage { widgets = page.data }
            error = nil
        } catch {
            if !error.isCancellation { self.error = error.userMessage }
        }
        loaded = true
    }

    func loadSkills() async {
        guard let api else { return }
        if let page: ChannelList<RoutingSkill> = try? await api.get("v1/skills") { skills = page.data }
    }

    func setEnabled(_ id: String, _ on: Bool) async throws {
        guard let api, let index = rules.firstIndex(where: { $0.id == id }) else { return }
        let before = rules[index]
        rules[index].enabled = on
        do {
            let saved: RoutingRule = try await api.patch("v1/routing-rules/\(id)", body: ["enabled": on])
            if let at = rules.firstIndex(where: { $0.id == id }) { rules[at] = saved }
        } catch {
            if let at = rules.firstIndex(where: { $0.id == id }) { rules[at] = before }
            throw error
        }
    }

    func move(_ index: Int, by offset: Int) async throws {
        guard let api else { return }
        let to = index + offset
        guard rules.indices.contains(index), rules.indices.contains(to) else { return }
        let before = rules
        rules.swapAt(index, to)
        for i in rules.indices { rules[i].position = i }
        do {
            let _: EmptyBody = try await api.post("v1/routing-rules/reorder", body: ["ids": rules.map(\.id)])
        } catch {
            rules = before
            throw error
        }
    }

    func remove(_ id: String) async throws {
        guard let api else { return }
        try await api.delete("v1/routing-rules/\(id)")
        rules.removeAll { $0.id == id }
    }

    func save(_ id: String?, _ body: ChannelBody) async throws -> RoutingRule {
        guard let api else { throw CancellationError() }
        let saved: RoutingRule
        if let id {
            saved = try await api.patch("v1/routing-rules/\(id)", body: body)
        } else {
            saved = try await api.post("v1/routing-rules", body: body, idempotencyKey: UUID().uuidString)
        }
        if let index = rules.firstIndex(where: { $0.id == saved.id }) { rules[index] = saved } else { rules.append(saved) }
        return saved
    }

    func test(_ body: ChannelBody) async throws -> RoutingTestResult {
        guard let api else { throw CancellationError() }
        return try await api.post("v1/routing/test", body: body)
    }

    // MARK: Fields

    func fields(env: AppEnvironment) -> [ChannelConditionField] {
        let text = ["contains", "not_contains", "equals", "not_equals", "starts_with"]
        let ids = ["equals", "not_equals"]
        let addresses = mailboxes.flatMap { m in [m.email] + m.aliases }.map { ChannelChoice(value: $0, label: $0) }
        var fields = [
            ChannelConditionField(key: "received_at", label: "Address received at", group: "Where it arrived", ops: ["equals", "not_equals", "contains", "starts_with"], value: .choiceOrText(addresses, "legal@")),
            ChannelConditionField(key: "mailbox_id", label: "Mailbox", group: "Where it arrived", ops: ids, value: .choice(mailboxes.map { ChannelChoice(value: $0.id, label: $0.email) })),
        ]
        if brands.count > 1 {
            fields.append(ChannelConditionField(key: "brand_id", label: "Brand", group: "Where it arrived", ops: ids, value: .choice(brands.map { ChannelChoice(value: $0.id, label: $0.name) })))
        }
        fields += [
            ChannelConditionField(key: "widget_id", label: "Chat widget", group: "Where it arrived", ops: ids, value: .choice(widgets.map { ChannelChoice(value: $0.id, label: $0.name) })),
            ChannelConditionField(key: "channel", label: "Channel", group: "Where it arrived", ops: ids, value: .choice(Self.channels.map { ChannelChoice(value: $0.0, label: $0.1) })),
            ChannelConditionField(key: "page_url", label: "Page URL", group: "Where it arrived", ops: text, value: .text("/pricing")),
            ChannelConditionField(key: "topic", label: "Topic", group: "Where it arrived", ops: ids, value: .choice(widgets.flatMap { w in w.settings.topics.filter { !$0.isNew }.map { ChannelChoice(value: $0.id, label: $0.label, group: w.name) } })),
            ChannelConditionField(key: "from_email", label: "From email", group: "Who sent it", ops: text, value: .text("ada@example.com")),
            ChannelConditionField(key: "from_domain", label: "From domain", group: "Who sent it", ops: ["equals", "not_equals", "contains"], value: .text("acme.com")),
        ]
        for field in env.directory.fields(for: "contact") {
            let numeric = field.type == .number
            fields.append(ChannelConditionField(key: "contact.field.\(field.key)", label: "Contact: \(field.name)", group: "Who sent it", ops: numeric ? ["equals", "not_equals", "gt", "lt"] : text, value: numeric ? .number : .text("Text")))
        }
        fields += [
            ChannelConditionField(key: "subject", label: "Subject", group: "What it says", ops: text, value: .text("Text")),
            ChannelConditionField(key: "body", label: "Message", group: "What it says", ops: ["contains", "not_contains"], value: .text("Text")),
            ChannelConditionField(key: "ai_decide", label: "Ollie decides", group: "Ollie", ops: ["gt", "lt"], value: .ollie),
        ]
        return fields
    }

    static let channels = [("email", "Email"), ("chat", "Chat"), ("api", "API"), ("portal", "Portal"), ("phone", "Phone")]

    /// "Subject contains invoice, or 2 more" for list rows.
    func summary(_ rule: RoutingRule, env: AppEnvironment) -> String {
        guard let first = rule.conditions.first else { return "No conditions" }
        let fields = fields(env: env)
        func one(_ c: ChannelCondition) -> String {
            if c.field == "ai_decide" { return "Ollie decides: \(c.question ?? "")" }
            let field = fields.first { $0.key == c.field }
            let label = field?.label ?? (c.field.hasPrefix("contact.field.") ? "Contact: \(c.field.dropFirst(14))" : c.field)
            var value = c.value
            switch field?.value {
            case let .choice(choices), let .choiceOrText(choices, _): value = choices.first { $0.value == c.value }?.label ?? c.value
            default: break
            }
            return "\(label) \(ChannelConditionOps.label(c.op, field: c.field)) \(value)"
        }
        let head = one(first)
        if rule.conditions.count == 1 { return head }
        return "\(head), \(rule.match == "all" ? "and" : "or") \(rule.conditions.count - 1) more"
    }
}
