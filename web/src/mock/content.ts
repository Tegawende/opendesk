// Written content for the mock seed: people, companies, ticket stories,
// generated ticket subjects and knowledge base articles.

export const TEAM = [
  { key: "maya", name: "Maya Chen", email: "maya@margin.app", role: "owner" },
  { key: "jonas", name: "Jonas Weber", email: "jonas@margin.app", role: "admin" },
  { key: "priya", name: "Priya Raman", email: "priya@margin.app", role: "agent" },
  { key: "sam", name: "Sam Okafor", email: "sam@margin.app", role: "agent" },
  { key: "lea", name: "Lea Martin", email: "lea@margin.app", role: "agent" },
  { key: "theo", name: "Theo Brandt", email: "theo@margin.app", role: "viewer" },
] as const;

export type TeamKey = (typeof TEAM)[number]["key"];

export const COMPANIES = [
  { name: "Northwind Traders", domain: "northwind.com", notes: "Enterprise plan, 240 seats. Renewal in March." },
  { name: "Halcyon Studio", domain: "halcyon.studio", notes: "Design agency. Heavy PDF export users." },
  { name: "Brightline Logistics", domain: "brightline.io", notes: "Annual billing via invoice." },
  { name: "Oakridge Health", domain: "oakridgehealth.org", notes: "SSO through Okta. HIPAA addendum signed." },
  { name: "Paperplane", domain: "paperplane.co", notes: "Startup plan. Very active on feature requests." },
  { name: "Vantage Legal", domain: "vantagelegal.com", notes: "Needs DPA and data residency in the EU." },
  { name: "Copperleaf", domain: "copperleaf.dev", notes: "Builds on our API and webhooks." },
  { name: "Meridian Schools", domain: "meridian.edu", notes: "Education discount. 3 campuses." },
  { name: "Fjord Analytics", domain: "fjord.ai", notes: "API heavy, syncs notes nightly." },
  { name: "Tidewater Media", domain: "tidewater.fm", notes: "Podcast network, 40 seats." },
  { name: "Kestrel Robotics", domain: "kestrel.tech", notes: "Mostly mobile users in the field." },
  { name: "Solstice Bakery", domain: "solsticebakery.com", notes: "Small team, monthly plan." },
] as const;

export const FIRST = [
  "Dana", "Marcus", "Elena", "Tomás", "Aisha", "Ben", "Chloé", "Daniel", "Freya", "Gabriel",
  "Hana", "Isaac", "Julia", "Kofi", "Laura", "Mateo", "Nina", "Oliver", "Paula", "Quentin",
  "Rosa", "Stefan", "Tara", "Umar", "Vera", "Will", "Ximena", "Yusuf", "Zoe", "Anders",
  "Bianca", "Carlos", "Delia", "Emil", "Farah", "Greta", "Hugo", "Ines", "Jakob", "Keiko",
  "Liam", "Mira", "Noah", "Olga", "Pablo",
] as const;

export const LAST = [
  "Whitfield", "Lindqvist", "Rossi", "García", "Bello", "Hartmann", "Dubois", "Kim", "Nilsen", "Moreau",
  "Sato", "Levy", "Novak", "Mensah", "Fischer", "Silva", "Petrova", "Grant", "Costa", "Laurent",
  "Ortega", "Bauer", "Singh", "Haddad", "Ivanova", "Turner", "Reyes", "Demir", "Park", "Holm",
  "Conti", "Mendes", "Fox", "Berg", "Rahman", "Vogel", "Martins", "Ferreira", "Klein", "Tanaka",
  "Murphy", "Shah", "Andersson", "Kowalski", "Navarro",
] as const;

export const TAGS = ["billing", "bug", "feature-request", "onboarding", "sso", "refund", "urgent-customer", "api", "export", "mobile"] as const;

export type StoryMsg = {
  from: "c" | TeamKey;
  note?: boolean;
  html: string;
  min: number; // minutes ago
  attach?: ("screenshot" | "screenshot2" | "pdf" | "log" | "voice")[];
  quoted?: boolean;
  delivery?: "failed" | "pending" | "bounced";
};

export type Story = {
  number: number;
  subject: string;
  contact: number; // index into contacts
  status: "open" | "pending" | "on_hold" | "solved" | "closed";
  priority: "low" | "normal" | "high" | "urgent";
  channel: "email" | "chat" | "api" | "portal";
  assignee: TeamKey | null;
  team: "Support" | "Billing" | null;
  tags: string[];
  mailbox: "support" | "billing";
  csat?: { score: number; comment: string };
  thread: StoryMsg[];
};

const mention = (key: string, name: string) => `<span class="mention" data-id="@@${key}">@${name}</span>`;

// A designed email as the server stores it: its cleaned style sheet first, then the body.
const receiptRow = (label: string, value: string, strong = false) =>
  `<tr><td style="padding:10px 0;font-size:14px;color:#414552;${strong ? "font-weight:600;border-top:1px solid #ebeef1" : ""}">${label}</td><td align="right" style="padding:10px 0;font-size:14px;color:#414552;${strong ? "font-weight:600;border-top:1px solid #ebeef1" : ""}">${value}</td></tr>`;
