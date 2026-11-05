import Foundation

// The Inbox settings group's API shapes (contracts/api.md: labels, custom fields, assignment,
// triage, business hours, SLA policies, automations). Every decoder is lenient: a missing
// member falls back to the server's default, so a newer or older server never blanks a page.

// MARK: - Labels

/// Where a label can be used (`applies_to`).
enum LabelScope: String, CaseIterable, Codable, Sendable {
    case tickets, contacts, companies

    var title: String {
        switch self {
        case .tickets: "Tickets"
        case .contacts: "Contacts"
        case .companies: "Companies"
        }
    }

    var symbol: String {
        switch self {
        case .tickets: "ticket"
        case .contacts: "person"
        case .companies: "building.2"
        }
    }
}

/// A label as the settings page reads it (`GET /v1/tags`), with its place, scope and usage.
struct SettingsLabel: Decodable, Hashable, Sendable, Identifiable {
    struct Usage: Decodable, Hashable, Sendable {
        var tickets = 0
        var contacts = 0
        var companies = 0

        init() {}

        init(from decoder: Decoder) throws {
            let c = try decoder.container(keyedBy: CodingKeys.self)
            tickets = c.lenient(Int.self, forKey: .tickets) ?? 0
            contacts = c.lenient(Int.self, forKey: .contacts) ?? 0
            companies = c.lenient(Int.self, forKey: .companies) ?? 0
        }

        enum CodingKeys: String, CodingKey { case tickets, contacts, companies }

        var total: Int { tickets + contacts + companies }
    }

    var id: String
    var name: String
    var color: Tone
    var description: String
    var groupID: String?
    var position: Int
    var archived: Bool
    var appliesTo: [LabelScope]
    var usage: Usage?

    enum CodingKeys: String, CodingKey {
        case id, name, color, description, position, archived, usage
        case groupID = "group_id"
        case appliesTo = "applies_to"
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        name = c.lenient(String.self, forKey: .name) ?? ""
        let raw = c.lenient(String.self, forKey: .color)
        color = raw.flatMap { Tone(rawValue: $0) } ?? LabelPalette.tone(for: name)
        description = c.lenient(String.self, forKey: .description) ?? ""
        groupID = c.lenient(String.self, forKey: .groupID)
        position = c.lenient(Int.self, forKey: .position) ?? 0
        archived = c.lenient(Bool.self, forKey: .archived) ?? false
        let scopes = (c.lenient([String].self, forKey: .appliesTo) ?? []).compactMap(LabelScope.init(rawValue:))
        appliesTo = scopes.isEmpty ? LabelScope.allCases : scopes
        usage = c.lenient(Usage.self, forKey: .usage)
    }
}

/// A label group (`GET /v1/tag-groups`); an exclusive group allows one label per record.
struct SettingsLabelGroup: Decodable, Hashable, Sendable, Identifiable {
    var id: String
    var name: String
    var color: Tone?
    var exclusive: Bool
    var position: Int

    enum CodingKeys: String, CodingKey { case id, name, color, exclusive, position }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        name = c.lenient(String.self, forKey: .name) ?? ""
        color = c.lenient(String.self, forKey: .color).flatMap { Tone(rawValue: $0) }
        exclusive = c.lenient(Bool.self, forKey: .exclusive) ?? false
        position = c.lenient(Int.self, forKey: .position) ?? 0
    }
}

/// The web's label colours (`TONES`) and its default colour for a name (`toneFor`).
enum LabelPalette {
    static let tones: [Tone] = Tone.allCases

    /// Hues that tell things apart without reading as a warning (the web's identity tones).
    private static let identity: [Tone] = [.blue, .indigo, .violet, .pink, .orange, .amber, .lime, .green, .teal, .cyan]

    /// The web's stable colour for a label name with none chosen.
    static func tone(for name: String) -> Tone {
        var hash: Int32 = 0
        for unit in (name.isEmpty ? "?" : name).utf16 { hash = hash &* 31 &+ Int32(unit) }
        return identity[abs(Int(hash)) % identity.count]
    }
}

// MARK: - Custom fields

extension CustomField.Kind {
    /// Every type, in the web's order (`FIELD_TYPES`).
    static let ordered: [CustomField.Kind] = [.text, .longText, .number, .select, .multiSelect, .date, .checkbox, .url, .email, .user]

