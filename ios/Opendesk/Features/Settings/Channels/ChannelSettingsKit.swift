import SwiftUI
import UIKit

// Pieces the Channels settings pages share, on top of the Account kit and SettingsKit.

/// A titled panel of form fields, spaced like the Account editors.
struct ChannelForm<Content: View>: View {
    var title: String?
    var footer: String?
    @ViewBuilder var content: () -> Content

    var body: some View {
        AccountSection(title: title, footer: footer) {
            VStack(alignment: .leading, spacing: 14) { content() }
                .padding(14)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
    }
}

/// A grey line of help under a field.
struct ChannelHint: View {
    let text: String
    init(_ text: String) { self.text = text }

    var body: some View {
        Text(text)
            .font(ODFont.footnote)
            .foregroundStyle(Theme.fg3)
            .fixedSize(horizontal: false, vertical: true)
    }
}

/// A labelled field that opens a menu of choices.
struct ChannelMenuField<Items: View>: View {
    let label: String
    let value: String
    var hint: String?
    var disabled = false
    @ViewBuilder var items: () -> Items

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            if !label.isEmpty {
                Text(label).font(ODFont.sans(13, weight: .medium)).foregroundStyle(Theme.fg2)
            }
            Menu {
                items()
            } label: {
                HStack {
                    Text(value)
                        .font(ODFont.sans(16))
                        .foregroundStyle(Theme.fg)
                        .lineLimit(1)
                    Spacer(minLength: 6)
                    Image(systemName: "chevron.up.chevron.down").font(.system(size: 11, weight: .semibold)).foregroundStyle(Theme.fg3)
                }
                .padding(.horizontal, 13)
                .frame(height: 46)
                .background(Theme.surface, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
                .overlay(RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous).strokeBorder(Theme.line, lineWidth: 1))
                .contentShape(Rectangle())
            }
            .disabled(disabled)
            .opacity(disabled ? 0.55 : 1)
            if let hint { ChannelHint(hint) }
        }
    }
}

/// A row whose value opens a menu of choices, inside a panel.
struct ChannelMenuRow<Items: View>: View {
    let title: String
    var subtitle: String?
    let value: String
    var disabled = false
    @ViewBuilder var items: () -> Items

    var body: some View {
        Menu {
            items()
        } label: {
            AccountRow(title: title, subtitle: subtitle) {
                HStack(spacing: 6) {
                    Text(value)
                        .font(ODFont.sans(14.5))
                        .foregroundStyle(Theme.fg2)
                        .lineLimit(1)
                    Image(systemName: "chevron.up.chevron.down").font(.system(size: 10, weight: .semibold)).foregroundStyle(Theme.fg4)
                }
            }
        }
        .disabled(disabled)
        .opacity(disabled ? 0.55 : 1)
    }
}

/// The team picker every channel uses, with its "none" option named by the caller.
struct ChannelTeamMenu: View {
    let label: String
    @Binding var teamID: String?
    var none = "No team"
    var hint: String?
    var disabled = false

    @Environment(AppEnvironment.self) private var env

    var body: some View {
        ChannelMenuField(label: label, value: env.directory.team(teamID)?.name ?? none, hint: hint, disabled: disabled) {
            Button(none) { teamID = nil }
            ForEach(env.directory.teams) { team in
                Button(team.name) { teamID = team.id }
            }
        }
    }
}

/// Text to copy (a webhook address, a forwarding address, a snippet), with a copy button.
struct ChannelCopyField: View {
    var label: String?
    let text: String
    var mono = true
    var copiedMessage = "Copied"

