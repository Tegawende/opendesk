import SwiftUI

/// One mailbox: its connection and health, how replies go out, its names and addresses,
/// signature, auto-reply, Ollie, replies from members' own addresses, and removing it.
/// One Save sends every change as the web's save bar does; enabling, removing and switching
/// to forwarding happen at once.
struct MailboxDetailView: View {
    let id: String

    @Environment(AppEnvironment.self) private var env
    @State private var store = MailboxSettingsStore()
    @State private var mailbox: SettingsMailbox?
    @State private var draft = MailboxDraft()
    @State private var original = MailboxDraft()
    @State private var loadError: String?
    @State private var missing = false
    @State private var saving = false
    @State private var saveError: String?
    @State private var testing = false
    @State private var testResult: MailboxTestResult?
    @State private var confirm: AccountConfirm?
    @State private var web: String?
    @State private var forwardTab = "gmail"
    @State private var editingAlias: String?
    @State private var checkingSendAs = false

    private var admin: Bool { env.session.isAdmin }
    private var dirty: Bool { draft != original }

    var body: some View {
        AccountPage(title: mailbox?.label ?? "Mailbox", spacing: 22) {
            if missing {
                EmptyState(symbol: "tray", title: "Mailbox not found", message: "It may have been removed.", actionTitle: "Back to mailboxes") {
                    if !env.router.accountPath.isEmpty { env.router.accountPath.removeLast() }
                }
                .padding(.top, 40)
            } else if let mailbox {
                header(mailbox)
                if !admin { SettingsReadOnlyNote() }
                connection(mailbox)
                if mailbox.readsDirectly { tidyUp(mailbox) }
                if mailbox.outboundKind == "system" || mailbox.outboundKind == "smtp" || mailbox.sendOnly { sending(mailbox) }
                names(mailbox)
                aliases(mailbox)
                contactForms(mailbox)
                signature
                autoReply
                ollie
                if env.directory.collab.enabled { personalReplies(mailbox) }
                if let saveError { ChannelBanner(tone: .red, title: "Couldn't save the mailbox", message: saveError) }
                if admin { danger(mailbox) }
            } else if let loadError {
                ErrorState(message: loadError) { Task { await load() } }.padding(.top, 40)
            } else {
                Panel { SkeletonRows(count: 5) }
            }
        }
        .disabled(saving)
        .settingsSaveBar(dirty: admin && mailbox != nil && dirty, saving: saving, canSave: draft.valid, save: save) { draft = original }
        .accountConfirm($confirm)
        .channelWebSheet($web) { Task { await load() } }
        .sheet(item: Binding(get: { editingAlias.map(MailboxAliasEdit.init) }, set: { editingAlias = $0?.alias })) { edit in
            MailboxAliasSheet(alias: edit.alias, draft: $draft, brands: store.brands, disabled: !admin)
        }
        .task {
            store.attach(env.session.api)
            async let extras: Void = store.loadExtras(admin: admin)
            await load()
            await extras
        }
    }

    private func load() async {
        do {
            let fresh = try await store.fetch(id)
            mailbox = fresh
            let next = MailboxDraft(fresh)
            if !dirty { draft = next }
            original = next
            loadError = nil
            forwardTab = MailboxForwarding.tab(for: fresh.config.mailHost)
        } catch let failure as APIError where failure.status == 404 {
            missing = true
        } catch {
            if !error.isCancellation { loadError = error.userMessage }
        }
    }

    // MARK: Header

    private func header(_ mailbox: SettingsMailbox) -> some View {
        HStack(spacing: 12) {
            AccountIconTile(symbol: MailboxKinds.symbol(mailbox), tone: MailboxKinds.tone(mailbox), size: 40)
            VStack(alignment: .leading, spacing: 3) {
                Text(mailbox.label).font(ODFont.sans(17, weight: .semibold)).foregroundStyle(Theme.fg)
                Text(mailbox.email).font(ODFont.mono(13)).foregroundStyle(Theme.fg2).lineLimit(1)
            }
            Spacer(minLength: 6)
            MailboxStatusPill(mailbox: mailbox)
        }
        .padding(.horizontal, 4)
    }

    // MARK: Connection

