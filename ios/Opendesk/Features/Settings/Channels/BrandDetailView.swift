import SwiftUI

/// One brand (the web's `BrandDetail`): overview, its addresses, its chat widgets and its help
/// center. Name, colour, language, slug and help center settings save together; pictures are
/// the web app's, which uploads them.
struct BrandDetailView: View {
    let brandID: String

    @Environment(AppEnvironment.self) private var env
    @State private var store = BrandsStore()
    @State private var tab = "overview"
    @State private var draft: BrandDraft?
    @State private var base: BrandDraft?
    @State private var saving = false
    @State private var confirm: AccountConfirm?
    @State private var typed: ChannelTypedConfirm?
    @State private var addingWidget = false
    @State private var web: String?

    private var brand: SettingsBrand? { store.brand(brandID) }

    var body: some View {
        AccountPage(title: brand?.name ?? "Brand") {
            if !env.session.isAdmin {
                EmptyState(symbol: "lock", title: "Only admins can see brands", message: "Ask a workspace admin to change brands, chat widgets and help centers.")
                    .padding(.top, 20)
            } else {
                ChannelLoading(loaded: store.loaded, error: brand == nil ? store.error : nil, retry: reload) {
                    if let brand, let draft = Binding($draft) {
                        content(brand, draft)
                    } else if store.loaded {
                        EmptyState(symbol: "building.2", title: "This brand is gone", message: "It was deleted, or the link is wrong.")
                            .padding(.top, 30)
                    }
                }
            }
        }
        .settingsSaveBar(dirty: env.session.isAdmin && draft != base, saving: saving, canSave: canSave, save: save) { draft = base }
        .accountConfirm($confirm)
        .channelTypedConfirm($typed)
        .channelWebSheet($web) { reload() }
        .sheet(isPresented: $addingWidget) {
            if let brand { WidgetNewSheet(store: store, brand: brand) }
        }
        .refreshable { await load() }
        .onAppear {
            store.attach(env.session.api)
            if env.session.isAdmin { reload() }
        }
    }

    private var canSave: Bool {
        guard let draft, draft != base, !saving else { return false }
        return !draft.name.trimmingCharacters(in: .whitespaces).isEmpty && ChannelValidate.slug(draft.slug) && draft.helpTitle.trimmingCharacters(in: .whitespaces).count <= 60 && (draft.color.isEmpty || ChannelColor.valid(draft.color))
    }

    private func reload() { Task { await load() } }

    private func load() async {
        await store.load()
        guard let brand else { return }
        let next = BrandDraft(brand)
        if draft == nil || draft == base { draft = next }
        base = next
    }