    @Environment(AppEnvironment.self) private var env

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            if let label { Text(label).font(ODFont.sans(13, weight: .medium)).foregroundStyle(Theme.fg2).fixedSize(horizontal: false, vertical: true) }
            HStack(alignment: .top, spacing: 10) {
                Text(text)
                    .font(mono ? ODFont.mono(13) : ODFont.sans(14))
                    .foregroundStyle(Theme.fg)
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .fixedSize(horizontal: false, vertical: true)
                Button {
                    UIPasteboard.general.string = text
                    Haptics.success()
                    env.toasts.show(copiedMessage, kind: .success)
                } label: {
                    Image(systemName: "doc.on.doc")
                        .font(.system(size: 13, weight: .medium))
                        .foregroundStyle(Theme.fg2)
                        .frame(width: 30, height: 30)
                        .background(Theme.active, in: RoundedRectangle(cornerRadius: 8, style: .continuous))
                }
                .buttonStyle(TapScaleStyle(scale: 0.9))
                .accessibilityLabel("Copy")
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 10)
            .background(Theme.sunken, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous).strokeBorder(Theme.line, lineWidth: 1))
        }
    }
}

/// A small outlined button for a row's trailing side.
struct ChannelRowButton: View {
    let title: String
    var tone: Tone?
    var busy = false
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Group {
                if busy {
                    ProgressView().controlSize(.small)
                } else {
                    Text(title).font(ODFont.sans(13.5, weight: .medium)).lineLimit(1)
                }
            }
            .foregroundStyle(tone?.text ?? Theme.fg)
            .padding(.horizontal, 12)
            .frame(height: 32)
            .background(Theme.surface, in: RoundedRectangle(cornerRadius: Theme.Radius.control - 2, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: Theme.Radius.control - 2, style: .continuous).strokeBorder(tone?.line ?? Theme.line, lineWidth: 1))
        }
        .buttonStyle(TapScaleStyle())
        .disabled(busy)
    }
}

/// A row with a title, a line of help and a button on the right.
struct ChannelActionRow: View {
    var symbol: String?
    var tone: Tone = .gray
    let title: String
    var subtitle: String?
    let button: String
    var buttonTone: Tone?
    var busy = false
    var disabled = false
    let action: () -> Void

    var body: some View {
        AccountRow(symbol: symbol, tone: tone, title: title, subtitle: subtitle) {
            ChannelRowButton(title: button, tone: buttonTone, busy: busy, action: action)
                .disabled(disabled)
                .opacity(disabled ? 0.5 : 1)
        }
    }
}

/// A row that opens the web app's page in the in-app browser, for what needs a large screen.
struct ChannelWebRow: View {
    var symbol = "safari"
    var title = "Open in the web app"
    var subtitle: String?
    let path: String
    var onClose: (() -> Void)?

    @State private var open: String?

    var body: some View {
        Button {
            Haptics.tap()
            open = path
        } label: {
            AccountRow(symbol: symbol, title: title, subtitle: subtitle) {
                Image(systemName: "arrow.up.right").font(.system(size: 12, weight: .semibold)).foregroundStyle(Theme.fg4)
            }
        }
        .buttonStyle(RowPressStyle())
        .channelWebSheet($open) { onClose?() }
    }
}

/// Opens the web app at `path` in the in-app browser from code, calling `onClose` when it shuts.
struct ChannelWebSheet: ViewModifier {
    @Binding var path: String?
    var onClose: () -> Void

    @Environment(AppEnvironment.self) private var env

    func body(content: Content) -> some View {
        content.sheet(isPresented: Binding(get: { path != nil }, set: { if !$0 { path = nil } }), onDismiss: onClose) {
            if let path, let url = env.session.instance.webURL(path: path) {
                AccountSafariView(url: url).ignoresSafeArea()
            }
        }
    }
}

extension View {
    func channelWebSheet(_ path: Binding<String?>, onClose: @escaping () -> Void = {}) -> some View {
        modifier(ChannelWebSheet(path: path, onClose: onClose))
    }
}

/// Loading, failed or loaded: the shared skeleton and retry around a page's content.
struct ChannelLoading<Content: View>: View {
    let loaded: Bool
    var error: String?
    let retry: () -> Void
    @ViewBuilder var content: () -> Content

    var body: some View {
        if !loaded {
            Panel { SkeletonRows(count: 4) }
        } else if let error {
            ErrorState(message: error, retry: retry).padding(.top, 40)
        } else {
            content()
        }
    }
}

/// A tile of a summary strip: a number, a label and a line of help.
struct ChannelStatTile: View {
    let value: String
    let label: String
    var hint: String?
    var tone: Tone = .gray

