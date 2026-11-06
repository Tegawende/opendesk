import SwiftUI
import UIKit

/// Settings › Members: everyone in the workspace with their role, pending invitations,
/// inviting people, and leaving (web `Members.tsx`). Owners manage admins; admins manage
/// agents and viewers; nobody changes their own role or the owner's.
struct MembersSettings: View {
    @Environment(AppEnvironment.self) private var env
    @State private var store = MembersSettingsStore()
    @State private var query = ""
    @State private var inviting = false
    @State private var confirm: AccountConfirm?

    private var me: Role { env.session.role }
    private var meID: String { env.session.user?.id ?? "" }
    private var admin: Bool { env.session.isAdmin }

    var body: some View {
        AccountPage(title: "Members") {
            if store.loaded {
                if !admin { SettingsReadOnlyNote(text: "Only admins can invite people and change roles.") }
                summary
                if admin { inviteBlock }
                search
                membersSection
                if admin && !store.invitations.isEmpty { invitationsSection }
                if me != .owner { leaveSection }
            } else if let error = store.error {
                ErrorState(message: error) { Task { await store.load(admin: admin) } }
            } else {
                WSLoading(rows: 6)
            }
        }
        .refreshable { await store.load(admin: admin) }
        .task {
            store.attach(env.session.api)
            await store.load(admin: admin)
        }
        .onChange(of: env.directory.members) { _, _ in
            Task { await store.load(admin: admin) }
        }
        .sheet(isPresented: $inviting) {
            WSInviteSheet(grantable: WSMemberRules.grantable(by: me), limit: store.limit) { emails, role in
                try await store.invite(emails, role: role)
            }
            .presentationDetents([.large])
            .presentationCornerRadius(Theme.Radius.sheet)
        }
        .accountConfirm($confirm)
    }

    // MARK: Sections

    private var summary: some View {
        let counts = Dictionary(grouping: store.members, by: \.role).mapValues(\.count)
        var items: [WSSummaryStrip.Item] = [
            .init(label: "Members", value: "\(store.members.count)"),
            .init(label: "Admins", value: "\((counts[.owner] ?? 0) + (counts[.admin] ?? 0))", tone: .blue),
            .init(label: "Agents", value: "\(counts[.agent] ?? 0)", tone: .green),
            .init(label: "Viewers", value: "\(counts[.viewer] ?? 0)", tone: .gray),
        ]
        if admin { items.append(.init(label: "Pending", value: "\(store.invitations.filter { !$0.expired }.count)", tone: .amber)) }
        return Panel { WSSummaryStrip(items: items) }
    }

