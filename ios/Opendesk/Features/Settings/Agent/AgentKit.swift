import SwiftUI

// Pieces the AI agent pages share: the "agent is off" notice, counted fields, a topic list,
// a stepper without spinners, a dollar field, the model picker, summary tiles and a save bar.

enum AgentText {
    /// Why a plain-text setting is refused (the server's rule), or nil.
    static func problem(_ text: String, max: Int) -> String? {
        if text.count > max { return "Up to \(max) characters" }
        if hasMarkup(text) { return "Plain text only, without < or >" }
        return nil
    }

    static func hasMarkup(_ text: String) -> Bool {
        text.unicodeScalars.contains { $0 == "<" || $0 == ">" || ($0.properties.generalCategory == .control && $0 != "\n" && $0 != "\r" && $0 != "\t") }
    }

    /// "$0.42", "$0.004", "$12.50".
    static func usd(_ value: Double) -> String {
        if value == 0 { return "$0" }
        if value < 0.01 { return String(format: "$%.3f", value) }
        return String(format: "$%.2f", value)
    }

    static func percent(_ part: Int, of whole: Int) -> String {
        guard whole > 0 else { return "0%" }
        return "\(Int((Double(part) / Double(whole) * 100).rounded()))%"
    }

    static func compact(_ n: Int) -> String {
        n >= 1_000_000 ? String(format: "%.1fM", Double(n) / 1_000_000) : n >= 10_000 ? "\(n / 1000)k" : n.formatted()
    }

    static func trimmed(_ s: String) -> String { s.trimmingCharacters(in: .whitespacesAndNewlines) }
}

extension AppEnvironment {
    /// The agent store, pointed at the open workspace.
    @MainActor var agent: AgentStore {
        let store = AgentStore.shared
        store.bind(self)
        return store
    }
}

extension Router {
    /// Opens a page the setup checklist or a link names by its web path (`/settings/ai/tools`).
    func openAgentHref(_ href: String) {
        let parts = href.split(separator: "/").map(String.init)
        if parts == ["settings", "widget"] {
            accountPath.append(.setting(.brands))
            return
        }
        let stack = AccountRoute.stack(from: parts)
        if !stack.isEmpty { accountPath.append(contentsOf: stack) }
    }
}

/// "The agent is off." with the way to turn AI on.
struct AgentOffNotice: View {
    var title = "The agent is off."
    var message = "Chats go straight to your team until you turn it on."
    var action = "Turn it on"

    @Environment(AppEnvironment.self) private var env

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            OllieMark(size: 26, state: .muted)
                .frame(width: 30, height: 30)
            VStack(alignment: .leading, spacing: 3) {
                Text(title).font(ODFont.sans(14.5, weight: .semibold)).foregroundStyle(Theme.fg)
                Text(message).font(ODFont.footnote).foregroundStyle(Theme.fg3).fixedSize(horizontal: false, vertical: true)
                Button(action) {
                    Haptics.tap()
                    env.router.accountPath.append(.setting(.ai))
                }
                .buttonStyle(ChipButtonStyle(tone: .violet))
                .padding(.top, 6)
            }
            Spacer(minLength: 0)
        }
        .padding(14)
        .background(Tone.violet.soft, in: RoundedRectangle(cornerRadius: Theme.Radius.card, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: Theme.Radius.card, style: .continuous).strokeBorder(Tone.violet.line, lineWidth: 1))
    }
}

/// "12 / 40" under a field, red when over.
struct AgentCounter: View {
    let count: Int
    let max: Int

    var body: some View {
        Text("\(count) / \(max)")
            .font(ODFont.mono(11))
            .foregroundStyle(count > max ? Tone.red.text : Theme.fg4)
            .monospacedDigit()
    }
}

/// A labelled field with a hint, a character count and the reason it is refused.
struct AgentField: View {
    let label: String
    @Binding var text: String
    var prompt = ""
    var hint: String?
    var max: Int?
    var multiline = false
    var mono = false
    var problem: String?
    var keyboard: UIKeyboardType = .default

