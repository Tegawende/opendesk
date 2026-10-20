import SwiftUI

enum AuthRoute: Hashable {
    case register
    case forgot
    case mfa(token: String)
    case link(token: String, email: String)
}

/// Sign-in: the server, then every way the server offers (OAuth providers,
/// passkeys on Cloud, email and password), then 2FA or account linking.
struct AuthFlowView: View {
    @Environment(AppEnvironment.self) private var env
    @State private var path: [AuthRoute] = []
    @State private var showServer = false

    var body: some View {
        NavigationStack(path: $path) {
            SignInView(path: $path, showServer: $showServer)
                .navigationDestination(for: AuthRoute.self) { route in
                    switch route {
                    case .register: RegisterView(path: $path)
                    case .forgot: ForgotPasswordView()
                    case let .mfa(token): MFAView(token: token)
                    case let .link(token, email): LinkAccountView(token: token, email: email, path: $path)
                    }
                }
        }
        .tint(Theme.fg)
        .sheet(isPresented: $showServer) {
            ServerSheet()
                .presentationDetents([.large])
                .presentationCornerRadius(Theme.Radius.sheet)
        }
    }
}

/// The page frame every auth step shares: chrome behind, content centred and calm.
struct AuthPage<Content: View>: View {
    var title: String
    var subtitle: String?
    @ViewBuilder var content: () -> Content

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 22) {
                VStack(alignment: .leading, spacing: 8) {
                    Text(title)
                        .font(ODFont.sans(27, weight: .semibold))
                        .tracking(-0.7)
                        .foregroundStyle(Theme.fg)
                    if let subtitle {
                        Text(subtitle)
                            .font(ODFont.callout)
                            .foregroundStyle(Theme.fg3)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                }
                content()
            }
            .padding(.horizontal, 24)
            .padding(.top, 12)
            .padding(.bottom, 40)
            .frame(maxWidth: 480)
            .frame(maxWidth: .infinity)
        }
        .scrollDismissesKeyboard(.interactively)
        .background(Theme.surface.ignoresSafeArea())
    }
}

struct SignInView: View {
    @Environment(AppEnvironment.self) private var env
    @Binding var path: [AuthRoute]
    @Binding var showServer: Bool

    @State private var email = AppInfo.debugValue("OPENDESK_DEBUG_EMAIL") ?? ""
    @State private var password = AppInfo.debugValue("OPENDESK_DEBUG_PASSWORD") ?? ""
    @State private var busy: String?
    @State private var error: String?
    @State private var oauth = OAuthSignIn()
    @State private var passkey = PasskeySignIn()

    private var session: SessionStore { env.session }
    private var config: AuthConfig? { session.authConfig }

