import SwiftUI

/// Settings › Assignment, natively: who new tickets go to.
struct AssignmentSettings: View {
    @Environment(AppEnvironment.self) private var env
    @State private var store = AssignmentStore()
    @State private var byTeam = true
    @State private var addingRule = false
    @State private var addingSkill = false
    @State private var skillMembers: AutoAssignSkill?
    @State private var capFor: AutoAssignMember?
    @State private var awayDraft = 30

    private var canEdit: Bool { env.session.isAdmin }

    var body: some View {
        InboxLoadGate(load: store.load, retry: { await store.reload() }) {
            if let config = store.config { page(config) }
        }
        .accountTitle(SettingsPage.assignment.title)
        .sheet(isPresented: $addingRule) {
            InboxOptionSheet(title: "Add rule", options: ruleTargets, prompt: "Find a team or mailbox", emptyText: "Every team and mailbox has a rule") { picked in
                if let value = picked.first { addRule(value) }
            }
            .presentationDetents([.medium, .large])
        }
        .sheet(isPresented: $addingSkill) {
            InboxOptionSheet(
                title: "Add skill",
                options: [],
                prompt: "Skill name",
                emptyText: "Enter a name to create this skill.",
                createTitle: { "Create skill \u{201C}\($0)\u{201D}" },
                onCreate: createSkill
            ) { _ in }
            .presentationDetents([.medium])
        }
        .sheet(item: $skillMembers) { skill in
            InboxOptionSheet(
                title: "Assign teammates",
                options: store.assignable.map { InboxOption(value: $0.id, label: $0.user.name, hint: $0.user.email, keywords: [$0.user.email ?? ""]) },
                multiple: true,
                selected: Set(skill.memberIDs),
                prompt: "Search"
            ) { ids in run { try await store.setSkillMembers(skill, ids) } }
            .presentationDetents([.medium, .large])
        }
        .sheet(item: $capFor) { member in
            AssignmentCapSheet(member: member) { cap in run { try await store.setCap(member, cap) } }
                .presentationDetents([.height(340)])
                .presentationCornerRadius(Theme.Radius.sheet)
        }
        .task { await store.start(api: env.session.api, directory: env.directory, isAdmin: canEdit) }
    }