    @FocusState private var focused: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(alignment: .firstTextBaseline) {
                Text(label).font(ODFont.sans(13, weight: .medium)).foregroundStyle(Theme.fg2)
                Spacer(minLength: 8)
                if let max, text.count > max * 3 / 4 { AgentCounter(count: text.count, max: max) }
            }
            Group {
                if multiline {
                    TextField(text: $text, axis: .vertical) { Text(prompt).foregroundStyle(Theme.fg4) }
                        .lineLimit(3 ... 14)
                        .padding(.vertical, 11)
                        .frame(minHeight: 96, alignment: .topLeading)
                } else {
                    TextField(text: $text) { Text(prompt).foregroundStyle(Theme.fg4) }
                        .frame(height: 46)
                }
            }
            .font(mono ? ODFont.mono(14) : ODFont.sans(15.5))
            .foregroundStyle(Theme.fg)
            .keyboardType(keyboard)
            .textInputAutocapitalization(keyboard == .URL || keyboard == .emailAddress ? .never : .sentences)
            .autocorrectionDisabled(keyboard == .URL || keyboard == .emailAddress || mono)
            .focused($focused)
            .padding(.horizontal, 13)
            .background(Theme.surface, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous)
                    .strokeBorder(shownProblem != nil ? Tone.red.solid.opacity(0.6) : focused ? Theme.lineStrong : Theme.line, lineWidth: focused ? 1.5 : 1)
            )
            .motion(Motion.fade, value: focused)
            if let shownProblem {
                FormError(text: shownProblem)
            } else if let hint {
                Text(hint).font(ODFont.footnote).foregroundStyle(Theme.fg3).fixedSize(horizontal: false, vertical: true)
            }
        }
    }

    private var shownProblem: String? {
        if let problem { return problem }
        if let max { return AgentText.problem(text, max: max) }
        return nil
    }
}

/// A list of short phrases (topics, paths): chips with a remove button, and a field to add one.
struct AgentChipEditor: View {
    let label: String
    var hint: String?
    @Binding var items: [String]
    var max = 20
    var prompt = "Add"
    var empty = "None yet."
    /// Why a phrase is refused, or nil.
    var check: (String) -> String? = { _ in nil }
    var mono = false

    @State private var text = ""
    @State private var error: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            VStack(alignment: .leading, spacing: 2) {
                Text(label).font(ODFont.sans(13, weight: .medium)).foregroundStyle(Theme.fg2)
                if let hint { Text(hint).font(ODFont.footnote).foregroundStyle(Theme.fg3).fixedSize(horizontal: false, vertical: true) }
            }
            if items.isEmpty {
                Text(empty).font(ODFont.footnote).foregroundStyle(Theme.fg4)
            } else {
                AgentFlow(spacing: 6) {
                    ForEach(items, id: \.self) { item in
                        HStack(spacing: 4) {
                            Text(item)
                                .font(mono ? ODFont.mono(12.5) : ODFont.sans(13))
                                .foregroundStyle(Theme.fg)
                                .lineLimit(1)
                            Button {
                                Haptics.tap()
                                withAnimation(Motion.snappy) { items.removeAll { $0 == item } }
                            } label: {
                                Image(systemName: "xmark").font(.system(size: 9, weight: .bold)).foregroundStyle(Theme.fg3)
                                    .frame(width: 18, height: 18)
                            }
                            .buttonStyle(.plain)
                            .accessibilityLabel("Remove \(item)")
                        }
                        .padding(.leading, 10)
                        .padding(.trailing, 4)
                        .frame(height: 28)
                        .background(Theme.sunken, in: Capsule())
                        .overlay(Capsule().strokeBorder(Theme.line, lineWidth: 1))
                    }
                }
            }
            if items.count < max {
                HStack(spacing: 8) {
                    TextField(text: $text) { Text(prompt).foregroundStyle(Theme.fg4) }
                        .font(mono ? ODFont.mono(14) : ODFont.sans(15))
                        .textInputAutocapitalization(mono ? .never : .sentences)
                        .autocorrectionDisabled(mono)
                        .submitLabel(.done)
                        .onSubmit(add)
                        .padding(.horizontal, 12)
                        .frame(height: 40)
                        .background(Theme.surface, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
                        .overlay(RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous).strokeBorder(Theme.line, lineWidth: 1))
                        .onChange(of: text) { _, _ in error = nil }
                    Button("Add", action: add)
                        .buttonStyle(OutlineButtonStyle(fullWidth: false, height: 40))
                        .disabled(AgentText.trimmed(text).isEmpty)
                }
            } else {
                Text("At most \(max)").font(ODFont.footnote).foregroundStyle(Theme.fg3)
            }
            FormError(text: error)
        }
    }

    private func add() {
        let value = AgentText.trimmed(text)
        guard !value.isEmpty else { return }
        if items.contains(where: { $0.caseInsensitiveCompare(value) == .orderedSame }) {
            error = "Already in the list"
            return
        }
        if let problem = check(value) {
            error = problem
            return
        }
        Haptics.tap()
        withAnimation(Motion.snappy) { items.append(value) }
        text = ""
    }
}