    var body: some View {
        AuthPage(title: "Sign in", subtitle: "Pick up where your team is.") {
            serverChip

            if let config {
                if !config.providers.isEmpty || (config.passkeys && session.instance.passkeysSupported) {
                    VStack(spacing: 10) {
                        ForEach(config.providers) { provider in
                            Button {
                                Task { await oauthSignIn(provider) }
                            } label: {
                                HStack(spacing: 10) {
                                    ProviderGlyph(id: provider.id)
                                    Text("Continue with \(provider.name)")
                                }
                                .opacity(busy == provider.id ? 0 : 1)
                                .overlay { if busy == provider.id { ProgressView().controlSize(.small) } }
                            }
                            .buttonStyle(OutlineButtonStyle())
                            .disabled(busy != nil)
                        }
                        if config.passkeys && session.instance.passkeysSupported {
                            Button {
                                Task { await passkeySignIn() }
                            } label: {
                                HStack(spacing: 10) {
                                    Image(systemName: "person.badge.key")
                                    Text("Sign in with a passkey")
                                }
                                .opacity(busy == "passkey" ? 0 : 1)
                                .overlay { if busy == "passkey" { ProgressView().controlSize(.small) } }
                            }
                            .buttonStyle(OutlineButtonStyle())
                            .disabled(busy != nil)
                        }
                    }
                    if config.passwordLogin { OrDivider() }
                }

                if config.emailSignInAvailable {
                    emailForm(config)
                } else if config.passwordLogin {
                    Notice(
                        text: config.providers.isEmpty
                            ? "This server checks sign-ins with a captcha the app can't show. Sign in on the web, or ask your admin to turn on App Attest."
                            : "Email sign-in needs the web on this server. Use one of the options above.",
                        tone: .amber,
                        symbol: "exclamationmark.triangle"
                    )
                }

                FormError(text: error)

                if config.signupEnabled {
                    HStack(spacing: 4) {
                        Text("New here?").foregroundStyle(Theme.fg3)
                        Button("Create an account") { path.append(.register) }
                            .foregroundStyle(Theme.fg)
                            .fontWeight(.medium)
                    }
                    .font(ODFont.callout)
                    .frame(maxWidth: .infinity)
                    .padding(.top, 4)
                }
            } else if let failure = session.authConfigError {
                ErrorState(title: "Can't reach \(session.instance.displayHost)", message: failure) {
                    Task { await session.loadAuthConfig() }
                }
            } else {
                ProgressView().frame(maxWidth: .infinity).padding(.vertical, 40)
            }
        }
        .motion(Motion.fade, value: error)
        .task {
            if session.authConfig == nil { await session.loadAuthConfig() }
        }
    }

    private var serverChip: some View {
        Button {
            showServer = true
        } label: {
            HStack(spacing: 8) {
                ODLogoMark().fill(Theme.fg).frame(width: 16, height: 16)
                Text(session.instance.displayHost)
                    .font(ODFont.sans(14, weight: .medium))
                    .foregroundStyle(Theme.fg)
                Text(session.instance.isCloud ? "Cloud" : "Self-hosted")
                    .font(ODFont.sans(11.5, weight: .medium))
                    .foregroundStyle(Theme.fg3)
                    .padding(.horizontal, 6)
                    .frame(height: 18)
                    .background(Theme.sunken, in: RoundedRectangle(cornerRadius: 4))
                Image(systemName: "chevron.down").font(.system(size: 11, weight: .semibold)).foregroundStyle(Theme.fg3)
            }
            .padding(.horizontal, 12)
            .frame(height: 36)
            .background(Theme.surface, in: Capsule())
            .overlay(Capsule().strokeBorder(Theme.line, lineWidth: 1))
        }
        .buttonStyle(TapScaleStyle())
        .accessibilityLabel("Server: \(session.instance.displayHost). Change server")
    }

    @ViewBuilder
    private func emailForm(_ config: AuthConfig) -> some View {
        VStack(spacing: 14) {
            ODTextField(label: "Email", text: $email, prompt: "you@company.com", contentType: .username, keyboard: .emailAddress)
            ODTextField(label: "Password", text: $password, secure: true, contentType: .password, submitLabel: .go) {
                Task { await passwordSignIn() }
            }
            HStack {
                Spacer()
                Button("Forgot password?") { path.append(.forgot) }
                    .font(ODFont.sans(13.5, weight: .medium))
                    .foregroundStyle(Theme.fg3)
            }
            Button {
                Task { await passwordSignIn() }
            } label: {
                ButtonLabel(title: "Sign in", busy: busy == "password")
            }
            .buttonStyle(InkButtonStyle())
            .disabled(email.isEmpty || password.isEmpty || busy != nil)
        }
    }

    private func passwordSignIn() async {
        guard busy == nil else { return }
        busy = "password"
        error = nil
        defer { busy = nil }
        do {
            let outcome = try await session.signIn(email: email.trimmingCharacters(in: .whitespaces), password: password)
            route(outcome)
        } catch {
            Haptics.error()
            self.error = error.userMessage
        }
    }

