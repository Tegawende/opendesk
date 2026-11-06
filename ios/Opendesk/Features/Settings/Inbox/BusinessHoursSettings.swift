import SwiftUI

/// Settings › Business hours, natively: when the team is open, in which time zone, and the
/// holidays it is closed. Edits are a draft saved with PUT, as on the web.
struct BusinessHoursSettings: View {
    @Environment(AppEnvironment.self) private var env
    @State private var store = BusinessHoursStore()
    @State private var draft: HoursConfig?
    @State private var pickingZone = false
    @State private var addingHoliday = false
    @State private var saving = false

    private var canEdit: Bool { env.session.isAdmin }
    private var dirty: Bool { draft != nil && draft != store.saved }
    private var invalid: Bool { draft.map { config in HoursDay.allCases.contains { HoursConfig.problem(config.ranges($0)) != nil } } ?? false }

    var body: some View {
        InboxLoadGate(load: store.load, retry: { await store.reload() }) {
            if let draft { page(draft) }
        }
        .accountTitle(SettingsPage.businessHours.title)
        .settingsSaveBar(dirty: dirty && canEdit, problem: invalid ? "Fix the hours marked in red to save." : nil, saving: saving, canSave: !invalid, save: save) {
            draft = store.saved
        }
        .sheet(isPresented: $pickingZone) {
            InboxOptionSheet(
                title: "Time zone",
                options: TimeZone.knownTimeZoneIdentifiers.map { InboxOption(value: $0, label: $0.replacingOccurrences(of: "_", with: " "), hint: BusinessHoursSettings.offset($0)) },
                selected: Set([draft?.timezone ?? ""]),
                prompt: "Search time zones"
            ) { picked in
                if let zone = picked.first { draft?.timezone = zone }
            }
            .presentationDetents([.large])
        }
        .sheet(isPresented: $addingHoliday) {
            HolidaySheet(existing: draft?.holidays ?? []) { holiday in
                draft?.holidays.append(holiday)
                draft?.holidays.sort { $0.date < $1.date }
            }
            .presentationDetents([.height(560), .large])
            .presentationCornerRadius(Theme.Radius.sheet)
        }
        .task {
            await store.start(api: env.session.api)
            if draft == nil { draft = store.saved }
        }
        .onChange(of: store.saved) { old, new in
            // A reload replaces the draft only when nothing was changed.
            if draft == nil || draft == old { draft = new }
        }
    }