    @ViewBuilder
    private func connection(_ mailbox: SettingsMailbox) -> some View {
        let provider = mailbox.providerName ?? "Google"
        VStack(alignment: .leading, spacing: 10) {
            if mailbox.imapOff {
                ChannelBanner(tone: .amber, title: "This mailbox isn't receiving mail", message: "Receiving over IMAP is off on this instance, so opendesk no longer fetches mail from \(mailbox.email). Switch to forwarding to keep getting it. Replies still go out the way they do now.", action: admin ? "Switch to forwarding" : nil) { askForwarding(mailbox) }
            } else if mailbox.status == "error" {
                ChannelBanner(tone: .red, title: "Last sync failed", message: mailbox.lastError, mono: true)
            } else if let days = mailbox.quietDays(now: AppClock.now) {
                ChannelBanner(tone: .amber, title: days == 0 ? "No mail has arrived yet" : "No mail for \(ChannelFormat.plural(days, "day"))", message: mailbox.inboundKind == "forwarding"
                    ? "If customers write to \(mailbox.email), check that it still forwards to the address below, then send a test from another address."
                    : "If customers write to \(mailbox.email), test the connection and reconnect if access was revoked.")
            }
            AccountSection(title: "Connection", footer: mailbox.connectionLabel) {
                AccountRow(title: "Last mail received", subtitle: "Mail that arrived at this address by any route, including mail that did not become a ticket.") {
                    Text(ChannelFormat.ago(mailbox.lastReceivedAt, never: "No mail yet")).font(ODFont.sans(14)).foregroundStyle(Theme.fg3)
                }
                AccountDivider(tiles: false)
                AccountRow(title: "Last synced", subtitle: mailbox.inboundKind == "forwarding" ? "Forwarded mail arrives as soon as it is sent." : nil) {
                    Text(ChannelFormat.ago(mailbox.lastSyncedAt, never: "Never")).font(ODFont.sans(14)).foregroundStyle(Theme.fg3)
                }
                if admin {
                    AccountDivider(tiles: false)
                    ChannelActionRow(title: "Test connection", subtitle: "Signs in to the server without sending anything.", button: "Test", busy: testing) { runTest() }
                }
                if (mailbox.inboundKind == "google" || mailbox.inboundKind == "microsoft"), !mailbox.sendOnly {
                    AccountDivider(tiles: false)
                    let hint = mailbox.readsDirectly
                        ? "Reads the inbox directly through \(provider). New emails become tickets on their own. Reconnect if access was revoked."
                        : (store.options.instanceIMAP ? "Connected with \(provider). Reconnect if access was revoked or the password changed." : "Connected with \(provider). Reconnecting connects it to send only, and its mail then arrives by forwarding.")
                    ChannelActionRow(title: mailbox.config.oauthEmail.isEmpty ? "Account" : mailbox.config.oauthEmail, subtitle: hint, button: "Reconnect", disabled: !admin) { openWeb(mailbox) }
                }
                if mailbox.inboundKind == "forwarding", admin {
                    let host = mailbox.config.mailHost
                    let p = MailboxKinds.isMicrosoft(host) ? "microsoft" : "google"
                    let opts = p == "microsoft" ? store.options.microsoft : store.options.google
                    if opts.apiSync, MailboxKinds.isGoogle(host) || MailboxKinds.isMicrosoft(host) || mailbox.sendOnly {
                        AccountDivider(tiles: false)
                        ChannelActionRow(title: "Read the inbox directly", subtitle: "New emails become tickets on their own. No forwarding to set up. You'll sign in with \(MailboxKinds.providerName(p)) to allow it, in the web app.", button: "Open") { openWeb(mailbox) }
                    }
                }
                if mailbox.inboundKind != "forwarding", admin {
                    AccountDivider(tiles: false)
                    ChannelActionRow(title: "Receiving", subtitle: mailbox.readsDirectly ? "opendesk reads new mail through \(provider). Forwarding works without it." : "opendesk signs in to the mailbox to fetch new mail. Forwarding works without signing in.", button: "Switch to forwarding") { askForwarding(mailbox) }
                }
            }
            if let testResult { ChannelTestResults(result: testResult) }
            if mailbox.inboundKind == "forwarding", !mailbox.forwardingAddress.isEmpty { forwardingInstructions(mailbox) }
            if mailbox.inboundKind == "imap", mailbox.receiving == "ok", admin {
                ChannelForm(title: "Incoming (IMAP)") {
                    ChannelServerFields(title: "Server", host: $draft.imapHost, port: $draft.imapPort, security: $draft.imapTLS, hostPrompt: "imap.company.com", tlsPort: 993, starttlsPort: 143)
                    ODTextField(label: "Username", text: $draft.imapUser, prompt: mailbox.email, keyboard: .emailAddress)
                    ODTextField(label: "Password", text: $draft.imapPassword, prompt: "••••••••", secure: true, contentType: .password)
                    ChannelHint("Leave empty to keep the saved one.")
                }
            }
        }
    }

    private func forwardingInstructions(_ mailbox: SettingsMailbox) -> some View {
        ChannelForm(title: "Forwarding") {
            ChannelCopyField(label: "Forward mail for \(mailbox.email) to", text: mailbox.forwardingAddress, copiedMessage: "Forwarding address copied")
            ScrollView(.horizontal, showsIndicators: false) {
                AccountSegmented(options: MailboxForwarding.tabs, selection: $forwardTab, label: MailboxForwarding.title)
                    .frame(width: 520)
            }
            VStack(alignment: .leading, spacing: 10) {
                ForEach(Array(MailboxForwarding.steps(forwardTab).enumerated()), id: \.offset) { index, step in
                    HStack(alignment: .firstTextBaseline, spacing: 10) {
                        Text("\(index + 1)")
                            .font(ODFont.mono(11.5, weight: .semibold))
                            .foregroundStyle(Theme.fg2)
                            .frame(width: 20, height: 20)
                            .background(Theme.active, in: Circle())
                        Text(MailboxForwarding.styled(step)).font(ODFont.sans(14)).foregroundStyle(Theme.fg).fixedSize(horizontal: false, vertical: true)
                    }
                }
            }
            if forwardTab == "microsoft" {
                Notice(text: "Microsoft 365 blocks automatic forwarding to outside addresses by default. An admin allows it once, then the forward is set on the mailbox.", tone: .amber, symbol: "exclamationmark.triangle")
            }
            if let link = MailboxForwarding.open(forwardTab, email: mailbox.email) {
                Link(destination: link.url) { Label(link.title, systemImage: "arrow.up.right.square").font(ODFont.sans(14, weight: .medium)) }
            }
            if let docs = URL(string: "https://docs.opendesk.sh/guides/mailboxes/forwarding/") {
                Link(destination: docs) { Label("Full guide in the opendesk docs", systemImage: "book").font(ODFont.sans(14, weight: .medium)) }
            }
            ChannelHint("To check it works, send an email to \(mailbox.email) from another account. It shows up as a new ticket in Unassigned.")
            if let mail = URL(string: "mailto:\(mailbox.email)?subject=Testing%20opendesk") {
                Link(destination: mail) { Label("Write a test email", systemImage: "square.and.pencil").font(ODFont.sans(14, weight: .medium)) }
            }
        }
    }

    // MARK: Provider tidy-up