    private func page(_ config: AutoAssignConfig) -> some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 24) {
                intro
                modeSection(config)
                if !store.teams.isEmpty { precedence }
                if canEdit { skillsSection }
                rulesSection(config)
                notifySection(config)
                followUpSection(config)
                awaySection(config)
                availabilitySection(config)
            }
            .padding(.horizontal, 16)
            .padding(.top, 8)
            .padding(.bottom, 120)
            .motion(Motion.snappy, value: config)
            .motion(Motion.snappy, value: store.skills)
        }
        .background(Theme.chrome.ignoresSafeArea())
        .refreshable { await store.reload() }
        .onAppear { if config.autoAwayMinutes > 0 { awayDraft = config.autoAwayMinutes } }
    }

    // MARK: Intro

    private var intro: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Decide who new tickets go to. Only teammates who are online and under their cap are picked.")
                .font(ODFont.footnote)
                .foregroundStyle(Theme.fg3)
                .fixedSize(horizontal: false, vertical: true)
            if !canEdit { SettingsReadOnlyNote() }
            let people = store.assignable
            let eligible = people.filter(\.eligible).count
            let atCap = people.filter(\.atCap).count
            InboxStatStrip(items: [
                .init(label: "Can take tickets", value: "\(eligible)", tone: eligible > 0 ? .green : .red),
                .init(label: "Online", value: "\(people.filter { $0.presence == .online }.count)", tone: .green),
                .init(label: "Away", value: "\(people.filter { $0.presence == .away }.count)", tone: .amber),
                .init(label: "Offline", value: "\(people.filter { $0.presence == .offline }.count)"),
                .init(label: "At their cap", value: "\(atCap)", tone: atCap > 0 ? .orange : nil),
            ])
        }
    }

    // MARK: Mode

    private func modeSection(_ config: AutoAssignConfig) -> some View {
        AccountSection(title: "Default mode", footer: "Used for every ticket unless its team or mailbox has its own rule.") {
            ForEach(Array(AutoAssignMode.allCases.enumerated()), id: \.element) { index, mode in
                if index > 0 { AccountDivider() }
                AccountChoiceRow(symbol: mode.symbol, tone: config.mode == mode ? mode.tone : .gray, title: mode.title, subtitle: mode.hint, chosen: config.mode == mode) {
                    guard canEdit, config.mode != mode else { return }
                    Haptics.selection()
                    run { try await store.setMode(mode) }
                }
                .disabled(!canEdit)
            }
        }
    }

    private var precedence: some View {
        VStack(alignment: .leading, spacing: 8) {
            SectionLabel("Which method applies").padding(.horizontal, 4)
            Panel {
                let steps: [(String, Tone, String, String)] = [
                    ("person.3", .teal, "The ticket's team", "Set on the team's page, or in the rules below."),
                    ("tray", .blue, "The address", "A mailbox rule below, for tickets without a team method."),
                    ("shuffle", .gray, "Workspace default", "Everything else, as chosen above."),
                ]
                ForEach(Array(steps.enumerated()), id: \.offset) { index, step in
                    if index > 0 { AccountDivider() }
                    HStack(alignment: .top, spacing: 12) {
                        AccountIconTile(symbol: step.0, tone: step.1, size: 28)
                        VStack(alignment: .leading, spacing: 2) {
                            HStack(spacing: 6) {
                                Text("\(index + 1)").font(ODFont.mono(11)).foregroundStyle(Theme.fg4)
                                Text(step.2).font(ODFont.sans(14.5, weight: .medium)).foregroundStyle(Theme.fg)
                            }
                            Text(step.3).font(ODFont.footnote).foregroundStyle(Theme.fg3).fixedSize(horizontal: false, vertical: true)
                        }
                        Spacer(minLength: 0)
                    }
                    .padding(.horizontal, 14)
                    .padding(.vertical, 10)
                }
            }
            Text("A new ticket is assigned with the first method that is set, in this order.")
                .font(ODFont.footnote)
                .foregroundStyle(Theme.fg3)
                .padding(.horizontal, 4)
        }
    }

    // MARK: Skills

    private var skillsSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                SectionLabel("Routing skills")
                Spacer()
                Button { addingSkill = true } label: { Label("Add skill", systemImage: "plus").font(ODFont.sans(13, weight: .medium)) }
                    .foregroundStyle(Theme.fg2)
            }
            .padding(.horizontal, 4)
            Panel {
                if store.skills.isEmpty {
                    EmptyState(symbol: "sparkles", title: "No skills yet", message: "Add skills such as Billing or Technical support.")
                        .padding(.vertical, 12)
                } else {
                    ForEach(Array(store.skills.enumerated()), id: \.element.id) { index, skill in
                        if index > 0 { AccountDivider() }
                        skillRow(skill)
                    }
                }
            }
            Text(store.skills.contains { !$0.archived }
                 ? "Describe the expertise needed for tickets and assign each skill to teammates. Skills are required by matching routing rules before automatic assignment."
                 : "Describe the expertise needed for tickets and assign each skill to teammates.")
                .font(ODFont.footnote)
                .foregroundStyle(Theme.fg3)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.horizontal, 4)
        }
    }

    private func skillRow(_ skill: AutoAssignSkill) -> some View {
        HStack(spacing: 12) {
            AccountIconTile(symbol: skill.archived ? "archivebox" : "sparkles", tone: skill.archived ? .gray : .violet)
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 6) {
                    Text(skill.name).font(ODFont.sans(15)).foregroundStyle(Theme.fg).lineLimit(1)
                    if skill.archived { TonePill(text: "Archived", tone: .gray, height: 18) }
                }
                Text(InboxText.count(skill.memberIDs.count, "teammate", "teammates")).font(ODFont.footnote).foregroundStyle(Theme.fg3)
            }
            Spacer(minLength: 8)
            Menu {
                if !skill.archived {
                    Button { skillMembers = skill } label: { Label("Assign teammates", systemImage: "person.badge.plus") }
                }
                Button { run { try await store.setSkillArchived(skill, !skill.archived) } } label: {
                    Label(skill.archived ? "Restore skill" : "Archive skill", systemImage: skill.archived ? "arrow.uturn.backward" : "archivebox")
                }
            } label: {
                Image(systemName: "ellipsis")
                    .font(.system(size: 15, weight: .medium))
                    .foregroundStyle(Theme.fg3)
                    .frame(width: 36, height: 36)
                    .contentShape(Rectangle())
            }
            .accessibilityLabel("Actions for \(skill.name)")
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 9)
        .opacity(skill.archived ? 0.65 : 1)
    }

    private func createSkill(_ name: String) {
        Task {
            do {
                let skill = try await store.createSkill(name)
                Haptics.success()
                env.toasts.show("Created \(skill.name)", kind: .success)
            } catch {
                env.toasts.error(error)
            }
        }
    }

    // MARK: Rules

    private func ruleName(_ rule: AutoAssignRule) -> String? {
        switch rule.targetType {
        case .team: store.teams.first { $0.id == rule.targetID }?.name
        case .mailbox: store.mailboxes.first { $0.id == rule.targetID }?.name
        }
    }

    private func ruleDetail(_ rule: AutoAssignRule) -> String {
        switch rule.targetType {
        case .team:
            let count = store.teams.first { $0.id == rule.targetID }?.members.count ?? 0
            return "Team \u{00B7} \(InboxText.count(count, "member", "members"))"
        case .mailbox:
            let email = store.mailboxes.first { $0.id == rule.targetID }?.email ?? ""
            return email.isEmpty ? "Mailbox" : "Mailbox \u{00B7} \(email)"
        }
    }

    private var ruleTargets: [InboxOption] {
        let taken = Set(store.config?.rules.map(\.key) ?? [])
        let teams = store.teams.map { InboxOption(value: "team:\($0.id)", label: $0.name, symbol: "person.3", group: "Teams") }
        let mailboxes = store.mailboxes.map { InboxOption(value: "mailbox:\($0.id)", label: $0.name, hint: $0.email, symbol: "tray", group: "Mailboxes") }
        return (teams + mailboxes).filter { !taken.contains($0.value) }
    }

    private func rulesSection(_ config: AutoAssignConfig) -> some View {
        let sorted = config.rules.sorted { ($0.targetType == .team ? 0 : 1) < ($1.targetType == .team ? 0 : 1) }
        return VStack(alignment: .leading, spacing: 8) {
            HStack {
                SectionLabel("Rules")
                Spacer()
                if canEdit && !config.rules.isEmpty {
                    Button { addingRule = true } label: { Label("Add rule", systemImage: "plus").font(ODFont.sans(13, weight: .medium)) }
                        .foregroundStyle(Theme.fg2)
                        .disabled(ruleTargets.isEmpty)
                }
            }
            .padding(.horizontal, 4)
            Panel {
                if sorted.isEmpty {
                    EmptyState(
                        symbol: "arrow.triangle.branch",
                        title: "No rules yet",
                        message: "Every ticket uses the default mode. Add a rule when one team or mailbox should share work differently, like load balancing billing.",
                        actionTitle: canEdit && !ruleTargets.isEmpty ? "Add rule" : nil,
                        action: canEdit ? { addingRule = true } : nil
                    )
                    .padding(.vertical, 12)
                } else {
                    ForEach(Array(sorted.enumerated()), id: \.element.key) { index, rule in
                        if index > 0 { AccountDivider() }
                        ruleRow(rule, rules: config.rules)
                    }
                }
            }
            Text("Give a team or a mailbox its own mode. A team rule wins over a mailbox rule.")
                .font(ODFont.footnote)
                .foregroundStyle(Theme.fg3)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.horizontal, 4)
        }
    }

    private func ruleRow(_ rule: AutoAssignRule, rules: [AutoAssignRule]) -> some View {
        let name = ruleName(rule)
        return HStack(spacing: 12) {
            AccountIconTile(symbol: rule.targetType == .team ? "person.3" : "tray", tone: rule.targetType == .team ? .teal : .blue)
            VStack(alignment: .leading, spacing: 2) {
                Text(name ?? "Deleted").font(ODFont.sans(15)).foregroundStyle(name == nil ? Theme.fg3 : Theme.fg).lineLimit(1)
                Text(ruleDetail(rule)).font(ODFont.footnote).foregroundStyle(Theme.fg3).lineLimit(1)
            }
            Spacer(minLength: 8)
            Menu {
                Picker("Mode for \(name ?? "")", selection: Binding(get: { rule.mode }, set: { mode in
                    let next = rules.map { $0.key == rule.key ? AutoAssignRule(targetType: $0.targetType, targetID: $0.targetID, mode: mode) : $0 }
                    run { try await store.setRules(next) }
                })) {
                    ForEach(AutoAssignMode.allCases) { mode in Label(mode.title, systemImage: mode.symbol).tag(mode) }
                }
                if canEdit {
                    Divider()
                    Button(role: .destructive) { removeRule(rule, rules: rules) } label: { Label("Remove rule", systemImage: "xmark") }
                }
            } label: {
                HStack(spacing: 4) {
                    Image(systemName: rule.mode.symbol).font(.system(size: 11, weight: .medium))
                    Text(rule.mode.title).font(ODFont.sans(13, weight: .medium))
                    Image(systemName: "chevron.up.chevron.down").font(.system(size: 9, weight: .semibold)).foregroundStyle(Theme.fg4)
                }
                .foregroundStyle(rule.mode == .none ? Theme.fg2 : rule.mode.tone.text)
                .padding(.horizontal, 10)
                .frame(height: 30)
                .background(Theme.sunken, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
            }
            .disabled(!canEdit)
            .accessibilityLabel("Mode for \(name ?? ""), \(rule.mode.title)")
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 9)
    }

    private func addRule(_ value: String) {
        guard let config = store.config else { return }
        let parts = value.split(separator: ":", maxSplits: 1).map(String.init)
        guard parts.count == 2 else { return }
        let mode: AutoAssignMode = config.mode == .none ? .roundRobin : config.mode
        let rule = AutoAssignRule(targetType: parts[0] == "team" ? .team : .mailbox, targetID: parts[1], mode: mode)
        run { try await store.setRules(config.rules + [rule]) }
    }

    private func removeRule(_ rule: AutoAssignRule, rules: [AutoAssignRule]) {
        let name = ruleName(rule) ?? ""
        Task {
            do {
                try await store.setRules(rules.filter { $0.key != rule.key })
                env.toasts.show(Toast(message: "Rule for \(name) removed", actionTitle: "Undo", action: { [store] in
                    Task { try? await store.setRules(rules) }
                }), seconds: 5)
            } catch {
                env.toasts.error(error)
            }
        }
    }

    // MARK: Notify and away

    private func notifySection(_ config: AutoAssignConfig) -> some View {
        AccountSection(title: "New unassigned tickets", footer: "Who hears when a ticket comes in and nobody is assigned to it. While Ollie answers a chat or an email itself, people hear when it hands over. Each person picks where it reaches them in their notification settings.") {
            AccountRow(symbol: config.newTicketNotify.symbol, title: "Notify about new unassigned tickets", subtitle: config.newTicketNotify.hint) {
                Menu {
                    Picker("Notify about new unassigned tickets", selection: Binding(get: { config.newTicketNotify }, set: { value in run { try await store.setNotify(value) } })) {
                        ForEach(UnassignedNotify.allCases) { option in Label(option.title, systemImage: option.symbol).tag(option) }
                    }
                } label: {
                    AccountDisclosure(value: config.newTicketNotify.title)
                }
                .disabled(!canEdit)
            }
        }
    }

    private func followUpSection(_ config: AutoAssignConfig) -> some View {
        AccountSection(title: "Follow-ups of closed tickets", footer: "An email that answers a closed ticket opens a new ticket, linked to the closed one.") {
            AccountToggleRow(
                symbol: "arrow.uturn.backward",
                tone: config.followUpKeepsAssignee ? .blue : .gray,
                title: "Give follow-ups to the closed ticket\u{2019}s assignee",
                subtitle: config.followUpKeepsAssignee ? "The follow-up goes to whoever had the closed ticket, if they can still take tickets and, when it goes to a private team, are on that team" : "Follow-ups arrive unassigned, like any new ticket",
                isOn: Binding(get: { config.followUpKeepsAssignee }, set: { value in run { try await store.setFollowUp(value) } }),
                disabled: !canEdit
            )
        }
    }

    private func awaySection(_ config: AutoAssignConfig) -> some View {
        let on = config.autoAwayMinutes > 0
        return AccountSection(title: "Away", footer: "Teammates who step away stop getting new tickets until they are back.") {
            AccountToggleRow(
                symbol: "moon",
                tone: on ? .amber : .gray,
                title: "Mark idle teammates away",
                subtitle: on ? (config.autoAwayMinutes == 1 ? "After 1 minute without activity" : "After \(config.autoAwayMinutes) minutes without activity") : "Teammates stay online until they change it themselves",
                isOn: Binding(get: { on }, set: { value in run { try await store.setAway(value ? max(10, awayDraft) : 0) } }),
                disabled: !canEdit
            )
            if on {
                AccountDivider()
                AccountRow(title: "Idle for", subtitle: "Between 10 minutes and 24 hours.") {
                    InboxStepper(value: Binding(get: { awayDraft }, set: { value in
                        awayDraft = value
                        run { try await store.setAway(value) }
                    }), range: 10...1440, step: 5, suffix: "min", disabled: !canEdit, label: "Idle for")
                }
            }
        }
    }

    // MARK: Availability

    private func availabilitySection(_ config: AutoAssignConfig) -> some View {
        let people = store.assignable
        return VStack(alignment: .leading, spacing: 8) {
            SectionLabel("Team availability").padding(.horizontal, 4)
            if !store.teams.isEmpty && !people.isEmpty {
                AccountSegmented(options: [true, false], selection: $byTeam) { $0 ? "By team" : "Everyone" }
            }
            if people.isEmpty {
                Panel {
                    EmptyState(symbol: "person.2", title: "Nobody to assign yet", message: "Invite agents or admins so new tickets have someone to go to.", actionTitle: canEdit ? "Invite people" : nil, action: canEdit ? { env.router.accountPath.append(.setting(.members)) } : nil)
                        .padding(.vertical, 12)
                }
            } else if byTeam && !store.teams.isEmpty {
                ForEach(store.teams) { team in teamGroup(team, people: people, mode: config.mode) }
                let loose = people.filter { member in !store.teams.contains { $0.assigns(member.id) } }
                if !loose.isEmpty {
                    groupHeader(title: "Not on a team", count: loose.count, tone: nil, trailing: nil)
                    Panel { memberRows(loose) }
                }
            } else {
                Panel { memberRows(people) }
            }
            Text("Who can be assigned right now and how much each person has open. Viewers are never assigned.")
                .font(ODFont.footnote)
                .foregroundStyle(Theme.fg3)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.horizontal, 4)
        }
    }

    @ViewBuilder
    private func teamGroup(_ team: AutoAssignTeam, people: [AutoAssignMember], mode: AutoAssignMode) -> some View {
        let members = people.filter { team.assigns($0.id) }
        let teamMode = team.assignMode ?? mode
        groupHeader(title: team.name, count: members.count, tone: team.color, trailing: team.assignMode == nil ? "\(teamMode.title) (default)" : teamMode.title, locked: team.isPrivate)
        Panel {
            if members.isEmpty {
                Text("No one on this team takes new tickets.")
                    .font(ODFont.footnote)
                    .foregroundStyle(Theme.fg4)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.horizontal, 14)
                    .padding(.vertical, 12)
            } else {
                memberRows(members)
            }
        }
    }

    private func groupHeader(title: String, count: Int, tone: Tone?, trailing: String?, locked: Bool = false) -> some View {
        HStack(spacing: 7) {
            if let tone {
                RoundedRectangle(cornerRadius: 2.5).fill(tone.solid).frame(width: 8, height: 8)
            } else {
                RoundedRectangle(cornerRadius: 2.5).strokeBorder(Theme.lineStrong, style: StrokeStyle(lineWidth: 1, dash: [2])).frame(width: 8, height: 8)
            }
            Text(title).font(ODFont.sans(13, weight: .medium)).foregroundStyle(Theme.fg2)
            if locked { Image(systemName: "lock.fill").font(.system(size: 9)).foregroundStyle(Theme.fg4) }
            Text(InboxText.count(count, "person", "people")).font(ODFont.footnote).foregroundStyle(Theme.fg4)
            Spacer()
            if let trailing { Text(trailing).font(ODFont.footnote).foregroundStyle(Theme.fg3) }
        }
        .padding(.horizontal, 4)
        .padding(.top, 6)
    }

    private func memberRows(_ members: [AutoAssignMember]) -> some View {
        ForEach(Array(members.enumerated()), id: \.element.id) { index, member in
            if index > 0 { Hairline().padding(.leading, 58) }
            AssignmentMemberRow(
                member: member,
                skills: store.skills.filter { $0.memberIDs.contains(member.id) },
                canEdit: canEdit,
                setAvailability: { value in run { try await store.setAvailability(member, value) } },
                editCap: { capFor = member }
            )
        }
    }

    // MARK: Plumbing

    private func run(_ work: @escaping @MainActor () async throws -> Void) {
        Task {
            do { try await work() } catch { env.toasts.error(error) }
        }
    }

    /// Assignment has no detail pages: a link to one opens the page.
    struct Detail: View {
        let id: String

        var body: some View {
            AssignmentSettings()
        }
    }
}