    @ViewBuilder
    private func content(_ brand: SettingsBrand, _ draft: Binding<BrandDraft>) -> some View {
        let addresses = store.mailboxes(of: brand.id)
        let widgets = store.widgets(of: brand.id)
        HStack(spacing: 12) {
            ChannelBrandMark(name: brand.name, color: draft.wrappedValue.color.isEmpty ? brand.color : draft.wrappedValue.color, size: 40)
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 6) {
                    Text(brand.name).font(ODFont.sans(18, weight: .semibold)).foregroundStyle(Theme.fg).lineLimit(1)
                    if brand.isDefault { TonePill(text: "Default", tone: .blue, height: 19) }
                }
                Text(brand.shortURL).font(ODFont.mono(12)).foregroundStyle(Theme.fg3).lineLimit(1)
            }
            Spacer(minLength: 0)
        }
        .padding(.horizontal, 4)

        ScrollView(.horizontal, showsIndicators: false) {
            AccountSegmented(options: ["overview", "addresses", "widgets", "help"], selection: $tab) { value in
                switch value {
                case "addresses": "Addresses \(addresses.count)"
                case "widgets": "Chat widgets \(widgets.count)"
                case "help": "Help center"
                default: "Overview"
                }
            }
            .frame(minWidth: 420)
        }

        switch tab {
        case "addresses": addressesSection(brand, addresses)
        case "widgets": widgetsSection(brand, widgets)
        case "help": BrandHelpCenter(store: store, brand: brand, draft: draft, base: base ?? BrandDraft(brand)) { web = $0 }
        default: overview(brand, draft, addresses: addresses, widgets: widgets)
        }
    }

    // MARK: Overview

    @ViewBuilder
    private func overview(_ brand: SettingsBrand, _ draft: Binding<BrandDraft>, addresses: [SettingsMailbox], widgets: [ChatWidget]) -> some View {
        AccountSection(title: "Brand", footer: "How the brand looks to customers.") {
            VStack(alignment: .leading, spacing: 14) {
                ODTextField(label: "Name", text: Binding(get: { draft.wrappedValue.name }, set: { draft.wrappedValue.name = String($0.prefix(60)) }))
                if draft.wrappedValue.name.trimmingCharacters(in: .whitespaces).isEmpty { FormError(text: "Give the brand a name.") }
                ChannelColorField(label: "Colour", hex: draft.color, hint: "Used by its widgets, help center and emails unless they set their own.", fallback: env.session.workspace?.workspace.settings?.brandColor)
                ChannelMenuField(label: "Language", value: BrandsStore.locales.first { $0.0 == draft.wrappedValue.locale }?.1 ?? "Workspace default", hint: "For its widgets, help center and customer emails.") {
                    Button("Workspace default") { draft.wrappedValue.locale = "" }
                    ForEach(BrandsStore.locales, id: \.0) { locale in Button(locale.1) { draft.wrappedValue.locale = locale.0 } }
                }
            }
            .padding(14)
            AccountDivider(tiles: false)
            ChannelWebRow(symbol: "photo", title: "Logo", subtitle: brand.logo == nil ? "Shown in emails, the widget and brand chips. Upload it in the web app." : "Shown in emails, the widget and brand chips. Change it in the web app.", path: "/settings/brands/\(brand.id)") { reload() }
        }

        AccountSection(title: "Default brand") {
            if brand.isDefault {
                AccountRow(title: "This is the default brand", subtitle: "New addresses and tickets without one belong to it. The default brand can't be deleted.")
            } else {
                ChannelActionRow(title: "Make \(brand.name) the default", subtitle: "New addresses and tickets without a brand belong to the default brand.", button: "Make default") {
                    confirm = AccountConfirm(title: "Make \(brand.name) the default", message: "\(brand.name) becomes the default brand. Its first widget answers the old widget settings and key.", confirmTitle: "Make default", destructive: false) {
                        do {
                            try await store.makeDefault(brand.id)
                            env.toasts.show("\(brand.name) is the default brand", kind: .success)
                        } catch { env.toasts.error(error) }
                    }
                }
                AccountDivider(tiles: false)
                ChannelActionRow(title: "Delete \(brand.name)", subtitle: addresses.isEmpty ? "Its widgets are deleted with it and stop loading on your sites." : (addresses.count == 1 ? "Move or remove its 1 address first." : "Move or remove its \(addresses.count) addresses first."), button: "Delete brand", buttonTone: .red, disabled: !addresses.isEmpty) {
                    typed = ChannelTypedConfirm(
                        title: "Delete \(brand.name)",
                        message: widgets.count == 1 ? "\(brand.name) and its 1 chat widget are deleted. The widget stops loading on your site." : "\(brand.name) and its \(widgets.count) chat widgets are deleted. They stop loading on your sites.",
                        name: brand.name,
                        confirmTitle: "Delete brand"
                    ) {
                        do {
                            try await store.deleteBrand(brand.id)
                            env.toasts.show("\(brand.name) deleted", kind: .success)
                            env.router.accountPath.removeLast()
                        } catch { env.toasts.error(error) }
                    }
                }
            }
        }
        if store.brands.count < 2 {
            Notice(text: "You have one brand. Add another when you support a second product, and brand pickers appear across opendesk.")
        }
    }

    // MARK: Addresses

    @ViewBuilder
    private func addressesSection(_ brand: SettingsBrand, _ list: [SettingsMailbox]) -> some View {
        AccountSection(title: "Addresses", footer: "Mailboxes that belong to \(brand.name). Replies to its tickets go out from them.") {
            if list.isEmpty {
                VStack(spacing: 12) {
                    EmptyState(symbol: "tray", title: "No addresses yet", message: "Connect an address like support@ for \(brand.name), and email to it opens tickets under this brand.", actionTitle: "Add an address") {
                        env.router.openSetting(.mailboxes, id: "new/\(brand.id)")
                    }
                    Label("Each address can send its email to a team.", systemImage: "tray").font(ODFont.sans(12.5)).foregroundStyle(Theme.fg3)
                }
                .padding(.vertical, 18)
                .padding(.horizontal, 12)
            } else {
                ForEach(Array(list.enumerated()), id: \.element.id) { index, mailbox in
                    if index > 0 { AccountDivider() }
                    SettingsDetailLink(page: .mailboxes, id: mailbox.id) {
                        HStack(spacing: 12) {
                            AccountIconTile(symbol: "tray", tone: .gray)
                            VStack(alignment: .leading, spacing: 2) {
                                HStack(spacing: 6) {
                                    Text(mailbox.email).font(ODFont.mono(13, weight: .medium)).foregroundStyle(Theme.fg).lineLimit(1)
                                    if brand.defaultMailboxID == mailbox.id { Text("Default reply address").font(ODFont.sans(11)).foregroundStyle(Theme.fg4) }
                                }
                                Text(mailbox.aliases.isEmpty ? mailbox.name : "Also \(mailbox.aliases.joined(separator: ", "))").font(ODFont.footnote).foregroundStyle(Theme.fg3).lineLimit(1)
                                Text(mailbox.teamID.flatMap { env.directory.team($0)?.name } ?? "No team").font(ODFont.sans(12)).foregroundStyle(Theme.fg3)
                            }
                            Spacer(minLength: 4)
                            Chevron()
                        }
                        .padding(.horizontal, 14)
                        .padding(.vertical, 11)
                        .contentShape(Rectangle())
                    }
                }
                AccountDivider(tiles: false)
                ChannelRowButton(title: "Add an address") { env.router.openSetting(.mailboxes, id: "new/\(brand.id)") }
            }
        }
    }

    // MARK: Widgets

    @ViewBuilder
    private func widgetsSection(_ brand: SettingsBrand, _ list: [ChatWidget]) -> some View {
        AccountSection(title: "Chat widgets", footer: "Each website gets its own widget, with its own look, topics and team.") {
            if list.isEmpty {
                VStack(spacing: 12) {
                    EmptyState(symbol: "bubble.left.and.bubble.right", title: "No chat widget yet", message: "Add a widget for each website where customers should be able to chat with you.", actionTitle: "Add a widget") {
                        addingWidget = true
                    }
                    Label("Topics let visitors pick what they need, and route to the right team.", systemImage: "bubble.left").font(ODFont.sans(12.5)).foregroundStyle(Theme.fg3)
                }
                .padding(.vertical, 18)
                .padding(.horizontal, 12)
            } else {
                ForEach(Array(list.enumerated()), id: \.element.id) { index, widget in
                    if index > 0 { AccountDivider() }
                    SettingsDetailLink(page: .brands, id: "widget/\(widget.id)") {
                        HStack(spacing: 12) {
                            Image(systemName: "bubble.left.fill")
                                .font(.system(size: 13))
                                .foregroundStyle(.white)
                                .frame(width: 30, height: 30)
                                .background(ChannelColor.color(widget.settings.color.isEmpty ? brand.color : widget.settings.color) ?? Theme.fg, in: Circle())
                            VStack(alignment: .leading, spacing: 2) {
                                Text(widget.name).font(ODFont.sans(15, weight: .medium)).foregroundStyle(Theme.fg).lineLimit(1)
                                HStack(spacing: 4) {
                                    Text(widget.settings.allowedOrigins.first ?? "Any site").font(ODFont.mono(11.5)).lineLimit(1)
                                    if !widget.settings.topics.isEmpty { Text("· \(ChannelFormat.plural(widget.settings.topics.count, "topic"))").font(ODFont.sans(12)) }
                                }
                                .foregroundStyle(Theme.fg3)
                                if let team = widget.teamID.flatMap({ env.directory.team($0) }) {
                                    Text(team.name).font(ODFont.sans(12)).foregroundStyle(Theme.fg3)
                                }
                            }
                            Spacer(minLength: 4)
                            TonePill(text: widget.statusText, tone: widget.statusTone)
                            Chevron()
                        }
                        .padding(.horizontal, 14)
                        .padding(.vertical, 11)
                        .contentShape(Rectangle())
                    }
                }
                AccountDivider(tiles: false)
                ChannelRowButton(title: "Add a widget") { addingWidget = true }
            }
        }
    }

    private func save() {
        guard let draft, let base, let brand else { return }
        var body: ChannelBody = [:]
        if draft.name != base.name { body["name"] = .string(draft.name.trimmingCharacters(in: .whitespaces)) }
        if draft.color != base.color { body["color"] = .string(draft.color) }
        if draft.locale != base.locale { body["locale"] = .string(draft.locale) }
        if draft.slug != base.slug { body["slug"] = .string(draft.slug) }
        if draft.helpEnabled != base.helpEnabled || draft.helpTitle != base.helpTitle {
            body["help_center"] = .object(["enabled": .bool(draft.helpEnabled), "title": .string(draft.helpTitle.trimmingCharacters(in: .whitespaces))])
        }
        guard !body.isEmpty else { return }
        saving = true
        Task {
            do {
                let saved = try await store.patchBrand(brand.id, body)
                self.draft = BrandDraft(saved)
                self.base = self.draft
                Haptics.success()
                env.toasts.show("Brand saved", kind: .success)
            } catch {
                Haptics.error()
                env.toasts.show("Couldn't save the brand. \(error.userMessage)", kind: .error)
            }
            saving = false
        }
    }
}