    private func tidyUp(_ mailbox: SettingsMailbox) -> some View {
        let app = mailbox.inboundKind == "microsoft" ? "Outlook" : "Gmail"
        return AccountSection(title: "In \(app)", footer: "Replies show up in the same \(app) conversation as the customer's email, and in Sent.") {
            AccountToggleRow(title: "Mark emails as read in \(app) once they become tickets", isOn: $draft.markRead, disabled: !admin)
            AccountDivider(tiles: false)
            AccountToggleRow(title: "Archive in \(app) when the ticket is solved", isOn: $draft.archiveOnSolve, disabled: !admin)
            AccountDivider(tiles: false)
            AccountToggleRow(title: "Label emails \u{201C}opendesk\u{201D} in \(app)", subtitle: app == "Outlook" ? "Outlook shows it as a category." : nil, isOn: $draft.label, disabled: !admin)
        }
    }

    // MARK: Sending

    @ViewBuilder
    private func sending(_ mailbox: SettingsMailbox) -> some View {
        let provider = MailboxKinds.providerName(mailbox.outboundKind)
        let kinds = sendKinds(mailbox)
        VStack(alignment: .leading, spacing: 10) {
            if mailbox.systemOff {
                ChannelBanner(tone: .amber, title: "This mailbox can't send replies", message: "Replies from \(mailbox.email) go out through your own email provider. Connect Google or Microsoft, or add SMTP from your mail server or a sending service such as Resend or Postmark. Until then, replies can't be sent.")
            }
            ChannelForm(title: "Sending", footer: "How replies from this mailbox reach customers.") {
                VStack(alignment: .leading, spacing: 6) {
                    Text("How replies are sent").font(ODFont.sans(13, weight: .medium)).foregroundStyle(Theme.fg2)
                    AccountSegmented(options: kinds, selection: Binding(get: { draft.outbound }, set: { value in
                        if value == "smtp", draft.outbound != "smtp", draft.smtpHost.isEmpty {
                            draft.smtpPort = 587
                            draft.smtpTLS = "starttls"
                            draft.smtpUser = mailbox.email
                        }
                        draft.outbound = value
                    }), label: { kind in
                        switch kind {
                        case "system": mailbox.systemOff ? "Not set up" : "Through opendesk"
                        case "smtp": "SMTP server"
                        default: "With \(provider)"
                        }
                    })
                    .disabled(!admin)
                    ChannelHint(draft.outbound == "smtp" ? "From \(mailbox.email) itself."
                        : draft.outbound == "system" ? "From \(store.options.systemSender.isEmpty ? "opendesk" : store.options.systemSender) with your mailbox's name, and replies to the customer come back to \(mailbox.email)."
                        : "From \(mailbox.email) itself, sent with \(provider).")
                }
                if mailbox.sendOnly, draft.outbound == mailbox.outboundKind {
                    ChannelHint("Connected with \(provider) to send only. opendesk can't read the mailbox. Reconnect if access was revoked.")
                    if admin { ChannelRowButton(title: "Reconnect") { openWeb(mailbox) } }
                }
                if mailbox.inboundKind == "forwarding", !mailbox.sendOnly, draft.outbound != "smtp", admin {
                    ChannelHint("Replies go out from \(mailbox.email) itself. Connecting only lets opendesk send as this address. It does not read the mailbox. Connecting opens the web app.")
                    ChannelRowButton(title: "Send with Google or Microsoft") { openWeb(mailbox) }
                }
                if draft.outbound == "smtp", admin {
                    ChannelServerFields(title: "SMTP server", host: $draft.smtpHost, port: $draft.smtpPort, security: $draft.smtpTLS, hostPrompt: "smtp.company.com", tlsPort: 465, starttlsPort: 587)
                    ODTextField(label: "Username", text: $draft.smtpUser, prompt: mailbox.email, keyboard: .emailAddress)
                    ODTextField(label: "Password", text: $draft.smtpPassword, prompt: "••••••••", secure: true, contentType: .password)
                    ChannelHint(draft.smtpPasswordNeeded(mailbox) ? "Enter the password for this server." : "Leave empty to keep the saved one.")
                    if MailboxKinds.isMicrosoft(mailbox.config.mailHost) {
                        Notice(text: "Microsoft is phasing out password sign-in for SMTP AUTH in Exchange Online, and many admins already have it off. If the sign-in is refused, send with Microsoft or through a sending service instead.", tone: .amber, symbol: "exclamationmark.triangle")
                    }
                }
            }
        }
    }

    private func sendKinds(_ mailbox: SettingsMailbox) -> [String] {
        var kinds: [String] = []
        if mailbox.sendOnly { kinds.append(mailbox.outboundKind) }
        if store.options.systemSend || mailbox.outboundKind == "system" { kinds.append("system") }
        kinds.append("smtp")
        return kinds
    }

    // MARK: Names and addresses

    private func names(_ mailbox: SettingsMailbox) -> some View {
        ChannelForm(title: "Name and addresses", footer: "How this mailbox appears to your team and to customers.") {
            ODTextField(label: "Name", text: $draft.name, prompt: "Support")
            ChannelHint("Shown to your team.")
            if draft.name.trimmingCharacters(in: .whitespaces).isEmpty { FormError(text: "Give the mailbox a name, like Support.") }
            ODTextField(label: "Sender name", text: $draft.displayName, prompt: draft.name)
            ChannelHint("Fills {mailbox} in the From name. Empty uses the name.")
            if store.brands.count > 1 {
                ChannelMenuField(label: "Brand", value: store.brands.first { $0.id == draft.brandID }?.name ?? "Default brand", hint: "Tickets to this address belong to this brand, and its replies carry the brand's name and logo.") {
                    ForEach(store.brands) { brand in Button(brand.name) { draft.brandID = brand.id } }
                }
            }
            ChannelTeamMenu(label: "New tickets to this address go to", teamID: $draft.teamID, hint: "The team that owns email to this address. Routing rules can still send a ticket elsewhere.")
            VStack(alignment: .leading, spacing: 6) {
                ODTextField(label: "From name", text: $draft.fromNameFormat, prompt: "{agent} at {mailbox}")
                FlowLayout(spacing: 6) {
                    ForEach(["{agent}", "{agent_first}", "{team}", "{brand}", "{mailbox}"], id: \.self) { variable in
                        Button(variable) { draft.fromNameFormat += (draft.fromNameFormat.isEmpty || draft.fromNameFormat.hasSuffix(" ") ? "" : " ") + variable }
                            .font(ODFont.mono(12))
                            .buttonStyle(ChipButtonStyle())
                    }
                }
                ChannelHint("What customers see as the sender. Tap a variable to add it. Empty uses {agent} at {mailbox}.")
                if !draft.fromNameValid { FormError(text: "Fix the From name first: use plain text and the variables above.") }
            }
        }
        .disabled(!admin)
    }