const RECEIPT =
  `<style>.od-wrap{width:100% !important}@media only screen and (max-width:620px){.od-card{width:100% !important}.od-pad{padding:24px 20px !important}}</style>` +
  `<p>Forwarding the receipt for the second charge.</p>` +
  `<div style="background-color:#f6f9fc;padding:32px 0;font-family:-apple-system, 'Segoe UI', Helvetica, Arial, sans-serif">` +
  `<table class="od-card" width="600" align="center" cellpadding="0" cellspacing="0" border="0" bgcolor="#ffffff" style="border-radius:12px;border:1px solid #ebeef1">` +
  `<tr><td align="center" style="background-color:#1f2937;border-radius:12px 12px 0 0;padding:28px 0"><span style="display:inline-block;width:44px;height:44px;line-height:44px;border-radius:22px;background-color:#ffffff;color:#1f2937;font-weight:700;font-size:20px">M</span></td></tr>` +
  `<tr><td class="od-pad" style="padding:32px 48px">` +
  `<p style="margin:0;font-size:13px;color:#687385">Receipt from Margin Labs GmbH</p>` +
  `<p style="margin:4px 0 24px;font-size:28px;font-weight:600;color:#1a1f36">€4,320.00</p>` +
  `<table width="100%" cellpadding="0" cellspacing="0" border="0"><tr>` +
  `<td valign="top" style="font-size:12px;color:#687385;text-transform:uppercase;letter-spacing:0.04em">Amount paid<br><span style="font-size:14px;color:#414552;text-transform:none;letter-spacing:0">€4,320.00</span></td>` +
  `<td valign="top" style="font-size:12px;color:#687385;text-transform:uppercase;letter-spacing:0.04em">Date paid<br><span style="font-size:14px;color:#414552;text-transform:none;letter-spacing:0">Oct 9, 2026, 08:14</span></td>` +
  `<td valign="top" style="font-size:12px;color:#687385;text-transform:uppercase;letter-spacing:0.04em">Payment method<br><span style="font-size:14px;color:#414552;text-transform:none;letter-spacing:0">Visa · 4417</span></td>` +
  `</tr></table>` +
  `<p style="margin:32px 0 8px;font-size:12px;color:#687385;text-transform:uppercase;letter-spacing:0.04em">Summary</p>` +
  `<table width="100%" cellpadding="0" cellspacing="0" border="0" style="background-color:#f6f8fa;border-radius:8px;padding:4px 16px">` +
  receiptRow("Margin Business, annual (12 seats)", "€4,320.00") +
  receiptRow("Amount paid", "€4,320.00", true) +
  `</table>` +
  `<p style="margin:32px 0 0;font-size:13px;line-height:20px;color:#687385">Questions? Reply to this email or visit <a href="https://margin.app/help" style="color:#635bff;text-decoration:none">margin.app/help</a>.</p>` +
  `</td></tr></table><img src="https://mail.margin.app/o/7f3a.gif" width="1" height="1" alt=""></div>`;

// A picture pasted into an email: an inline image, kept in the message as the API serves it.
const EXPECTED_PAGE =
  "data:image/svg+xml;utf8," +
  encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg" width="480" height="240" viewBox="0 0 480 240"><rect width="480" height="240" rx="10" fill="#f6f8fa"/><rect x="24" y="24" width="180" height="14" rx="7" fill="#1a1f36"/><rect x="24" y="52" width="300" height="8" rx="4" fill="#c9ced6"/><rect x="24" y="68" width="260" height="8" rx="4" fill="#c9ced6"/><rect x="24" y="96" width="200" height="120" rx="8" fill="#635bff" opacity="0.18"/><rect x="240" y="96" width="216" height="120" rx="8" fill="#20c997" opacity="0.2"/><text x="124" y="162" font-family="Helvetica, Arial" font-size="13" text-anchor="middle" fill="#3c3f8f">Chart</text><text x="348" y="162" font-family="Helvetica, Arial" font-size="13" text-anchor="middle" fill="#14735a">Photo</text></svg>`,
  );