struct BrandDraft: Equatable {
    var name: String
    var color: String
    var locale: String
    var slug: String
    var helpEnabled: Bool
    var helpTitle: String

    init(_ brand: SettingsBrand) {
        name = brand.name
        color = brand.color
        locale = brand.locale
        slug = brand.slug
        helpEnabled = brand.helpEnabled
        helpTitle = brand.helpTitle
    }
}

// MARK: - Help center

private struct BrandHelpCenter: View {
    let store: BrandsStore
    let brand: SettingsBrand
    @Binding var draft: BrandDraft
    let base: BrandDraft
    let openWeb: (String) -> Void

    @Environment(AppEnvironment.self) private var env
    @Environment(\.openURL) private var openURL

    var body: some View {
        let slugBad = !ChannelValidate.slug(draft.slug)
        let onDomain = brand.domain?.status == "active"
        AccountSection(title: "Help center", footer: "\(brand.name)'s public knowledge base.") {
            AccountToggleRow(title: "Published", subtitle: draft.helpEnabled ? "Anyone with the link can read it." : "Off: the link shows nothing.", isOn: $draft.helpEnabled)
            AccountDivider(tiles: false)
            VStack(alignment: .leading, spacing: 14) {
                VStack(alignment: .leading, spacing: 6) {
                    ODTextField(label: "Title", text: Binding(get: { draft.helpTitle }, set: { draft.helpTitle = String($0.prefix(70)) }), prompt: "\(brand.name) help")
                    if draft.helpTitle.trimmingCharacters(in: .whitespaces).count > 60 { FormError(text: "Up to 60 characters") } else { ChannelHint("Shown at the top and in search results.") }
                }
                VStack(alignment: .leading, spacing: 6) {
                    ChannelCopyField(label: "Public link", text: BrandsStore.bare(brand.helpURL), copiedMessage: "Link copied")
                    ChannelHint(!brand.helpEnabled ? "Live once the help center is published." : onDomain ? "On your custom domain. The default address redirects here." : "Share it, or link it from your site and widget.")
                    if brand.helpEnabled, let url = URL(string: brand.helpURL) {
                        Button { openURL(url) } label: { Label("Open the help center", systemImage: "arrow.up.right.square") }
                            .buttonStyle(OutlineButtonStyle(fullWidth: false, height: 34))
                    }
                }
                VStack(alignment: .leading, spacing: 6) {
                    ODTextField(label: "Default address", text: Binding(get: { draft.slug }, set: { draft.slug = String($0.lowercased().prefix(40)) }), prompt: brand.slug)
                    Text(BrandsStore.slugIsSubdomain(brand) ? "\(draft.slug).\(BrandsStore.bare(brand.helpDefaultURL).split(separator: ".", maxSplits: 1).last ?? "")" : "/help/\(draft.slug)")
                        .font(ODFont.mono(12)).foregroundStyle(Theme.fg3)
                    if slugBad {
                        FormError(text: "Use 3 to 40 lowercase letters, numbers and dashes.")
                    } else if draft.slug != base.slug {
                        Notice(
                            text: onDomain
                                ? "The default address moves to \(BrandsStore.withSlug(brand, draft.slug)), and links to \(BrandsStore.bare(brand.helpDefaultURL)) stop working. \(brand.domain?.hostname ?? "") stays as it is."
                                : "Links to \(BrandsStore.bare(brand.helpDefaultURL)) stop working. The help center moves to \(BrandsStore.withSlug(brand, draft.slug)).",
                            tone: .amber,
                            symbol: "exclamationmark.triangle"
                        )
                    } else {
                        ChannelHint("Names this brand in the address every help center gets, even with a custom domain.")
                    }
                }
            }
            .padding(14)
        }

        BrandHelpDomainSection(store: store, brand: brand)

        AccountSection(title: "Look", footer: "Pictures for the help center only. Empty uses the brand logo.") {
            ChannelWebRow(symbol: "photo.on.rectangle", title: "Logo and favicon", subtitle: "Upload them in the web app.", path: "/settings/brands/\(brand.id)?tab=help") {}
            AccountDivider(tiles: false)
            Button {
                env.router.show(.kb, [])
            } label: {
                AccountRow(symbol: "book", title: "Articles", subtitle: "Write and publish articles in the knowledge base.") { Chevron() }
            }
            .buttonStyle(RowPressStyle())
        }
        if let updated = brand.updatedAt {
            Text("Updated \(Format.relLong(updated))").font(ODFont.sans(12)).foregroundStyle(Theme.fg4).padding(.horizontal, 4)
        }
    }
}

