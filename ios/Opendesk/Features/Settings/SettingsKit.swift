import SwiftUI

// Pieces the workspace settings pages share, on top of the Account kit (AccountPage,
// AccountSection, AccountRow, AccountToggleRow, AccountSheetHeader, AccountConfirm).

/// A row on the Account tab's home that opens a settings page, or for a page that stays on
/// the web, that page in the in-app browser.
struct SettingsLinkRow: View {
    let page: SettingsPage
    var value: String?
    var subtitle: String?

    @Environment(AppEnvironment.self) private var env
    @State private var web: AccountWebPage?

    var body: some View {
        if page.webOnly {
            Button {
                Haptics.tap()
                web = env.session.instance.webURL(path: page.webPath).map(AccountWebPage.init)
            } label: {
                AccountRow(symbol: page.symbol, tone: page.tone, title: page.title, subtitle: subtitle) {
                    Image(systemName: "arrow.up.right")
                        .font(.system(size: 12, weight: .semibold))
                        .foregroundStyle(Theme.fg4)
                }
            }
            .buttonStyle(RowPressStyle())
            .accountWebSheet($web)
        } else {
            AccountLink(route: .setting(page)) {
                AccountRow(symbol: page.symbol, tone: page.tone, title: page.title, subtitle: subtitle) {
                    AccountDisclosure(value: value)
                }
            }
        }
    }
}

/// Opens something a settings page lists (a team, a mailbox, a webhook) on the stack above it.
/// `id` names it to the page's own `Detail`; a nested one joins ids with "/" ("brand/widget").
struct SettingsDetailLink<Label: View>: View {
    let page: SettingsPage
    let id: String
    @ViewBuilder var label: () -> Label

    @Environment(AppEnvironment.self) private var env

    var body: some View {
        Button {
            env.router.accountPath.append(.detail(page, id))
        } label: {
            label()
        }
        .buttonStyle(RowPressStyle())
    }
}

extension Router {
    /// Opens a settings page's detail from code, after creating one.
    func openSetting(_ page: SettingsPage, id: String) {
        accountPath.append(.detail(page, id))
    }
}

/// Unsaved changes on a settings page, in the tab bar's place and shape (`settingsSaveBar`): a wide
/// Save and a round glass Discard beside it, where Ollie's button sits. What stops it saving
/// floats above them.
struct SettingsSaveBar: View {
    var saveTitle = "Save changes"
    /// Why it can't save yet.
    var problem: String?
    let saving: Bool
    var canSave = true
    let save: () -> Void
    let discard: () -> Void

    var body: some View {
        VStack(spacing: 10) {
            if let problem {
                Label(problem, systemImage: "exclamationmark.circle.fill")
                    .font(ODFont.sans(13, weight: .medium))
                    .foregroundStyle(Tone.red.text)
                    .padding(.horizontal, 14)
                    .padding(.vertical, 8)
                    .glassEffect(.regular.tint(Theme.glassTint), in: Capsule())
                    .transition(.move(edge: .bottom).combined(with: .opacity))
            }
            GlassEffectContainer(spacing: 14) {
                HStack(spacing: 10) {
                    Button(action: save) {
                        ButtonLabel(title: saveTitle, busy: saving)
                            .font(ODFont.sans(16, weight: .semibold))
                            .foregroundStyle(Theme.inverseFg)
                            .frame(maxWidth: .infinity)
                            .frame(height: 56)
                            .background(Theme.inverse.opacity(canSave ? 1 : 0.35), in: Capsule())
                            .contentShape(Capsule())
                    }
                    .buttonStyle(TapScaleStyle(scale: 0.97))
                    .disabled(!canSave || saving)

                    Button(action: discard) {
                        Image(systemName: "arrow.uturn.backward")
                            .font(.system(size: 18, weight: .medium))
                            .foregroundStyle(Theme.fg)
                            .frame(width: 56, height: 56)
                            .contentShape(Circle())
                    }
                    .buttonStyle(TapScaleStyle(scale: 0.9))
                    .glassEffect(.regular.tint(Theme.glassTint).interactive(), in: Circle())
                    .disabled(saving)
                    .accessibilityLabel("Discard changes")
                }
            }
        }
        .motion(Motion.snappy, value: problem)
    }
}