export const STORIES: Story[] = [
  {
    number: 1072,
    subject: "SSO login loops back to sign-in page",
    contact: 0,
    status: "open",
    priority: "urgent",
    channel: "email",
    assignee: "maya",
    team: "Support",
    tags: ["sso", "urgent-customer"],
    mailbox: "support",
    thread: [
      {
        from: "c",
        min: 95,
        attach: ["screenshot"],
        html: `<p>Hi Margin team,</p><p>Since this morning nobody at Oakridge can sign in through Okta. After entering credentials we land on the Margin sign-in page again, and it loops. Clearing cookies doesn't help.</p><ul><li>Started around 07:40 CET</li><li>Affects all 180 users on the Okta tile</li><li>Password login for our admin account still works</li></ul><p>Screenshot of the network tab attached. This is blocking our clinical documentation team, so any help is appreciated.</p><p>Dana Whitfield<br>IT Operations, Oakridge Health</p>`,
      },
      {
        from: "maya",
        min: 82,
        html: `<p>Hi Dana,</p><p>Thanks for the detailed report and the screenshot, that helps a lot. I can see the SAML responses reaching us, but they're being rejected before a session is created.</p><p>Could you check whether the signing certificate in your Okta app was rotated recently? If it was, send me the new certificate fingerprint and I'll compare it with what we have on file.</p><p>Best,<br>Maya</p>`,
      },
      {
        from: "maya",
        note: true,
        min: 78,
        html: `<p>${mention("jonas", "Jonas Weber")} can you pull the SAML assertion logs for Oakridge from the last 3 hours? I suspect a certificate rotation on their side, but the error code is <code>invalid_signature</code> on every request.</p>`,
      },
      {
        from: "jonas",
        note: true,
        min: 61,
        html: `<p>Confirmed. Every assertion since 07:38 is signed with a cert we don't have (fingerprint starts with <code>4F:1A:9C</code>). The old one expires next week, so they rotated early. Once she confirms we can add the new cert and keep both active.</p>`,
      },
      {
        from: "c",
        min: 6,
        quoted: true,
        html: `<p>Hi Maya,</p><p>You were right, our identity team rotated the certificate last night without telling us. The new fingerprint is <code>4F:1A:9C:22:E0:7B:51:D3</code>.</p><p>Can you add it on your side? We'd rather not roll back.</p><p>Thanks,<br>Dana</p>`,
      },
    ],
  },
  {
    number: 1071,
    subject: "Can't export notebook to PDF",
    contact: 4,
    status: "open",
    priority: "high",
    channel: "email",
    assignee: null,
    team: "Support",
    tags: ["bug", "export"],
    mailbox: "support",
    thread: [
      {
        from: "c",
        min: 14,
        attach: ["pdf", "screenshot2"],
        html: `<p>Hello,</p><p>When I export our "Client brief" notebook to PDF the download starts, then fails with <strong>"Export failed, try again later"</strong>. Smaller notebooks export fine. This one has about 60 pages and a lot of images.</p><p>I attached what I get when I export a single page from it, the images are missing. This is how that page should look:</p><p><img src="${EXPECTED_PAGE}" alt="The page as it should look" width="480" height="240"></p><ul><li>Pages with only text: fine</li><li>Pages with charts or photos: <em>images missing</em></li></ul><p>We need this for a client presentation tomorrow morning.</p><p>Elena</p>`,
      },
      {
        from: "jonas",
        note: true,
        min: 9,
        html: `<p>Reproduced with a 60 page notebook: the image step times out after 30 s. Asked the export team to raise the limit for her workspace.</p>`,
      },
    ],
  },
  {
    number: 1070,
    subject: "Invoice shows wrong VAT number",
    contact: 2,
    status: "pending",
    priority: "normal",
    channel: "email",
    assignee: "lea",
    team: "Billing",
    tags: ["billing"],
    mailbox: "billing",
    thread: [
      {
        from: "c",
        min: 60 * 26,
        html: `<p>Hi,</p><p>Our October invoice (INV-2026-10-0412) lists our old VAT number DE298113420. We changed it in the billing settings two weeks ago. Our accounting can't book the invoice like this.</p><p>Could you reissue it?</p><p>Marcus Lindqvist<br>Finance, Northwind Traders</p>`,
      },
      {
        from: "lea",
        min: 60 * 25,
        html: `<p>Hi Marcus,</p><p>Sorry about that. The change was saved after the invoice was generated, so the October run still used the old number. I've voided INV-2026-10-0412 and issued INV-2026-10-0498 with DE311402967.</p><p>Can you confirm the new one works for your accounting team?</p><p>Lea</p>`,
      },
      {
        from: "lea",
        note: true,
        min: 60 * 25 - 5,
        html: `<p>Filed with billing engineering: VAT changes should regenerate open invoices. Linked in the billing queue.</p>`,
      },
    ],
  },
  {
    number: 1069,
    subject: "Feature request: nested tags",
    contact: 6,
    status: "open",
    priority: "low",
    channel: "chat",
    assignee: null,
    team: null,
    tags: ["feature-request"],
    mailbox: "support",
    thread: [
      { from: "c", min: 41, html: `<p>hey! quick one: is there any way to nest tags? like <code>clients/acme</code> and <code>clients/globex</code></p>` },
      { from: "c", min: 40, html: `<p>we have ~200 tags now and the sidebar is getting wild 😅</p>` },
      { from: "c", min: 39, html: "", attach: ["voice"] },
    ],
  },
  {
    number: 1068,
    subject: "Refund for duplicate annual charge",
    contact: 3,
    status: "open",
    priority: "high",
    channel: "email",
    assignee: "maya",
    team: "Billing",
    tags: ["billing", "refund"],
    mailbox: "billing",
    thread: [
      {
        from: "c",
        min: 60 * 5,
        html: `<p>Hello,</p><p>We were charged twice for our annual renewal on October 9th (2 × €4,320). Can you refund the duplicate? Card ending 4417.</p><p>Tomás García, Brightline Logistics</p>`,
      },
      {
        from: "c",
        min: 60 * 5 - 2,
        html: RECEIPT,
      },
      {
        from: "maya",
        min: 60 * 4 + 20,
        delivery: "failed",
        html: `<p>Hi Tomás,</p><p>You're right, the renewal retried after a timeout and both attempts went through. I've refunded the second charge of €4,320. It usually shows on your statement within 5 to 10 business days.</p><p>Apologies for the trouble.</p><p>Maya</p>`,
      },
      {
        from: "c",
        min: 35,
        html: `<p>Hi, just checking in on this. I haven't heard back yet. Is the refund being processed?</p>`,
      },
      {
        from: "maya",
        note: true,
        min: 20,
        html: `<p>Checked with billing: the refund went through on the 9th. My reply bounced, resending it from the billing address.</p>`,
      },
    ],
  },
  {
    number: 1067,
    subject: "Mobile app crashes when opening shared notebook",
    contact: 10,
    status: "open",
    priority: "urgent",
    channel: "email",
    assignee: null,
    team: "Support",
    tags: ["bug", "mobile"],
    mailbox: "support",
    thread: [
      {
        from: "c",
        min: 60 * 3 + 10,
        attach: ["screenshot", "log"],
        html: `<p>Hi,</p><p>The iOS app (4.12.0) crashes immediately when any of our field engineers open the "Site inspections" shared notebook. Other notebooks open fine. iPad and iPhone, iOS 26.0.1.</p><p>Crash screenshot and the log exported from TestFlight attached. About 30 people are affected and they can't fill in their reports.</p><p>Aisha Bello<br>Kestrel Robotics</p>`,
      },
    ],
  },
  {
    number: 1066,
    subject: "How do I invite my team?",
    contact: 7,
    status: "solved",
    priority: "normal",
    channel: "portal",
    assignee: "sam",
    team: "Support",
    tags: ["onboarding"],
    mailbox: "support",
    csat: { score: 5, comment: "Super quick, thanks Sam!" },
    thread: [
      { from: "c", min: 60 * 30, html: `<p>Hi, we just upgraded to the Education plan. How do I invite the other teachers? I can't find it in settings.</p>` },
      {
        from: "sam",
        min: 60 * 29,
        html: `<p>Hi Ben,</p><p>Welcome aboard! Go to <strong>Settings → Members</strong> and click <strong>Invite</strong>. You can paste a list of addresses separated by commas, and choose their role.</p><ol><li>Open Settings from the bottom of the sidebar</li><li>Choose Members</li><li>Paste the addresses and press Send invites</li></ol><p>If your school uses Google Workspace you can also turn on domain sign-up so teachers join without an invite.</p><p>Sam</p>`,
      },
      { from: "c", min: 60 * 28, html: `<p>Perfect, all 14 invites went out. Thank you!</p>` },
    ],
  },
  {
    number: 1065,
    subject: "API returns 429 when syncing notes",
    contact: 8,
    status: "open",
    priority: "high",
    channel: "api",
    assignee: "priya",
    team: "Support",
    tags: ["api"],
    mailbox: "support",
    thread: [
      {
        from: "c",
        min: 60 * 2,
        html: `<p>Our nightly sync started failing with <code>429 Too Many Requests</code> after about 1,200 calls. We're well under the documented 10k/hour.</p><pre><code>GET /v1/notes?updated_since=2026-10-10T00:00:00Z
HTTP/1.1 429
RateLimit-Remaining: 0
RateLimit-Reset: 3600</code></pre><p>Has something changed?</p>`,
      },
      {
        from: "priya",
        min: 1,
        delivery: "pending",
        html: `<p>Hi Freya,</p><p>Thanks for the request log. The limit you're hitting is the per-endpoint burst limit on <code>/v1/notes</code> (1,200 per 10 minutes), not the hourly one. If you page with <code>cursor</code> instead of <code>updated_since</code> windows, you'll need about a tenth of the calls.</p><p>I've also raised your burst limit to 3,000 for tonight's run.</p><p>Priya</p>`,
      },
    ],
  },
  {
    number: 1064,
    subject: "Webhook signature doesn't validate",
    contact: 9,
    status: "on_hold",
    priority: "normal",
    channel: "email",
    assignee: "jonas",
    team: "Support",
    tags: ["api", "bug"],
    mailbox: "support",
    thread: [
      {
        from: "c",
        min: 60 * 50,
        html: `<p>We're verifying <code>OD-Signature</code> exactly like the docs show, but about 1 in 20 deliveries fails validation. The failing ones all have unicode in the note title.</p>`,
      },
      {
        from: "jonas",
        min: 60 * 47,
        html: `<p>Hi Gabriel,</p><p>Good catch. Make sure you compute the HMAC over the raw request bytes, before any JSON parsing or re-encoding. Some frameworks normalize unicode escapes, which changes the body.</p><p>If you're already doing that, send me two event ids that failed and I'll compare them with what we signed.</p>`,
      },
      { from: "c", min: 60 * 30, html: `<p>We use the raw body. Failing ids: <code>0192f0c4-77ab</code> and <code>0192f0c9-1e02</code>.</p>` },
      {
        from: "jonas",
        note: true,
        min: 60 * 29,
        html: `<p>Reproduced. Our relay re-serializes titles with escaped unicode. Fix is in review, putting this on hold until it ships.</p>`,
      },
    ],
  },
  {
    number: 1063,
    subject: "Can we get a DPA signed?",
    contact: 5,
    status: "pending",
    priority: "normal",
    channel: "email",
    assignee: "maya",
    team: null,
    tags: [],
    mailbox: "support",
    thread: [
      {
        from: "c",
        min: 60 * 70,
        html: `<p>Hello,</p><p>Before we roll Margin out firm-wide our compliance team needs a signed data processing agreement, and confirmation that our data stays in the EU region.</p><p>Hugo Moreau, Vantage Legal</p>`,
      },
      {
        from: "maya",
        min: 60 * 68,
        attach: ["pdf"],
        html: `<p>Hi Hugo,</p><p>Of course. Our standard DPA is attached, already countersigned on our side. Your workspace is hosted in Frankfurt and backups stay in the EU.</p><p>Once your team signs, reply with the PDF and we'll file it.</p><p>Maya</p>`,
      },
    ],
  },
  {
    number: 1062,
    subject: "Search doesn't find text inside images",
    contact: 11,
    status: "open",
    priority: "normal",
    channel: "email",
    assignee: "maya",
    team: "Support",
    tags: ["feature-request"],
    mailbox: "support",
    thread: [
      {
        from: "c",
        min: 60 * 9,
        html: `<p>We scan a lot of handwritten notes into Margin. Is there a way to search the text inside those images? Right now search only finds the captions.</p>`,
      },
    ],
  },
  {
    number: 1061,
    subject: "Thank you!",
    contact: 12,
    status: "solved",
    priority: "low",
    channel: "chat",
    assignee: "maya",
    team: null,
    tags: [],
    mailbox: "support",
    csat: { score: 4, comment: "" },
    thread: [
      { from: "c", min: 60 * 20, html: `<p>Just wanted to say the new recipe templates are lovely. Our bakers use them every morning.</p>` },
      { from: "maya", min: 60 * 19, html: `<p>That made our day, thank you for telling us! I'll pass it on to the team that built them.</p>` },
    ],
  },
];

