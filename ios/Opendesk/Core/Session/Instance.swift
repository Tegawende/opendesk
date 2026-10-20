import Foundation

/// One opendesk server: Cloud or a self-hosted instance, chosen at runtime
/// (contracts/native.md "Finding an instance").
struct Instance: Codable, Hashable, Sendable {
    /// Where the web app lives (`https://app.opendesk.sh`, or the self-host's one domain).
    var appOrigin: URL
    var apiURL: URL
    var gatewayURL: URL?
    var selfHosted: Bool

    static let cloudAppOrigin = URL(string: "https://app.opendesk.sh")!

    static let cloud = Instance(
        appOrigin: cloudAppOrigin,
        apiURL: URL(string: "https://api.opendesk.sh")!,
        gatewayURL: URL(string: "wss://gateway.opendesk.sh/socket"),
        selfHosted: false
    )

    #if DEBUG
    /// `make api`, `make realtime` and `make web` on this Mac (the simulator shares its localhost).
    static let localDev = Instance(
        appOrigin: URL(string: "http://localhost:5173")!,
        apiURL: URL(string: "http://localhost:8080")!,
        gatewayURL: URL(string: "ws://localhost:4000/socket"),
        selfHosted: true
    )
    #endif

    var isCloud: Bool { appOrigin.host() == Self.cloudAppOrigin.host() && !selfHosted }

    /// Passkeys need the app associated with the RP host: only Cloud's.
    var passkeysSupported: Bool { isCloud }

    /// The address as people type it.
    var displayHost: String {
        if isCloud { return "opendesk.sh" }
        var host = appOrigin.host() ?? appOrigin.absoluteString
        if let port = appOrigin.port { host += ":\(port)" }
        return host
    }

    /// The name the Keychain and preferences use for this server.
    var storageKey: String { apiURL.absoluteString.lowercased() }

    /// The gateway the web app would use: the configured one, or `/socket` on the app origin.
    var resolvedGatewayURL: URL {
        if let gatewayURL { return gatewayURL }
        var components = URLComponents(url: appOrigin, resolvingAgainstBaseURL: false) ?? URLComponents()
        components.scheme = appOrigin.scheme == "http" ? "ws" : "wss"
        components.path = "/socket"
        return components.url ?? appOrigin
    }

    /// An app path (`/inbox/mine/<id>`) on the web app.
    func webURL(path: String) -> URL? {
        URL(string: path, relativeTo: appOrigin)?.absoluteURL
    }
}

enum InstanceError: LocalizedError, Equatable {
    case empty
    case invalid
    case insecure
    case notFound
    case unreachable(String)

    var errorDescription: String? {
        switch self {
        case .empty: "Enter your opendesk address."
        case .invalid: "That doesn't look like a web address."
        case .insecure: "Use an https address."
        case .notFound: "No opendesk server answered at that address."
        case let .unreachable(detail): "Couldn't reach that server. \(detail)"
        }
    }
}

/// Turns what someone typed into an instance, by reading the runtime config
/// the web app already uses, then checking the API answers.
struct InstanceResolver: Sendable {
    var session: URLSession = .shared

    /// `opendesk.sh`, `help.acme.com`, `https://desk.acme.com/inbox` → an origin.
    static func normalize(_ raw: String) throws -> URL {
        var text = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty else { throw InstanceError.empty }
        let lowered = text.lowercased()
        let hadScheme = lowered.hasPrefix("http://") || lowered.hasPrefix("https://")
        if !hadScheme {
            if lowered.contains("://") { throw InstanceError.invalid }
            text = "https://" + text
        }
        guard var components = URLComponents(string: text), let host = components.host?.lowercased(), !host.isEmpty,
              host.contains(".") || host == "localhost"
        else { throw InstanceError.invalid }

        if ["opendesk.sh", "www.opendesk.sh", "app.opendesk.sh"].contains(host) {
            return Instance.cloudAppOrigin
        }

        components.host = host
        components.path = ""
        components.query = nil
        components.fragment = nil
        components.user = nil
        components.password = nil

        let local = isLocalOrPrivate(host)
        if components.scheme == "http" || (!hadScheme && local) {
            #if DEBUG
            guard local else { throw InstanceError.insecure }
            // Typed without a scheme, a dev machine is plain http; an explicit https:// (the self-host Caddy) stays.
            components.scheme = "http"
            #else
            throw InstanceError.insecure
            #endif
        }
        guard let url = components.url else { throw InstanceError.invalid }
        return url
    }