/// Lays chips out in rows that wrap.
struct AgentFlow: Layout {
    var spacing: CGFloat = 6

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let width = proposal.width ?? .infinity
        var x: CGFloat = 0, y: CGFloat = 0, row: CGFloat = 0, widest: CGFloat = 0
        for view in subviews {
            let size = view.sizeThatFits(.unspecified)
            if x > 0 && x + size.width > width {
                y += row + spacing
                x = 0
                row = 0
            }
            x += size.width + spacing
            row = Swift.max(row, size.height)
            widest = Swift.max(widest, x - spacing)
        }
        return CGSize(width: proposal.width ?? widest, height: y + row)
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        var x = bounds.minX, y = bounds.minY, row: CGFloat = 0
        for view in subviews {
            let size = view.sizeThatFits(.unspecified)
            if x > bounds.minX && x + size.width > bounds.maxX {
                y += row + spacing
                x = bounds.minX
                row = 0
            }
            view.place(at: CGPoint(x: x, y: y), proposal: ProposedViewSize(size))
            x += size.width + spacing
            row = Swift.max(row, size.height)
        }
    }
}

/// Minus and plus around a number, without the system's spinner.
struct AgentStepper: View {
    @Binding var value: Int
    let range: ClosedRange<Int>
    var step = 1
    var suffix: String?