/** Generated tickets: subject and the customer's first message. */
export const GENERATED: { subject: string; body: string; tags: string[] }[] = [
  { subject: "Two-factor codes not accepted", body: "My authenticator codes are rejected every time since I got a new phone. I moved the app over with the transfer tool.", tags: [] },
  { subject: "Change billing email address", body: "Invoices still go to our former office manager. Can you send them to accounts@ from now on?", tags: ["billing"] },
  { subject: "Notebook sharing link expired too soon", body: "I set a share link to expire in 30 days but our client says it stopped working after a week.", tags: ["bug"] },
  { subject: "Request: dark mode for the web clipper", body: "The browser clipper is blinding at night. Any chance of a dark theme?", tags: ["feature-request"] },
  { subject: "Imported Evernote notes lost formatting", body: "We imported 3,000 notes from Evernote. Tables and checklists came through as plain text.", tags: ["bug", "onboarding"] },
  { subject: "Upgrade from Team to Business mid-cycle", body: "If we upgrade halfway through the month, how is the difference billed?", tags: ["billing"] },
  { subject: "Calendar integration stopped syncing", body: "Meeting notes no longer appear for events since Tuesday. Google Calendar is still connected in settings.", tags: ["bug"] },
  { subject: "How do I archive old notebooks?", body: "We want to hide last year's project notebooks without deleting them. Is that possible?", tags: ["onboarding"] },
  { subject: "Slow loading on large notebooks", body: "Our wiki notebook with 900 pages takes almost 20 seconds to open in the web app.", tags: ["bug"] },
  { subject: "Add SCIM provisioning", body: "We'd like users created and deactivated from Azure AD automatically. Do you support SCIM?", tags: ["sso", "feature-request"] },
  { subject: "Refund request for unused seats", body: "We bought 20 extra seats in August but only used 6. Could we get a partial refund?", tags: ["billing", "refund"] },
  { subject: "Offline mode on Android", body: "Notes I write offline on Android disappear when I reconnect. This has happened three times now.", tags: ["bug", "mobile"] },
  { subject: "API token permissions", body: "Is it possible to create a read-only API token that can only access one notebook?", tags: ["api"] },
  { subject: "Templates missing after workspace merge", body: "After merging our two workspaces, the shared templates from the second one are gone.", tags: ["bug"] },
  { subject: "Student discount eligibility", body: "I'm a PhD student. Does the education discount apply to individual plans?", tags: ["billing"] },
  { subject: "Pasting from Google Docs adds extra spacing", body: "Every paragraph pasted from Docs gets a blank line after it. Very annoying for long documents.", tags: ["bug"] },
  { subject: "Can guests comment without an account?", body: "We want clients to leave comments on a shared notebook without signing up. Is that supported?", tags: ["feature-request"] },
  { subject: "Okta app missing from integration catalog", body: "Searching for Margin in the Okta integration network finds nothing. How do we set up SSO?", tags: ["sso", "onboarding"] },
  { subject: "Export all data before cancelling", body: "We're consolidating tools. What's the best way to export everything, including attachments?", tags: ["export"] },
  { subject: "Webhook retries flooding our endpoint", body: "Our endpoint was down for an hour and now we're receiving hundreds of retried deliveries at once.", tags: ["api"] },
  { subject: "Handwriting recognition on iPad", body: "Apple Pencil notes are not converted to text even though the setting is on.", tags: ["mobile"] },
  { subject: "Wrong timezone on reminders", body: "Reminders fire an hour late since the clocks changed. My timezone is set to Europe/Lisbon.", tags: ["bug"] },
  { subject: "Bulk move notes between notebooks", body: "Is there a way to select 50 notes and move them at once? Dragging one by one takes forever.", tags: ["feature-request"] },
  { subject: "Payment failed but card is valid", body: "We got a payment failed email but the card works everywhere else. Can you retry the charge?", tags: ["billing"] },
  { subject: "Attachment size limit", body: "I can't upload a 120 MB video to a note. What's the limit on our plan?", tags: [] },
  { subject: "Shared notebook shows as empty for one user", body: "One teammate sees the Roadmap notebook as empty. Everyone else sees all pages.", tags: ["bug"] },
  { subject: "Setting up SAML with Google Workspace", body: "Following your guide, but Google asks for an ACS URL that isn't listed. Where can I find it?", tags: ["sso"] },
  { subject: "Rename workspace URL", body: "We rebranded. Can we change our workspace URL from oldname to our new name?", tags: [] },
  { subject: "Markdown export drops images", body: "Exporting to Markdown gives me the text but every image link points to a temporary URL.", tags: ["export", "bug"] },
  { subject: "Zapier trigger for new notes", body: "The Zapier 'New note' trigger only fires for notes in the default notebook.", tags: ["api"] },
  { subject: "Account locked after password reset", body: "I reset my password and now it says my account is locked for 30 minutes.", tags: [] },
  { subject: "Volume pricing for 500 seats", body: "We're evaluating Margin for the whole company. Do you offer volume pricing over 500 seats?", tags: ["billing"] },
  { subject: "Widget for embedding notes in our intranet", body: "Is there an embed code to show a published note inside our SharePoint intranet?", tags: ["feature-request"] },
  { subject: "Checklist items reorder themselves", body: "When I tick a checklist item it jumps to the bottom. Can that be turned off?", tags: [] },
  { subject: "Duplicate notes after sync conflict", body: "I edited a note on my laptop and phone and now I have three copies of it.", tags: ["bug", "mobile"] },
  { subject: "Where is the audit log?", body: "Our security team wants to see who accessed which notebook last month.", tags: [] },
  { subject: "Invoice in USD instead of EUR", body: "Our invoices switched to USD this month. We're based in Spain and always paid in EUR.", tags: ["billing"] },
  { subject: "Keyboard shortcuts on Windows", body: "Half of the shortcuts listed in the help center don't work on Windows with a German keyboard.", tags: ["bug"] },
  { subject: "Onboarding call for new team", body: "We just signed up 25 people. Can we schedule a short onboarding session?", tags: ["onboarding"] },
  { subject: "API pagination returns duplicates", body: "When paging through /v1/notes with cursor, some notes show up on two pages.", tags: ["api", "bug"] },
  { subject: "Print layout cuts off tables", body: "Printing a note with a wide table cuts off the last two columns.", tags: ["export"] },
  { subject: "Can't remove former employee", body: "Removing a member fails with 'owner cannot be removed' but they're not the owner anymore.", tags: [] },
  { subject: "Feature request: version history diff", body: "Version history is great but it would help to see what changed between versions.", tags: ["feature-request"] },
  { subject: "Sync paused on macOS app", body: "The macOS app says 'Sync paused' and the resume button does nothing.", tags: ["bug"] },
  { subject: "Tax exemption certificate", body: "We're a non-profit. Where can we upload our tax exemption certificate?", tags: ["billing"] },
  { subject: "Mention notifications not arriving", body: "When colleagues @mention me I don't get an email, though notifications are on.", tags: ["bug"] },
  { subject: "Transfer ownership of workspace", body: "Our founder is leaving and owns the workspace. How do we transfer ownership?", tags: [] },
  { subject: "Search results ignore accents", body: "Searching 'cafe' doesn't find notes with 'café'. Is accent-insensitive search possible?", tags: ["feature-request"] },
  { subject: "Images blurry in shared links", body: "Images in a public share link look heavily compressed compared to inside the app.", tags: [] },
  { subject: "Connect Microsoft Teams", body: "Is there a Teams integration so we can share notes into channels?", tags: ["feature-request"] },
  { subject: "Downgrade to free plan", body: "We'd like to downgrade at the end of the billing period. What happens to our extra notebooks?", tags: ["billing"] },
  { subject: "Login with Apple fails on web", body: "Sign in with Apple works on iPhone but on the web I get 'invalid client'.", tags: ["sso", "bug"] },
  { subject: "Read receipts for shared notes", body: "Can I see whether a client has opened the note I shared with them?", tags: ["feature-request"] },
  { subject: "Notebook permissions for contractors", body: "We want contractors to edit one notebook only, without seeing the rest of the workspace.", tags: ["onboarding"] },
  { subject: "Data retention settings", body: "Can we automatically delete notes older than 3 years for compliance?", tags: [] },
  { subject: "Copying tables into Excel", body: "Copying a table from a note into Excel pastes everything into a single cell.", tags: [] },
];