    private func page(_ config: HoursConfig) -> some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 24) {
                intro(config)
                weekSection(config)
                holidaysSection(config)
            }
            .padding(.horizontal, 16)
            .padding(.top, 8)
            .padding(.bottom, 120)
        }
        .background(Theme.chrome.ignoresSafeArea())
        .refreshable { await store.reload() }
    }

    private var binding: Binding<HoursConfig> {
        Binding(get: { draft ?? store.saved ?? HoursConfig() }, set: { draft = $0 })
    }

    // MARK: Intro

    private func intro(_ config: HoursConfig) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("When your team is open. SLA policies can count only these hours, so nights, weekends and holidays don't run the clock.")
                .font(ODFont.footnote)
                .foregroundStyle(Theme.fg3)
                .fixedSize(horizontal: false, vertical: true)
            if !canEdit { SettingsReadOnlyNote() }
            if config.enabled, let open = store.saved?.isOpen(at: .now) {
                TonePill(text: open ? "Open now" : "Closed now", tone: open ? .green : .gray) {
                    Circle().fill(open ? Tone.green.solid : Theme.fg4).frame(width: 6, height: 6)
                }
            }
            AccountSection {
                AccountToggleRow(
                    symbol: "clock",
                    tone: config.enabled ? .green : .gray,
                    title: "Use business hours",
                    subtitle: config.enabled ? "SLA policies that count business hours pause outside these times" : "Every minute counts toward SLA targets",
                    isOn: binding.enabled,
                    disabled: !canEdit
                )
                AccountDivider()
                Button { pickingZone = true } label: {
                    AccountRow(symbol: "globe", title: "Time zone", subtitle: "Opening hours and holidays follow this time zone. It is also the workspace time zone.") {
                        AccountDisclosure(value: config.timezone.replacingOccurrences(of: "_", with: " "))
                    }
                }
                .buttonStyle(RowPressStyle())
                .disabled(!canEdit)
                AccountDivider()
                Button { env.router.accountPath.append(.setting(.sla)) } label: {
                    AccountRow(symbol: "gauge.with.dots.needle.67percent", tone: .blue, title: "SLA policies", subtitle: "Turn on business hours per policy.") {
                        AccountDisclosure(value: nil)
                    }
                }
                .buttonStyle(RowPressStyle())
            }
        }
    }

    // MARK: Week

    private func weekSection(_ config: HoursConfig) -> some View {
        let hours = Double(config.weeklyMinutes) / 60
        let total = hours.truncatingRemainder(dividingBy: 1) == 0 ? "\(Int(hours))" : String(format: "%.1f", hours)
        return VStack(alignment: .leading, spacing: 8) {
            HStack {
                SectionLabel("Weekly hours")
                Spacer()
                Text("\(total) h a week").font(ODFont.mono(12)).foregroundStyle(Theme.fg3).contentTransition(.numericText())
            }
            .padding(.horizontal, 4)
            Panel {
                ForEach(Array(HoursDay.allCases.enumerated()), id: \.element) { index, day in
                    if index > 0 { Hairline() }
                    dayRow(day, config)
                }
            }
            Text("Up to three ranges a day, on the quarter hour.")
                .font(ODFont.footnote)
                .foregroundStyle(Theme.fg3)
                .padding(.horizontal, 4)
        }
        .opacity(config.enabled ? 1 : 0.6)
        .motion(Motion.snappy, value: config.schedule)
    }

    private func dayRow(_ day: HoursDay, _ config: HoursConfig) -> some View {
        let ranges = config.ranges(day)
        let open = !ranges.isEmpty
        let problem = HoursConfig.problem(ranges)
        return VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 10) {
                Toggle("Open on \(day.title)", isOn: Binding(get: { open }, set: { on in
                    draft?.schedule[day.rawValue] = on ? [HoursRange(start: "09:00", end: "17:00")] : []
                }))
                .labelsHidden()
                .tint(Tone.green.solid)
                .disabled(!canEdit)
                Text(day.title).font(ODFont.sans(15, weight: open ? .medium : .regular)).foregroundStyle(open ? Theme.fg : Theme.fg3)
                Spacer()
                if !open { Text("Closed").font(ODFont.footnote).foregroundStyle(Theme.fg4) }
                if canEdit {
                    Menu {
                        Button { addRange(day) } label: { Label("Add hours", systemImage: "plus") }
                            .disabled(ranges.count >= 3)
                        Button { copy(day, to: HoursDay.weekdays) } label: { Label("Copy to all weekdays", systemImage: "doc.on.doc") }
                        Button { copy(day, to: HoursDay.allCases) } label: { Label("Copy to every day", systemImage: "calendar") }
                    } label: {
                        Image(systemName: "ellipsis")
                            .font(.system(size: 14, weight: .medium))
                            .foregroundStyle(Theme.fg3)
                            .frame(width: 32, height: 32)
                            .contentShape(Rectangle())
                    }
                    .accessibilityLabel("More for \(day.title)")
                }
            }
            if open {
                ForEach(Array(ranges.enumerated()), id: \.offset) { index, range in
                    HStack(spacing: 8) {
                        HoursTimeMenu(label: "\(day.title) range \(index + 1) start", value: range.start, ends: false, onPick: setRange(day, index) { $0.start = $1 })
                            .disabled(!canEdit)
                        Text("to").font(ODFont.footnote).foregroundStyle(Theme.fg4)
                        HoursTimeMenu(label: "\(day.title) range \(index + 1) end", value: range.end, ends: true, onPick: setRange(day, index) { $0.end = $1 })
                            .disabled(!canEdit)
                        Spacer(minLength: 0)
                        if canEdit && ranges.count > 1 {
                            Button { draft?.schedule[day.rawValue]?.remove(at: index) } label: {
                                Image(systemName: "xmark").font(.system(size: 11, weight: .semibold)).foregroundStyle(Theme.fg3).frame(width: 30, height: 30)
                            }
                            .accessibilityLabel("Remove \(day.title) range \(index + 1)")
                        }
                    }
                    .padding(.leading, 60)
                }
                if canEdit && ranges.count < 3 {
                    Button { addRange(day) } label: {
                        Label("Add hours", systemImage: "plus").font(ODFont.sans(12.5, weight: .medium)).foregroundStyle(Theme.fg3)
                    }
                    .buttonStyle(.plain)
                    .padding(.leading, 60)
                    .accessibilityLabel("Add hours on \(day.title)")
                }
                if let problem { FormError(text: problem).padding(.leading, 60) }
            }
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
    }

    private func setRange(_ day: HoursDay, _ index: Int, _ apply: @escaping (inout HoursRange, String) -> Void) -> (String) -> Void {
        { value in
            guard var ranges = draft?.schedule[day.rawValue], ranges.indices.contains(index) else { return }
            apply(&ranges[index], value)
            draft?.schedule[day.rawValue] = ranges
        }
    }

    private func addRange(_ day: HoursDay) {
        guard var ranges = draft?.schedule[day.rawValue], ranges.count < 3 else { return }
        let lastEnd = ranges.map(\.endMinutes).max() ?? 8 * 60
        let start = min(lastEnd + 60, 23 * 60)
        let end = min(start + 120, 24 * 60)
        ranges.append(HoursRange(start: HoursRange.hhmm(start), end: HoursRange.hhmm(end)))
        draft?.schedule[day.rawValue] = ranges
    }

    private func copy(_ day: HoursDay, to days: [HoursDay]) {
        let ranges = draft?.schedule[day.rawValue] ?? []
        for other in days { draft?.schedule[other.rawValue] = ranges }
        Haptics.success()
    }

    // MARK: Holidays

    private func holidaysSection(_ config: HoursConfig) -> some View {
        let today = Self.isoToday(config.timezone)
        let past = config.holidays.filter { $0.date < today }
        return VStack(alignment: .leading, spacing: 8) {
            HStack {
                SectionLabel("Holidays")
                Spacer()
                if canEdit && !config.holidays.isEmpty {
                    Button { addingHoliday = true } label: { Label("Add holiday", systemImage: "plus").font(ODFont.sans(13, weight: .medium)) }
                        .foregroundStyle(Theme.fg2)
                        .disabled(config.holidays.count >= 100)
                }
            }
            .padding(.horizontal, 4)
            Panel {
                if config.holidays.isEmpty {
                    EmptyState(
                        symbol: "calendar.badge.minus",
                        title: "No holidays yet",
                        message: "Add the days your team is closed, like public holidays, so SLA clocks don't run while nobody is around.",
                        actionTitle: canEdit ? "Add holiday" : nil,
                        action: canEdit ? { addingHoliday = true } : nil
                    )
                    .padding(.vertical, 12)
                } else {
                    ForEach(Array(config.holidays.enumerated()), id: \.element.date) { index, holiday in
                        if index > 0 { AccountDivider() }
                        holidayRow(holiday, today: today)
                    }
                }
            }
            if !past.isEmpty && canEdit {
                HStack(spacing: 6) {
                    Text(past.count == 1 ? "1 past holiday." : "\(past.count) past holidays.").font(ODFont.footnote).foregroundStyle(Theme.fg3)
                    Button("Clear past holidays") { draft?.holidays.removeAll { $0.date < today } }
                        .font(ODFont.sans(12.5, weight: .medium))
                        .foregroundStyle(Theme.fg2)
                }
                .padding(.horizontal, 4)
            }
            Text("Closed all day. SLA clocks that count business hours stay paused.")
                .font(ODFont.footnote)
                .foregroundStyle(Theme.fg3)
                .padding(.horizontal, 4)
        }
        .motion(Motion.snappy, value: config.holidays)
    }

    private func holidayRow(_ holiday: HoursHoliday, today: String) -> some View {
        let days = Self.days(from: today, to: holiday.date)
        let isPast = holiday.date < today
        return HStack(spacing: 12) {
            VStack(spacing: 0) {
                Text(Self.month(holiday.date)).font(ODFont.sans(9.5, weight: .semibold)).foregroundStyle(isPast ? Theme.fg4 : Tone.red.text)
                Text(Self.dayOfMonth(holiday.date)).font(ODFont.mono(15, weight: .medium)).foregroundStyle(isPast ? Theme.fg3 : Theme.fg)
            }
            .frame(width: 38, height: 38)
            .background(Theme.sunken, in: RoundedRectangle(cornerRadius: 8, style: .continuous))
            VStack(alignment: .leading, spacing: 2) {
                Text(holiday.name).font(ODFont.sans(15)).foregroundStyle(isPast ? Theme.fg3 : Theme.fg).lineLimit(1)
                Text("\(Self.long(holiday.date)) \u{00B7} \(days == 0 ? "today" : isPast ? "past" : (days == 1 ? "in 1 day" : "in \(days) days"))")
                    .font(ODFont.footnote)
                    .foregroundStyle(Theme.fg3)
            }
            Spacer(minLength: 6)
            if canEdit {
                Button { draft?.holidays.removeAll { $0.date == holiday.date } } label: {
                    Image(systemName: "xmark").font(.system(size: 11, weight: .semibold)).foregroundStyle(Theme.fg3).frame(width: 32, height: 32)
                }
                .accessibilityLabel("Remove \(holiday.name)")
            }
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 9)
        .opacity(isPast ? 0.7 : 1)
    }

    // MARK: Save

    private func save() {
        guard let draft else { return }
        saving = true
        Task {
            do {
                try await store.save(draft)
                self.draft = store.saved
                Haptics.success()
                env.toasts.show("Business hours saved", kind: .success)
            } catch {
                env.toasts.error(error)
            }
            saving = false
        }
    }

    // MARK: Dates

    static func offset(_ identifier: String) -> String? {
        guard let zone = TimeZone(identifier: identifier) else { return nil }
        let seconds = zone.secondsFromGMT()
        let sign = seconds < 0 ? "-" : "+"
        let minutes = abs(seconds) / 60
        return minutes % 60 == 0 ? "UTC\(sign)\(minutes / 60)" : String(format: "UTC%@%d:%02d", sign, minutes / 60, minutes % 60)
    }

    static func isoToday(_ timezone: String) -> String {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: timezone) ?? .current
        let parts = calendar.dateComponents([.year, .month, .day], from: .now)
        return String(format: "%04d-%02d-%02d", parts.year ?? 0, parts.month ?? 0, parts.day ?? 0)
    }

    private static func date(_ iso: String) -> Date? {
        let parts = iso.split(separator: "-").compactMap { Int($0) }
        guard parts.count == 3 else { return nil }
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC") ?? .current
        return calendar.date(from: DateComponents(year: parts[0], month: parts[1], day: parts[2]))
    }

    static func days(from: String, to: String) -> Int {
        guard let a = date(from), let b = date(to) else { return 0 }
        return Int((b.timeIntervalSince(a) / 86_400).rounded())
    }

    private static func formatted(_ iso: String, _ style: Date.FormatStyle) -> String {
        guard let value = date(iso) else { return iso }
        var style = style
        style.timeZone = TimeZone(identifier: "UTC") ?? .current
        return value.formatted(style)
    }

    static func month(_ iso: String) -> String { formatted(iso, .dateTime.month(.abbreviated)).uppercased() }
    static func dayOfMonth(_ iso: String) -> String { formatted(iso, .dateTime.day()) }
    static func long(_ iso: String) -> String { formatted(iso, .dateTime.weekday(.abbreviated).month(.abbreviated).day().year()) }

    /// Business hours have no detail pages: a link to one opens the page.
    struct Detail: View {
        let id: String

        var body: some View {
            BusinessHoursSettings()
        }
    }
}

