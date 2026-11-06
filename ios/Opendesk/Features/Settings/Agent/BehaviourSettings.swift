import SwiftUI

/// Settings > AI agent > Behaviour: how the agent answers chat widget visitors: whether it answers,
/// who it answers as, its model and limits, its guardrails, and the mailboxes where it answers email.
/// Unsaved edits are kept in the store, so the playground and test runs can try them first.
struct BehaviourSettings: View {
    @Environment(AppEnvironment.self) private var env
    @State private var edit = AgentBehaviour()
    @State private var started = false
    @State private var saving = false
    @State private var running = false

    /// Behaviour lists nothing of its own to open; a link to one lands on the page.
    struct Detail: View {
        let id: String

        var body: some View { BehaviourSettings() }
    }

    var body: some View {
        let store = env.agent
        let base = store.config?.behaviour ?? AgentBehaviour()
        let dirty = started && edit != base
        AccountPage(title: "Behaviour", spacing: 22) {
            AgentIntro(text: "How the AI agent answers chat widget visitors: what it knows, which model it uses and where it stops.")
            if store.config == nil {
                if let error = store.configError {
                    Panel { ErrorState(message: error) { Task { await store.loadConfig() } } }
                } else {
                    Panel { SkeletonRows(count: 4) }
                }
            } else {
                if !store.aiOn {
                    AgentOffNotice(title: "AI is off for this workspace.", message: "The agent can't answer anyone until AI is on.", action: "Open AI settings")
                }
                actions(store, dirty: dirty)
                Panel {
                    HStack(alignment: .top, spacing: 12) {
                        OllieMark(size: 22, state: edit.widgetAnswers ? .idle : .muted)
                            .frame(width: 32, height: 32)
                            .background(edit.widgetAnswers ? Tone.violet.soft : Theme.active, in: RoundedRectangle(cornerRadius: 9, style: .continuous))
                        VStack(alignment: .leading, spacing: 3) {
                            Text("Let the agent answer chats").font(ODFont.sans(15.5, weight: .medium)).foregroundStyle(Theme.fg)
                            Text("It replies to visitors in the chat widget, uses your tools and knowledge, and hands off to your team when it should. Replies are labelled as AI.")
                                .font(ODFont.footnote).foregroundStyle(Theme.fg3).fixedSize(horizontal: false, vertical: true)
                        }
                        Spacer(minLength: 8)
                        Toggle("Let the agent answer chats", isOn: $edit.widgetAnswers)
                            .labelsHidden()
                            .tint(Tone.green.solid)
                    }
                    .padding(14)
                    .sensoryFeedback(.selection, trigger: edit.widgetAnswers)
                }
                bots(store)
                VStack(alignment: .leading, spacing: 8) {
                    AgentGroupTitle(title: "Model and limits", hint: "A larger model answers harder questions and costs more per reply.")
                    Panel {
                        AgentModelRow(title: "Agent model", subtitle: "Each reply costs what it uses on this model, so a pricier model or a longer conversation costs more.", model: $edit.widgetModel)
                        AccountDivider()
                        AccountRow(symbol: "arrow.left.arrow.right", tone: .violet, title: "Replies per conversation", subtitle: "After this many replies in one conversation the agent hands off to your team.") {
                            AgentStepper(value: $edit.widgetMaxReplies, range: 1 ... 100)
                        }
                    }
                }
                VStack(alignment: .leading, spacing: 8) {
                    AgentGroupTitle(title: "Guardrails", hint: "Where the agent stops, and how much it may spend.")
                    Panel(padding: 14) {
                        AgentChipEditor(label: "Topics to decline", hint: "The agent politely declines these and offers a person instead.", items: $edit.widgetBlockedTopics, max: 20, prompt: "Legal advice", empty: "No blocked topics yet.", check: BotLimits.topicProblem)
                    }
                    Panel(padding: 14) {
                        VStack(alignment: .leading, spacing: 8) {
                            Text("Monthly spending cap").font(ODFont.sans(13, weight: .medium)).foregroundStyle(Theme.fg2)
                            Text("When the agent has spent this much of the AI balance in a calendar month, it goes quiet and your team takes over. Owners and admins get one notification.")
                                .font(ODFont.footnote).foregroundStyle(Theme.fg3).fixedSize(horizontal: false, vertical: true)
                            AgentUSDField(value: $edit.widgetMonthlyCapUSD, presets: [0, 5, 20, 50, 100])
                        }
                    }
                    Panel {
                        AccountToggleRow(symbol: "envelope.badge", tone: .blue, title: "Ask for an email before handing off", subtitle: "When the visitor hasn't shared an email, the agent asks for one so your team can follow up.", isOn: $edit.widgetCollectEmail)
                    }
                }
                email(store)
            }
        }
        // The draft stays in the store for the playground and test runs, so leaving keeps it.
        .settingsSaveBar(dirty: dirty, guarded: false, saving: saving) {
            Task { await save(store) }
        } discard: {
            withAnimation(Motion.snappy) { edit = base }
        }
        .onChange(of: edit) { _, next in
            guard started else { return }
            store.draft = next == (store.config?.behaviour ?? AgentBehaviour()) ? nil : next
        }
        .onChange(of: store.config?.behaviour) { old, next in
            // Follow the server until the first edit.
            guard let next, edit == (old ?? AgentBehaviour()) else { return }
            edit = next
        }
        .refreshable { await store.loadConfig() }
        .task {
            await store.loadConfig()
            if !started {
                edit = store.draft ?? store.config?.behaviour ?? AgentBehaviour()
                started = true
            }
            await store.loadBots()
            await store.loadMailboxes()
        }
    }