    @ViewBuilder
    private var inviteBlock: some View {
        VStack(alignment: .leading, spacing: 10) {
            Button { inviting = true } label: { ButtonLabel(title: "Invite people", symbol: "person.badge.plus") }
                .buttonStyle(InkButtonStyle())
                .disabled(store.limit?.atLimit == true)
            if let limit = store.limit, limit.limited {
                Text(limit.atLimit
                    ? "This workspace has \(limit.used) of \(limit.max) members, its plan\u{2019}s limit. Remove a member or an invitation to invite someone else."
                    : "\(limit.used) of \(limit.max) members on this workspace\u{2019}s plan.")
                    .font(ODFont.footnote)
                    .foregroundStyle(limit.atLimit ? Tone.amber.text : Theme.fg3)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.horizontal, 4)
            }
        }
    }

    private var search: some View {
        HStack(spacing: 8) {
            Image(systemName: "magnifyingglass").font(.system(size: 14)).foregroundStyle(Theme.fg4)
            TextField("Search members", text: $query)
                .font(ODFont.sans(15.5))
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .submitLabel(.search)
            if !query.isEmpty {
                Button { query = "" } label: {
                    Image(systemName: "xmark.circle.fill").foregroundStyle(Theme.fg4)
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Clear search")
            }
        }
        .padding(.horizontal, 12)
        .frame(height: 40)
        .background(Theme.surface, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous).strokeBorder(Theme.line, lineWidth: 1))
    }

    private var shown: [WSMember] {
        let needle = query.trimmingCharacters(in: .whitespaces).lowercased()
        return store.members
            .filter { needle.isEmpty || $0.displayName.lowercased().contains(needle) || ($0.user.email ?? "").lowercased().contains(needle) }
            .sorted { ($0.rank, $0.displayName.lowercased()) < ($1.rank, $1.displayName.lowercased()) }
    }

    private var membersSection: some View {
        AccountSection(title: "Members") {
            let list = shown
            if list.isEmpty {
                EmptyState(symbol: "person.2", title: "No one matches", message: "Try a name or an email address.")
            }
            ForEach(Array(list.enumerated()), id: \.element.id) { index, member in
                if index > 0 { Hairline(inset: 58) }
                memberRow(member)
            }
        }
    }

    private func memberRow(_ member: WSMember) -> some View {
        let manageable = WSMemberRules.canManage(me: me, meID: meID, target: member)
        return SettingsDetailLink(page: .members, id: member.id) {
            WSMemberLine(member: member, isMe: member.id == meID)
        }
        .contextMenu {
            if manageable {
                Menu("Change role") {
                    ForEach(WSMemberRules.grantable(by: me), id: \.self) { role in
                        Button {
                            Task { await setRole(member, role) }
                        } label: {
                            if role == member.role { Label(role.title, systemImage: "checkmark") } else { Text(role.title) }
                        }
                    }
                }
                Button("Remove from workspace", systemImage: "person.badge.minus", role: .destructive) { askRemove(member) }
            }
        }
    }

    private var invitationsSection: some View {
        AccountSection(title: "Pending invitations", footer: "Invitations last seven days. Copy a link to send it yourself.") {
            ForEach(Array(store.invitations.enumerated()), id: \.element.id) { index, invite in
                if index > 0 { AccountDivider() }
                AccountRow(
                    symbol: "envelope", tone: invite.expired ? .gray : .amber,
                    title: invite.email,
                    subtitle: invitationLine(invite)
                ) {
                    HStack(spacing: 8) {
                        TonePill(text: invite.role.title, tone: WSMemberRules.tone(invite.role))
                        Menu {
                            Button("Copy invite link", systemImage: "link") { Task { await copyLink(invite) } }
                            Button("Resend email", systemImage: "arrow.clockwise") { Task { await resend(invite) } }
                            Button("Revoke", systemImage: "xmark.circle", role: .destructive) { askRevoke(invite) }
                        } label: {
                            Image(systemName: "ellipsis.circle")
                                .font(.system(size: 18))
                                .foregroundStyle(Theme.fg3)
                                .frame(width: 30, height: 30)
                        }
                        .accessibilityLabel("Invitation options for \(invite.email)")
                    }
                }
            }
        }
    }

    private func invitationLine(_ invite: WSInvitation) -> String {
        var parts: [String] = []
        if let by = invite.invitedBy?.name, !by.isEmpty { parts.append("Invited by \(by)") }
        if invite.expired {
            parts.append("Expired")
        } else if let expires = invite.expiresAt {
            parts.append("Expires \(Format.relLong(expires))")
        }
        return parts.joined(separator: " \u{00B7} ")
    }

    private var leaveSection: some View {
        AccountSection(footer: "You lose access to this workspace. Your tickets stay, unassigned.") {
            Button { askLeave() } label: {
                AccountRow(symbol: "rectangle.portrait.and.arrow.right", tone: .red, title: "Leave this workspace", titleColor: Tone.red.text)
            }
            .buttonStyle(RowPressStyle())
        }
    }

    // MARK: Actions

    private func setRole(_ member: WSMember, _ role: Role) async {
        guard role != member.role else { return }
        do {
            try await store.setRole(member.id, role)
            Haptics.success()
            env.toasts.show("\(member.displayName) is now \(role == .admin ? "an" : "a") \(role.title.lowercased())", kind: .success)
        } catch {
            env.toasts.error(error)
        }
    }

    private func askRemove(_ member: WSMember) {
        confirm = AccountConfirm(
            title: "Remove member?",
            message: "\(member.displayName) loses access right away. Their tickets stay, unassigned.",
            confirmTitle: "Remove"
        ) {
            do {
                try await store.remove(member.id)
                env.toasts.show("\(member.displayName) removed", kind: .success)
            } catch {
                env.toasts.error(error)
            }
        }
    }

    private func askRevoke(_ invite: WSInvitation) {
        confirm = AccountConfirm(
            title: "Revoke invitation?",
            message: "The link sent to \(invite.email) stops working.",
            confirmTitle: "Revoke"
        ) {
            do {
                try await store.revoke(invite.id)
                env.toasts.show("Invitation revoked", kind: .success)
            } catch {
                env.toasts.error(error)
            }
        }
    }

    private func copyLink(_ invite: WSInvitation) async {
        do {
            let url = try await store.link(invite.id)
            UIPasteboard.general.string = url
            Haptics.success()
            env.toasts.show("Invite link copied", kind: .success)
        } catch {
            env.toasts.error(error)
        }
    }

    private func resend(_ invite: WSInvitation) async {
        do {
            try await store.resend(invite.id)
            env.toasts.show("Invitation sent again", kind: .success)
        } catch {
            env.toasts.error(error)
        }
    }

    private func askLeave() {
        let name = env.session.workspace?.workspace.name ?? "this workspace"
        confirm = AccountConfirm(
            title: "Leave this workspace?",
            message: "You lose access to \(name) right away. Someone has to invite you again to come back.",
            confirmTitle: "Leave"
        ) {
            do {
                try await store.remove(meID)
                Haptics.success()
                await env.session.membershipChanged()
            } catch {
                env.toasts.error(error)
            }
        }
    }

    struct Detail: View {
        let id: String

        var body: some View {
            WSMemberDetail(id: id)
        }
    }
}