    var body: some View {
        VStack(alignment: .leading, spacing: 3) {
            Text(value)
                .font(ODFont.sans(20, weight: .semibold))
                .foregroundStyle(tone == .gray ? Theme.fg : tone.text)
                .monospacedDigit()
            Text(label).font(ODFont.sans(12.5, weight: .medium)).foregroundStyle(Theme.fg2).lineLimit(1)
            if let hint { Text(hint).font(ODFont.sans(11.5)).foregroundStyle(Theme.fg3).lineLimit(2) }
        }
        .padding(12)
        .frame(maxWidth: .infinity, minHeight: 78, alignment: .topLeading)
        .background(Theme.surface, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous).strokeBorder(Theme.line, lineWidth: 1))
    }
}

/// A row with a number that steps up and down.
struct ChannelStepperRow: View {
    let title: String
    var subtitle: String?
    @Binding var value: Int
    let range: ClosedRange<Int>
    var step = 1
    var suffix = ""
    var disabled = false

    var body: some View {
        AccountRow(title: title, subtitle: subtitle) {
            HStack(spacing: 10) {
                Text("\(value)\(suffix)").font(ODFont.mono(14)).foregroundStyle(Theme.fg2).monospacedDigit()
                Stepper(title, value: $value, in: range, step: step).labelsHidden().disabled(disabled)
            }
        }
    }
}

struct ChannelChipProblem: Error {
    let message: String
    init(_ message: String) { self.message = message }
}

/// A list of short values (aliases, origins, contact forms) with an add field.
struct ChannelChipEditor: View {
    let label: String
    @Binding var items: [String]
    var placeholder = ""
    var empty: String?
    var max = 20
    var mono = true
    var disabled = false
    var keyboard: UIKeyboardType = .emailAddress
    /// Cleans what was typed, or explains why it can't be added.
    var check: (String, [String]) -> Result<String, ChannelChipProblem> = { text, _ in .success(text) }

    @State private var draft = ""
    @State private var problem: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            if !label.isEmpty { Text(label).font(ODFont.sans(13, weight: .medium)).foregroundStyle(Theme.fg2) }
            if items.isEmpty, let empty {
                ChannelHint(empty)
            } else {
                FlowLayout(spacing: 6) {
                    ForEach(items, id: \.self) { item in
                        HStack(spacing: 5) {
                            Text(item).font(mono ? ODFont.mono(12.5) : ODFont.sans(13)).foregroundStyle(Theme.fg).lineLimit(1)
                            if !disabled {
                                Button {
                                    items.removeAll { $0 == item }
                                    Haptics.tap()
                                } label: {
                                    Image(systemName: "xmark").font(.system(size: 9, weight: .bold)).foregroundStyle(Theme.fg3)
                                }
                                .buttonStyle(.plain)
                                .accessibilityLabel("Remove \(item)")
                            }
                        }
                        .padding(.horizontal, 9)
                        .frame(height: 28)
                        .background(Theme.active, in: RoundedRectangle(cornerRadius: Theme.Radius.pill, style: .continuous))
                    }
                }
            }
            if !disabled, items.count < max {
                HStack(spacing: 8) {
                    TextField(text: $draft) { Text(placeholder).foregroundStyle(Theme.fg4) }
                        .font(mono ? ODFont.mono(14) : ODFont.sans(15))
                        .keyboardType(keyboard)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .submitLabel(.done)
                        .onSubmit(add)
                        .padding(.horizontal, 12)
                        .frame(height: 40)
                        .background(Theme.surface, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
                        .overlay(RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous).strokeBorder(Theme.line, lineWidth: 1))
                    ChannelRowButton(title: "Add", action: add)
                        .disabled(draft.trimmingCharacters(in: .whitespaces).isEmpty)
                }
                .onChange(of: draft) { _, _ in problem = nil }
            }
            FormError(text: problem)
        }
    }

    private func add() {
        let text = draft.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty else { return }
        switch check(text, items) {
        case let .success(clean):
            if items.contains(where: { $0.caseInsensitiveCompare(clean) == .orderedSame }) {
                problem = "Already in the list"
                return
            }
            items.append(clean)
            draft = ""
            Haptics.tap()
        case let .failure(failure):
            problem = failure.message
            Haptics.error()
        }
    }
}