/// One teammate: presence (an admin can change it), skills, open tickets against their cap.
private struct AssignmentMemberRow: View {
    let member: AutoAssignMember
    let skills: [AutoAssignSkill]
    let canEdit: Bool
    let setAvailability: (Availability) -> Void
    let editCap: () -> Void

    private var presenceTone: Tone {
        switch member.presence {
        case .online: .green
        case .away: .amber
        case .offline: .gray
        }
    }

    private var loadTone: Tone {
        guard let cap = member.maxOpenTickets else { return .gray }
        let ratio = Double(member.openTickets) / Double(max(1, cap))
        return ratio >= 1 ? .red : ratio >= 0.75 ? .amber : .green
    }

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            Avatar(name: member.user.name, url: member.user.avatarURL, seed: member.id, size: 32)
                .overlay(alignment: .bottomTrailing) {
                    Circle().fill(presenceTone.solid).frame(width: 9, height: 9).overlay(Circle().strokeBorder(Theme.surface, lineWidth: 1.5))
                }
            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: 6) {
                    Text(member.user.name).font(ODFont.sans(15, weight: .medium)).foregroundStyle(Theme.fg).lineLimit(1)
                    if member.role.isAdmin { TonePill(text: member.role.title, tone: .gray, height: 18) }
                }
                presence
                if !skills.isEmpty {
                    HStack(spacing: 4) {
                        ForEach(skills.prefix(3)) { skill in InboxTagChip(name: skill.name, tone: skill.archived ? .gray : .violet) }
                    }
                }
            }
            Spacer(minLength: 6)
            VStack(alignment: .trailing, spacing: 5) {
                load
                if canEdit {
                    Button(action: editCap) {
                        Text(member.maxOpenTickets.map { "Cap \($0)" } ?? "Set cap")
                            .font(ODFont.sans(12.5, weight: .medium))
                            .foregroundStyle(Theme.fg2)
                            .padding(.horizontal, 9)
                            .frame(height: 26)
                            .background(Theme.sunken, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
                    }
                    .buttonStyle(TapScaleStyle())
                    .accessibilityLabel("Ticket cap for \(member.user.name)")
                }
            }
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
    }

    @ViewBuilder
    private var presence: some View {
        let label = HStack(spacing: 5) {
            Text(member.idle ? "Away, idle" : member.presence.title).font(ODFont.footnote).foregroundStyle(Theme.fg3)
            if canEdit { Image(systemName: "chevron.down").font(.system(size: 8, weight: .semibold)).foregroundStyle(Theme.fg4) }
        }
        if canEdit {
            Menu {
                Picker("Set availability for \(member.user.name)", selection: Binding(get: { member.availability }, set: { setAvailability($0) })) {
                    ForEach(Availability.allCases, id: \.self) { value in Text(value.title).tag(value) }
                }
            } label: { label }
        } else {
            label
        }
    }

    private var load: some View {
        VStack(alignment: .trailing, spacing: 4) {
            HStack(spacing: 3) {
                Text("\(member.openTickets)").font(ODFont.mono(13, weight: .medium)).foregroundStyle(Theme.fg)
                if let cap = member.maxOpenTickets { Text("/ \(cap)").font(ODFont.mono(12)).foregroundStyle(Theme.fg4) }
                Text(member.maxOpenTickets == nil ? "no cap" : "open").font(ODFont.sans(11)).foregroundStyle(Theme.fg4)
            }
            Capsule()
                .fill(Theme.hover)
                .frame(width: 72, height: 4)
                .overlay(alignment: .leading) {
                    if let cap = member.maxOpenTickets {
                        Capsule()
                            .fill(loadTone.solid)
                            .frame(width: max(4, 72 * min(1, Double(member.openTickets) / Double(max(1, cap)))), height: 4)
                    }
                }
                .accessibilityHidden(true)
        }
    }
}