/// Under the save bar the page blurs and fades into the chrome, from clear at the top to most of
/// the way at the buttons, so the bar never sits on a hard line.
private struct SettingsSaveBarFade: View {
    var body: some View {
        ZStack {
            Rectangle()
                .fill(.ultraThinMaterial)
                .mask(LinearGradient(stops: [.init(color: .clear, location: 0), .init(color: .black, location: 0.45)], startPoint: .top, endPoint: .bottom))
            LinearGradient(stops: [.init(color: Theme.chrome.opacity(0), location: 0), .init(color: Theme.chrome.opacity(0.85), location: 0.6)], startPoint: .top, endPoint: .bottom)
        }
        .ignoresSafeArea(edges: .bottom)
        .allowsHitTesting(false)
    }
}

private struct SettingsSaveBarModifier: ViewModifier {
    let dirty: Bool
    let guarded: Bool
    let bar: SettingsSaveBar

    @Environment(\.dismiss) private var dismiss
    @Environment(AppEnvironment.self) private var env
    @State private var confirm: AccountConfirm?
    @State private var leaving = false

    func body(content: Content) -> some View {
        content
            .safeAreaInset(edge: .bottom, spacing: 0) {
                if dirty {
                    bar
                        .padding(.horizontal, 18)
                        .padding(.top, 30)
                        .padding(.bottom, 4)
                        .background { SettingsSaveBarFade() }
                        .transition(.move(edge: .bottom).combined(with: .opacity))
                }
            }
            .motion(Motion.snappy, value: dirty)
            .tabBarHidden(dirty)
            .navigationBarBackButtonHidden(guarded)
            .toolbar {
                if guarded {
                    ToolbarItem(placement: .topBarLeading) {
                        Button {
                            Haptics.tap()
                            askToLeave()
                        } label: {
                            Image(systemName: "chevron.left")
                        }
                        .accessibilityLabel("Back")
                    }
                }
            }
            .sheet(item: $confirm, onDismiss: {
                guard leaving else { return }
                leaving = false
                dismiss()
            }) { confirm in AccountConfirmSheet(confirm: confirm) }
            #if DEBUG
            // The QA harness's `/demo/settings-back`: the back button on a page with changes.
            .onChange(of: env.router.demoAction) { _, action in
                guard action == "settings-back", guarded else { return }
                env.router.demoAction = nil
                askToLeave()
            }
            #endif
    }

    private func askToLeave() {
        confirm = AccountConfirm(
            title: "Discard your changes?",
            message: "What you changed on this page isn't saved yet.",
            confirmTitle: "Discard"
        ) {
            bar.discard()
            leaving = true
        }
    }
}

extension View {
    /// While `dirty`, the save bar takes the tab bar's place under the page, and going back asks
    /// before throwing the changes away. `guarded` turns that question off for a page whose draft
    /// outlives it.
    func settingsSaveBar(
        dirty: Bool,
        guarded: Bool? = nil,
        saveTitle: String = "Save changes",
        problem: String? = nil,
        saving: Bool,
        canSave: Bool = true,
        save: @escaping () -> Void,
        discard: @escaping () -> Void
    ) -> some View {
        modifier(SettingsSaveBarModifier(
            dirty: dirty,
            guarded: guarded ?? dirty,
            bar: SettingsSaveBar(saveTitle: saveTitle, problem: problem, saving: saving, canSave: canSave, save: save, discard: discard)
        ))
    }
}

/// A note at the top of a page a member can read but only admins change.
struct SettingsReadOnlyNote: View {
    var text = "Only admins can change these settings."

    var body: some View {
        HStack(spacing: 10) {
            Image(systemName: "lock")
                .font(.system(size: 12, weight: .semibold))
                .foregroundStyle(Theme.fg3)
            Text(text)
                .font(ODFont.footnote)
                .foregroundStyle(Theme.fg2)
                .fixedSize(horizontal: false, vertical: true)
            Spacer(minLength: 0)
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 11)
        .background(Theme.active.opacity(0.6), in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
    }
}

/// A settings page that lives on the web app (`SettingsPage.webOnly`): what it is for, and
/// the web app's page in the in-app browser.
struct SettingsWebFallback: View {
    let page: SettingsPage
    var id: String?

    @Environment(AppEnvironment.self) private var env
    @State private var web: AccountWebPage?

    var body: some View {
        AccountPage(title: page.title) {
            EmptyState(
                symbol: page.symbol,
                title: "\(page.title) is in the web app",
                message: "It opens the web app here. The web app keeps its own sign-in and may ask you to sign in once.",
                actionTitle: "Open in the web app"
            ) { open() }
            .frame(maxWidth: .infinity)
            .padding(.top, 60)
        }
        .accountWebSheet($web)
    }

    private func open() {
        let path = id.map { "\(page.webPath)/\($0)" } ?? page.webPath
        web = env.session.instance.webURL(path: path).map(AccountWebPage.init)
    }
}