/// The help center's custom domain: connect one, its DNS records and status, check and remove.
private struct BrandHelpDomainSection: View {
    let store: BrandsStore
    let brand: SettingsBrand

    @Environment(AppEnvironment.self) private var env
    @State private var hostname = ""
    @State private var error: String?
    @State private var busy = false
    @State private var confirm: AccountConfirm?

    var body: some View {
        AccountSection(title: "Custom domain", footer: "Serve the help center from an address of your own, like help.acme.com.") {
            if let domain = brand.domain {
                card(domain)
            } else {
                VStack(alignment: .leading, spacing: 10) {
                    ODTextField(label: "Domain", text: $hostname, prompt: "help.acme.com", keyboard: .URL, submitLabel: .go, onSubmit: connect)
                    ChannelHint("A subdomain you control. You add two DNS records, and HTTPS is set up for you.")
                    FormError(text: error)
                    Button(action: connect) { ButtonLabel(title: busy ? "Checking" : "Connect", busy: busy) }
                        .buttonStyle(InkButtonStyle())
                        .disabled(busy || hostname.trimmingCharacters(in: .whitespaces).isEmpty)
                    if busy { ChannelHint("Looking up its DNS records. This takes a few seconds.") }
                }
                .padding(14)
            }
        }
        .accountConfirm($confirm)
    }