/// A time on the quarter hour, chosen from a menu; an end can be 24:00.
private struct HoursTimeMenu: View {
    let label: String
    let value: String
    let ends: Bool
    let onPick: (String) -> Void

    private var choices: [String] {
        let range = ends ? stride(from: 15, through: 24 * 60, by: 15) : stride(from: 0, through: 24 * 60 - 15, by: 15)
        return range.map(HoursRange.hhmm)
    }

    var body: some View {
        Menu {
            Picker(label, selection: Binding(get: { value }, set: { onPick($0) })) {
                ForEach(choices, id: \.self) { choice in Text(InboxText.time(choice)).tag(choice) }
            }
        } label: {
            Text(InboxText.time(value))
                .font(ODFont.mono(13.5))
                .foregroundStyle(Theme.fg)
                .monospacedDigit()
                .padding(.horizontal, 10)
                .frame(minWidth: 84, minHeight: 32)
                .background(Theme.sunken, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
                .overlay(RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous).strokeBorder(Theme.line, lineWidth: 1))
        }
        .accessibilityLabel(label)
        .accessibilityValue(InboxText.time(value))
    }
}

/// Add a holiday: a date and a name.
private struct HolidaySheet: View {
    let existing: [HoursHoliday]
    let add: (HoursHoliday) -> Void