export const SPAM = [
  { subject: "Boost your SEO ranking today 🚀", from: "growth@rank-boost-pro.biz", name: "Rank Boost", body: "We guarantee first page on Google in 7 days. Reply YES for a free audit!!!" },
  { subject: "You have won a $500 gift card", from: "rewards@prize-center.top", name: "Prize Center", body: "Congratulations! Click the link to claim your $500 gift card before it expires." },
  { subject: "Partnership opportunity: guest posts", from: "outreach@linkfarm.net", name: "Kelly from LinkFarm", body: "We'd love to publish a sponsored article on your blog. Our rates start at $50." },
  { subject: "URGENT: verify your account", from: "security@rnargin-support.com", name: "Margin Security", body: "Your account will be suspended in 24 hours. Verify your password here immediately." },
];

export const FOLLOW_UPS = [
  "Thanks, that worked!",
  "Any update on this? It's still happening on our side.",
  "I tried that but no luck. Same result as before.",
  "Great, appreciate the quick reply.",
  "Following up here, we'd love a fix this week.",
  "That makes sense. One more question: does this also apply to the mobile app?",
];

export const AGENT_REPLIES = [
  "Thanks for reaching out. I've reproduced this on our side and passed it to the engineering team with your details. I'll update you as soon as there's a fix.",
  "Good question. You can find this under Settings, then Workspace. If the option is greyed out, your role might need to be admin.",
  "Sorry for the trouble. Could you send me the exact time it happened and the browser you were using? That will help us find it in the logs.",
  "I've made the change on our side. It may take a few minutes to show up. Let me know if anything still looks off.",
  "That's not possible today, but I've added your vote to the request. It's something we're looking at for next quarter.",
];