    var body: some View {
        HStack(spacing: 0) {
            button("minus", enabled: value > range.lowerBound) { value = Swift.max(range.lowerBound, value - step) }
            Text(suffix.map { "\(value) \($0)" } ?? "\(value)")
                .font(ODFont.sans(14.5, weight: .medium))
                .monospacedDigit()
                .foregroundStyle(Theme.fg)
                .frame(minWidth: 44)
                .contentTransition(.numericText(value: Double(value)))
            button("plus", enabled: value < range.upperBound) { value = Swift.min(range.upperBound, value + step) }
        }
        .frame(height: 34)
        .background(Theme.active.opacity(0.7), in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
        .motion(Motion.snappy, value: value)
        .accessibilityElement(children: .ignore)
        .accessibilityValue(suffix.map { "\(value) \($0)" } ?? "\(value)")
        .accessibilityAdjustableAction { direction in
            switch direction {
            case .increment: value = Swift.min(range.upperBound, value + step)
            case .decrement: value = Swift.max(range.lowerBound, value - step)
            @unknown default: break
            }
        }
    }

    private func button(_ symbol: String, enabled: Bool, action: @escaping () -> Void) -> some View {
        Button {
            Haptics.selection()
            action()
        } label: {
            Image(systemName: symbol)
                .font(.system(size: 12, weight: .semibold))
                .foregroundStyle(enabled ? Theme.fg : Theme.fg4)
                .frame(width: 36, height: 34)
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .disabled(!enabled)
        .buttonRepeatBehavior(.enabled)
    }
}

/// A US dollar amount with quick picks; 0 is "No cap".
struct AgentUSDField: View {
    @Binding var value: Double
    var presets: [Double] = [0, 25, 50, 100, 250]
    var max: Double = 10_000

    @State private var text = ""
    @FocusState private var focused: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 6) {
                Text("$").font(ODFont.sans(15.5)).foregroundStyle(Theme.fg3)
                TextField(text: $text) { Text("No cap").foregroundStyle(Theme.fg4) }
                    .font(ODFont.sans(15.5))
                    .keyboardType(.decimalPad)
                    .focused($focused)
                    .onChange(of: text) { _, next in
                        let cleaned = next.replacingOccurrences(of: ",", with: ".")
                        if cleaned.isEmpty { value = 0 } else if let number = Double(cleaned) { value = Swift.min(max, Swift.max(0, number)) }
                    }
                Text("a month").font(ODFont.footnote).foregroundStyle(Theme.fg3)
            }
            .padding(.horizontal, 13)
            .frame(height: 46)
            .background(Theme.surface, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous).strokeBorder(focused ? Theme.lineStrong : Theme.line, lineWidth: focused ? 1.5 : 1))
            HStack(spacing: 6) {
                ForEach(presets, id: \.self) { preset in
                    Button(preset == 0 ? "No cap" : "$\(Int(preset))") {
                        Haptics.selection()
                        value = preset
                        text = preset == 0 ? "" : "\(Int(preset))"
                    }
                    .buttonStyle(ChipButtonStyle(tone: value == preset ? .violet : nil, filled: value == preset))
                }
            }
        }
        .onAppear { text = value == 0 ? "" : Self.plain(value) }
        .onChange(of: value) { _, next in
            if !focused { text = next == 0 ? "" : Self.plain(next) }
        }
    }

    private static func plain(_ value: Double) -> String {
        value.rounded() == value ? "\(Int(value))" : String(format: "%.2f", value)
    }
}

/// A row that shows the chosen model and opens the picker.
struct AgentModelRow: View {
    let title: String
    var subtitle: String?
    @Binding var model: String
    /// What "" means: "Same as the main model".
    var emptyTitle = "Same as the main model"

    @Environment(AppEnvironment.self) private var env
    @State private var open = false

    var body: some View {
        let store = env.agent
        Button {
            Haptics.tap()
            open = true
        } label: {
            AccountRow(symbol: "cpu", tone: .violet, title: title, subtitle: subtitle) {
                AccountDisclosure(value: model.isEmpty ? emptyTitle : store.models?.data.first { $0.id == model }?.name ?? model)
            }
        }
        .buttonStyle(RowPressStyle())
        .sheet(isPresented: $open) {
            AgentModelPicker(model: $model, emptyTitle: emptyTitle)
                .presentationDetents([.medium, .large])
                .presentationCornerRadius(Theme.Radius.sheet)
        }
        .task { await store.loadModels() }
    }
}

struct AgentModelPicker: View {
    @Binding var model: String
    var emptyTitle: String

    @Environment(AppEnvironment.self) private var env
    @Environment(\.dismiss) private var dismiss
    @State private var query = ""

