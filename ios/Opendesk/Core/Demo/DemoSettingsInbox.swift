#if DEBUG
import Foundation

// The demo's Inbox settings (`Features/Settings/Inbox`): labels and their groups, custom fields,
// assignment and skills, triage, business hours, SLA policies and automations. Labels and fields
// stay in the arrays other screens read (`ws.tags`, `ws.directory`, `ws.ticketExtras.fields`);
// what only these pages know (groups, order, caps, rules) lives here.

struct DemoInboxSettings: Sendable {
    struct LabelGroup: Sendable {
        var id: String
        var name: String
        var color: String?
        var exclusive: Bool
        var position: Int
    }

    struct Rule: Sendable {
        var targetType: String
        var targetID: String
        var mode: String
    }

    struct Skill: Sendable {
        var id: String
        var name: String
        var archived = false
        var memberIDs: [String]
    }

    struct Policy: Sendable {
        var id: String
        var name: String
        var priority: String?
        var firstResponse: Int
        var resolution: Int
        var businessHours: Bool
        var enabled = true
        var position: Int
        var teamID: String?
    }

    struct Automation: Sendable {
        var id: String
        var name: String
        var trigger: String
        var conditions: [AutomationCondition]
        var actions: [AutomationAction]
        var enabled = true
        var position: Int
        var runCount: Int
    }

    var applied = false
    var groups: [LabelGroup] = []
    /// Label id to the group it starts in, written into the labels on the first call.
    var startingGroups: [String: String] = [:]
    var positions: [String: Int] = [:]
    var labelCreation = "anyone"
    var severityEnabled = true

    var mode = "none"
    var rules: [Rule] = []
    var autoAwayMinutes = 0
    var newTicketNotify = "auto"
    var followUpKeepsAssignee = false
    var caps: [String: Int] = [:]
    var skills: [Skill] = []

    var autoTriage = false
    var triageFields = ["priority", "labels"]
    var assignAfterTriage = false
    var threshold = 0.8
    var engine: [String: JSONValue] = ["provider": .string("llm"), "model": .string(""), "connected": .bool(true)]

    var hours = HoursConfig()
    var policies: [Policy] = []
    var automations: [Automation] = []

    static func seed(_ ws: DemoWorkspace) -> DemoInboxSettings {
        ws.id == DemoSeed.marginID ? margin(ws) : lumen(ws)
    }

    private static func id(_ n: Int) -> String { demoID(.made, 9600 + n) }

    private static func week(_ days: [HoursDay], _ start: String, _ end: String) -> [String: [HoursRange]] {
        var schedule: [String: [HoursRange]] = [:]
        for day in days { schedule[day.rawValue] = [HoursRange(start: start, end: end)] }
        return schedule
    }