// MARK: - Store

@MainActor
@Observable
final class MembersSettingsStore {
    private(set) var members: [WSMember] = []
    private(set) var invitations: [WSInvitation] = []
    private(set) var limit: WSBilling.Limit?
    private(set) var loaded = false
    private(set) var error: String?

    @ObservationIgnored private var api: APIClient?

    func attach(_ api: APIClient) { self.api = api }

    func member(_ id: String) -> WSMember? { members.first { $0.id == id } }

    func load(admin: Bool) async {
        guard let api else { return }
        do {
            let list: WSList<WSMember> = try await api.get("v1/members")
            members = list.data
            if admin {
                let invites: WSList<WSInvitation>? = try? await api.get("v1/invitations")
                invitations = (invites?.data ?? []).sorted { ($0.createdAt ?? .distantPast) > ($1.createdAt ?? .distantPast) }
                if let billing: WSBilling = try? await api.get("v1/billing"), !billing.selfHosted {
                    var members = billing.limit(members: true)
                    members.used = Swift.max(members.used, self.members.count + invitations.filter { !$0.expired }.count)
                    limit = members
                } else {
                    limit = nil
                }
            }
            loaded = true
            error = nil
        } catch {
            if !error.isCancellation, !loaded { self.error = error.userMessage }
        }
    }

    func setRole(_ id: String, _ role: Role) async throws {
        guard let api else { return }
        let _: EmptyBody = try await api.patch("v1/members/\(id)", body: ["role": role.rawValue])
        if let index = members.firstIndex(where: { $0.id == id }) { members[index].role = role }
    }

    func setDeleteRule(_ id: String, _ rule: String) async throws {
        guard let api else { return }
        let _: EmptyBody = try await api.patch("v1/members/\(id)/permissions", body: ["message_delete": rule])
        if let index = members.firstIndex(where: { $0.id == id }) { members[index].messageDelete = rule == "default" ? nil : rule }
    }

    func saveProfile(_ id: String, _ profile: MemberProfile) async throws {
        guard let api else { return }
        let saved: WSMember = try await api.patch("v1/members/\(id)/profile", body: profile)
        if let index = members.firstIndex(where: { $0.id == id }) { members[index].profile = saved.profile }
    }

    func remove(_ id: String) async throws {
        guard let api else { return }
        try await api.delete("v1/members/\(id)")
        members.removeAll { $0.id == id }
    }

    /// Sends each invitation; the ones that failed come back with their reason.
    func invite(_ emails: [String], role: Role) async throws -> [(email: String, error: String)] {
        guard let api else { return [] }
        struct Body: Encodable, Sendable { let email: String; let role: String }
        var failed: [(email: String, error: String)] = []
        for email in emails {
            do {
                let invite: WSInvitation = try await api.post("v1/invitations", body: Body(email: email, role: role.rawValue))
                invitations.removeAll { $0.id == invite.id || $0.email.lowercased() == invite.email.lowercased() }
                invitations.insert(invite, at: 0)
            } catch {
                failed.append((email, error.userMessage))
            }
        }
        if var limit { limit.used += emails.count - failed.count; self.limit = limit }
        return failed
    }

    func revoke(_ id: String) async throws {
        guard let api else { return }
        try await api.delete("v1/invitations/\(id)")
        invitations.removeAll { $0.id == id }
        if var limit { limit.used = Swift.max(0, limit.used - 1); self.limit = limit }
    }

    func resend(_ id: String) async throws {
        guard let api else { return }
        let _: EmptyBody = try await api.post("v1/invitations/\(id)/resend")
    }