    var body: some View {
        let store = env.agent
        let models = store.models?.data ?? []
        let text = query.lowercased()
        let shown = text.isEmpty ? models : models.filter { $0.name.lowercased().contains(text) || $0.vendor.lowercased().contains(text) || $0.id.lowercased().contains(text) }
        VStack(alignment: .leading, spacing: 14) {
            AccountSheetHeader(title: "Choose a model", subtitle: "Each reply costs what it uses on this model.") { dismiss() }
            HStack(spacing: 8) {
                Image(systemName: "magnifyingglass").font(.system(size: 13)).foregroundStyle(Theme.fg3)
                TextField("Search models", text: $query)
                    .font(ODFont.sans(15))
                    .autocorrectionDisabled()
                    .textInputAutocapitalization(.never)
            }
            .padding(.horizontal, 12)
            .frame(height: 40)
            .background(Theme.sunken, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
            ScrollView {
                Panel {
                    if text.isEmpty {
                        AccountChoiceRow(symbol: "arrow.uturn.backward", title: emptyTitle, chosen: model.isEmpty) { pick("") }
                        if !models.isEmpty { AccountDivider() }
                    }
                    if store.models == nil {
                        SkeletonRows(count: 3)
                    } else if shown.isEmpty {
                        Text("No models available").font(ODFont.footnote).foregroundStyle(Theme.fg3).padding(14)
                    }
                    ForEach(Array(shown.enumerated()), id: \.element.id) { index, item in
                        if index > 0 { AccountDivider() }
                        AccountChoiceRow(symbol: "cpu", tone: .violet, title: item.name.isEmpty ? item.id : item.name, subtitle: subtitle(item, isDefault: item.id == store.models?.defaultID), chosen: model == item.id) { pick(item.id) }
                    }
                }
                .padding(.bottom, 24)
            }
            .scrollDismissesKeyboard(.interactively)
        }
        .padding(.horizontal, 20)
        .padding(.top, 22)
        .background(Theme.chrome)
        .task { await store.loadModels() }
    }

    private func subtitle(_ model: AgentModel, isDefault: Bool) -> String {
        var parts: [String] = []
        if isDefault { parts.append("Default") }
        if !model.vendor.isEmpty { parts.append(model.vendor) }
        if let input = model.inputUSD, let output = model.outputUSD { parts.append("\(AgentText.usd(input)) / \(AgentText.usd(output)) per 1M") }
        if let context = model.contextLength, context > 0 { parts.append("\(AgentText.compact(context)) context") }
        return parts.joined(separator: " · ")
    }

    private func pick(_ id: String) {
        model = id
        dismiss()
    }
}

/// One figure on a summary strip: a label, a big number and a line under it.
struct AgentStat: View {
    let label: String
    let value: String
    var hint: String?
    var tone: Tone?

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(label).font(ODFont.sans(12, weight: .medium)).foregroundStyle(Theme.fg3).lineLimit(1)
            Text(value)
                .font(ODFont.sans(22, weight: .semibold))
                .tracking(-0.4)
                .foregroundStyle(tone?.text ?? Theme.fg)
                .lineLimit(1)
                .minimumScaleFactor(0.7)
                .contentTransition(.numericText())
            if let hint {
                Text(hint).font(ODFont.sans(11.5)).foregroundStyle(Theme.fg3).lineLimit(2).fixedSize(horizontal: false, vertical: true)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(12)
        .background(Theme.surface, in: RoundedRectangle(cornerRadius: Theme.Radius.card, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: Theme.Radius.card, style: .continuous).strokeBorder(Theme.line, lineWidth: 1))
    }
}

/// Two figures to a row.
struct AgentStatGrid<Content: View>: View {
    @ViewBuilder var content: () -> Content

    var body: some View {
        LazyVGrid(columns: [GridItem(.flexible(), spacing: 10), GridItem(.flexible(), spacing: 10)], spacing: 10) { content() }
    }
}

/// A short title with its line under it, above a group of fields.
struct AgentGroupTitle: View {
    let title: String
    var hint: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(title).font(ODFont.sans(14.5, weight: .semibold)).foregroundStyle(Theme.fg)
            if let hint { Text(hint).font(ODFont.footnote).foregroundStyle(Theme.fg3).fixedSize(horizontal: false, vertical: true) }
        }
        .padding(.horizontal, 4)
    }
}

/// The help line at the top of an agent page.
struct AgentIntro: View {
    let text: String

    var body: some View {
        Text(text)
            .font(ODFont.callout)
            .foregroundStyle(Theme.fg3)
            .fixedSize(horizontal: false, vertical: true)
            .padding(.horizontal, 4)
    }
}

/// A small sunken note with a lightbulb, for empty states' tips.
struct AgentTip: View {
    let text: String

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            Image(systemName: "lightbulb").font(.system(size: 12)).foregroundStyle(Tone.amber.text)
            Text(text).font(ODFont.footnote).foregroundStyle(Theme.fg2).fixedSize(horizontal: false, vertical: true)
        }
    }
}