    @ViewBuilder
    private func card(_ domain: BrandHelpDomain) -> some View {
        let target = domain.records.first { $0.type == "CNAME" }?.value ?? ""
        let fallback = BrandsStore.bare(brand.helpDefaultURL)
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 10) {
                AccountIconTile(symbol: "globe", tone: domain.status == "active" ? .green : domain.status == "failed" ? .red : .gray)
                VStack(alignment: .leading, spacing: 2) {
                    Text(domain.hostname).font(ODFont.mono(13.5, weight: .medium)).foregroundStyle(Theme.fg).lineLimit(1)
                    Text(domain.checkedAt.map { "Checked \(Format.relLong($0))" } ?? "Not checked yet")
                        .font(ODFont.sans(12)).foregroundStyle(Theme.fg3)
                }
                Spacer(minLength: 4)
                let view = status(domain)
                TonePill(text: view.0, tone: view.1)
            }
            if domain.status == "active", domain.problem.isEmpty {
                Text("Your help center now lives at https://\(domain.hostname). The default address, \(fallback), redirects there.").font(ODFont.footnote).foregroundStyle(Theme.fg2)
            } else if domain.status == "active" {
                Text("The help center still answers here, but fix this soon: \(problem(domain, target: target))").font(ODFont.footnote).foregroundStyle(Tone.amber.text)
            } else if domain.status == "failed" {
                Text("Checks stopped after 72 hours without finding the records. Fix them at your DNS provider, then check again. \(problem(domain, target: target))").font(ODFont.footnote).foregroundStyle(Tone.red.text)
            } else if !domain.problem.isEmpty {
                Text(problem(domain, target: target)).font(ODFont.footnote).foregroundStyle(Theme.fg2)
            }
            if !brand.helpEnabled {
                Notice(text: "The help center is not published, so this domain shows nothing until you turn it on.", tone: .amber)
            }
            if domain.status != "active" || domain.records.contains(where: { !$0.ok }) {
                SectionLabel("DNS records")
                ForEach(Array(domain.records.enumerated()), id: \.offset) { _, record in
                    VStack(alignment: .leading, spacing: 6) {
                        HStack {
                            Text(record.type).font(ODFont.mono(12, weight: .semibold)).foregroundStyle(Theme.fg)
                            Spacer()
                            TonePill(text: record.ok ? "Found" : "Not found", tone: record.ok ? .green : .gray)
                        }
                        ChannelCopyField(label: "Name", text: record.name, copiedMessage: "Name copied")
                        ChannelCopyField(label: "Value", text: record.value, copiedMessage: "Value copied")
                    }
                    .padding(10)
                    .background(Theme.sunken, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
                }
                let zone = domain.hostname.split(separator: ".").suffix(2).joined(separator: ".")
                ChannelHint("Add these at the company where you manage \(zone)'s DNS. DNS changes can take a few minutes, sometimes a few hours. Checks keep running, so you can leave this page.")
            }
            HStack(spacing: 8) {
                Button { check(domain) } label: { ButtonLabel(title: "Check now", busy: busy) }
                    .buttonStyle(OutlineButtonStyle(fullWidth: false, height: 34))
                    .disabled(busy)
                Spacer()
                Button("Remove domain") {
                    confirm = AccountConfirm(title: "Remove \(domain.hostname)?", message: "The help center stops answering on \(domain.hostname) and is only at \(fallback) again. You can delete the DNS records afterwards.", confirmTitle: "Remove domain") {
                        do {
                            try await store.removeDomain(brand.id)
                            env.toasts.show("\(domain.hostname) removed", kind: .success)
                        } catch { env.toasts.error(error) }
                    }
                }
                .buttonStyle(ChipButtonStyle())
            }
        }
        .padding(14)
    }

    private func status(_ domain: BrandHelpDomain) -> (String, Tone) {
        if domain.status == "failed" { return ("Failed", .red) }
        if domain.status == "active", !domain.problem.isEmpty { return ("Needs attention", .amber) }
        if domain.status == "active" { return ("Active", .green) }
        if domain.problem == "certificate_pending" { return ("Setting up HTTPS", .amber) }
        return ("Waiting for DNS", .amber)
    }

    private func problem(_ domain: BrandHelpDomain, target: String) -> String {
        switch domain.problem {
        case "txt_missing": "The TXT record isn't there yet. Add it at your DNS provider: it proves the domain is yours."
        case "cname_missing": "The CNAME record isn't there yet, or it points somewhere else. Point \(domain.hostname) to \(target)."
        case "domain_taken": "Another help center already uses this domain. Remove it there, then check again."
        case "certificate_pending": "Both records are in place. The HTTPS certificate is being issued, which usually takes a few minutes."
        case "certificate_failed": "The HTTPS certificate couldn't be issued. Check that the CNAME record points to \(target) and isn't proxied, then check again."
        case "provider_error": "Setting up the domain hit a problem on our side. It is tried again on its own, or check again in a few minutes."
        default: ""
        }
    }

    private func connect() {
        var host = hostname.trimmingCharacters(in: .whitespaces).lowercased()
        if let range = host.range(of: "://") { host = String(host[range.upperBound...]) }
        host = String(host.split(whereSeparator: { "/?#".contains($0) }).first ?? "")
        if let colon = host.firstIndex(of: ":") { host = String(host[..<colon]) }
        while host.hasSuffix(".") { host.removeLast() }
        guard host.count <= 253, host.range(of: #"^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]([a-z0-9-]{0,61}[a-z0-9])?$"#, options: .regularExpression) != nil else {
            error = "Enter a domain name like help.acme.com, without https:// or a path."
            return
        }
        hostname = host
        error = nil
        busy = true
        Task {
            do {
                _ = try await store.setDomain(brand.id, hostname: host)
                Haptics.success()
            } catch let failure as APIError where failure.code == "domain_taken" {
                error = "Another help center already uses this domain. Remove it there first."
            } catch let failure as APIError where failure.code == "rate_limited" || failure.status == 429 {
                error = "That was a lot of checks in a short time. Try again in a minute."
            } catch {
                self.error = (error as? APIError)?.detail ?? error.userMessage
            }
            busy = false
        }
    }

    private func check(_ domain: BrandHelpDomain) {
        busy = true
        Task {
            do {
                let next = try await store.checkDomain(brand.id).domain
                if next?.status == "active", domain.status != "active" {
                    env.toasts.show("\(domain.hostname) is live", kind: .success)
                } else if let next, next.status == domain.status, next.problem == domain.problem {
                    env.toasts.show("Checked. Nothing changed yet.")
                }
            } catch let failure as APIError where failure.status == 429 {
                env.toasts.show("That was a lot of checks in a short time. Try again in a minute.")
            } catch { env.toasts.error(error) }
            busy = false
        }
    }
}