    var kindTitle: String {
        switch self {
        case .text: "Text"
        case .longText: "Long text"
        case .number: "Number"
        case .select: "Select"
        case .multiSelect: "Multi-select"
        case .date: "Date"
        case .checkbox: "Checkbox"
        case .url: "Link"
        case .email: "Email"
        case .user: "Person"
        }
    }

    /// A quiet hue per kind of value, so a list of fields scans by type (the web's `typeTone`).
    var kindTone: Tone {
        switch self {
        case .text, .longText: .gray
        case .number: .blue
        case .select: .violet
        case .multiSelect: .purple
        case .date: .amber
        case .checkbox: .green
        case .url: .cyan
        case .email: .teal
        case .user: .indigo
        }
    }

    var takesOptions: Bool { self == .select || self == .multiSelect }
}

/// Which records a custom field belongs to.
enum FieldEntity: String, CaseIterable, Sendable {
    case ticket, contact, company

    var title: String {
        switch self {
        case .ticket: "Ticket"
        case .contact: "Contact"
        case .company: "Company"
        }
    }

    var plural: String {
        switch self {
        case .ticket: "Tickets"
        case .contact: "Contacts"
        case .company: "Companies"
        }
    }

    var emptyHint: String {
        switch self {
        case .ticket: "Add fields like order number or product area to capture details on every ticket."
        case .contact: "Add fields like plan or account manager to keep customer details in one place."
        case .company: "Add fields like renewal date or seats to track accounts."
        }
    }
}

// MARK: - Assignment

/// How new tickets are handed out.
enum AutoAssignMode: String, LenientEnum, CaseIterable, Identifiable {
    case none
    case roundRobin = "round_robin"
    case loadBalanced = "load_balanced"
    static let fallback = AutoAssignMode.none
    var id: String { rawValue }

    var title: String {
        switch self {
        case .none: "Off"
        case .roundRobin: "Round robin"
        case .loadBalanced: "Load balanced"
        }
    }

    var hint: String {
        switch self {
        case .none: "Tickets wait in Unassigned until someone takes them."
        case .roundRobin: "Teammates take turns, so the next ticket goes to the next person."
        case .loadBalanced: "The next ticket goes to whoever has the fewest open tickets."
        }
    }

    var symbol: String {
        switch self {
        case .none: "circle.slash"
        case .roundRobin: "repeat"
        case .loadBalanced: "scalemass"
        }
    }

    var tone: Tone {
        switch self {
        case .none: .gray
        case .roundRobin: .blue
        case .loadBalanced: .violet
        }
    }
}

/// Who hears about a new ticket nobody is assigned to (`settings.new_ticket_notify`).
enum UnassignedNotify: String, LenientEnum, CaseIterable, Identifiable {
    case auto, everyone, leads, off
    static let fallback = UnassignedNotify.auto
    var id: String { rawValue }

    var title: String {
        switch self {
        case .auto: "Automatic"
        case .everyone: "Everyone on the team"
        case .leads: "Leads and admins only"
        case .off: "Off"
        }
    }

    var hint: String {
        switch self {
        case .auto: "Everyone on small teams, only leads when more than 5 people would be notified."
        case .everyone: "Everyone on the ticket's team, or everyone in the workspace when it has no team."
        case .leads: "The team's leads, or the owners and admins when it has no team or no lead."
        case .off: "Nobody is notified. New tickets wait in Unassigned."
        }
    }

    var symbol: String {
        switch self {
        case .auto: "wand.and.stars"
        case .everyone: "person.3"
        case .leads: "crown"
        case .off: "bell.slash"
        }
    }
}

/// A team or a mailbox with a mode of its own.
struct AutoAssignRule: Codable, Hashable, Sendable {
    enum Target: String, LenientEnum {
        case team, mailbox
        static let fallback = Target.mailbox
    }

    var targetType: Target
    var targetID: String
    var mode: AutoAssignMode

    var key: String { "\(targetType.rawValue):\(targetID)" }

    enum CodingKeys: String, CodingKey {
        case mode
        case targetType = "target_type"
        case targetID = "target_id"
    }
}

/// `GET /v1/assignment`.
struct AutoAssignConfig: Decodable, Hashable, Sendable {
    var mode: AutoAssignMode
    var rules: [AutoAssignRule]
    /// 0 is off, else 10..1440.
    var autoAwayMinutes: Int
    var newTicketNotify: UnassignedNotify
    /// A customer's follow-up of a closed ticket goes to that ticket's assignee.
    var followUpKeepsAssignee: Bool