    // MARK: Aliases

    private func aliases(_ mailbox: SettingsMailbox) -> some View {
        ChannelForm(title: "Aliases", footer: "Other addresses that deliver into this mailbox, like help@ or a side project's hello@. Each works like a mailbox of its own, with its own sender name, signature, team and brand. Aliases are free on every plan and don't count as mailboxes. Up to 20.") {
            ChannelChipEditor(label: "", items: $draft.aliases, placeholder: "help@company.com", empty: "No aliases", max: 20, disabled: !admin) { text, _ in
                let clean = text.lowercased()
                guard ChannelValidate.email(clean) else { return .failure(ChannelChipProblem("That doesn't look like an email address.")) }
                guard clean != mailbox.email.lowercased() else { return .failure(ChannelChipProblem("That's this mailbox's own address.")) }
                return .success(clean)
            }
            if !draft.aliases.isEmpty {
                VStack(spacing: 0) {
                    ForEach(draft.aliases, id: \.self) { alias in
                        Button { editingAlias = alias } label: {
                            AccountRow(title: alias, subtitle: aliasSummary(alias, mailbox)) { Chevron() }
                        }
                        .buttonStyle(RowPressStyle())
                        if alias != draft.aliases.last { Hairline() }
                    }
                }
                .background(Theme.surface, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
                .overlay(RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous).strokeBorder(Theme.line, lineWidth: 1))
            }
            if let sendAs = mailbox.providerSendAs, admin { gmailSendAs(sendAs, mailbox) }
        }
    }

    private func aliasSummary(_ alias: String, _ mailbox: SettingsMailbox) -> String {
        var parts: [String] = []
        if let name = draft.aliasNames[alias], !name.isEmpty { parts.append(name) }
        parts.append(env.directory.team(draft.aliasTeams[alias])?.name ?? "No team")
        if mailbox.outboundKind != "system" {
            let sendAs = mailbox.aliasSettings[alias]?.sendAs ?? "unknown"
            parts.append(sendAs == "verified" || mailbox.outboundKind == "smtp" ? "Sends as itself" : "Replies from \(mailbox.email)")
        }
        return parts.joined(separator: " · ")
    }