// MARK: - New widget

/// "Add a widget": a name and a team; then its secret once, or how to reveal one.
struct WidgetNewSheet: View {
    let store: BrandsStore
    let brand: SettingsBrand

    @Environment(AppEnvironment.self) private var env
    @Environment(\.dismiss) private var dismiss
    @State private var name = ""
    @State private var teamID: String?
    @State private var saving = false
    @State private var error: String?
    @State private var created: ChatWidget?

    var body: some View {
        if let created {
            ChannelSheet(title: "\(created.name) is ready") {
                if created.secretAvailable, let secret = created.secret, !secret.isEmpty {
                    Text("This is its identity verification secret. Copy it now: it is shown once.").font(ODFont.callout).foregroundStyle(Theme.fg2)
                    ChannelCopyField(text: secret, copiedMessage: "Secret copied")
                    Notice(text: "Treat it like a password. Anyone with it can impersonate your users in chat.", tone: .amber)
                    Button("I've saved it") { open(created) }.buttonStyle(InkButtonStyle())
                } else {
                    Notice(text: "An owner can reveal the secret after confirming their sign-in. Rotating the secret shows a new one. Until then, the widget works but cannot verify signed-in users.", tone: .amber, symbol: "key")
                    Button("Open the widget") { open(created) }.buttonStyle(InkButtonStyle())
                }
            }
            .interactiveDismissDisabled()
        } else {
            ChannelSheet(title: "Add a widget", subtitle: "One widget per website. It uses \(brand.name)'s colour and logo until you change them.") {
                VStack(alignment: .leading, spacing: 6) {
                    ODTextField(label: "Name", text: Binding(get: { name }, set: { name = String($0.prefix(60)) }), prompt: "Marketing site")
                    ChannelHint("Usually the website, like Marketing site.")
                }
                ChannelTeamMenu(label: "Team", teamID: $teamID, none: "No team", hint: "New conversations go to this team, unless a topic or rule says otherwise.")
                FormError(text: error)
                Button(action: create) { ButtonLabel(title: "Create widget", busy: saving) }
                    .buttonStyle(InkButtonStyle())
                    .disabled(saving)
            }
        }
    }

    private func create() {
        guard !name.trimmingCharacters(in: .whitespaces).isEmpty else { error = "Give the widget a name."; return }
        saving = true
        error = nil
        Task {
            do {
                created = try await store.createWidget(["brand_id": .string(brand.id), "name": .string(name.trimmingCharacters(in: .whitespaces)), "team_id": .chString(teamID)])
                Haptics.success()
            } catch {
                Haptics.error()
                self.error = (error as? APIError)?.detail ?? error.userMessage
            }
            saving = false
        }
    }

    private func open(_ widget: ChatWidget) {
        dismiss()
        env.router.openSetting(.brands, id: "widget/\(widget.id)")
    }
}