    func link(_ id: String) async throws -> String {
        guard let api else { return "" }
        struct Link: Decodable, Sendable { let url: String }
        let link: Link = try await api.post("v1/invitations/\(id)/link")
        return link.url
    }
}

// MARK: - Pieces

/// A member's avatar, name, email and role.
struct WSMemberLine: View {
    let member: WSMember
    var isMe = false

    var body: some View {
        HStack(spacing: 12) {
            Avatar(name: member.displayName, url: member.user.avatarURL, seed: member.id, size: 32)
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 6) {
                    Text(member.displayName).font(ODFont.sans(15.5, weight: .medium)).foregroundStyle(Theme.fg).lineLimit(1)
                    if isMe { Text("You").font(ODFont.sans(12)).foregroundStyle(Theme.fg4) }
                }
                if let email = member.user.email, !email.isEmpty {
                    Text(email).font(ODFont.sans(13)).foregroundStyle(Theme.fg3).lineLimit(1)
                }
            }
            Spacer(minLength: 8)
            TonePill(text: member.role.title, tone: WSMemberRules.tone(member.role))
            Image(systemName: "chevron.right").font(.system(size: 12, weight: .semibold)).foregroundStyle(Theme.fg4)
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
        .contentShape(Rectangle())
    }
}

/// One member: role, what they may delete, their teams and the profile customers see.
struct WSMemberDetail: View {
    let id: String

    @Environment(AppEnvironment.self) private var env
    @Environment(\.dismiss) private var dismiss
    @State private var store = MembersSettingsStore()
    @State private var profile = MemberProfile.empty
    @State private var signature = ""
    @State private var savedProfile = MemberProfile.empty
    @State private var savedSignature = ""
    @State private var saving = false
    @State private var confirm: AccountConfirm?

    private var me: Role { env.session.role }
    private var meID: String { env.session.user?.id ?? "" }

    var body: some View {
        AccountPage(title: store.member(id)?.displayName ?? "Member") {
            if let member = store.member(id) {
                header(member)
                roleSection(member)
                if member.role == .agent { deleteSection(member) }
                teamsSection(member)
                profileSection(member)
                if WSMemberRules.canManage(me: me, meID: meID, target: member) { removeSection(member) }
            } else if store.loaded {
                EmptyState(symbol: "person.crop.circle.badge.questionmark", title: "Not a member anymore", message: "This person has left the workspace or was removed.")
            } else if let error = store.error {
                ErrorState(message: error) { Task { await load() } }
            } else {
                WSLoading(rows: 4)
            }
        }
        .refreshable { await load() }
        .task {
            store.attach(env.session.api)
            await load()
        }
        .settingsSaveBar(dirty: profileDirty, saving: saving, save: saveProfile) {
            profile = savedProfile
            signature = savedSignature
        }
        .accountConfirm($confirm)
    }

    /// Members edit their own; the owner anyone's, admins those of agents and viewers.
    private var canEditProfile: Bool {
        guard let target = store.member(id) else { return false }
        if id == meID || me == .owner { return true }
        return me == .admin && (target.role == .agent || target.role == .viewer)
    }
    private var profileDirty: Bool { canEditProfile && (profile != savedProfile || signature != savedSignature) }

    private func load() async {
        await store.load(admin: false)
        if let member = store.member(id), !profileDirty {
            profile = member.profile
            savedProfile = member.profile
            signature = HTMLText.plain(from: member.profile.signatureHTML)
            savedSignature = signature
        }
    }

    private func header(_ member: WSMember) -> some View {
        Panel {
            HStack(spacing: 14) {
                Avatar(name: member.displayName, url: member.user.avatarURL, seed: member.id, size: 52)
                VStack(alignment: .leading, spacing: 3) {
                    Text(member.displayName).font(ODFont.sans(18, weight: .semibold)).foregroundStyle(Theme.fg)
                    if let email = member.user.email { Text(email).font(ODFont.sans(13.5)).foregroundStyle(Theme.fg3) }
                    HStack(spacing: 6) {
                        TonePill(text: member.role.title, tone: WSMemberRules.tone(member.role))
                        if let joined = member.createdAt {
                            Text("Joined \(Format.shortDate(joined))").font(ODFont.sans(12.5)).foregroundStyle(Theme.fg4)
                        }
                    }
                    .padding(.top, 2)
                }
                Spacer(minLength: 0)
            }
            .padding(14)
        }
    }