    private static func margin(_ ws: DemoWorkspace) -> DemoInboxSettings {
        var s = DemoInboxSettings()
        let type = id(1), area = id(2)
        s.groups = [
            LabelGroup(id: type, name: "Type", color: "blue", exclusive: true, position: 0),
            LabelGroup(id: area, name: "Product area", color: "violet", exclusive: false, position: 1),
        ]
        for tag in ws.tags {
            switch tag.name {
            case "Question", "Problem", "Bug", "Feature request", "Feedback": s.startingGroups[tag.id] = type
            case "SSO", "API", "Sync", "Export", "Mobile", "Onboarding": s.startingGroups[tag.id] = area
            default: break
            }
        }

        s.mode = "round_robin"
        s.rules = [
            Rule(targetType: "team", targetID: DemoSeed.billing, mode: "load_balanced"),
            Rule(targetType: "mailbox", targetID: DemoSeed.billingMailbox, mode: "round_robin"),
        ]
        s.autoAwayMinutes = 30
        s.newTicketNotify = "auto"
        s.caps = [DemoPerson.priya.id: 8, DemoPerson.sam.id: 12, DemoPerson.lea.id: 6]
        s.skills = [
            Skill(id: id(11), name: "Billing and refunds", memberIDs: [DemoPerson.lea.id, DemoPerson.maya.id]),
            Skill(id: id(12), name: "SSO and identity", memberIDs: [DemoPerson.priya.id, DemoPerson.jonas.id]),
            Skill(id: id(13), name: "API and webhooks", memberIDs: [DemoPerson.sam.id, DemoPerson.jonas.id]),
            Skill(id: id(14), name: "German", archived: true, memberIDs: [DemoPerson.lea.id]),
        ]

        s.autoTriage = true
        s.triageFields = ["priority", "labels", "team"]
        s.assignAfterTriage = true
        s.threshold = 0.8
        s.engine = ["provider": .string("typesafe"), "model": .string(""), "connected": .bool(true)]

        var schedule = week(HoursDay.weekdays, "09:00", "17:30")
        schedule["fri"] = [HoursRange(start: "09:00", end: "12:30"), HoursRange(start: "13:30", end: "16:00")]
        s.hours = HoursConfig(enabled: true, timezone: ws.workspace.settings?.timezone ?? DemoMode.timeZone, schedule: schedule, holidays: [
            HoursHoliday(date: "2026-10-03", name: "Day of German Unity"),
            HoursHoliday(date: "2026-12-24", name: "Christmas Eve"),
            HoursHoliday(date: "2026-12-25", name: "Christmas Day"),
            HoursHoliday(date: "2026-12-26", name: "Boxing Day"),
            HoursHoliday(date: "2026-12-31", name: "New Year's Eve"),
            HoursHoliday(date: "2027-01-01", name: "New Year's Day"),
        ])

        s.policies = [
            Policy(id: id(21), name: "Urgent tickets", priority: "urgent", firstResponse: 30, resolution: 240, businessHours: false, position: 0),
            Policy(id: id(22), name: "High priority", priority: "high", firstResponse: 60, resolution: 480, businessHours: true, position: 1),
            Policy(id: id(23), name: "Billing questions", priority: nil, firstResponse: 240, resolution: 2880, businessHours: true, position: 2, teamID: DemoSeed.billing),
            Policy(id: id(24), name: "Weekend enterprise cover", priority: "high", firstResponse: 120, resolution: 1440, businessHours: false, enabled: false, position: 3),
            Policy(id: id(25), name: "Everything else", priority: nil, firstResponse: 480, resolution: 4320, businessHours: true, position: 4),
        ]

        s.automations = [
            Automation(id: id(31), name: "Route billing mail to the billing team", trigger: "ticket_created",
                       conditions: [AutomationCondition(field: "mailbox_id", op: "equals", value: DemoSeed.billingMailbox)],
                       actions: [AutomationAction(type: "assign_team", value: DemoSeed.billing), AutomationAction(type: "add_tag", value: "Billing")],
                       position: 0, runCount: 412),
            Automation(id: id(32), name: "Label sign-in questions", trigger: "ticket_created",
                       conditions: [AutomationCondition(field: "subject", op: "any", value: "SSO\nSAML\nOkta\nAzure AD")],
                       actions: [AutomationAction(type: "add_tag", value: "SSO"), AutomationAction(type: "set_priority", value: "high")],
                       position: 1, runCount: 87),
            Automation(id: id(33), name: "Escalate frustrated customers", trigger: "customer_replied",
                       conditions: [AutomationCondition(field: "sentiment", op: "equals", value: "frustrated")],
                       actions: [AutomationAction(type: "set_priority", value: "urgent"), AutomationAction(type: "add_note", value: "<p>The customer sounds frustrated. A lead should take a look.</p>")],
                       position: 2, runCount: 23),
            Automation(id: id(34), name: "Close obvious spam", trigger: "ticket_created",
                       conditions: [AutomationCondition(field: "from_email", op: "any", value: "@promo-mail.biz\n@seo-boost.io")],
                       actions: [AutomationAction(type: "mark_spam")],
                       position: 3, runCount: 156),
            Automation(id: id(35), name: "Merge quick follow-ups", trigger: "ticket_created",
                       conditions: [AutomationCondition(field: "channel", op: "equals", value: "email")],
                       actions: [AutomationAction(type: "merge_into_open_ticket", value: "3", sameSubject: true)],
                       position: 4, runCount: 64),
            Automation(id: id(36), name: "Ask for steps on new bugs", trigger: "ticket_updated",
                       conditions: [AutomationCondition(field: "tag", op: "equals", value: "Bug"), AutomationCondition(field: "assignee_id", op: "equals", value: "none")],
                       actions: [AutomationAction(type: "send_reply", value: "<p>Thanks for the report. Could you share the steps that lead to this and the app version you're on?</p>")],
                       enabled: false, position: 5, runCount: 9),
        ]
        return s
    }

    private static func lumen(_ ws: DemoWorkspace) -> DemoInboxSettings {
        var s = DemoInboxSettings()
        s.hours = HoursConfig(enabled: false, timezone: ws.workspace.settings?.timezone ?? "America/New_York", schedule: week(HoursDay.weekdays, "09:00", "17:00"), holidays: [
            HoursHoliday(date: "2026-11-26", name: "Thanksgiving"),
            HoursHoliday(date: "2026-12-25", name: "Christmas Day"),
        ])
        s.policies = [Policy(id: id(121), name: "Standard", priority: nil, firstResponse: 240, resolution: 2880, businessHours: false, position: 0)]
        s.automations = [
            Automation(id: id(131), name: "Label how-to questions", trigger: "ticket_created",
                       conditions: [AutomationCondition(field: "subject", op: "contains", value: "how do I")],
                       actions: [AutomationAction(type: "add_tag", value: "Question")],
                       position: 0, runCount: 31),
        ]
        return s
    }
}

// MARK: - Routes

extension DemoBackend {
    /// The answer (nil inside for 204), or nil when the route isn't this group's.
    func inboxSettingsRoutes(_ call: Call, in ws: inout DemoWorkspace) throws -> Data?? {
        if !ws.settings.inbox.applied { applyInboxSettings(&ws) }
        if let answer = try labelRoutes(call, in: &ws) { return answer }
        if let answer = try fieldRoutes(call, in: &ws) { return answer }
        if let answer = try assignmentRoutes(call, in: &ws) { return answer }
        if let answer = try policyRoutes(call, in: &ws) { return answer }
        return nil
    }

    private func applyInboxSettings(_ ws: inout DemoWorkspace) {
        ws.settings.inbox.applied = true
        for index in ws.tags.indices {
            if let group = ws.settings.inbox.startingGroups[ws.tags[index].id] { ws.tags[index].groupID = group }
        }
    }

    private func requireAdmin(_ ws: DemoWorkspace) throws {
        guard ws.role.isAdmin else { throw Failure(status: 403, code: "forbidden", detail: "Only admins can change this.") }
    }

    private func invalid(_ detail: String) -> Failure {
        Failure(status: 422, code: "validation_failed", detail: detail)
    }

    private func number(_ value: Int) -> JSONValue { .number(Double(value)) }

    // MARK: Labels

    /// Ticket labels and contact and company labels, as one list.
    private func allLabels(_ ws: DemoWorkspace) -> [Tag] {
        var seen = Set<String>()
        return (ws.tags + ws.directory.tags).filter { seen.insert($0.id).inserted }
    }