/// A colour: the default (empty), a few presets, or a typed hex.
struct ChannelColorField: View {
    let label: String
    @Binding var hex: String
    var hint: String?
    /// The colour the default stands for, if known.
    var fallback: String?
    var disabled = false

    static let presets = ["#111111", "#2563eb", "#4f46e5", "#7c3aed", "#db2777", "#dc2626", "#ea580c", "#16a34a", "#0d9488"]

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(label).font(ODFont.sans(13, weight: .medium)).foregroundStyle(Theme.fg2)
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 10) {
                    swatch(nil)
                    ForEach(Self.presets, id: \.self) { swatch($0) }
                }
                .padding(4)
            }
            HStack(spacing: 8) {
                Circle().fill(ChannelColor.color(hex.isEmpty ? fallback : hex) ?? Theme.active).frame(width: 18, height: 18)
                TextField(text: $hex) { Text(fallback.map { "Default (\($0))" } ?? "Default").foregroundStyle(Theme.fg4) }
                    .font(ODFont.mono(14))
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
            }
            .padding(.horizontal, 12)
            .frame(height: 40)
            .background(Theme.surface, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous).strokeBorder(valid ? Theme.line : Tone.red.line, lineWidth: 1))
            if !valid { FormError(text: "Use a hex colour like #4f46e5.") } else if let hint { ChannelHint(hint) }
        }
        .disabled(disabled)
    }

    var valid: Bool { ChannelColor.valid(hex) }

    private func swatch(_ value: String?) -> some View {
        let chosen = value == nil ? hex.isEmpty : hex.lowercased() == value
        return Button {
            Haptics.selection()
            hex = value ?? ""
        } label: {
            ZStack {
                Circle().fill(ChannelColor.color(value ?? fallback) ?? Theme.active)
                if value == nil {
                    Image(systemName: "arrow.uturn.backward").font(.system(size: 10, weight: .bold)).foregroundStyle(fallback == nil ? Theme.fg3 : .white)
                }
            }
            .frame(width: 26, height: 26)
            .overlay(Circle().strokeBorder(chosen ? Theme.fg : Color.clear, lineWidth: 2).padding(-3))
        }
        .buttonStyle(.plain)
        .accessibilityLabel(value ?? "Default colour")
        .accessibilityAddTraits(chosen ? .isSelected : [])
    }
}

enum ChannelColor {
    static func valid(_ hex: String) -> Bool {
        hex.isEmpty || hex.range(of: "^#[0-9a-fA-F]{6}$", options: .regularExpression) != nil
    }

    static func color(_ hex: String?) -> Color? {
        guard let hex, hex.count == 7, hex.hasPrefix("#"), let value = UInt32(hex.dropFirst(), radix: 16) else { return nil }
        return Color(red: Double((value >> 16) & 0xFF) / 255, green: Double((value >> 8) & 0xFF) / 255, blue: Double(value & 0xFF) / 255)
    }
}

/// A confirmation that asks for a name to be typed before it removes something for good.
struct ChannelTypedConfirm: Identifiable {
    let id = UUID()
    var title: String
    var message: String
    var name: String
    var confirmTitle: String
    var action: @MainActor () async -> Void
}

struct ChannelTypedConfirmSheet: View {
    let confirm: ChannelTypedConfirm
    @Environment(\.dismiss) private var dismiss
    @State private var typed = ""
    @State private var busy = false

    private var matches: Bool { typed.trimmingCharacters(in: .whitespaces) == confirm.name }