    @ViewBuilder
    private func roleSection(_ member: WSMember) -> some View {
        let manageable = WSMemberRules.canManage(me: me, meID: meID, target: member)
        AccountSection(title: "Role", footer: manageable ? nil : roleFooter(member)) {
            if manageable {
                let roles = WSMemberRules.grantable(by: me)
                ForEach(Array(roles.enumerated()), id: \.element) { index, role in
                    if index > 0 { AccountDivider(tiles: false) }
                    AccountChoiceRow(title: role.title, subtitle: WSMemberRules.hint(role), chosen: member.role == role) {
                        Task {
                            guard role != member.role else { return }
                            do {
                                try await store.setRole(member.id, role)
                                Haptics.selection()
                            } catch {
                                env.toasts.error(error)
                            }
                        }
                    }
                }
            } else {
                AccountRow(title: member.role.title, subtitle: WSMemberRules.hint(member.role))
            }
        }
    }

    private func roleFooter(_ member: WSMember) -> String {
        if member.id == meID { return "You can't change your own role." }
        if member.role == .owner { return "The owner can hand the workspace to someone else in General." }
        return "Only an owner can change an admin's role."
    }

    private func deleteSection(_ member: WSMember) -> some View {
        let rule = member.messageDelete ?? "default"
        return AccountSection(title: "Deleting messages", footer: "The workspace rule applies unless this agent has their own. Admins can always delete, read and restore messages.") {
            if env.session.isAdmin {
                VStack(alignment: .leading, spacing: 8) {
                    AccountSegmented(
                        options: ["default", "off", "own", "all"],
                        selection: Binding(get: { rule }, set: { next in
                            Task {
                                do { try await store.setDeleteRule(member.id, next) } catch { env.toasts.error(error) }
                            }
                        })
                    ) {
                        switch $0 {
                        case "default": "Workspace"
                        case "off": "Off"
                        case "all": "Any"
                        default: "Own notes"
                        }
                    }
                    Text(rule == "default" ? "Follows the workspace rule in General." : WSMemberRules.deleteRule(rule))
                        .font(ODFont.footnote)
                        .foregroundStyle(Theme.fg3)
                }
                .padding(14)
            } else {
                AccountRow(title: rule == "default" ? "Follows the workspace rule" : WSMemberRules.deleteRule(rule))
            }
        }
    }

    private func teamsSection(_ member: WSMember) -> some View {
        let teams = env.directory.teams.filter { $0.memberIDs?.contains(member.id) == true }
            .sorted { $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending }
        return AccountSection(title: "Teams") {
            if teams.isEmpty {
                AccountRow(symbol: "person.3", title: "Not on a team", subtitle: "Add them to a team in Teams.")
            }
            ForEach(Array(teams.enumerated()), id: \.element.id) { index, team in
                if index > 0 { AccountDivider() }
                SettingsDetailLink(page: .teams, id: team.id) {
                    AccountRow(symbol: "person.3", tone: .blue, title: team.name) { AccountDisclosure() }
                }
            }
        }
    }

    private func profileSection(_ member: WSMember) -> some View {
        let disabled = !canEditProfile
        return AccountSection(title: "Workspace profile", footer: "Customers see the title and pronouns with replies. The signature is added to email replies when the mailbox uses one.") {
            WSPanelField(label: "Job title", text: $profile.title, prompt: "Support lead", disabled: disabled)
            AccountDivider(tiles: false)
            WSPanelField(label: "Pronouns", text: $profile.pronouns, prompt: "she/her", disabled: disabled)
            AccountDivider(tiles: false)
            WSPanelField(label: "Phone", text: $profile.phone, prompt: "+1 555 0100", disabled: disabled)
            AccountDivider(tiles: false)
            WSPanelField(label: "Signature", text: $signature, prompt: "Best,\n\(member.displayName)", axis: .vertical, disabled: disabled)
        }
    }

    private func removeSection(_ member: WSMember) -> some View {
        AccountSection {
            Button {
                confirm = AccountConfirm(
                    title: "Remove member?",
                    message: "\(member.displayName) loses access right away. Their tickets stay, unassigned.",
                    confirmTitle: "Remove"
                ) {
                    do {
                        try await store.remove(member.id)
                        env.toasts.show("\(member.displayName) removed", kind: .success)
                        dismiss()
                    } catch {
                        env.toasts.error(error)
                    }
                }
            } label: {
                AccountRow(symbol: "person.badge.minus", tone: .red, title: "Remove from workspace", titleColor: Tone.red.text)
            }
            .buttonStyle(RowPressStyle())
        }
    }