    private func scopes(_ tag: Tag) -> [String] {
        let list = tag.appliesTo ?? ["tickets"]
        return list.isEmpty ? ["tickets", "contacts", "companies"] : list
    }

    private func position(_ tag: Tag, in ws: DemoWorkspace) -> Int {
        ws.settings.inbox.positions[tag.id] ?? (allLabels(ws).firstIndex { $0.id == tag.id } ?? 0)
    }

    private func labelJSON(_ tag: Tag, in ws: DemoWorkspace) -> JSONValue {
        let tickets = ws.tickets.filter { !$0.spam && $0.tags.contains { $0.id == tag.id } }.count
        let contacts = ws.directory.contacts.filter { $0.tags.contains { $0.id == tag.id } }.count
        let companies = ws.directory.companies.filter { $0.tags.contains { $0.id == tag.id } }.count
        var object = DemoSeed.json(tag).object ?? [:]
        object["description"] = .string(tag.description ?? "")
        object["group_id"] = tag.groupID.map(JSONValue.string) ?? .null
        object["archived"] = .bool(tag.archived ?? false)
        object["applies_to"] = .array(scopes(tag).map(JSONValue.string))
        object["position"] = number(position(tag, in: ws))
        object["usage"] = .object(["tickets": number(tickets), "contacts": number(contacts), "companies": number(companies)])
        return .object(object)
    }

    /// Changes a label wherever it lives; nil when there is none.
    private func editLabel(_ id: String, in ws: inout DemoWorkspace, _ change: (inout Tag) -> Void) -> Tag? {
        if let index = ws.tags.firstIndex(where: { $0.id == id }) {
            change(&ws.tags[index])
            return ws.tags[index]
        }
        if let index = ws.directory.tags.firstIndex(where: { $0.id == id }) {
            change(&ws.directory.tags[index])
            return ws.directory.tags[index]
        }
        return nil
    }

    /// Rewrites a label on every ticket, contact and company; nil drops it.
    private func rewriteLabel(_ id: String, in ws: inout DemoWorkspace, _ transform: (TagRef) -> TagRef?) {
        func apply(_ refs: [TagRef]) -> [TagRef] {
            var seen = Set<String>()
            return refs.compactMap { $0.id == id ? transform($0) : $0 }.filter { seen.insert($0.id).inserted }
        }
        for index in ws.tickets.indices where ws.tickets[index].tags.contains(where: { $0.id == id }) { ws.tickets[index].tags = apply(ws.tickets[index].tags) }
        for index in ws.directory.contacts.indices where ws.directory.contacts[index].tags.contains(where: { $0.id == id }) {
            ws.directory.contacts[index].tags = apply(ws.directory.contacts[index].tags)
        }
        for index in ws.directory.companies.indices where ws.directory.companies[index].tags.contains(where: { $0.id == id }) {
            ws.directory.companies[index].tags = apply(ws.directory.companies[index].tags)
        }
    }