    enum CodingKeys: String, CodingKey {
        case mode, rules
        case autoAwayMinutes = "auto_away_minutes"
        case newTicketNotify = "new_ticket_notify"
        case followUpKeepsAssignee = "followup_keeps_assignee"
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        mode = c.lenient(AutoAssignMode.self, forKey: .mode) ?? .none
        rules = c.lenient([AutoAssignRule].self, forKey: .rules) ?? []
        autoAwayMinutes = c.lenient(Int.self, forKey: .autoAwayMinutes) ?? 0
        newTicketNotify = c.lenient(UnassignedNotify.self, forKey: .newTicketNotify) ?? .auto
        followUpKeepsAssignee = c.lenient(Bool.self, forKey: .followUpKeepsAssignee) ?? false
    }
}

/// A routing skill (`/v1/skills`): expertise a routing rule can require.
struct AutoAssignSkill: Decodable, Hashable, Sendable, Identifiable {
    var id: String
    var name: String
    var archived: Bool
    var memberIDs: [String]

    enum CodingKeys: String, CodingKey {
        case id, name, archived
        case memberIDs = "member_ids"
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        name = c.lenient(String.self, forKey: .name) ?? ""
        archived = c.lenient(Bool.self, forKey: .archived) ?? false
        memberIDs = c.lenient([String].self, forKey: .memberIDs) ?? []
    }
}

/// A member as the assignment page reads them: availability, presence, load and cap.
struct AutoAssignMember: Decodable, Hashable, Sendable, Identifiable {
    var user: MemberUser
    var role: Role
    var availability: Availability
    var presence: Availability
    var maxOpenTickets: Int?
    var openTickets: Int

    var id: String { user.id }

    enum CodingKeys: String, CodingKey {
        case user, role, availability, presence
        case maxOpenTickets = "max_open_tickets"
        case openTickets = "open_tickets"
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        user = try c.decode(MemberUser.self, forKey: .user)
        role = c.lenient(Role.self, forKey: .role) ?? .viewer
        availability = c.lenient(Availability.self, forKey: .availability) ?? .offline
        presence = c.lenient(Availability.self, forKey: .presence) ?? availability
        maxOpenTickets = c.lenient(Int.self, forKey: .maxOpenTickets)
        openTickets = c.lenient(Int.self, forKey: .openTickets) ?? 0
    }

    /// Online and under their cap: who automatic assignment can pick now.
    var eligible: Bool { presence == .online && !atCap }
    var atCap: Bool { maxOpenTickets.map { openTickets >= $0 } ?? false }
    /// Set online, but away because they went idle.
    var idle: Bool { availability == .online && presence == .away }
}

/// A team as the assignment page reads it: who takes tickets, and its own mode.
struct AutoAssignTeam: Decodable, Hashable, Sendable, Identifiable {
    struct Seat: Decodable, Hashable, Sendable {
        var userID: String
        var role: String

        enum CodingKeys: String, CodingKey {
            case role
            case userID = "user_id"
        }
    }

    var id: String
    var name: String
    var color: Tone
    var isPrivate: Bool
    var members: [Seat]
    var assignMode: AutoAssignMode?

    enum CodingKeys: String, CodingKey {
        case id, name, color, members
        case isPrivate = "private"
        case memberIDs = "member_ids"
        case assignMode = "assign_mode"
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        name = c.lenient(String.self, forKey: .name) ?? ""
        color = Tone(label: c.lenient(String.self, forKey: .color) ?? Theme.avatarTone(for: id).rawValue)
        isPrivate = c.lenient(Bool.self, forKey: .isPrivate) ?? false
        assignMode = c.lenient(AutoAssignMode.self, forKey: .assignMode)
        if let seats = c.lenient([Seat].self, forKey: .members) {
            members = seats
        } else {
            // Older servers list ids only: everyone takes tickets.
            members = (c.lenient([String].self, forKey: .memberIDs) ?? []).map { Seat(userID: $0, role: "member") }
        }
    }

    /// Leads and members take tickets; observers never do.
    func assigns(_ userID: String) -> Bool {
        members.contains { $0.userID == userID && $0.role != "observer" }
    }
}