    private func gmailSendAs(_ sendAs: MailboxSendAs, _ mailbox: SettingsMailbox) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("From Gmail").font(ODFont.sans(14, weight: .semibold)).foregroundStyle(Theme.fg)
            if !sendAs.readable {
                ChannelHint("Google doesn't share this account's other send-as addresses with a send-only connection. Add them above by hand.")
            } else {
                ChannelHint("Other addresses \(mailbox.config.oauthEmail.isEmpty ? mailbox.email : mailbox.config.oauthEmail) can send as in Gmail. Add one to receive and answer its mail here." + (sendAs.checkedAt.map { " Checked \(Format.relLong($0))." } ?? ""))
                let others = sendAs.addresses.filter { $0.email.lowercased() != mailbox.email.lowercased() }
                if others.isEmpty {
                    ChannelHint("Gmail lists no other addresses.")
                } else {
                    ForEach(others, id: \.email) { address in
                        HStack {
                            Text(address.email).font(ODFont.mono(13)).foregroundStyle(Theme.fg)
                            Spacer()
                            if draft.aliases.contains(address.email.lowercased()) {
                                Image(systemName: "checkmark").foregroundStyle(Tone.green.text)
                            } else if !address.verified {
                                Text("Waiting for confirmation in Gmail").font(ODFont.footnote).foregroundStyle(Theme.fg3)
                            } else {
                                ChannelRowButton(title: "Add") { draft.aliases.append(address.email.lowercased()) }
                                    .accessibilityLabel("Add \(address.email) as an alias")
                            }
                        }
                    }
                }
                ChannelRowButton(title: "Check again", busy: checkingSendAs) {
                    checkingSendAs = true
                    Task {
                        do {
                            try await store.checkSendAs(mailbox.id)
                            if let fresh = store.mailbox(mailbox.id) { self.mailbox = fresh }
                            env.toasts.show("Checked Gmail's send-as addresses", kind: .success)
                        } catch {
                            env.toasts.error(error)
                        }
                        checkingSendAs = false
                    }
                }
            }
        }
        .padding(12)
        .background(Theme.sunken, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
    }

    // MARK: Contact forms

    private func contactForms(_ mailbox: SettingsMailbox) -> some View {
        ChannelForm(title: "Contact forms", footer: "Add the address your website's contact form sends from. When its email passes the sender check, the visitor who filled in the form becomes the contact and replies go to them.") {
            ChannelChipEditor(label: "", items: $draft.trusted, placeholder: "forms@company.com or @company.com", empty: "No contact forms. Enter an address or @domain.", max: 50, disabled: !admin) { text, _ in
                let clean = text.lowercased()
                if clean.hasPrefix("@") {
                    let domain = String(clean.dropFirst())
                    guard ChannelValidate.host(domain) else { return .failure(ChannelChipProblem("Enter an address, or @ and a domain.")) }
                    if MailboxDetailView.publicDomains.contains(domain) { return .failure(ChannelChipProblem("\(domain) is a public email provider. Enter the form's exact address instead.")) }
                    return .success(clean)
                }
                guard ChannelValidate.email(clean) else { return .failure(ChannelChipProblem("Enter an address, or @ and a domain.")) }
                guard clean != mailbox.email.lowercased() else { return .failure(ChannelChipProblem("That's this mailbox's own address.")) }
                return .success(clean)
            }
        }
    }

    static let publicDomains: Set<String> = ["gmail.com", "googlemail.com", "outlook.com", "hotmail.com", "live.com", "yahoo.com", "icloud.com", "me.com", "aol.com", "proton.me", "protonmail.com", "gmx.com", "gmx.de", "yandex.com", "mail.com"]

    // MARK: Signature and auto-reply

    private var signature: some View {
        ChannelForm(title: "Signature", footer: "Added below every reply sent from this address, unless the ticket's team has its own. Include each sender's own lines with {{ agent.signature }}.") {
            AccountTextEditor(label: "Signature", text: $draft.signature.text, prompt: "The Support team at Company", minHeight: 100)
            if draft.signature.originalHTML.contains("<img") || draft.signature.originalHTML.contains("<table") {
                ChannelHint("This signature has images or a layout from the web app. Editing it here saves it as plain text.")
            }
        }
        .disabled(!admin)
    }

    private var autoReply: some View {
        ChannelForm(title: "Auto-reply", footer: "Sent once when a customer opens a new ticket by email.") {
            Toggle("Send an automatic reply", isOn: $draft.autoReplyEnabled).font(ODFont.sans(15.5)).tint(Tone.green.solid)
            if draft.autoReplyEnabled {
                ODTextField(label: "Subject", text: $draft.autoReplySubject, prompt: "We got your message")
                AccountTextEditor(label: "Message", text: $draft.autoReply.text, prompt: "Thanks for writing in. We usually reply within a few hours.", minHeight: 120)
                ChannelHint("Variables like {{ ticket.number }} and {{ contact.first_name }} are filled in. Sender variables, like the agent's name, are empty in auto-replies.")
            }
        }
        .disabled(!admin)
    }

    // MARK: Ollie

    private var ollie: some View {
        let name = store.bots.first { $0.id == draft.botID }?.name ?? env.directory.ai.assistantName ?? "Ollie"
        let aiOn = env.directory.ai.enabled
        return AccountSection(title: name, footer: aiOn ? "Let \(name) answer emails to this mailbox from your knowledge." : "AI is off for this workspace. Turn it on in Settings > AI.") {
            AccountToggleRow(symbol: "sparkles", tone: .violet, title: "\(name) answers emails to this mailbox", subtitle: "Off by default. Spam, auto-replies, lists and mail from your team are never answered.", isOn: $draft.aiAnswers, disabled: !admin || (!aiOn && !draft.aiAnswers))
            if draft.aiAnswers {
                if !store.bots.isEmpty {
                    AccountDivider()
                    ChannelMenuRow(title: "Bot", value: store.bots.first { $0.id == draft.botID }?.name ?? "Default bot", disabled: !admin) {
                        Button("Default bot") { draft.botID = nil }
                        ForEach(store.bots) { bot in Button(bot.name) { draft.botID = bot.id } }
                    }
                }
                AccountDivider()
                VStack(alignment: .leading, spacing: 8) {
                    Text("Mode").font(ODFont.sans(13, weight: .medium)).foregroundStyle(Theme.fg2)
                    AccountSegmented(options: ["draft", "send"], selection: $draft.aiMode) { $0 == "draft" ? "Draft for review" : "Send automatically" }
                        .disabled(!admin)
                    ChannelHint(draft.aiMode == "draft"
                        ? "\(name) writes a suggested reply on the ticket. A teammate sends it with one click, edits or discards it."
                        : "\(name) replies by email right away and hands the email to your team when unsure.")
                }
                .padding(14)
                AccountDivider()
                AccountToggleRow(title: "Only outside business hours", subtitle: "During business hours your team answers as usual.", isOn: $draft.aiOutsideHours, disabled: !admin)
                AccountDivider()
                Text("When \(name) can't help, the ticket waits for your team, unassigned.")
                    .font(ODFont.footnote).foregroundStyle(Theme.fg3).padding(14)
            }
        }
    }

    // MARK: Personal replies

    private func personalReplies(_ mailbox: SettingsMailbox) -> some View {
        let workspace = env.session.workspace?.workspace.settings
        let defaultOn = workspace?.collabTicketReplies ?? true
        let defaultReplyTo = workspace?.collabTicketReplyTo ?? "address"
        let domains = Set(([mailbox.email] + draft.aliases).compactMap { $0.split(separator: "@").last.map(String.init) }).sorted().joined(separator: ", ")
        let allowed = draft.personal.enabled ?? defaultOn
        return AccountSection(title: "Replies from members' own addresses", footer: "With Collab, members can answer this address's tickets from one of their own mail accounts, one reply at a time. The customer's answers still come back to the ticket.") {
            ChannelMenuRow(title: "Allowed", subtitle: allowed ? "Members can pick one of their own addresses in the reply box on this address's tickets." : "Replies on this address's tickets always go from this address.", value: draft.personal.enabled.map { $0 ? "On" : "Off" } ?? "Default (\(defaultOn ? "On" : "Off"))", disabled: !admin) {
                Button("Default (\(defaultOn ? "On" : "Off"))") { draft.personal.enabled = nil }
                Button("On") { draft.personal.enabled = true }
                Button("Off") { draft.personal.enabled = false }
            }
            if allowed {
                AccountDivider(tiles: false)
                ChannelMenuRow(title: "Answers go to", subtitle: "Where the customer's answers go unless the member picks another. Either way they come back to the ticket.", value: draft.personal.replyTo.map { $0 == "mailbox" ? "This address" : "Their address" } ?? "Default (\(defaultReplyTo == "mailbox" ? "This address" : "Their address"))", disabled: !admin) {
                    Button("Default (\(defaultReplyTo == "mailbox" ? "This address" : "Their address"))") { draft.personal.replyTo = nil }
                    Button("Their address") { draft.personal.replyTo = "address" }
                    Button("This address") { draft.personal.replyTo = "mailbox" }
                }
                AccountDivider(tiles: false)
                ChannelMenuRow(title: "Which addresses", subtitle: draft.personal.accounts == "shared" ? "Only accounts shared with the team, never someone's private mail." : draft.personal.accounts == "domain" ? "Only addresses at \(domains)." : "Any address of an account the member can send from, private or shared.", value: draft.personal.accounts == "shared" ? "Shared accounts" : draft.personal.accounts == "domain" ? "At \(domains)" : "Any", disabled: !admin) {
                    Button("Any") { draft.personal.accounts = "any" }
                    Button("Shared accounts") { draft.personal.accounts = "shared" }
                    Button("At \(domains)") { draft.personal.accounts = "domain" }
                }
            }
        }
    }

    // MARK: Danger zone

    private func danger(_ mailbox: SettingsMailbox) -> some View {
        AccountSection(title: "Danger zone") {
            ChannelActionRow(title: mailbox.disabled ? "Enable mailbox" : "Disable mailbox", subtitle: mailbox.disabled ? "Start receiving and sending mail again." : "Stop syncing and sending without removing anything.", button: mailbox.disabled ? "Enable" : "Disable") {
                Task {
                    do {
                        self.mailbox = try await store.patch(mailbox.id, ["status": .string(mailbox.disabled ? "active" : "disabled")])
                        Haptics.success()
                    } catch {
                        env.toasts.error(error)
                    }
                }
            }
            AccountDivider(tiles: false)
            ChannelActionRow(title: "Remove mailbox", subtitle: "Existing tickets stay. New mail to this address stops arriving.", button: "Remove", buttonTone: .red) { askRemove(mailbox) }
        }
    }

    // MARK: Actions

    private func openWeb(_ mailbox: SettingsMailbox) {
        web = "/settings/mailboxes/\(mailbox.id)"
    }

    private func runTest() {
        testing = true
        Task {
            do {
                testResult = try await store.test(id)
            } catch {
                env.toasts.show("The test couldn't run", kind: .error)
            }
            testing = false
        }
    }

    private func askForwarding(_ mailbox: SettingsMailbox) {
        confirm = AccountConfirm(title: "Switch to forwarding?", message: "opendesk stops signing in to \(mailbox.email) to fetch mail and gives it an address to forward to. New mail arrives once the forward is set up. Replies still go out the way they do now.", confirmTitle: "Switch to forwarding", destructive: false) {
            do {
                self.mailbox = try await store.patch(mailbox.id, ["inbound_kind": .string("forwarding")])
                await load()
                Haptics.success()
                env.toasts.show("Switched to forwarding. Set up the forward below.", kind: .success)
            } catch {
                env.toasts.error(error)
            }
        }
    }

    private func askRemove(_ mailbox: SettingsMailbox) {
        Task {
            let usage = await store.usage(mailbox.id)
            confirm = AccountConfirm(title: "Remove mailbox?", message: MailboxSettingsStore.removeMessage(mailbox, usage: usage), confirmTitle: "Remove") {
                do {
                    try await store.remove(mailbox.id)
                    Haptics.success()
                    env.toasts.show("Mailbox removed", kind: .success)
                    if !env.router.accountPath.isEmpty { env.router.accountPath.removeLast() }
                } catch {
                    env.toasts.error(error)
                }
            }
        }
    }

    private func save() {
        guard let mailbox, draft.valid else { return }
        saving = true
        saveError = nil
        Task {
            do {
                let saved = try await store.patch(mailbox.id, draft.patch(from: original, mailbox: mailbox, imapShown: mailbox.inboundKind == "imap" && mailbox.receiving == "ok"))
                self.mailbox = saved
                draft = MailboxDraft(saved)
                original = draft
                testResult = nil
                Haptics.success()
                env.toasts.show("Mailbox saved", kind: .success)
            } catch {
                Haptics.error()
                saveError = (error as? APIError)?.detail ?? error.userMessage
            }
            saving = false
        }
    }
}