    private func cleanLabelName(_ raw: String?, except: String?, in ws: DemoWorkspace) throws -> String {
        let name = (raw ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        guard !name.isEmpty else { throw invalid("Give the label a name.") }
        guard name.count <= 50 else { throw invalid("Keep the name under 50 characters.") }
        guard name.allSatisfy({ $0.isLetter || $0.isNumber || " _-./:&+#".contains($0) }) else { throw invalid("Use letters, numbers, spaces and - _ . / : & + #.") }
        if allLabels(ws).contains(where: { $0.id != except && $0.name.caseInsensitiveCompare(name) == .orderedSame }) {
            throw Failure(status: 409, code: "conflict", detail: "A label with this name already exists.")
        }
        return name
    }

    private func groupJSON(_ group: DemoInboxSettings.LabelGroup) -> JSONValue {
        .object([
            "id": .string(group.id), "name": .string(group.name), "color": group.color.map(JSONValue.string) ?? .null,
            "exclusive": .bool(group.exclusive), "position": number(group.position),
        ])
    }

    private func labelRoutes(_ call: Call, in ws: inout DemoWorkspace) throws -> Data?? {
        let query = call.query
        if call.match("GET", "tags") != nil {
            var list = allLabels(ws)
            if let scope = query["applies_to"], !scope.isEmpty { list = list.filter { scopes($0).contains(scope) } }
            switch query["archived"] {
            case "true": list = list.filter { $0.archived == true }
            case "false": list = list.filter { $0.archived != true }
            case nil where query["applies_to"] == "contacts" || query["applies_to"] == "companies":
                // The directory's pickers have always read live labels only.
                list = list.filter { $0.archived != true }
            default: break
            }
            if let q = query["q"]?.trimmingCharacters(in: .whitespaces), !q.isEmpty { list = list.filter { $0.name.localizedCaseInsensitiveContains(q) } }
            if let group = query["group_id"], !group.isEmpty { list = list.filter { $0.groupID == group } }
            list.sort { (position($0, in: ws), $0.name.lowercased()) < (position($1, in: ws), $1.name.lowercased()) }
            return .some(encode(JSONValue.object(["data": .array(list.map { labelJSON($0, in: ws) })])))
        }
        if call.match("POST", "tags/reorder") != nil {
            try requireAdmin(ws)
            let ids = call.body["ids"]?.array?.compactMap(\.string) ?? []
            let slots = ids.map { id in allLabels(ws).first { $0.id == id }.map { position($0, in: ws) } ?? 0 }.sorted()
            for (offset, id) in ids.enumerated() { ws.settings.inbox.positions[id] = slots.indices.contains(offset) ? slots[offset] : offset }
            let list = allLabels(ws).filter { ids.contains($0.id) }.map { labelJSON($0, in: ws) }
            return .some(encode(JSONValue.object(["data": .array(list)])))
        }
        if call.match("POST", "tags") != nil {
            if ws.settings.inbox.labelCreation == "admins" || !ws.role.canWrite { try requireAdmin(ws) }
            let name = try cleanLabelName(call.string("name"), except: nil, in: ws)
            let scopes = call.body["applies_to"]?.array?.compactMap(\.string) ?? ["tickets"]
            let tag = Tag(
                id: made(), name: name, color: call.string("color") ?? LabelPalette.tone(for: name).rawValue,
                description: call.string("description") ?? "", groupID: call.string("group_id"), archived: false, appliesTo: scopes
            )
            ws.settings.inbox.positions[tag.id] = (allLabels(ws).map { position($0, in: ws) }.max() ?? 0) + 1
            if scopes.contains("tickets") { ws.tags.append(tag) } else { ws.directory.tags.append(tag) }
            return .some(encode(labelJSON(tag, in: ws)))
        }
        if let id = call.match("POST", "tags/*/merge")?.first {
            try requireAdmin(ws)
            guard let into = call.string("into"), into != id, let target = allLabels(ws).first(where: { $0.id == into }) else { throw invalid("Choose another label to merge into.") }
            guard allLabels(ws).contains(where: { $0.id == id }) else { throw notFound("label") }
            rewriteLabel(id, in: &ws) { _ in TagRef(id: target.id, name: target.name, color: target.color, groupID: target.groupID) }
            ws.tags.removeAll { $0.id == id }
            ws.directory.tags.removeAll { $0.id == id }
            return .some(encode(labelJSON(target, in: ws)))
        }
        if let id = call.match("PATCH", "tags/*")?.first {
            try requireAdmin(ws)
            let body = call.body
            let name = body["name"] == nil ? nil : try cleanLabelName(body["name"]?.string, except: id, in: ws)
            if let group = body["group_id"]?.string, !ws.settings.inbox.groups.contains(where: { $0.id == group }) { throw invalid("That group isn't here any more.") }
            guard let tag = editLabel(id, in: &ws, { tag in
                if let name { tag.name = name }
                if let color = body["color"]?.string { tag.color = color }
                if let description = body["description"]?.string { tag.description = String(description.prefix(300)) }
                if let group = body["group_id"] { tag.groupID = group.string }
                if let archived = body["archived"]?.bool { tag.archived = archived }
                if let scopes = body["applies_to"]?.array?.compactMap(\.string) { tag.appliesTo = scopes }
            }) else { throw notFound("label") }
            if let position = body["position"]?.int { ws.settings.inbox.positions[id] = position }
            rewriteLabel(id, in: &ws) { _ in TagRef(id: tag.id, name: tag.name, color: tag.color, groupID: tag.groupID) }
            return .some(encode(labelJSON(tag, in: ws)))
        }
        if let id = call.match("DELETE", "tags/*")?.first {
            try requireAdmin(ws)
            guard allLabels(ws).contains(where: { $0.id == id }) else { throw notFound("label") }
            rewriteLabel(id, in: &ws) { _ in nil }
            ws.tags.removeAll { $0.id == id }
            ws.directory.tags.removeAll { $0.id == id }
            return .some(nil)
        }

        // Label groups.
        if call.match("GET", "tag-groups") != nil {
            return .some(encode(JSONValue.object(["data": .array(ws.settings.inbox.groups.sorted { $0.position < $1.position }.map(groupJSON))])))
        }
        if call.match("POST", "tag-groups") != nil {
            try requireAdmin(ws)
            let name = (call.string("name") ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            guard !name.isEmpty else { throw invalid("Give the group a name.") }
            let group = DemoInboxSettings.LabelGroup(
                id: made(), name: String(name.prefix(50)), color: call.body["color"]?.string,
                exclusive: call.body["exclusive"]?.bool ?? false, position: (ws.settings.inbox.groups.map(\.position).max() ?? -1) + 1
            )
            ws.settings.inbox.groups.append(group)
            return .some(encode(groupJSON(group)))
        }
        if let id = call.match("PATCH", "tag-groups/*")?.first {
            try requireAdmin(ws)
            guard let index = ws.settings.inbox.groups.firstIndex(where: { $0.id == id }) else { throw notFound("group") }
            if let name = call.string("name")?.trimmingCharacters(in: .whitespacesAndNewlines), !name.isEmpty { ws.settings.inbox.groups[index].name = String(name.prefix(50)) }
            if let color = call.body["color"] { ws.settings.inbox.groups[index].color = color.string }
            if let exclusive = call.body["exclusive"]?.bool { ws.settings.inbox.groups[index].exclusive = exclusive }
            return .some(encode(groupJSON(ws.settings.inbox.groups[index])))
        }
        if let id = call.match("DELETE", "tag-groups/*")?.first {
            try requireAdmin(ws)
            ws.settings.inbox.groups.removeAll { $0.id == id }
            for index in ws.tags.indices where ws.tags[index].groupID == id { ws.tags[index].groupID = nil }
            for index in ws.directory.tags.indices where ws.directory.tags[index].groupID == id { ws.directory.tags[index].groupID = nil }
            return .some(nil)
        }

        // Who may create labels, and severity: the workspace settings these pages read.
        if call.match("GET", "workspace") != nil {
            return .some(encode(workspaceJSON(ws)))
        }
        if call.match("PATCH", "workspace") != nil, let settings = call.body["settings"]?.object,
           Set(settings.keys).isSubset(of: ["label_creation", "severity_enabled"]), call.body.object?.count == 1 {
            try requireAdmin(ws)
            if let value = settings["label_creation"]?.string {
                guard value == "anyone" || value == "admins" else { throw invalid("label_creation must be anyone or admins.") }
                ws.settings.inbox.labelCreation = value
            }
            if let value = settings["severity_enabled"]?.bool { ws.settings.inbox.severityEnabled = value }
            return .some(encode(workspaceJSON(ws)))
        }
        return nil
    }

    private func workspaceJSON(_ ws: DemoWorkspace) -> JSONValue {
        var object = DemoSeed.json(ws.workspace).object ?? [:]
        var settings = object["settings"]?.object ?? [:]
        settings["label_creation"] = .string(ws.settings.inbox.labelCreation)
        settings["severity_enabled"] = .bool(ws.settings.inbox.severityEnabled)
        object["settings"] = .object(settings)
        return .object(object)
    }

    // MARK: Custom fields

    /// Ticket fields live with the tickets, contact and company fields in the directory.
    private func allFields(_ ws: DemoWorkspace) -> [CustomField] {
        ws.ticketExtras.fields.filter { $0.entity == "ticket" } + ws.directory.fields
    }

    private func editField(_ id: String, in ws: inout DemoWorkspace, _ change: (inout CustomField) -> Void) -> CustomField? {
        if let index = ws.ticketExtras.fields.firstIndex(where: { $0.id == id && $0.entity == "ticket" }) {
            change(&ws.ticketExtras.fields[index])
            return ws.ticketExtras.fields[index]
        }
        if let index = ws.directory.fields.firstIndex(where: { $0.id == id }) {
            change(&ws.directory.fields[index])
            return ws.directory.fields[index]
        }
        return nil
    }

    private func cleanOptions(_ value: JSONValue?) throws -> [FieldOption] {
        let options = value?.array?.compactMap { item -> FieldOption? in
            guard let label = item["label"]?.string?.trimmingCharacters(in: .whitespaces), !label.isEmpty else { return nil }
            let id = item["id"]?.string ?? label.lowercased()
            return FieldOption(id: id, label: String(label.prefix(60)), color: item["color"]?.string ?? "gray")
        } ?? []
        guard Set(options.map(\.id)).count == options.count else { throw invalid("Each option needs its own id.") }
        return options
    }

    private func fieldRoutes(_ call: Call, in ws: inout DemoWorkspace) throws -> Data?? {
        let query = call.query
        if call.match("GET", "custom-fields") != nil {
            var list = allFields(ws)
            if let entity = query["entity"], !entity.isEmpty { list = list.filter { $0.entity == entity } }
            switch query["archived"] {
            case "true": list = list.filter(\.archived)
            case "false": list = list.filter { !$0.archived }
            default: break
            }
            return .some(encode(ListBody(data: list.sorted { ($0.entity, $0.position) < ($1.entity, $1.position) })))
        }
        if call.match("POST", "custom-fields/reorder") != nil {
            try requireAdmin(ws)
            let entity = call.string("entity") ?? "ticket"
            let ids = call.body["ids"]?.array?.compactMap(\.string) ?? []
            for (offset, id) in ids.enumerated() { _ = editField(id, in: &ws) { $0.position = offset + 1 } }
            return .some(encode(ListBody(data: allFields(ws).filter { $0.entity == entity }.sorted { $0.position < $1.position })))
        }
        if call.match("POST", "custom-fields") != nil {
            try requireAdmin(ws)
            let entity = call.string("entity") ?? "ticket"
            guard ["ticket", "contact", "company"].contains(entity) else { throw invalid("entity must be ticket, contact or company.") }
            let key = call.string("key") ?? ""
            guard key.range(of: "^[a-z][a-z0-9_]{0,39}$", options: .regularExpression) != nil else { throw invalid("Start with a letter; use lowercase letters, numbers and _.") }
            guard !allFields(ws).contains(where: { $0.entity == entity && $0.key == key }) else {
                throw Failure(status: 409, code: "conflict", detail: "Another field already uses this key.")
            }
            let name = (call.string("name") ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            guard !name.isEmpty else { throw invalid("Give the field a name.") }
            let type = call.string("type").flatMap(CustomField.Kind.init(rawValue:)) ?? .text
            let options = try cleanOptions(call.body["options"])
            if type.takesOptions && options.isEmpty { throw invalid("Add at least one option.") }
            let field = CustomField(
                id: made(), entity: entity, key: key, name: String(name.prefix(60)), type: type, options: type.takesOptions ? options : [],
                requiredOnSolve: entity == "ticket" && (call.body["required_on_solve"]?.bool ?? false),
                description: call.string("description") ?? "",
                position: (allFields(ws).filter { $0.entity == entity }.map(\.position).max() ?? 0) + 1
            )
            if entity == "ticket" { ws.ticketExtras.fields.append(field) } else { ws.directory.fields.append(field) }
            return .some(encode(field))
        }
        if let id = call.match("PATCH", "custom-fields/*")?.first {
            try requireAdmin(ws)
            let body = call.body
            let options = body["options"] == nil ? nil : try cleanOptions(body["options"])
            if let name = body["name"]?.string, name.trimmingCharacters(in: .whitespaces).isEmpty { throw invalid("Give the field a name.") }
            guard let field = editField(id, in: &ws, { field in
                if let name = body["name"]?.string { field.name = String(name.trimmingCharacters(in: .whitespacesAndNewlines).prefix(60)) }
                if let options, field.type.takesOptions { field.options = options }
                if let required = body["required_on_solve"]?.bool, field.entity == "ticket" { field.requiredOnSolve = required }
                if let description = body["description"]?.string { field.description = description }
                if let archived = body["archived"]?.bool { field.archived = archived }
            }) else { throw notFound("field") }
            return .some(encode(field))
        }
        if let id = call.match("DELETE", "custom-fields/*")?.first {
            try requireAdmin(ws)
            guard let field = allFields(ws).first(where: { $0.id == id }) else { throw notFound("field") }
            if field.entity == "ticket" {
                ws.ticketExtras.fields.removeAll { $0.id == id && $0.entity == "ticket" }
                for index in ws.tickets.indices { ws.tickets[index].fields[field.key] = nil }
            } else {
                ws.directory.fields.removeAll { $0.id == id }
            }
            return .some(nil)
        }
        return nil
    }

    // MARK: Assignment, skills and members

    private func assignmentJSON(_ ws: DemoWorkspace) -> JSONValue {
        let s = ws.settings.inbox
        return .object([
            "mode": .string(s.mode),
            "rules": .array(s.rules.map { .object(["target_type": .string($0.targetType), "target_id": .string($0.targetID), "mode": .string($0.mode)]) }),
            "auto_away_minutes": number(s.autoAwayMinutes),
            "new_ticket_notify": .string(s.newTicketNotify),
            "followup_keeps_assignee": .bool(s.followUpKeepsAssignee),
        ])
    }

    private func skillJSON(_ skill: DemoInboxSettings.Skill) -> JSONValue {
        .object(["id": .string(skill.id), "name": .string(skill.name), "archived": .bool(skill.archived), "member_ids": .array(skill.memberIDs.map(JSONValue.string))])
    }

    private func triageJSON(_ ws: DemoWorkspace) -> JSONValue {
        let s = ws.settings.inbox
        return .object([
            "auto_triage": .bool(s.autoTriage),
            "fields": .array(s.triageFields.map(JSONValue.string)),
            "assign_after_triage": .bool(s.assignAfterTriage),
            "threshold": .number(s.threshold),
            "engine": .object(s.engine),
            "available": .bool(true),
        ])
    }

    /// A member with their cap, in the shape the members list answers.
    private func memberJSON(_ member: Member, in ws: DemoWorkspace) -> JSONValue {
        var object = DemoSeed.json(member).object ?? [:]
        object["max_open_tickets"] = ws.settings.inbox.caps[member.id].map(number) ?? .null
        return .object(object)
    }

    private func assignmentRoutes(_ call: Call, in ws: inout DemoWorkspace) throws -> Data?? {
        let modes = ["none", "round_robin", "load_balanced"]
        if call.match("GET", "assignment") != nil { return .some(encode(assignmentJSON(ws))) }
        if call.match("PATCH", "assignment") != nil {
            try requireAdmin(ws)
            let body = call.body
            if let mode = body["mode"]?.string {
                guard modes.contains(mode) else { throw invalid("mode must be none, round_robin or load_balanced.") }
                ws.settings.inbox.mode = mode
            }
            if let minutes = body["auto_away_minutes"]?.int {
                guard minutes == 0 || (10...1440).contains(minutes) else { throw invalid("auto_away_minutes must be 0 (off) or between 10 and 1440.") }
                ws.settings.inbox.autoAwayMinutes = minutes
            }
            if let notify = body["new_ticket_notify"]?.string {
                guard ["auto", "everyone", "leads", "off"].contains(notify) else { throw invalid("new_ticket_notify must be auto, everyone, leads or off.") }
                ws.settings.inbox.newTicketNotify = notify
            }
            if let keep = body["followup_keeps_assignee"] {
                guard let on = keep.bool else { throw invalid("followup_keeps_assignee must be true or false.") }
                ws.settings.inbox.followUpKeepsAssignee = on
            }
            if let rules = body["rules"]?.array {
                ws.settings.inbox.rules = try rules.map { rule in
                    guard let type = rule["target_type"]?.string, type == "team" || type == "mailbox", let target = rule["target_id"]?.string,
                          let mode = rule["mode"]?.string, modes.contains(mode) else { throw invalid("Each rule needs a team or mailbox and a mode.") }
                    return DemoInboxSettings.Rule(targetType: type, targetID: target, mode: mode)
                }
            }
            return .some(encode(assignmentJSON(ws)))
        }

        // Members, with the cap automatic assignment respects.
        if call.match("GET", "members") != nil {
            guard case let .some(.some(data)) = try accountRoutes(call, in: &ws), let parsed = try? JSONValue.parse(data) else { return nil }
            let rows = (parsed["data"]?.array ?? []).map { row -> JSONValue in
                guard var object = row.object, let id = object["user"]?["id"]?.string else { return row }
                object["max_open_tickets"] = ws.settings.inbox.caps[id].map(number) ?? .null
                return .object(object)
            }
            return .some(encode(JSONValue.object(["data": .array(rows)])))
        }
        if let key = call.match("PATCH", "members/*/availability")?.first {
            let id = key == "me" ? world.user.id : key
            if id != world.user.id || call.body["max_open_tickets"] != nil { try requireAdmin(ws) }
            guard let index = ws.members.firstIndex(where: { $0.id == id }) else { throw notFound("member") }
            if let value = call.string("availability") {
                guard let availability = Availability(rawValue: value) else { throw invalid("availability must be online, away or offline.") }
                ws.members[index].availability = availability
                ws.members[index].presence = availability
            }
            if let cap = call.body["max_open_tickets"] {
                if let n = cap.int {
                    guard (1...500).contains(n) else { throw invalid("max_open_tickets must be between 1 and 500.") }
                    ws.settings.inbox.caps[id] = n
                } else {
                    ws.settings.inbox.caps[id] = nil
                }
            }
            publish("member.updated", ["member": DemoSeed.json(ws.members[index])], in: &ws)
            return .some(encode(memberJSON(ws.members[index], in: ws)))
        }

        // Routing skills.
        if call.match("GET", "skills") != nil {
            try requireAdmin(ws)
            return .some(encode(JSONValue.object(["data": .array(ws.settings.inbox.skills.map(skillJSON))])))
        }
        if call.match("POST", "skills") != nil {
            try requireAdmin(ws)
            let name = (call.string("name") ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            guard !name.isEmpty else { throw invalid("Give the skill a name.") }
            guard !ws.settings.inbox.skills.contains(where: { $0.name.caseInsensitiveCompare(name) == .orderedSame }) else {
                throw Failure(status: 409, code: "conflict", detail: "A skill with this name already exists.")
            }
            let skill = DemoInboxSettings.Skill(id: made(), name: String(name.prefix(80)), memberIDs: [])
            ws.settings.inbox.skills.append(skill)
            return .some(encode(skillJSON(skill)))
        }
        if let id = call.match("PATCH", "skills/*")?.first {
            try requireAdmin(ws)
            guard let index = ws.settings.inbox.skills.firstIndex(where: { $0.id == id }) else { throw notFound("skill") }
            if let archived = call.body["archived"]?.bool { ws.settings.inbox.skills[index].archived = archived }
            if let name = call.string("name")?.trimmingCharacters(in: .whitespacesAndNewlines), !name.isEmpty { ws.settings.inbox.skills[index].name = name }
            return .some(encode(skillJSON(ws.settings.inbox.skills[index])))
        }
        if let id = call.match("PUT", "skills/*/members")?.first {
            try requireAdmin(ws)
            guard let index = ws.settings.inbox.skills.firstIndex(where: { $0.id == id }) else { throw notFound("skill") }
            let ids = call.body["member_ids"]?.array?.compactMap(\.string) ?? []
            ws.settings.inbox.skills[index].memberIDs = ids.filter { id in ws.members.contains { $0.id == id } }
            return .some(encode(skillJSON(ws.settings.inbox.skills[index])))
        }

        // Triage.
        if call.match("GET", "triage") != nil { return .some(encode(triageJSON(ws))) }
        if call.match("PATCH", "triage") != nil {
            try requireAdmin(ws)
            let body = call.body
            if let value = body["auto_triage"]?.bool { ws.settings.inbox.autoTriage = value }
            if let value = body["assign_after_triage"]?.bool { ws.settings.inbox.assignAfterTriage = value }
            if let fields = body["fields"]?.array?.compactMap(\.string) {
                guard fields.allSatisfy({ ["priority", "severity", "labels", "team"].contains($0) }) else { throw invalid("fields takes priority, severity, labels and team.") }
                ws.settings.inbox.triageFields = fields
            }
            if let value = body["threshold"]?.double {
                guard (0.5...0.99).contains(value) else { throw invalid("threshold must be between 0.5 and 0.99.") }
                ws.settings.inbox.threshold = (value * 100).rounded() / 100
            }
            return .some(encode(triageJSON(ws)))
        }
        return nil
    }

    // MARK: Business hours, SLA policies and automations

    private func policyJSON(_ policy: DemoInboxSettings.Policy) -> JSONValue {
        .object([
            "id": .string(policy.id), "name": .string(policy.name), "priority": policy.priority.map(JSONValue.string) ?? .null,
            "first_response_minutes": number(policy.firstResponse), "resolution_minutes": number(policy.resolution),
            "business_hours": .bool(policy.businessHours), "enabled": .bool(policy.enabled), "position": number(policy.position),
            "team_id": policy.teamID.map(JSONValue.string) ?? .null,
        ])
    }

    private func automationJSON(_ rule: DemoInboxSettings.Automation) -> JSONValue {
        .object([
            "id": .string(rule.id), "name": .string(rule.name), "trigger": .string(rule.trigger),
            "conditions": DemoSeed.json(rule.conditions), "actions": DemoSeed.json(rule.actions),
            "enabled": .bool(rule.enabled), "position": number(rule.position), "run_count": number(rule.runCount),
        ])
    }

    private func checkHours(_ hours: HoursConfig) throws {
        guard TimeZone(identifier: hours.timezone) != nil else { throw invalid("timezone must be an IANA time zone.") }
        guard hours.holidays.count <= 100 else { throw invalid("Up to 100 holidays.") }
        guard Set(hours.holidays.map(\.date)).count == hours.holidays.count else { throw invalid("There is already a holiday on that date.") }
        for day in HoursDay.allCases {
            let ranges = hours.ranges(day)
            guard ranges.count <= 3 else { throw invalid("Up to three ranges a day.") }
            let quarter = ranges.allSatisfy { $0.startMinutes % 15 == 0 && $0.endMinutes % 15 == 0 && $0.endMinutes <= 1440 }
            guard quarter else { throw invalid("Times go on the quarter hour.") }
            if let problem = HoursConfig.problem(ranges) { throw invalid(problem) }
        }
    }

    private func checkMinutes(_ value: Int?) throws -> Int? {
        guard let value else { return nil }
        guard (1...525_600).contains(value) else { throw invalid("Enter each time as a number, from 1 minute to 365 days.") }
        return value
    }

    private func policyRoutes(_ call: Call, in ws: inout DemoWorkspace) throws -> Data?? {
        // Business hours.
        if call.match("GET", "business-hours") != nil { return .some(encode(ws.settings.inbox.hours)) }
        if call.match("PUT", "business-hours") != nil {
            try requireAdmin(ws)
            var hours = try call.body.decode(HoursConfig.self)
            hours.normalize()
            try checkHours(hours)
            ws.settings.inbox.hours = hours
            ws.workspace.settings?.timezone = hours.timezone
            return .some(encode(hours))
        }

        // SLA policies.
        if call.match("GET", "sla-policies") != nil {
            return .some(encode(JSONValue.object(["data": .array(ws.settings.inbox.policies.sorted { $0.position < $1.position }.map(policyJSON))])))
        }
        if call.match("POST", "sla-policies") != nil {
            try requireAdmin(ws)
            let body = call.body
            let name = (body["name"]?.string ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            guard !name.isEmpty else { throw invalid("Give the policy a name.") }
            let policy = DemoInboxSettings.Policy(
                id: made(), name: String(name.prefix(80)), priority: body["priority"]?.string,
                firstResponse: try checkMinutes(body["first_response_minutes"]?.int) ?? 60,
                resolution: try checkMinutes(body["resolution_minutes"]?.int) ?? 1440,
                businessHours: body["business_hours"]?.bool ?? false, enabled: body["enabled"]?.bool ?? true,
                position: body["position"]?.int ?? ws.settings.inbox.policies.count, teamID: body["team_id"]?.string
            )
            ws.settings.inbox.policies.append(policy)
            return .some(encode(policyJSON(policy)))
        }
        if let id = call.match("PATCH", "sla-policies/*")?.first {
            try requireAdmin(ws)
            guard let index = ws.settings.inbox.policies.firstIndex(where: { $0.id == id }) else { throw notFound("policy") }
            let body = call.body
            var policy = ws.settings.inbox.policies[index]
            if let name = body["name"]?.string {
                let trimmed = name.trimmingCharacters(in: .whitespacesAndNewlines)
                guard !trimmed.isEmpty else { throw invalid("Give the policy a name.") }
                policy.name = String(trimmed.prefix(80))
            }
            if let value = body["priority"] { policy.priority = value.string }
            if let value = body["team_id"] { policy.teamID = value.string }
            if let value = try checkMinutes(body["first_response_minutes"]?.int) { policy.firstResponse = value }
            if let value = try checkMinutes(body["resolution_minutes"]?.int) { policy.resolution = value }
            if let value = body["business_hours"]?.bool { policy.businessHours = value }
            if let value = body["enabled"]?.bool { policy.enabled = value }
            if let value = body["position"]?.int { policy.position = value }
            ws.settings.inbox.policies[index] = policy
            return .some(encode(policyJSON(policy)))
        }
        if let id = call.match("DELETE", "sla-policies/*")?.first {
            try requireAdmin(ws)
            ws.settings.inbox.policies.removeAll { $0.id == id }
            return .some(nil)
        }

        // Automations.
        if call.match("GET", "automations") != nil {
            return .some(encode(JSONValue.object(["data": .array(ws.settings.inbox.automations.sorted { $0.position < $1.position }.map(automationJSON))])))
        }
        if call.match("POST", "automations") != nil {
            try requireAdmin(ws)
            var rule = DemoInboxSettings.Automation(id: made(), name: "", trigger: "ticket_created", conditions: [], actions: [], position: ws.settings.inbox.automations.count, runCount: 0)
            try applyAutomation(call.body, to: &rule)
            guard !rule.name.isEmpty else { throw invalid("Name the automation.") }
            guard !rule.actions.isEmpty else { throw invalid("Add at least one action.") }
            ws.settings.inbox.automations.append(rule)
            return .some(encode(automationJSON(rule)))
        }
        if let id = call.match("PATCH", "automations/*")?.first {
            try requireAdmin(ws)
            guard let index = ws.settings.inbox.automations.firstIndex(where: { $0.id == id }) else { throw notFound("automation") }
            var rule = ws.settings.inbox.automations[index]
            try applyAutomation(call.body, to: &rule)
            guard !rule.name.isEmpty else { throw invalid("Name the automation.") }
            guard !rule.actions.isEmpty else { throw invalid("Add at least one action.") }
            ws.settings.inbox.automations[index] = rule
            return .some(encode(automationJSON(rule)))
        }
        if let id = call.match("DELETE", "automations/*")?.first {
            try requireAdmin(ws)
            ws.settings.inbox.automations.removeAll { $0.id == id }
            return .some(nil)
        }
        return nil
    }

    private func applyAutomation(_ body: JSONValue, to rule: inout DemoInboxSettings.Automation) throws {
        if let name = body["name"]?.string { rule.name = String(name.trimmingCharacters(in: .whitespacesAndNewlines).prefix(80)) }
        if let trigger = body["trigger"]?.string {
            guard AutomationTrigger(rawValue: trigger) != nil else { throw invalid("Choose when the automation runs.") }
            rule.trigger = trigger
        }
        if let value = body["conditions"] { rule.conditions = (try? value.decode([AutomationCondition].self)) ?? rule.conditions }
        if let value = body["actions"] { rule.actions = (try? value.decode([AutomationAction].self)) ?? rule.actions }
        if let enabled = body["enabled"]?.bool { rule.enabled = enabled }
        if let position = body["position"]?.int { rule.position = position }
    }
}
#endif