/// A mailbox, the part rules and automations name (`GET /v1/mailboxes`).
struct InboxMailboxRef: Decodable, Hashable, Sendable, Identifiable {
    var id: String
    var name: String
    var email: String

    enum CodingKeys: String, CodingKey { case id, name, email, address }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        email = c.lenient(String.self, forKey: .email) ?? c.lenient(String.self, forKey: .address) ?? ""
        let name = c.lenient(String.self, forKey: .name) ?? ""
        self.name = name.isEmpty ? email : name
    }
}

// MARK: - Triage

/// What triage may set on a ticket.
enum TriageField: String, CaseIterable, Sendable {
    case priority, severity, labels, team

    var title: String {
        switch self {
        case .priority: "Priority"
        case .severity: "Severity"
        case .labels: "Labels"
        case .team: "Team"
        }
    }

    var hint: String {
        switch self {
        case .priority: "Raise the priority when a ticket looks urgent. It never lowers it."
        case .severity: "Set Sev 1 to Sev 4 from the business impact. It only ever raises a severity."
        case .labels: "Add labels that match what the ticket is about."
        case .team: "Route a ticket with no team to the team that fits it best."
        }
    }

    var symbol: String {
        switch self {
        case .priority: "gauge.with.dots.needle.50percent"
        case .severity: "exclamationmark.triangle"
        case .labels: "tag"
        case .team: "person.3"
        }
    }
}

/// The decision engine behind triage. The decision model is never named.
struct TriageEngine: Decodable, Hashable, Sendable {
    var provider: String
    var model: String
    var connected: Bool

    enum CodingKeys: String, CodingKey { case provider, model, connected }

    init(provider: String = "none", model: String = "", connected: Bool = false) {
        self.provider = provider
        self.model = model
        self.connected = connected
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        provider = c.lenient(String.self, forKey: .provider) ?? "none"
        model = c.lenient(String.self, forKey: .model) ?? ""
        connected = c.lenient(Bool.self, forKey: .connected) ?? false
    }

    var isDecisionModel: Bool { provider == "typesafe" }
    var isLLM: Bool { provider == "llm" }
}

/// `GET /v1/triage`.
struct TriageConfig: Decodable, Hashable, Sendable {
    var autoTriage: Bool
    var fields: [TriageField]
    var assignAfterTriage: Bool
    /// 0.5..0.99: decisions at or above it apply, below it they become suggestions.
    var threshold: Double
    var engine: TriageEngine
    var available: Bool

    enum CodingKeys: String, CodingKey {
        case fields, threshold, engine, available
        case autoTriage = "auto_triage"
        case assignAfterTriage = "assign_after_triage"
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        autoTriage = c.lenient(Bool.self, forKey: .autoTriage) ?? false
        fields = (c.lenient([String].self, forKey: .fields) ?? []).compactMap(TriageField.init(rawValue:))
        assignAfterTriage = c.lenient(Bool.self, forKey: .assignAfterTriage) ?? false
        threshold = c.lenient(Double.self, forKey: .threshold) ?? 0.8
        engine = c.lenient(TriageEngine.self, forKey: .engine) ?? TriageEngine()
        available = c.lenient(Bool.self, forKey: .available) ?? false
    }
}

// MARK: - Business hours

enum HoursDay: String, CaseIterable, Sendable {
    case mon, tue, wed, thu, fri, sat, sun

    var title: String {
        switch self {
        case .mon: "Monday"
        case .tue: "Tuesday"
        case .wed: "Wednesday"
        case .thu: "Thursday"
        case .fri: "Friday"
        case .sat: "Saturday"
        case .sun: "Sunday"
        }
    }

    static let weekdays: [HoursDay] = [.mon, .tue, .wed, .thu, .fri]
}

/// "HH:MM", 24h, on the quarter hour; "24:00" ends a day.
struct HoursRange: Codable, Hashable, Sendable {
    var start: String
    var end: String

    var startMinutes: Int { HoursRange.minutes(start) }
    var endMinutes: Int { HoursRange.minutes(end) }

    static func minutes(_ hhmm: String) -> Int {
        let parts = hhmm.split(separator: ":").compactMap { Int($0) }
        guard parts.count == 2 else { return 0 }
        return parts[0] * 60 + parts[1]
    }

    static func hhmm(_ minutes: Int) -> String {
        let clamped = max(0, min(24 * 60, Int((Double(minutes) / 15).rounded()) * 15))
        return String(format: "%02d:%02d", clamped / 60, clamped % 60)
    }
}