private struct MailboxAliasEdit: Identifiable {
    let alias: String
    var id: String { alias }
}

/// One alias: its sender name, team and brand.
private struct MailboxAliasSheet: View {
    let alias: String
    @Binding var draft: MailboxDraft
    let brands: [SettingsBrand]
    let disabled: Bool

    var body: some View {
        ChannelSheet(title: alias, subtitle: "Works like a mailbox of its own.") {
            ODTextField(label: "Sender name", text: Binding(get: { draft.aliasNames[alias] ?? "" }, set: { draft.aliasNames[alias] = String($0.prefix(64)) }), prompt: "")
            ChannelHint("What {mailbox} reads in the From name. Empty uses the mailbox's.")
            ChannelTeamMenu(label: "New tickets go to", teamID: Binding(get: { draft.aliasTeams[alias] }, set: { draft.aliasTeams[alias] = $0 }))
            if brands.count > 1 {
                ChannelMenuField(label: "Brand", value: brands.first { $0.id == draft.aliasBrands[alias] }?.name ?? "The mailbox's brand", hint: "New tickets to this alias take this brand's emails, help center and survey.") {
                    Button("The mailbox's brand") { draft.aliasBrands[alias] = nil }
                    ForEach(brands) { brand in Button(brand.name) { draft.aliasBrands[alias] = brand.id } }
                }
            }
            ChannelHint("Changes are saved with the mailbox. The alias's own signature is edited in the web app.")
        }
        .disabled(disabled)
    }
}