    private func saveProfile() {
        saving = true
        Task {
            defer { saving = false }
            var next = profile
            next.title = next.title.trimmingCharacters(in: .whitespaces)
            next.pronouns = next.pronouns.trimmingCharacters(in: .whitespaces)
            next.phone = next.phone.trimmingCharacters(in: .whitespaces)
            // An unchanged signature keeps its formatting from the web.
            next.signatureHTML = signature == savedSignature ? savedProfile.signatureHTML : HTMLText.html(fromPlain: signature)
            do {
                try await store.saveProfile(id, next)
                let saved = store.member(id)?.profile ?? next
                profile = saved
                savedProfile = saved
                signature = HTMLText.plain(from: saved.signatureHTML)
                savedSignature = signature
                Haptics.success()
                env.toasts.show("Profile saved", kind: .success)
            } catch {
                env.toasts.error(error)
            }
        }
    }
}

/// Inviting people by email with one role.
struct WSInviteSheet: View {
    let grantable: [Role]
    let limit: WSBilling.Limit?
    let send: ([String], Role) async throws -> [(email: String, error: String)]

    @Environment(AppEnvironment.self) private var env
    @Environment(\.dismiss) private var dismiss
    @State private var text = ""
    @State private var role: Role = .agent
    @State private var busy = false
    @State private var error: String?

    private var emails: [String] {
        var seen = Set<String>()
        return text
            .components(separatedBy: CharacterSet(charactersIn: ",; \n\t"))
            .map { $0.trimmingCharacters(in: .whitespaces).lowercased() }
            .filter { !$0.isEmpty && seen.insert($0).inserted }
    }

    private var invalid: [String] {
        emails.filter { email in
            let parts = email.split(separator: "@")
            return parts.count != 2 || !parts[1].contains(".") || parts[1].hasSuffix(".")
        }
    }

    private var overLimit: Bool {
        guard let limit, limit.limited else { return false }
        return limit.used + emails.count > limit.max
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                AccountSheetHeader(title: "Invite people", subtitle: "They get an email with a link to join. Invitations last seven days.") { dismiss() }
                AccountTextEditor(label: "Email addresses", text: $text, prompt: "One or more addresses, separated by commas", minHeight: 90)
                    .keyboardType(.emailAddress)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                if !invalid.isEmpty {
                    Text("Check \(invalid.joined(separator: ", ")).")
                        .font(ODFont.footnote)
                        .foregroundStyle(Tone.red.text)
                }
                VStack(alignment: .leading, spacing: 8) {
                    SectionLabel("Role")
                    Panel {
                        VStack(spacing: 0) {
                            ForEach(Array(grantable.enumerated()), id: \.element) { index, option in
                                if index > 0 { AccountDivider(tiles: false) }
                                AccountChoiceRow(title: option.title, subtitle: WSMemberRules.hint(option), chosen: role == option) { role = option }
                            }
                        }
                    }
                }
                if overLimit, let limit {
                    Notice(text: "This workspace\u{2019}s plan has room for \(Swift.max(0, limit.max - limit.used)) more \(limit.max - limit.used == 1 ? "member" : "members").", tone: .amber, symbol: "exclamationmark.triangle")
                }
                FormError(text: error)
                Button(action: submit) {
                    ButtonLabel(title: emails.count > 1 ? "Send \(emails.count) invitations" : "Send invitation", busy: busy)
                }
                .buttonStyle(InkButtonStyle())
                .disabled(emails.isEmpty || !invalid.isEmpty || overLimit || busy)
            }
            .padding(22)
        }
        .scrollDismissesKeyboard(.interactively)
        .background(Theme.surface)
        .onAppear { if !grantable.contains(role) { role = grantable.last ?? .viewer } }
    }

    private func submit() {
        busy = true
        error = nil
        let list = emails
        Task {
            defer { busy = false }
            do {
                let failed = try await send(list, role)
                let sent = list.count - failed.count
                if failed.isEmpty {
                    Haptics.success()
                    env.toasts.show(sent == 1 ? "Invitation sent" : "\(sent) invitations sent", kind: .success)
                    dismiss()
                } else {
                    Haptics.error()
                    text = failed.map(\.email).joined(separator: ", ")
                    error = failed.map { "\($0.email): \($0.error)" }.joined(separator: "\n")
                    if sent > 0 { env.toasts.show(sent == 1 ? "Invitation sent" : "\(sent) invitations sent", kind: .success) }
                }
            } catch {
                self.error = error.userMessage
            }
        }
    }
}