    private func oauthSignIn(_ provider: AuthProvider) async {
        guard busy == nil else { return }
        busy = provider.id
        error = nil
        defer { busy = nil }
        let verifier = PKCE.verifier()
        guard let url = session.oauthStartURL(provider: provider.id, challenge: PKCE.challenge(for: verifier)) else { return }
        do {
            let code = try await oauth.run(startURL: url)
            let outcome = try await session.exchangeOAuth(code: code, verifier: verifier)
            route(outcome)
        } catch OAuthSignIn.Failure.cancelled {
            return
        } catch {
            Haptics.error()
            self.error = error.userMessage
        }
    }

    private func passkeySignIn() async {
        guard busy == nil else { return }
        busy = "passkey"
        error = nil
        defer { busy = nil }
        do {
            let begin = try await session.beginPasskey()
            let credential = try await passkey.assert(options: begin.options, fallbackRP: session.instance.appOrigin.host() ?? "app.opendesk.sh")
            let outcome = try await session.finishPasskey(token: begin.passkeyToken, credential: credential)
            route(outcome)
        } catch PasskeySignIn.Failure.cancelled {
            return
        } catch {
            Haptics.error()
            self.error = error.userMessage
        }
    }

    private func route(_ outcome: SignInOutcome) {
        switch outcome {
        case .signedIn: Haptics.success()
        case let .mfa(token): path.append(.mfa(token: token))
        case let .link(token, email): path.append(.link(token: token, email: email))
        }
    }
}

struct OrDivider: View {
    var body: some View {
        HStack(spacing: 12) {
            Rectangle().fill(Theme.line).frame(height: 1)
            Text("or").font(ODFont.footnote).foregroundStyle(Theme.fg4)
            Rectangle().fill(Theme.line).frame(height: 1)
        }
    }
}

/// A provider's mark: a symbol where the system has one, else its initial.
struct ProviderGlyph: View {
    let id: String

    var body: some View {
        switch id {
        case "github":
            Image(systemName: "chevron.left.forwardslash.chevron.right").font(.system(size: 13, weight: .semibold))
        case "oidc":
            Image(systemName: "building.2").font(.system(size: 14))
        case "microsoft":
            Grid(horizontalSpacing: 1.5, verticalSpacing: 1.5) {
                GridRow { Rectangle().fill(Color(hex: 0xF25022)); Rectangle().fill(Color(hex: 0x7FBA00)) }
                GridRow { Rectangle().fill(Color(hex: 0x00A4EF)); Rectangle().fill(Color(hex: 0xFFB900)) }
            }
            .frame(width: 14, height: 14)
        default:
            Text(id.prefix(1).uppercased())
                .font(ODFont.sans(14, weight: .bold))
                .frame(width: 18, height: 18)
        }
    }
}

// MARK: - Two-factor

struct MFAView: View {
    @Environment(AppEnvironment.self) private var env
    let token: String
    @State private var code = ""
    @State private var busy = false
    @State private var error: String?
    @State private var recovery = false

    var body: some View {
        AuthPage(
            title: "Two-factor code",
            subtitle: recovery ? "Enter one of the recovery codes you saved when you turned on 2FA." : "Enter the 6-digit code from your authenticator app."
        ) {
            ODTextField(
                label: recovery ? "Recovery code" : "Code",
                text: $code,
                prompt: recovery ? "xxxx-xxxx-xxxx-xxxx" : "123456",
                contentType: .oneTimeCode,
                keyboard: recovery ? .asciiCapable : .numberPad,
                submitLabel: .go
            ) { Task { await verify() } }
            .onChange(of: code) { _, value in
                if !recovery, value.filter(\.isNumber).count == 6, value.count == 6 { Task { await verify() } }
            }
            FormError(text: error)
            Button {
                Task { await verify() }
            } label: {
                ButtonLabel(title: "Verify", busy: busy)
            }
            .buttonStyle(InkButtonStyle())
            .disabled(code.isEmpty || busy)
            Button(recovery ? "Use an authenticator code" : "Use a recovery code") {
                recovery.toggle()
                code = ""
                error = nil
            }
            .font(ODFont.sans(14, weight: .medium))
            .foregroundStyle(Theme.fg3)
            .frame(maxWidth: .infinity)
        }
    }