/// What the mailbox page edits, and the PATCH it sends.
struct MailboxDraft: Equatable {
    var name = ""
    var displayName = ""
    var teamID: String?
    var brandID: String?
    var aliases: [String] = []
    var aliasTeams: [String: String] = [:]
    var aliasNames: [String: String] = [:]
    var aliasBrands: [String: String] = [:]
    var trusted: [String] = []
    var fromNameFormat = ""
    var signature = ChannelRichText(html: "")
    var autoReplyEnabled = false
    var autoReplySubject = ""
    var autoReply = ChannelRichText(html: "")
    var aiAnswers = false
    var aiMode = "draft"
    var aiOutsideHours = false
    var botID: String?
    var personal = MailboxPersonalReplies()
    var outbound = "smtp"
    var imapHost = ""
    var imapPort = 993
    var imapTLS = "tls"
    var imapUser = ""
    var imapPassword = ""
    var smtpHost = ""
    var smtpPort = 587
    var smtpTLS = "starttls"
    var smtpUser = ""
    var smtpPassword = ""
    var markRead = false
    var archiveOnSolve = false
    var label = false

    init() {}

    init(_ m: SettingsMailbox) {
        name = m.name
        displayName = m.displayName
        teamID = m.teamID
        brandID = m.brandID
        aliases = m.aliases
        aliasTeams = m.aliasTeams
        aliasNames = m.aliasSettings.compactMapValues { $0.displayName.isEmpty ? nil : $0.displayName }
        aliasBrands = m.aliasSettings.compactMapValues(\.brandID)
        trusted = m.trustedFormSenders
        fromNameFormat = m.fromNameFormat
        signature = ChannelRichText(html: m.signatureHTML)
        autoReplyEnabled = m.autoReplyEnabled
        autoReplySubject = m.autoReplySubject
        autoReply = ChannelRichText(html: m.autoReplyHTML)
        aiAnswers = m.aiAnswers
        aiMode = m.aiAnswersMode
        aiOutsideHours = m.aiAnswersOutsideHours
        botID = m.botID
        personal = m.personalReplies
        outbound = m.outboundKind
        imapHost = m.config.imapHost
        imapPort = m.config.imapPort
        imapTLS = m.config.imapTLS
        imapUser = m.config.imapUsername
        smtpHost = m.config.smtpHost
        smtpPort = m.config.smtpPort
        smtpTLS = m.config.smtpTLS
        smtpUser = m.config.smtpUsername
        markRead = m.config.providerMarkRead
        archiveOnSolve = m.config.providerArchiveOnSolve
        label = m.config.providerLabel
    }

    /// Plain text, with only the known variables in braces.
    var fromNameValid: Bool {
        let text = fromNameFormat
        guard ChannelValidate.plain(text), text.count <= 200 else { return false }
        let known: Set<String> = ["agent", "agent_first", "team", "brand", "mailbox"]
        var rest = Substring(text)
        while let open = rest.firstIndex(of: "{") {
            guard let close = rest[open...].firstIndex(of: "}") else { return false }
            if !known.contains(String(rest[rest.index(after: open) ..< close])) { return false }
            rest = rest[rest.index(after: close)...]
        }
        return !rest.contains("}")
    }

    func smtpPasswordNeeded(_ mailbox: SettingsMailbox) -> Bool { outbound == "smtp" && mailbox.outboundKind != "smtp" }

    var valid: Bool {
        guard !name.trimmingCharacters(in: .whitespaces).isEmpty, fromNameValid, (1 ... 65535).contains(imapPort) else { return false }
        if outbound == "smtp" {
            return ChannelValidate.host(smtpHost) && (1 ... 65535).contains(smtpPort)
        }
        return true
    }

    func patch(from old: MailboxDraft, mailbox: SettingsMailbox, imapShown: Bool) -> ChannelBody {
        var body: ChannelBody = [
            "name": .string(name.trimmingCharacters(in: .whitespaces)),
            "display_name": .string(displayName.trimmingCharacters(in: .whitespaces)),
            "team_id": .string(teamID ?? ""),
            "signature_html": .string(signature.html),
            "auto_reply_enabled": .bool(autoReplyEnabled),
            "auto_reply_subject": .string(autoReplySubject),
            "auto_reply_html": .string(autoReply.html),
        ]
        if brandID != old.brandID, let brandID { body["brand_id"] = .string(brandID) }
        if aliases != old.aliases { body["aliases"] = .chStrings(aliases) }
        if aliasTeams != old.aliasTeams || aliases != old.aliases {
            body["alias_teams"] = .object(aliasTeams.filter { aliases.contains($0.key) }.mapValues(JSONValue.string))
        }
        if aliasNames != old.aliasNames || aliasBrands != old.aliasBrands || aliases != old.aliases {
            var settings: ChannelBody = [:]
            for alias in aliases {
                settings[alias] = .object(["display_name": .string(aliasNames[alias] ?? ""), "brand_id": .chString(aliasBrands[alias])])
            }
            body["alias_settings"] = .object(settings)
        }
        if trusted != old.trusted { body["trusted_form_senders"] = .chStrings(trusted) }
        if fromNameFormat != old.fromNameFormat { body["from_name_format"] = .string(fromNameFormat.trimmingCharacters(in: .whitespaces)) }
        if aiAnswers != old.aiAnswers { body["ai_answers"] = .bool(aiAnswers) }
        if aiMode != old.aiMode { body["ai_answers_mode"] = .string(aiMode) }
        if aiOutsideHours != old.aiOutsideHours { body["ai_answers_outside_hours"] = .bool(aiOutsideHours) }
        if botID != old.botID { body["bot_id"] = .chString(botID) }
        if personal != old.personal { body["personal_replies"] = personal.json }
        if outbound != old.outbound { body["outbound_kind"] = .string(outbound) }

        var config: ChannelBody = [:]
        if imapShown {
            config["imap_host"] = .string(imapHost.trimmingCharacters(in: .whitespaces).lowercased())
            config["imap_port"] = .chInt(imapPort)
            config["imap_username"] = .string(imapUser)
            config["imap_tls"] = .string(imapTLS)
        }
        if outbound == "smtp" {
            config["smtp_host"] = .string(smtpHost.trimmingCharacters(in: .whitespaces).lowercased())
            config["smtp_port"] = .chInt(smtpPort)
            config["smtp_username"] = .string(smtpUser)
            config["smtp_tls"] = .string(smtpTLS)
        }
        if markRead != old.markRead || archiveOnSolve != old.archiveOnSolve || label != old.label {
            config["provider_mark_read"] = .bool(markRead)
            config["provider_archive_on_solve"] = .bool(archiveOnSolve)
            config["provider_label"] = .bool(label)
        }
        if !config.isEmpty { body["config"] = .object(config) }
        if !imapPassword.isEmpty { body["password"] = .string(imapPassword) }
        if !smtpPassword.isEmpty { body["smtp_password"] = .string(smtpPassword) }

        let imapChanged = imapShown && (imapHost != old.imapHost || imapPort != old.imapPort || imapUser != old.imapUser || imapTLS != old.imapTLS)
        let smtpChanged = outbound == "smtp" && (smtpHost != old.smtpHost || smtpPort != old.smtpPort || smtpUser != old.smtpUser || smtpTLS != old.smtpTLS)
        if imapChanged || smtpChanged || !imapPassword.isEmpty || !smtpPassword.isEmpty || outbound != old.outbound { body["verify"] = .bool(true) }
        return body
    }
}

