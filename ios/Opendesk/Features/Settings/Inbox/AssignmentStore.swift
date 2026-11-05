import Foundation
import Observation

/// Settings › Assignment: the workspace mode, team and mailbox rules, who hears about unassigned
/// tickets, auto-away, routing skills, and each teammate's availability and cap.
@MainActor
@Observable
final class AssignmentStore {
    private(set) var config: AutoAssignConfig?
    private(set) var members: [AutoAssignMember] = []
    private(set) var teams: [AutoAssignTeam] = []
    private(set) var mailboxes: [InboxMailboxRef] = []
    private(set) var skills: [AutoAssignSkill] = []
    private(set) var load: InboxLoad = .loading

    @ObservationIgnored private var api: APIClient?
    @ObservationIgnored private var directory: DirectoryStore?
    @ObservationIgnored private var isAdmin = false

    func start(api: APIClient, directory: DirectoryStore, isAdmin: Bool) async {
        self.api = api
        self.directory = directory
        self.isAdmin = isAdmin
        await reload()
    }

    func reload() async {
        guard let api else { return }
        do {
            async let config: AutoAssignConfig = api.get("v1/assignment")
            async let members: [AutoAssignMember]? = try? api.all("v1/members")
            async let teams: Page<AutoAssignTeam>? = try? api.get("v1/teams", query: [URLQueryItem(name: "limit", value: "100")])
            async let mailboxes: Page<InboxMailboxRef>? = try? api.get("v1/mailboxes")
            async let skills: Page<AutoAssignSkill>? = isAdmin ? try? api.get("v1/skills") : nil
            self.config = try await config
            if let all = await members { self.members = all }
            if let page = await teams { self.teams = page.data.sorted { $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending } }
            if let page = await mailboxes { self.mailboxes = page.data }
            if let page = await skills { self.skills = page.data }
            load = .ready
        } catch {
            if error.isCancellation { return }
            if load != .ready { load = .failed(error.userMessage) }
        }
    }

    /// Viewers are never assigned.
    var assignable: [AutoAssignMember] { members.filter { $0.role != .viewer } }

    // MARK: Settings

    private struct Patch: Encodable, Sendable {
        var mode: AutoAssignMode?
        var rules: [AutoAssignRule]?
        var autoAwayMinutes: Int?
        var newTicketNotify: UnassignedNotify?
        var followUpKeepsAssignee: Bool?

        enum CodingKeys: String, CodingKey {
            case mode, rules
            case autoAwayMinutes = "auto_away_minutes"
            case newTicketNotify = "new_ticket_notify"
            case followUpKeepsAssignee = "followup_keeps_assignee"
        }
    }

    /// Applied at once, put back if the server refuses it.
    private func write(_ patch: Patch, apply: (inout AutoAssignConfig) -> Void) async throws {
        guard let api, var next = config else { return }
        let before = config
        apply(&next)
        config = next
        do {
            config = try await api.patch("v1/assignment", body: patch)
        } catch {
            config = before
            throw error
        }
    }

    func setMode(_ mode: AutoAssignMode) async throws {
        try await write(Patch(mode: mode)) { $0.mode = mode }
    }

    func setRules(_ rules: [AutoAssignRule]) async throws {
        try await write(Patch(rules: rules)) { $0.rules = rules }
        changed()
    }

    func setNotify(_ value: UnassignedNotify) async throws {
        try await write(Patch(newTicketNotify: value)) { $0.newTicketNotify = value }
    }

    func setFollowUp(_ on: Bool) async throws {
        try await write(Patch(followUpKeepsAssignee: on)) { $0.followUpKeepsAssignee = on }
    }

    func setAway(_ minutes: Int) async throws {
        try await write(Patch(autoAwayMinutes: minutes)) { $0.autoAwayMinutes = minutes }
    }

    // MARK: Skills

    func createSkill(_ name: String) async throws -> AutoAssignSkill {
        guard let api else { throw APIError.signedOut }
        let skill: AutoAssignSkill = try await api.post("v1/skills", body: ["name": name])
        upsert(skill)
        return skill
    }

    func setSkillArchived(_ skill: AutoAssignSkill, _ archived: Bool) async throws {
        guard let api else { return }
        let saved: AutoAssignSkill = try await api.patch("v1/skills/\(skill.id)", body: ["archived": archived])
        upsert(saved)
    }

    func setSkillMembers(_ skill: AutoAssignSkill, _ ids: [String]) async throws {
        guard let api else { return }
        struct Body: Encodable, Sendable { let member_ids: [String] }
        let saved: AutoAssignSkill = try await api.send(APIRequest(method: "PUT", path: "v1/skills/\(skill.id)/members", body: try APICoding.encoder.encode(Body(member_ids: ids))))
        upsert(saved)
    }

    private func upsert(_ skill: AutoAssignSkill) {
        if let index = skills.firstIndex(where: { $0.id == skill.id }) { skills[index] = skill } else { skills.append(skill) }
    }

    // MARK: Members

    func setAvailability(_ member: AutoAssignMember, _ value: Availability) async throws {
        try await patchMember(member, ["availability": .string(value.rawValue)]) {
            $0.availability = value
            $0.presence = value
        }
    }

    func setCap(_ member: AutoAssignMember, _ cap: Int?) async throws {
        try await patchMember(member, ["max_open_tickets": cap.map { .number(Double($0)) } ?? .null]) { $0.maxOpenTickets = cap }
    }

    private func patchMember(_ member: AutoAssignMember, _ body: [String: JSONValue], apply: (inout AutoAssignMember) -> Void) async throws {
        guard let api else { return }
        let before = members
        if let index = members.firstIndex(where: { $0.id == member.id }) { apply(&members[index]) }
        do {
            let _: EmptyBody = try await api.patch("v1/members/\(member.id)/availability", body: JSONValue.object(body))
            changed()
        } catch {
            members = before
            throw error
        }
    }

    /// Assignee pickers and presence read members from the directory.
    private func changed() {
        guard let directory else { return }
        Task { await directory.load() }
    }
}