    /// localhost, `.local`, and private, loopback and link-local IPv4.
    static func isLocalOrPrivate(_ host: String) -> Bool {
        if host == "localhost" || host.hasSuffix(".localhost") || host.hasSuffix(".local") || host == "::1" || host == "[::1]" {
            return true
        }
        let parts = host.split(separator: ".").compactMap { Int($0) }
        guard parts.count == 4 else { return false }
        switch (parts[0], parts[1]) {
        case (10, _), (127, _), (192, 168), (169, 254): return true
        case (172, let b) where (16 ... 31).contains(b): return true
        default: return false
        }
    }

    /// Reads `/config.json`, falling back to the single-domain self-host shape
    /// (API on the same origin, gateway at `/socket`). Returns the instance and its sign-in options.
    func resolve(_ raw: String) async throws -> (Instance, AuthConfig) {
        let origin = try Self.normalize(raw)
        #if DEBUG
        // Any address is the demo server: nothing leaves the device.
        if DemoMode.isOn { return (DemoMode.instance, DemoMode.authConfig) }
        #endif
        if origin == Instance.cloudAppOrigin {
            return (Instance.cloud, try await authConfig(for: Instance.cloud.apiURL))
        }
        var candidate = Instance(appOrigin: origin, apiURL: origin, gatewayURL: nil, selfHosted: true)
        if let runtime = try? await runtimeConfig(at: origin) {
            if let api = URL(string: runtime.apiUrl, relativeTo: origin)?.absoluteURL, !runtime.apiUrl.isEmpty {
                candidate.apiURL = Self.trimmed(api)
            }
            if let gateway = runtime.gatewayUrl, !gateway.isEmpty {
                candidate.gatewayURL = Self.gatewayURL(gateway, origin: origin)
            }
            candidate.selfHosted = runtime.selfHosted ?? true
            if candidate.appOrigin.host() == Instance.cloudAppOrigin.host() { candidate.selfHosted = false }
        }
        // The refresh token and the socket travel to these, so config.json can't point
        // them at plain http or ws.
        guard Self.isSecure(candidate.apiURL), candidate.gatewayURL.map(Self.isSecure) ?? true else {
            throw InstanceError.insecure
        }
        let auth = try await authConfig(for: candidate.apiURL)
        return (candidate, auth)
    }

    func runtimeConfig(at origin: URL) async throws -> RuntimeConfig {
        #if DEBUG
        if DemoMode.isOn { return RuntimeConfig(apiUrl: DemoMode.instance.apiURL.absoluteString, gatewayUrl: nil, selfHosted: false, turnstileSiteKey: nil) }
        #endif
        var request = URLRequest(url: origin.appending(path: "config.json"))
        request.timeoutInterval = 10
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse, http.statusCode == 200 else { throw InstanceError.notFound }
        return try JSONDecoder().decode(RuntimeConfig.self, from: data)
    }

    func authConfig(for apiURL: URL) async throws -> AuthConfig {
        #if DEBUG
        if DemoMode.isOn { return DemoMode.authConfig }
        #endif
        var request = URLRequest(url: apiURL.appending(path: "v1/auth/config"))
        request.timeoutInterval = 12
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        request.setValue(AppInfo.clientHeader, forHTTPHeaderField: "OD-Client")
        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await session.data(for: request)
        } catch let error as URLError {
            throw InstanceError.unreachable(error.localizedDescription)
        }
        guard let http = response as? HTTPURLResponse, http.statusCode == 200,
              let config = try? APICoding.decoder.decode(AuthConfig.self, from: data)
        else { throw InstanceError.notFound }
        return config
    }

    /// `wss://gateway.opendesk.sh/socket`, or a path on the origin (`/socket`).
    static func gatewayURL(_ raw: String, origin: URL) -> URL? {
        if raw.hasPrefix("ws://") || raw.hasPrefix("wss://") { return URL(string: raw) }
        guard let resolved = URL(string: raw, relativeTo: origin)?.absoluteURL,
              var components = URLComponents(url: resolved, resolvingAgainstBaseURL: false)
        else { return nil }
        components.scheme = components.scheme == "http" ? "ws" : "wss"
        return components.url
    }

    /// https or wss; a debug build also takes http and ws on a local or private host.
    static func isSecure(_ url: URL) -> Bool {
        switch url.scheme?.lowercased() {
        case "https", "wss": return true
        case "http", "ws":
            #if DEBUG
            return isLocalOrPrivate(url.host()?.lowercased() ?? "")
            #else
            return false
            #endif
        default: return false
        }
    }

    /// Drops a trailing slash so `appending(path:)` builds clean URLs.
    static func trimmed(_ url: URL) -> URL {
        var text = url.absoluteString
        while text.hasSuffix("/") { text.removeLast() }
        return URL(string: text) ?? url
    }
}