    var body: some View {
        VStack(alignment: .leading, spacing: 18) {
            VStack(alignment: .leading, spacing: 8) {
                Text(confirm.title).font(ODFont.sans(19, weight: .semibold)).tracking(-0.3).foregroundStyle(Theme.fg)
                Text(confirm.message).font(ODFont.callout).foregroundStyle(Theme.fg2).fixedSize(horizontal: false, vertical: true)
            }
            ODTextField(label: "Type \(confirm.name) to confirm", text: $typed, prompt: confirm.name, keyboard: .asciiCapable)
            HStack(spacing: 10) {
                Button("Cancel") { dismiss() }.buttonStyle(OutlineButtonStyle())
                Button {
                    busy = true
                    Task {
                        await confirm.action()
                        busy = false
                        dismiss()
                    }
                } label: { ButtonLabel(title: confirm.confirmTitle, busy: busy) }
                    .buttonStyle(AccountDangerButtonStyle())
                    .disabled(busy || !matches)
                    .opacity(matches ? 1 : 0.5)
            }
        }
        .padding(.horizontal, 24)
        .padding(.top, 28)
        .padding(.bottom, 12)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .background(Theme.surface)
        .presentationDetents([.height(360)])
        .presentationCornerRadius(Theme.Radius.sheet)
    }
}

extension View {
    func channelTypedConfirm(_ item: Binding<ChannelTypedConfirm?>) -> some View {
        sheet(item: item) { ChannelTypedConfirmSheet(confirm: $0) }
    }
}

/// A sheet's scrolling body: header, content, and the surface behind it.
struct ChannelSheet<Content: View>: View {
    let title: String
    var subtitle: String?
    @ViewBuilder var content: () -> Content

    @Environment(\.dismiss) private var dismiss

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                AccountSheetHeader(title: title, subtitle: subtitle) { dismiss() }
                content()
            }
            .padding(22)
        }
        .scrollDismissesKeyboard(.interactively)
        .background(Theme.surface)
        .presentationDetents([.large])
        .presentationCornerRadius(Theme.Radius.sheet)
    }
}

/// A connection check, each side then the cause and its fix (the web's `TestResults`).
struct ChannelTestResults: View {
    let result: MailboxTestResult
    var only: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if only != "outbound", let leg = result.inbound { line("Receiving", leg) }
            if only == nil, result.inbound != nil, result.outbound != nil { Hairline() }
            if only != "inbound", let leg = result.outbound { line("Sending", leg) }
            if let cause = result.cause, result.failed {
                Hairline()
                VStack(alignment: .leading, spacing: 3) {
                    Text(cause.title).font(ODFont.sans(13.5, weight: .medium)).foregroundStyle(Theme.fg)
                    Text(cause.fix).font(ODFont.footnote).foregroundStyle(Theme.fg2).fixedSize(horizontal: false, vertical: true)
                }
                .padding(12)
            }
        }
        .background(Theme.sunken, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous).strokeBorder(Theme.line, lineWidth: 1))
    }

    private func line(_ what: String, _ leg: MailboxTestResult.Leg) -> some View {
        HStack(alignment: .top, spacing: 10) {
            Image(systemName: leg.ok ? "checkmark" : "xmark")
                .font(.system(size: 10, weight: .bold))
                .foregroundStyle(leg.ok ? Tone.green.text : Tone.red.text)
                .frame(width: 18, height: 18)
                .background((leg.ok ? Tone.green : Tone.red).soft, in: RoundedRectangle(cornerRadius: 5, style: .continuous))
            VStack(alignment: .leading, spacing: 2) {
                Text(leg.ok ? "\(what) works" : "\(what) failed").font(ODFont.sans(13.5, weight: .medium)).foregroundStyle(Theme.fg)
                if !leg.ok, !leg.error.isEmpty { Text(leg.error).font(ODFont.footnote).foregroundStyle(Theme.fg3).fixedSize(horizontal: false, vertical: true) }
                if !leg.ok, !leg.detail.isEmpty, leg.detail != leg.error {
                    Text("The server said: \(leg.detail)").font(ODFont.mono(11.5)).foregroundStyle(Theme.fg4).fixedSize(horizontal: false, vertical: true)
                }
            }
            Spacer(minLength: 0)
        }
        .padding(12)
    }
}