/// The open ticket cap for one teammate.
private struct AssignmentCapSheet: View {
    let member: AutoAssignMember
    let save: (Int?) -> Void

    @Environment(\.dismiss) private var dismiss
    @State private var limited = false
    @State private var cap = 10

    var body: some View {
        VStack(alignment: .leading, spacing: 18) {
            AccountSheetHeader(
                title: "Open ticket cap",
                subtitle: "\(member.user.name.split(separator: " ").first.map(String.init) ?? member.user.name) stops getting new tickets at this many open and pending tickets."
            ) { dismiss() }
            InboxSwitchBox(title: "Limit open tickets", isOn: $limited)
            if limited {
                InboxStepper(value: $cap, range: 1...500, suffix: "tickets", label: "Open ticket cap")
            }
            Spacer(minLength: 0)
            Button {
                save(limited ? cap : nil)
                dismiss()
            } label: { ButtonLabel(title: "Save", busy: false) }
                .buttonStyle(InkButtonStyle())
        }
        .padding(22)
        .frame(maxHeight: .infinity, alignment: .top)
        .background(Theme.surface)
        .motion(Motion.snappy, value: limited)
        .onAppear {
            limited = member.maxOpenTickets != nil
            cap = member.maxOpenTickets ?? 10
        }
    }
}