/// The forwarding steps for each provider (the web's `forwarding.steps`).
enum MailboxForwarding {
    static let tabs = ["gmail", "workspace", "microsoft", "outlook", "other"]

    static func title(_ tab: String) -> String {
        switch tab {
        case "gmail": "Gmail"
        case "workspace": "Google Workspace"
        case "microsoft": "Microsoft 365"
        case "outlook": "Outlook.com"
        default: "Other"
        }
    }

    static func tab(for host: String) -> String {
        switch host {
        case "gmail": "gmail"
        case "google_workspace": "workspace"
        case "microsoft365": "microsoft"
        case "outlook": "outlook"
        case "": "gmail"
        default: "other"
        }
    }

    static func steps(_ tab: String) -> [String] {
        switch tab {
        case "gmail": [
            "Open Gmail's <b>Forwarding and POP/IMAP</b> settings, signed in as this address.",
            "Choose <b>Add a forwarding address</b> and paste the address above.",
            "Gmail sends a confirmation email. It arrives in opendesk as a ticket: open it and click the link, or copy the code into Gmail.",
            "Back in Gmail, select <b>Forward a copy of incoming mail to</b> the new address and save.",
        ]
        case "workspace": [
            "For one mailbox, follow the Gmail steps while signed in as that user.",
            "For a group or many users, an admin opens <b>Routing</b> in the Google Admin console (<b>Apps</b>, <b>Google Workspace</b>, <b>Gmail</b>).",
            "Add a routing rule for <b>Inbound</b> mail to the address, choose <b>Modify message</b>, then <b>Add more recipients</b>, and add the address above.",
            "Save. Changes can take a few minutes to apply.",
        ]
        case "microsoft": [
            "An admin opens the <b>Anti-spam</b> policies in Microsoft Defender, edits the <b>Outbound</b> policy and sets <b>Automatic forwarding rules</b> to <b>On</b>.",
            "In the <b>Exchange admin center</b>, open <b>Mailboxes</b> and select the mailbox.",
            "Under <b>Mail flow settings</b>, edit <b>Email forwarding</b>: turn it on, paste the address above and keep a copy in the mailbox.",
            "No access to the admin center? After step 1, open <b>Forwarding</b> in Outlook's settings as the mailbox, turn it on, paste the address above and keep a copy.",
        ]
        case "outlook": [
            "Open Outlook.com's <b>Forwarding</b> settings, signed in as this address.",
            "Turn on <b>Enable forwarding</b> and paste the address above.",
            "Turn on <b>Keep a copy of forwarded messages</b> and save.",
        ]
        default: [
            "In your email provider's settings, find forwarding. Some call it a redirect or an alias.",
            "Forward all mail for this address to the address above, keeping a copy if you want one.",
        ]
        }
    }

    static func open(_ tab: String, email: String) -> (title: String, url: URL)? {
        let encoded = email.addingPercentEncoding(withAllowedCharacters: .alphanumerics) ?? email
        switch tab {
        case "gmail": return URL(string: "https://mail.google.com/mail/?authuser=\(encoded)#settings/fwdandpop").map { ("Open Gmail forwarding settings", $0) }
        case "workspace": return URL(string: "https://admin.google.com/ac/apps/gmail/routing").map { ("Open Gmail routing in the Admin console", $0) }
        case "microsoft": return URL(string: "https://admin.exchange.microsoft.com/#/mailboxes").map { ("Open mailboxes in the Exchange admin center", $0) }
        case "outlook": return URL(string: "https://outlook.live.com/mail/0/options/mail/forwarding").map { ("Open Outlook.com forwarding settings", $0) }
        default: return nil
        }
    }

    /// `<b>` in the web's strings, as bold.
    static func styled(_ step: String) -> AttributedString {
        let markdown = step.replacingOccurrences(of: "<b>", with: "**").replacingOccurrences(of: "</b>", with: "**")
        return (try? AttributedString(markdown: markdown)) ?? AttributedString(step.replacingOccurrences(of: "<b>", with: "").replacingOccurrences(of: "</b>", with: ""))
    }
}
