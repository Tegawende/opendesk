import Foundation

/// Plain text and HTML conversions for the composer and previews. The app
/// writes plain text; the API stores sanitized HTML (bluemonday server-side).
enum HTMLText {
    static func escape(_ text: String) -> String {
        var out = ""
        out.reserveCapacity(text.count)
        for character in text {
            switch character {
            case "&": out += "&amp;"
            case "<": out += "&lt;"
            case ">": out += "&gt;"
            case "\"": out += "&quot;"
            case "'": out += "&#39;"
            default: out.append(character)
            }
        }
        return out
    }

    /// Paragraphs on blank lines, `<br>` on single line breaks, bare links made clickable.
    static func html(fromPlain text: String) -> String {
        let normalized = text.replacingOccurrences(of: "\r\n", with: "\n").trimmingCharacters(in: .whitespacesAndNewlines)
        guard !normalized.isEmpty else { return "" }
        let paragraphs = normalized.components(separatedBy: "\n\n").map { $0.trimmingCharacters(in: .newlines) }.filter { !$0.isEmpty }
        return paragraphs.map { paragraph in
            let lines = paragraph.components(separatedBy: "\n").map { linkify(escape($0)) }
            return "<p>" + lines.joined(separator: "<br>") + "</p>"
        }.joined()
    }

    private static func linkify(_ escaped: String) -> String {
        guard escaped.contains("http") else { return escaped }
        let pattern = #"(https?://[^\s<]+[^\s<.,;:!?)\]'"])"#
        guard let regex = try? NSRegularExpression(pattern: pattern) else { return escaped }
        let range = NSRange(escaped.startIndex..., in: escaped)
        return regex.stringByReplacingMatches(in: escaped, range: range, withTemplate: "<a href=\"$1\">$1</a>")
    }