export const SAVED_REPLIES = [
  {
    title: "Greeting and thanks",
    shortcut: "thanks",
    body: "<p>Hi {{contact.first_name}},</p><p>Thanks for reaching out. I'm looking into this now and will get back to you shortly.</p>",
  },
  {
    title: "Ask for steps to reproduce",
    shortcut: "repro",
    body: "<p>Could you share the steps that lead to this, the browser or app version you're on, and roughly when it happened? A screenshot helps too.</p>",
  },
  {
    title: "Refund processed",
    shortcut: "refund",
    body: "<p>I've processed the refund. It usually shows on your statement within 5 to 10 business days, depending on your bank.</p>",
  },
  {
    title: "SSO setup guide",
    shortcut: "sso",
    body: '<p>Here is our step-by-step guide for setting up SAML single sign-on: <a href="https://help.margin.app/sso">help.margin.app/sso</a>. Your ACS URL is shown in Settings, then Security.</p>',
  },
  {
    title: "Feature request logged",
    shortcut: "fr",
    body: "<p>Thanks for the idea. I've logged it with our product team and added your use case. We'll let you know if it lands on the roadmap.</p>",
  },
  {
    title: "Closing, no reply",
    shortcut: "close",
    body: "<p>I haven't heard back in a few days, so I'll close this for now. Just reply to this email if you need anything else and it will reopen.</p>",
  },
];