struct HoursHoliday: Codable, Hashable, Sendable {
    /// "YYYY-MM-DD".
    var date: String
    var name: String
}

/// `GET`/`PUT /v1/business-hours`; the full object is written, and `timezone` is the workspace's.
struct HoursConfig: Codable, Hashable, Sendable {
    var enabled: Bool
    var timezone: String
    var schedule: [String: [HoursRange]]
    var holidays: [HoursHoliday]

    enum CodingKeys: String, CodingKey { case enabled, timezone, schedule, holidays }

    init(enabled: Bool = false, timezone: String = "UTC", schedule: [String: [HoursRange]] = [:], holidays: [HoursHoliday] = []) {
        self.enabled = enabled
        self.timezone = timezone
        self.schedule = schedule
        self.holidays = holidays
        normalize()
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        enabled = c.lenient(Bool.self, forKey: .enabled) ?? false
        timezone = c.lenient(String.self, forKey: .timezone) ?? "UTC"
        schedule = c.lenient([String: [HoursRange]].self, forKey: .schedule) ?? [:]
        holidays = c.lenient([HoursHoliday].self, forKey: .holidays) ?? []
        normalize()
    }

    /// Every day present, holidays in date order (the web's `norm`).
    mutating func normalize() {
        var full: [String: [HoursRange]] = [:]
        for day in HoursDay.allCases { full[day.rawValue] = schedule[day.rawValue] ?? [] }
        schedule = full
        holidays.sort { $0.date < $1.date }
    }

    func ranges(_ day: HoursDay) -> [HoursRange] { schedule[day.rawValue] ?? [] }

    var weeklyMinutes: Int {
        HoursDay.allCases.reduce(0) { sum, day in sum + ranges(day).reduce(0) { $0 + max(0, $1.endMinutes - $1.startMinutes) } }
    }

    /// The first problem in a day's ranges: one that ends before it starts, or two that overlap.
    static func problem(_ ranges: [HoursRange]) -> String? {
        let sorted = ranges.sorted { $0.startMinutes < $1.startMinutes }
        if sorted.contains(where: { $0.startMinutes >= $0.endMinutes }) { return "Each range has to end after it starts." }
        for index in sorted.indices.dropFirst() where sorted[index].startMinutes < sorted[index - 1].endMinutes {
            return "These ranges overlap."
        }
        return nil
    }

    /// Whether the workspace is open at `date` in its time zone.
    func isOpen(at date: Date) -> Bool? {
        guard let zone = TimeZone(identifier: timezone) else { return nil }
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = zone
        let parts = calendar.dateComponents([.year, .month, .day, .weekday, .hour, .minute], from: date)
        guard let year = parts.year, let month = parts.month, let dayOfMonth = parts.day, let weekday = parts.weekday else { return nil }
        let iso = String(format: "%04d-%02d-%02d", year, month, dayOfMonth)
        if holidays.contains(where: { $0.date == iso }) { return false }
        // Calendar weekdays start on Sunday.
        let day = [HoursDay.sun, .mon, .tue, .wed, .thu, .fri, .sat][(weekday - 1) % 7]
        let at = (parts.hour ?? 0) * 60 + (parts.minute ?? 0)
        return ranges(day).contains { at >= $0.startMinutes && at < $0.endMinutes }
    }
}

// MARK: - SLA policies

struct SLAPolicy: Decodable, Hashable, Sendable, Identifiable {
    var id: String
    var name: String
    var priority: Priority?
    var firstResponseMinutes: Int
    var resolutionMinutes: Int
    var businessHours: Bool
    var enabled: Bool
    var position: Int
    var teamID: String?

    enum CodingKeys: String, CodingKey {
        case id, name, priority, enabled, position
        case firstResponseMinutes = "first_response_minutes"
        case resolutionMinutes = "resolution_minutes"
        case businessHours = "business_hours"
        case teamID = "team_id"
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        name = c.lenient(String.self, forKey: .name) ?? ""
        priority = c.lenient(String.self, forKey: .priority).flatMap(Priority.init(rawValue:))
        firstResponseMinutes = c.lenient(Int.self, forKey: .firstResponseMinutes) ?? 60
        resolutionMinutes = c.lenient(Int.self, forKey: .resolutionMinutes) ?? 1440
        businessHours = c.lenient(Bool.self, forKey: .businessHours) ?? false
        enabled = c.lenient(Bool.self, forKey: .enabled) ?? true
        position = c.lenient(Int.self, forKey: .position) ?? 0
        teamID = c.lenient(String.self, forKey: .teamID)
    }
}