    private func verify() async {
        guard !busy else { return }
        busy = true
        error = nil
        defer { busy = false }
        do {
            try await env.session.verifyMFA(token: token, code: code)
            Haptics.success()
        } catch {
            Haptics.error()
            self.error = error.userMessage
            code = ""
        }
    }
}

// MARK: - Link an existing account

struct LinkAccountView: View {
    @Environment(AppEnvironment.self) private var env
    let token: String
    let email: String
    @Binding var path: [AuthRoute]
    @State private var password = ""
    @State private var busy = false
    @State private var error: String?

    var body: some View {
        AuthPage(title: "Link your account", subtitle: "\(email) already has a password on this server. Enter it once to sign in this way from now on.") {
            ODTextField(label: "Password", text: $password, secure: true, contentType: .password, submitLabel: .go) {
                Task { await link() }
            }
            FormError(text: error)
            Button {
                Task { await link() }
            } label: {
                ButtonLabel(title: "Link and sign in", busy: busy)
            }
            .buttonStyle(InkButtonStyle())
            .disabled(password.isEmpty || busy)
        }
    }

    private func link() async {
        guard !busy else { return }
        busy = true
        error = nil
        defer { busy = false }
        do {
            let outcome = try await env.session.linkAccount(token: token, password: password)
            if case let .mfa(mfaToken) = outcome { path.append(.mfa(token: mfaToken)) }
        } catch {
            Haptics.error()
            self.error = error.userMessage
        }
    }
}

// MARK: - Password strength

/// A rough strength estimate for a new password (ASVS 2.1.8), shown once it is long enough:
/// how many kinds of characters it mixes and how long it is, less for repeats and for the
/// person's own name or email in it. The server's rules (length, breached list) still decide.
enum PasswordStrength: Int {
    case weak = 1, fair, good, strong

    init(_ password: String, userInputs: [String]) {
        let scalars = Array(password.unicodeScalars)
        var kinds = 0
        if scalars.contains(where: { CharacterSet.lowercaseLetters.contains($0) }) { kinds += 1 }
        if scalars.contains(where: { CharacterSet.uppercaseLetters.contains($0) }) { kinds += 1 }
        if scalars.contains(where: { CharacterSet.decimalDigits.contains($0) }) { kinds += 1 }
        if scalars.contains(where: { !CharacterSet.alphanumerics.contains($0) }) { kinds += 1 }
        let distinct = Set(scalars).count
        var score = 0
        if scalars.count >= 12 { score += 1 }
        if scalars.count >= 16 { score += 1 }
        if scalars.count >= 20 { score += 1 }
        if kinds >= 3 { score += 1 }
        if distinct < max(4, scalars.count / 3) { score -= 2 }
        let lower = password.lowercased()
        for input in userInputs {
            let parts = input.lowercased().split(whereSeparator: { " @.+-_".contains($0) })
            if parts.contains(where: { $0.count >= 3 && lower.contains($0) }) {
                score -= 1
                break
            }
        }
        self = PasswordStrength(rawValue: min(max(score, 1), 4)) ?? .weak
    }

    var label: String {
        switch self {
        case .weak: "Weak: add more words or characters."
        case .fair: "Fair: longer is stronger."
        case .good: "Good."
        case .strong: "Strong."
        }
    }

    var color: Color {
        switch self {
        case .weak: .red
        case .fair: .orange
        case .good, .strong: .green
        }
    }
}

struct PasswordStrengthMeter: View {
    let strength: PasswordStrength

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(spacing: 4) {
                ForEach(1...4, id: \.self) { i in
                    Capsule()
                        .fill(i <= strength.rawValue ? strength.color : Color.secondary.opacity(0.2))
                        .frame(height: 3)
                }
            }
            Text(strength.label).font(ODFont.footnote).foregroundStyle(Theme.fg3)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Password strength: \(strength.label)")
    }
}