    @Environment(\.dismiss) private var dismiss
    @State private var date = Date.now
    @State private var name = ""
    @State private var error: String?

    private var iso: String {
        let parts = Calendar.current.dateComponents([.year, .month, .day], from: date)
        return String(format: "%04d-%02d-%02d", parts.year ?? 0, parts.month ?? 0, parts.day ?? 0)
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                AccountSheetHeader(title: "Add holiday", subtitle: "Closed all day. SLA clocks that count business hours stay paused.") { dismiss() }
                DatePicker("Holiday date", selection: $date, displayedComponents: .date)
                    .datePickerStyle(.graphical)
                    .tint(Theme.fg)
                ODTextField(label: "Holiday name", text: $name, prompt: "New Year's Day", submitLabel: .done)
                FormError(text: error)
                Button(action: confirm) { ButtonLabel(title: "Add", busy: false) }
                    .buttonStyle(InkButtonStyle())
            }
            .padding(22)
            .motion(Motion.snappy, value: error)
        }
        .scrollDismissesKeyboard(.interactively)
        .background(Theme.surface)
        .onChange(of: name) { _, _ in error = nil }
        .onChange(of: date) { _, _ in error = nil }
    }

    private func confirm() {
        let trimmed = name.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { error = "Give the holiday a name."; return }
        guard !existing.contains(where: { $0.date == iso }) else { error = "There is already a holiday on that date."; return }
        guard existing.count < 100 else { error = "You can add up to 100 holidays."; return }
        add(HoursHoliday(date: iso, name: String(trimmed.prefix(80))))
        Haptics.success()
        dismiss()
    }
}

// MARK: - Store

@MainActor
@Observable
final class BusinessHoursStore {
    private(set) var saved: HoursConfig?
    private(set) var load: InboxLoad = .loading

    @ObservationIgnored private var api: APIClient?

    func start(api: APIClient) async {
        self.api = api
        await reload()
    }

    func reload() async {
        guard let api else { return }
        do {
            saved = try await api.get("v1/business-hours")
            load = .ready
        } catch {
            if error.isCancellation { return }
            if load != .ready { load = .failed(error.userMessage) }
        }
    }

    func save(_ config: HoursConfig) async throws {
        guard let api else { return }
        var clean = config
        for day in HoursDay.allCases {
            clean.schedule[day.rawValue] = clean.ranges(day).sorted { $0.startMinutes < $1.startMinutes }
        }
        let answer: HoursConfig = try await api.send(APIRequest(method: "PUT", path: "v1/business-hours", body: try APICoding.encoder.encode(clean)))
        saved = answer
    }
}