/// A banner inside a page: a title, a body and an optional action.
struct ChannelBanner: View {
    let tone: Tone
    let title: String
    var message: String?
    var mono = false
    var action: String?
    var perform: (() -> Void)?

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .firstTextBaseline, spacing: 8) {
                Image(systemName: tone == .red ? "exclamationmark.octagon" : "exclamationmark.triangle").font(.system(size: 13, weight: .semibold))
                Text(title).font(ODFont.sans(14, weight: .semibold)).fixedSize(horizontal: false, vertical: true)
            }
            .foregroundStyle(tone.text)
            if let message, !message.isEmpty {
                Text(message)
                    .font(mono ? ODFont.mono(12) : ODFont.footnote)
                    .foregroundStyle(Theme.fg2)
                    .fixedSize(horizontal: false, vertical: true)
            }
            if let action, let perform {
                ChannelRowButton(title: action, tone: tone, action: perform)
            }
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(tone.soft, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous).strokeBorder(tone.line, lineWidth: 1))
    }
}

/// A server, port and security block for IMAP or SMTP.
struct ChannelServerFields: View {
    let title: String
    @Binding var host: String
    @Binding var port: Int
    @Binding var security: String
    var hostPrompt = "imap.company.com"
    /// The ports the security choices switch to.
    var tlsPort = 993
    var starttlsPort = 143
    var allowNone = false

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text(title).font(ODFont.sans(14, weight: .semibold)).foregroundStyle(Theme.fg)
            ODTextField(label: "Server", text: $host, prompt: hostPrompt, keyboard: .URL)
            ODTextField(label: "Port", text: Binding(get: { port == 0 ? "" : String(port) }, set: { port = Int($0.filter(\.isNumber).prefix(5)) ?? 0 }), prompt: String(tlsPort), keyboard: .numberPad)
            VStack(alignment: .leading, spacing: 6) {
                Text("Security").font(ODFont.sans(13, weight: .medium)).foregroundStyle(Theme.fg2)
                AccountSegmented(options: allowNone || security == "none" ? ["tls", "starttls", "none"] : ["tls", "starttls"], selection: Binding(get: { security }, set: { value in
                    security = value
                    if value == "tls" { port = tlsPort } else if value == "starttls" { port = starttlsPort }
                }), label: { $0 == "tls" ? "TLS" : $0 == "starttls" ? "STARTTLS" : "None" })
            }
            if !host.isEmpty, !ChannelValidate.host(host) { FormError(text: "Enter a server name, like \(hostPrompt).") }
            if !(1 ... 65535).contains(port) { FormError(text: "Enter a port from 1 to 65535.") }
            if security == "none" { Notice(text: "Passwords travel unencrypted. Use this only on a private network.", tone: .amber, symbol: "exclamationmark.triangle") }
        }
    }

    static func valid(host: String, port: Int) -> Bool { ChannelValidate.host(host) && (1 ... 65535).contains(port) }
}

enum ChannelValidate {
    static func email(_ text: String) -> Bool {
        text.trimmingCharacters(in: .whitespaces).range(of: #"^[^\s@]+@[^\s@]+\.[^\s@]+$"#, options: .regularExpression) != nil
    }

    static func host(_ text: String) -> Bool {
        text.trimmingCharacters(in: .whitespaces).lowercased().range(of: #"^[a-z0-9.-]+\.[a-z]{2,}$"#, options: .regularExpression) != nil
    }

    static func plain(_ text: String) -> Bool { !text.contains("<") && !text.contains(">") }

    static func slug(_ text: String) -> Bool { text.range(of: "^[a-z0-9-]{3,40}$", options: .regularExpression) != nil }
}

/// HTML a person edits as plain text: the stored HTML stays as it was unless the text changed.
struct ChannelRichText: Equatable {
    var originalHTML: String
    var text: String

    init(html: String) {
        originalHTML = html
        text = HTMLText.plain(from: html)
    }

    var changed: Bool { text != HTMLText.plain(from: originalHTML) }

    var html: String {
        guard changed else { return originalHTML }
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? "" : HTMLText.html(fromPlain: trimmed)
    }
}

enum ChannelFormat {
    /// "5 minutes ago", or `never` without a date.
    static func ago(_ date: Date?, never: String) -> String {
        guard let date else { return never }
        return Format.relLong(date)
    }

    static func plural(_ count: Int, _ one: String, _ many: String? = nil) -> String {
        "\(count) \(count == 1 ? one : (many ?? one + "s"))"
    }
}