// MARK: - Register

struct RegisterView: View {
    @Environment(AppEnvironment.self) private var env
    @Binding var path: [AuthRoute]
    @State private var name = ""
    @State private var email = ""
    @State private var password = ""
    @State private var busy = false
    @State private var error: String?

    private var passwordHint: String? {
        guard !password.isEmpty else { return "At least 12 characters." }
        // Counted in code points, as the server counts them.
        let length = password.unicodeScalars.count
        if length < 12 { return "\(12 - length) more characters." }
        if length > 128 { return "At most 128 characters." }
        return nil
    }

    var body: some View {
        AuthPage(title: "Create your account", subtitle: "On \(env.session.instance.displayHost).") {
            if env.session.authConfig?.emailSignInAvailable == false {
                Notice(text: "This server checks new accounts with a captcha the app can't show. Create your account on the web.", tone: .amber, symbol: "exclamationmark.triangle")
            }
            VStack(spacing: 14) {
                ODTextField(label: "Name", text: $name, prompt: "Ada Lovelace", contentType: .name)
                ODTextField(label: "Work email", text: $email, prompt: "you@company.com", contentType: .username, keyboard: .emailAddress)
                VStack(alignment: .leading, spacing: 6) {
                    ODTextField(label: "Password", text: $password, secure: true, contentType: .newPassword, submitLabel: .go) {
                        Task { await register() }
                    }
                    if let passwordHint {
                        Text(passwordHint).font(ODFont.footnote).foregroundStyle(Theme.fg3)
                    } else {
                        PasswordStrengthMeter(strength: PasswordStrength(password, userInputs: [name, email]))
                    }
                }
            }
            FormError(text: error)
            Button {
                Task { await register() }
            } label: {
                ButtonLabel(title: "Create account", busy: busy)
            }
            .buttonStyle(InkButtonStyle())
            .disabled(name.trimmingCharacters(in: .whitespaces).isEmpty || email.isEmpty || passwordHint != nil || busy)
        }
    }

    private func register() async {
        guard !busy, passwordHint == nil else { return }
        busy = true
        error = nil
        defer { busy = false }
        do {
            let outcome = try await env.session.register(name: name.trimmingCharacters(in: .whitespaces), email: email.trimmingCharacters(in: .whitespaces), password: password)
            if case let .mfa(token) = outcome { path.append(.mfa(token: token)) }
        } catch {
            Haptics.error()
            self.error = error.userMessage
        }
    }
}

// MARK: - Forgot password

struct ForgotPasswordView: View {
    @Environment(AppEnvironment.self) private var env
    @State private var email = ""
    @State private var busy = false
    @State private var sent = false
    @State private var error: String?

    var body: some View {
        AuthPage(
            title: sent ? "Check your email" : "Reset your password",
            subtitle: sent
                ? "If \(email) has an account, a reset link is on its way. Open it on this phone or any browser to choose a new password."
                : "We'll email you a link to choose a new one."
        ) {
            if !sent {
                ODTextField(label: "Email", text: $email, prompt: "you@company.com", contentType: .username, keyboard: .emailAddress, submitLabel: .send) {
                    Task { await send() }
                }
                FormError(text: error)
                Button {
                    Task { await send() }
                } label: {
                    ButtonLabel(title: "Send reset link", busy: busy)
                }
                .buttonStyle(InkButtonStyle())
                .disabled(email.isEmpty || busy)
            }
        }
        .motion(Motion.fade, value: sent)
    }

    private func send() async {
        guard !busy else { return }
        busy = true
        error = nil
        defer { busy = false }
        do {
            try await env.session.forgotPassword(email: email.trimmingCharacters(in: .whitespaces))
            sent = true
        } catch {
            self.error = error.userMessage
        }
    }
}