    /// Readable text from stored HTML (saved replies and drafts into the composer, previews).
    static func plain(from html: String) -> String {
        var text = html
        let replacements: [(String, String)] = [
            (#"(?is)<(script|style|head)[^>]*>.*?</\1>"#, ""),
            (#"(?i)<br\s*/?>"#, "\n"),
            (#"(?i)</(p|div|h[1-6]|blockquote|pre|tr|table)>"#, "\n\n"),
            (#"(?i)<li[^>]*>"#, "• "),
            (#"(?i)</li>"#, "\n"),
            (#"<[^>]+>"#, ""),
        ]
        for (pattern, template) in replacements {
            text = text.replacingOccurrences(of: pattern, with: template, options: .regularExpression)
        }
        text = decodeEntities(text)
        text = text.replacingOccurrences(of: #"[ \t]+\n"#, with: "\n", options: .regularExpression)
        text = text.replacingOccurrences(of: #"\n{3,}"#, with: "\n\n", options: .regularExpression)
        return text.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    static func decodeEntities(_ text: String) -> String {
        guard text.contains("&") else { return text }
        var out = text
        let named: [String: String] = [
            "&nbsp;": " ", "&lt;": "<", "&gt;": ">", "&quot;": "\"", "&#39;": "'", "&apos;": "'",
            "&rsquo;": "\u{2019}", "&lsquo;": "\u{2018}", "&rdquo;": "\u{201D}", "&ldquo;": "\u{201C}",
            "&hellip;": "\u{2026}", "&ndash;": "\u{2013}", "&mdash;": "\u{2014}", "&copy;": "\u{00A9}",
        ]
        for (entity, value) in named { out = out.replacingOccurrences(of: entity, with: value) }
        if let regex = try? NSRegularExpression(pattern: "&#(x?)([0-9a-fA-F]+);") {
            let ns = out as NSString
            var result = ""
            var cursor = 0
            for match in regex.matches(in: out, range: NSRange(location: 0, length: ns.length)) {
                result += ns.substring(with: NSRange(location: cursor, length: match.range.location - cursor))
                let hex = ns.substring(with: match.range(at: 1)) == "x"
                let digits = ns.substring(with: match.range(at: 2))
                if let code = UInt32(digits, radix: hex ? 16 : 10), let scalar = Unicode.Scalar(code) {
                    result.unicodeScalars.append(scalar)
                } else {
                    result += ns.substring(with: match.range)
                }
                cursor = match.range.location + match.range.length
            }
            result += ns.substring(from: cursor)
            out = result
        }
        return out.replacingOccurrences(of: "&amp;", with: "&")
    }

    /// Whether stored HTML is more than paragraphs of text (tables, images, layout): show it in a web view.
    static func needsRichRendering(_ html: String) -> Bool {
        html.range(of: #"(?i)<(table|img|blockquote|pre|h[1-6]|ul|ol|hr|font|center)\b|style\s*="#, options: .regularExpression) != nil
    }

    /// A designed email (tables, painted backgrounds): it keeps the sender's look on a light pane, as on the web.
    static func isDesignedEmail(_ html: String) -> Bool {
        html.range(of: #"(?i)<table[\s>]|\sbgcolor=|background(-color|-image)?\s*:"#, options: .regularExpression) != nil
    }

    /// The CSP of a message document. Images and fonts from other sites are held back
    /// until the reader asks for them (remoteImages), as on the web; data: URIs and the
    /// base URL's own files (the API's attachments) always load.
    static func csp(remoteImages: Bool) -> String {
        let remote = remoteImages ? " https:" : ""
        return "default-src 'none'; img-src 'self' data:\(remote); style-src 'unsafe-inline'; font-src data:\(remote)"
    }

    /// A self-contained document for a message, for a web view with JavaScript off, with
    /// no referrer. Remote images load only with remoteImages.
    static func document(body: String, dark: Bool, textColor: String, linkColor: String, quoteColor: String, fontSize: Int = 15, remoteImages: Bool = false) -> String {
        if isDesignedEmail(body) { return designedDocument(body: body, inset: dark, fontSize: fontSize, remoteImages: remoteImages) }
        return """
        <!doctype html><html><head><meta charset="utf-8">
        <meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=1">
        <meta name="referrer" content="no-referrer">
        <meta http-equiv="Content-Security-Policy" content="\(csp(remoteImages: remoteImages))">
        <meta name="color-scheme" content="\(dark ? "dark" : "light")">
        <style>
        :root { color-scheme: \(dark ? "dark" : "light"); }
        html, body { margin: 0; padding: 0; background: transparent; }
        body { font: -apple-system-body; font-size: \(fontSize)px; line-height: 1.5; color: \(textColor); word-wrap: break-word; overflow-wrap: anywhere; -webkit-text-size-adjust: 100%; }
        a { color: \(linkColor); }
        img { max-width: 100% !important; height: auto !important; }
        table { max-width: 100% !important; }
        pre, code { white-space: pre-wrap; font-family: ui-monospace, monospace; font-size: 0.92em; }
        blockquote { margin: 0.5em 0; padding-left: 0.8em; border-left: 2px solid \(quoteColor); color: \(quoteColor); }
        p { margin: 0 0 0.75em; } p:last-child { margin-bottom: 0; }
        \(dark ? "body * { background-color: transparent !important; color: inherit !important; border-color: \(quoteColor) !important; } a { color: \(linkColor) !important; }" : "")
        </style></head><body>\(body)</body></html>
        """
    }

    /// A designed email in its own light document; on a dark screen the pane gets a margin.
    private static func designedDocument(body: String, inset: Bool, fontSize: Int, remoteImages: Bool) -> String {
        """
        <!doctype html><html><head><meta charset="utf-8">
        <meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=1">
        <meta name="referrer" content="no-referrer">
        <meta http-equiv="Content-Security-Policy" content="\(csp(remoteImages: remoteImages))">
        <meta name="color-scheme" content="light">
        <style>
        :root { color-scheme: light; }
        html, body { margin: 0; padding: 0; background: #fff; }
        body { padding: \(inset ? 12 : 0)px; border-radius: 8px; color: #1f2328; font: -apple-system-body; font-size: \(fontSize)px; line-height: 1.5; overflow-wrap: break-word; -webkit-text-size-adjust: 100%; }
        img { max-width: 100% !important; }
        img[width][height] { height: auto !important; }
        table { max-width: 100% !important; }
        </style></head><body>\(body)</body></html>
        """
    }
}