// MARK: - Automations

struct AutomationRule: Decodable, Hashable, Sendable, Identifiable {
    var id: String
    var name: String
    var trigger: AutomationTrigger
    var conditions: [AutomationCondition]
    var actions: [AutomationAction]
    var enabled: Bool
    var position: Int
    var runCount: Int

    enum CodingKeys: String, CodingKey {
        case id, name, trigger, conditions, actions, enabled, position
        case runCount = "run_count"
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        name = c.lenient(String.self, forKey: .name) ?? ""
        trigger = c.lenient(AutomationTrigger.self, forKey: .trigger) ?? .ticketCreated
        conditions = c.lenient([AutomationCondition].self, forKey: .conditions) ?? []
        actions = c.lenient([AutomationAction].self, forKey: .actions) ?? []
        enabled = c.lenient(Bool.self, forKey: .enabled) ?? true
        position = c.lenient(Int.self, forKey: .position) ?? 0
        runCount = c.lenient(Int.self, forKey: .runCount) ?? 0
    }
}

/// A rule's condition: a built-in field, or `field.<key>` / `contact.field.<key>` with its type's operators.
struct AutomationCondition: Codable, Hashable, Sendable {
    var field: String
    var op: String
    var value: String
    /// `ai_decide`: the yes/no question.
    var question: String?

    init(field: String, op: String, value: String, question: String? = nil) {
        self.field = field
        self.op = op
        self.value = value
        self.question = question
    }

    enum CodingKeys: String, CodingKey { case field, op, value, question }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        field = c.lenient(String.self, forKey: .field) ?? "subject"
        op = c.lenient(String.self, forKey: .op) ?? "contains"
        value = c.lenient(String.self, forKey: .value) ?? ""
        question = c.lenient(String.self, forKey: .question)
    }

    func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(field, forKey: .field)
        try c.encode(op, forKey: .op)
        try c.encode(value, forKey: .value)
        if field == "ai_decide" { try c.encode(question ?? "", forKey: .question) }
    }
}

struct AutomationAction: Codable, Hashable, Sendable {
    var type: String
    var value: String
    /// `set_field`: the ticket field's key.
    var field: String?
    /// `merge_into_open_ticket`: only a ticket with the same subject.
    var sameSubject: Bool?

    init(type: String, value: String = "", field: String? = nil, sameSubject: Bool? = nil) {
        self.type = type
        self.value = value
        self.field = field
        self.sameSubject = sameSubject
    }

    enum CodingKeys: String, CodingKey {
        case type, value, field
        case sameSubject = "same_subject"
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        type = c.lenient(String.self, forKey: .type) ?? "add_tag"
        value = c.lenient(String.self, forKey: .value) ?? ""
        field = c.lenient(String.self, forKey: .field)
        sameSubject = c.lenient(Bool.self, forKey: .sameSubject)
    }

    func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(type, forKey: .type)
        try c.encode(value, forKey: .value)
        if let field { try c.encode(field, forKey: .field) }
        if type == "merge_into_open_ticket" { try c.encode(sameSubject ?? true, forKey: .sameSubject) }
    }
}

// MARK: - Workspace flags the group reads

/// The workspace settings these pages read (`GET /v1/workspace`): who creates labels and whether severity is used.
struct InboxWorkspaceFlags: Decodable, Sendable {
    var labelCreation: String
    var severityEnabled: Bool

    init(labelCreation: String = "anyone", severityEnabled: Bool = true) {
        self.labelCreation = labelCreation
        self.severityEnabled = severityEnabled
    }

    enum Root: String, CodingKey { case settings }
    enum Keys: String, CodingKey {
        case labelCreation = "label_creation"
        case severityEnabled = "severity_enabled"
    }

    init(from decoder: Decoder) throws {
        let root = try decoder.container(keyedBy: Root.self)
        let settings = try? root.nestedContainer(keyedBy: Keys.self, forKey: .settings)
        labelCreation = settings?.lenient(String.self, forKey: .labelCreation) ?? "anyone"
        severityEnabled = settings?.lenient(Bool.self, forKey: .severityEnabled) ?? true
    }
}