/** `brand`: the slug of the one brand it belongs to; without it, every brand shows it. */
export type ArticleSeed = { collection: number; title: string; status: "published" | "draft"; excerpt: string; body: string; views: number; brand?: string };

export const COLLECTIONS: { name: string; slug: string; description: string; brand?: string }[] = [
  { name: "Getting started", slug: "getting-started", description: "Set up your workspace and learn the basics." },
  { name: "Account & billing", slug: "account-billing", description: "Plans, invoices, seats and payment methods." },
  { name: "Integrations", slug: "integrations", description: "Connect Margin with the tools you already use." },
  { name: "Troubleshooting", slug: "troubleshooting", description: "Fixes for common problems with sync, export and sign-in." },
  { name: "Publishing", slug: "publishing", description: "Send issues, grow your list and run your publication on Pressroom.", brand: "pressroom" },
];

export const ARTICLES: ArticleSeed[] = [
  {
    collection: 0,
    title: "Create your first notebook",
    status: "published",
    views: 4210,
    excerpt: "Notebooks hold your notes. Here's how to create one and invite others.",
    body: `<p>Notebooks are the top-level containers in Margin. Every note lives in exactly one notebook, and every notebook has its own sharing settings.</p><h2>Create a notebook</h2><ol><li>Click <strong>New</strong> at the top of the sidebar.</li><li>Choose <strong>Notebook</strong> and give it a name.</li><li>Pick an icon, or leave the default.</li></ol><p>The notebook appears in your sidebar right away. Drag it to reorder.</p><h2>Share it</h2><p>Open the notebook and press <strong>Share</strong>. You can invite teammates by email or create a link anyone in your workspace can open.</p><blockquote>Tip: press <code>N</code> anywhere to create a new note in the current notebook.</blockquote>`,
  },
  {
    collection: 0,
    title: "Invite your team",
    status: "published",
    views: 3108,
    excerpt: "Add teammates, choose their role and set up domain sign-up.",
    body: `<p>You can invite people from <strong>Settings → Members</strong>. Paste one or more addresses, choose a role and send.</p><h2>Roles</h2><ul><li><strong>Owner</strong> manages billing and can delete the workspace.</li><li><strong>Admin</strong> manages members and settings.</li><li><strong>Member</strong> creates and edits notes.</li><li><strong>Guest</strong> only sees notebooks shared with them.</li></ul><h2>Domain sign-up</h2><p>Turn on domain sign-up to let anyone with your company email join without an invite. This works with Google Workspace and Microsoft 365 accounts.</p>`,
  },
  {
    collection: 0,
    title: "Keyboard shortcuts",
    status: "published",
    views: 1890,
    excerpt: "Move faster with the shortcuts power users rely on.",
    body: `<p>Press <code>?</code> anywhere in the app to see every shortcut. These are the ones people use most:</p><ul><li><code>⌘ K</code> opens search and commands</li><li><code>N</code> creates a note</li><li><code>⌘ ⇧ L</code> toggles a checklist</li><li><code>⌘ /</code> opens the block menu</li></ul><p>On Windows and Linux, use <code>Ctrl</code> instead of <code>⌘</code>.</p>`,
  },
  {
    collection: 0,
    title: "Import from Evernote and Notion",
    status: "published",
    views: 2544,
    excerpt: "Bring your existing notes with formatting, tags and attachments.",
    body: `<p>Margin imports <code>.enex</code> files from Evernote and the Markdown and CSV export from Notion.</p><h2>From Evernote</h2><ol><li>In Evernote, export a notebook as <code>.enex</code>.</li><li>In Margin, open <strong>Settings → Import</strong>.</li><li>Drop the file and choose a destination notebook.</li></ol><p>Tags become Margin tags. Checklists and tables keep their formatting.</p><h2>From Notion</h2><p>Export your workspace as <strong>Markdown &amp; CSV</strong>, then upload the zip. Databases are imported as tables.</p>`,
  },
  {
    collection: 1,
    title: "Change your plan",
    status: "published",
    views: 1320,
    excerpt: "Upgrade, downgrade, and how mid-cycle changes are billed.",
    body: `<p>Owners can change the plan from <strong>Settings → Billing</strong>.</p><h2>Upgrading</h2><p>Upgrades take effect immediately. You're charged the prorated difference for the rest of the current period.</p><h2>Downgrading</h2><p>Downgrades take effect at the end of the current period. If you're over the new plan's limits, extra notebooks become read-only until you archive some.</p>`,
  },
  {
    collection: 1,
    title: "Update your VAT number and billing details",
    status: "published",
    views: 870,
    excerpt: "Keep invoices correct for your accounting team.",
    body: `<p>Billing details are used for every invoice generated after you save them. Invoices that were already issued are not changed automatically.</p><ol><li>Open <strong>Settings → Billing</strong>.</li><li>Choose <strong>Billing details</strong>.</li><li>Update your company name, address and VAT number.</li></ol><p>Need a corrected copy of an earlier invoice? Contact us and we'll reissue it.</p>`,
  },
  {
    collection: 1,
    title: "Refunds and cancellations",
    status: "published",
    views: 640,
    excerpt: "Our refund policy for monthly and annual plans.",
    body: `<p>Monthly plans can be cancelled at any time and stay active until the end of the month. Annual plans can be refunded in full within 14 days of purchase.</p><p>Duplicate or mistaken charges are always refunded. Reply to your receipt email and our billing team will take care of it.</p>`,
  },
  {
    collection: 1,
    title: "Education and non-profit discounts",
    status: "draft",
    views: 0,
    excerpt: "Who qualifies and how to apply.",
    body: `<p>Accredited schools, universities and registered non-profits get 50% off Team and Business plans.</p><p>To apply, send proof of status from your organization's email address.</p>`,
  },
  {
    collection: 2,
    title: "Set up SAML single sign-on",
    status: "published",
    views: 2980,
    excerpt: "Connect Okta, Microsoft Entra ID or Google Workspace.",
    body: `<p>Business and Enterprise plans can require sign-in through your identity provider.</p><h2>What you need</h2><ul><li>Admin access to your identity provider</li><li>The ACS URL and Entity ID from <strong>Settings → Security</strong></li></ul><h2>Steps</h2><ol><li>Create a SAML app in your identity provider.</li><li>Paste the ACS URL and Entity ID.</li><li>Upload the IdP metadata XML to Margin.</li><li>Test with one account before enforcing SSO.</li></ol><h2>Certificate rotation</h2><p>When your identity provider rotates its signing certificate, add the new certificate in Margin <em>before</em> switching. Both stay valid during the overlap.</p>`,
  },
  {
    collection: 2,
    title: "Using the Margin API",
    status: "published",
    views: 1760,
    excerpt: "Authentication, pagination and rate limits.",
    body: `<p>Create a token in <strong>Settings → API</strong>. Send it as a bearer token:</p><pre><code>curl https://api.margin.app/v1/notes \\
  -H "Authorization: Bearer fn_live_..."</code></pre><h2>Pagination</h2><p>Lists return a <code>next_cursor</code>. Pass it as <code>?cursor=</code> to get the next page.</p><h2>Rate limits</h2><p>10,000 requests per hour, with a burst limit of 1,200 per 10 minutes per endpoint. Check the <code>RateLimit-Remaining</code> header.</p>`,
  },
  {
    collection: 2,
    title: "Webhooks",
    status: "published",
    views: 990,
    excerpt: "Get notified when notes change, and verify signatures.",
    body: `<p>Webhooks send a POST request to your URL when something happens in your workspace.</p><h2>Verifying signatures</h2><p>Every request has an <code>OD-Signature</code> header. Compute an HMAC-SHA256 of <code>timestamp.body</code> with your secret, using the <strong>raw</strong> request body.</p><h2>Retries</h2><p>Failed deliveries are retried with exponential backoff for up to 24 hours.</p>`,
  },
  {
    collection: 2,
    title: "Google Calendar integration",
    status: "draft",
    views: 0,
    excerpt: "Create meeting notes automatically from calendar events.",
    body: `<p>Connect Google Calendar in <strong>Settings → Integrations</strong> to create a meeting note for every event with two or more attendees.</p>`,
  },
  {
    collection: 3,
    title: "PDF export fails for large notebooks",
    status: "published",
    views: 1430,
    excerpt: "What to do when an export times out or images are missing.",
    body: `<p>Exports run in the background. Very large notebooks, especially ones with many high-resolution images, can take several minutes.</p><h2>If the export fails</h2><ul><li>Export sections separately using <strong>Export selection</strong>.</li><li>Turn on <strong>Compress images</strong> in the export dialog.</li><li>Make sure every image has finished uploading (no spinner).</li></ul><p>Still stuck? Contact us with the notebook name and we'll run the export for you.</p>`,
  },
  {
    collection: 3,
    title: "Sync is paused or stuck",
    status: "published",
    views: 2210,
    excerpt: "Get the desktop and mobile apps syncing again.",
    body: `<p>The apps pause sync when they can't reach our servers or when you're over your storage limit.</p><ol><li>Check your connection and any VPN or proxy.</li><li>Sign out and back in.</li><li>On desktop, choose <strong>Help → Reset local cache</strong>. Your notes are safe on the server.</li></ol><p>If you see duplicate notes after a conflict, the newest copy is the one with your latest edits.</p>`,
  },
  {
    collection: 4,
    title: "Schedule an issue",
    status: "published",
    views: 640,
    excerpt: "Pick when an issue goes out, in your publication's time zone.",
    body: `<p>Open the issue and click <strong>Schedule</strong>. Choose a day and time; issues go out in the time zone set under <strong>Publication → Settings</strong>.</p><p>Scheduled issues can be edited until they start sending. To send one right away, open it and click <strong>Send now</strong>.</p>`,
  },
  {
    collection: 4,
    title: "Connect a custom domain",
    status: "published",
    views: 410,
    excerpt: "Serve your publication from your own domain.",
    body: `<p>Add a CNAME record that points your domain at <strong>domains.pressroom.app</strong>, then enter the domain under <strong>Publication → Domain</strong>.</p><p>DNS changes can take up to 48 hours. The domain switches to active on its own once the record is found.</p>`,
  },
  {
    collection: 4,
    title: "Import subscribers from a CSV",
    status: "draft",
    views: 0,
    excerpt: "Bring your list over from Substack, Mailchimp or a spreadsheet.",
    body: `<p>Export your list as a CSV with an <strong>email</strong> column, then choose <strong>Subscribers → Import</strong>.</p>`,
  },
  {
    collection: 1,
    title: "Paid subscriptions and payouts",
    status: "published",
    views: 290,
    brand: "pressroom",
    excerpt: "Charge readers for issues and get paid every month.",
    body: `<p>Connect a Stripe account under <strong>Publication → Payments</strong> to offer paid subscriptions. Payouts arrive on the first working day of each month.</p>`,
  },
];