    private func actions(_ store: AgentStore, dirty: Bool) -> some View {
        HStack(spacing: 8) {
            Button {
                Task { await runTests(store) }
            } label: {
                if running { ProgressView().controlSize(.small) } else { Label(dirty ? "Run tests on changes" : "Run tests", systemImage: "checklist") }
            }
            .buttonStyle(ChipButtonStyle())
            .disabled(running)
            Button {
                env.router.accountPath.append(.setting(.playground))
            } label: {
                Label(dirty ? "Try changes in playground" : "Open playground", systemImage: "flask")
            }
            .buttonStyle(ChipButtonStyle(tone: .violet))
        }
    }

    private func bots(_ store: AgentStore) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .firstTextBaseline) {
                AgentGroupTitle(title: "Bots", hint: "Who answers your customers: their name, look and personality. Rules on this page apply to every bot.")
                Spacer()
                Button("Manage bots") { env.router.accountPath.append(.setting(.bots)) }
                    .buttonStyle(ChipButtonStyle())
            }
            Panel {
                if !store.bots.loaded {
                    SkeletonRows(count: 2)
                } else {
                    ForEach(Array(store.bots.value.enumerated()), id: \.element.id) { index, bot in
                        if index > 0 { AccountDivider() }
                        SettingsDetailLink(page: .bots, id: bot.id) {
                            HStack(spacing: 12) {
                                BotFace(look: bot.look, image: bot.imageURL, size: 30)
                                VStack(alignment: .leading, spacing: 2) {
                                    HStack(spacing: 6) {
                                        Text(bot.name).font(ODFont.sans(15)).foregroundStyle(Theme.fg)
                                        if bot.isDefault { TonePill(text: "Default", tone: .violet, height: 18) }
                                    }
                                    Text(bot.usageLine).font(ODFont.footnote).foregroundStyle(Theme.fg3).lineLimit(1)
                                }
                                Spacer()
                                Chevron()
                            }
                            .padding(.horizontal, 14)
                            .padding(.vertical, 10)
                        }
                    }
                }
            }
        }
    }

    private func email(_ store: AgentStore) -> some View {
        let answering = store.mailboxes.value.filter(\.aiAnswers)
        return VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .firstTextBaseline) {
                AgentGroupTitle(title: "Email", hint: "Mailboxes where \(store.assistantName) answers customer emails. Turn it on or off in each mailbox's settings.")
                Spacer()
                Button("Open mailboxes") { env.router.accountPath.append(.setting(.mailboxes)) }
                    .buttonStyle(ChipButtonStyle())
            }
            Panel {
                if !store.mailboxes.loaded {
                    SkeletonRows(count: 1)
                } else if answering.isEmpty {
                    Text("\(store.assistantName) doesn't answer emails in any mailbox yet.")
                        .font(ODFont.footnote)
                        .foregroundStyle(Theme.fg3)
                        .padding(14)
                } else {
                    ForEach(Array(answering.enumerated()), id: \.element.id) { index, mailbox in
                        if index > 0 { AccountDivider() }
                        SettingsDetailLink(page: .mailboxes, id: mailbox.id) {
                            AccountRow(symbol: "tray", tone: .blue, title: mailbox.name.isEmpty ? mailbox.email : mailbox.name, subtitle: mailbox.outsideHours ? "\(mailbox.email) · Outside business hours" : mailbox.email) {
                                HStack(spacing: 8) {
                                    TonePill(text: mailbox.sends ? "Sends automatically" : "Draft for review", tone: mailbox.sends ? .violet : .gray)
                                    Chevron()
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    private func save(_ store: AgentStore) async {
        saving = true
        defer { saving = false }
        do {
            try await store.saveBehaviour(edit)
            Haptics.success()
            env.toasts.show("Agent settings saved", kind: .success)
        } catch {
            env.toasts.error(error)
        }
    }

    private func runTests(_ store: AgentStore) async {
        running = true
        defer { running = false }
        do {
            try await store.startRun()
            Haptics.success()
            env.router.accountPath.append(.setting(.tests))
        } catch {
            env.toasts.error(error)
        }
    }
}
