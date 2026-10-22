---
title: Account and auth
description: Sign in, sessions, two-factor authentication and your own user account.
---

These endpoints act on the signed-in user, not a workspace, so they do not take `OD-Workspace`. They are for user sessions; API keys cannot use them.

## Sign-in

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/v1/auth/config` | Providers, password login, sign-up and challenge settings |
| `POST` | `/v1/auth/register` | `{email, password, name, turnstile_token?}` |
| `POST` | `/v1/auth/login` | `{email, password, turnstile_token?}` |
| `POST` | `/v1/auth/mfa` | `{mfa_token, code}` |
| `POST` | `/v1/auth/refresh` | `{user_id?}` + the account's refresh cookie → new access token, rotates that account's refresh token |
| `POST` | `/v1/auth/accounts` | The accounts this browser is signed in to, with their workspaces. Rotates nothing |
| `POST` | `/v1/auth/logout` | Revokes this account's session in the browser and closes its sockets; `{all: true}` for every account |
| `POST` | `/v1/auth/password/forgot` | `{email}`, always `204` |
| `POST` | `/v1/auth/password/reset` | `{token, password}` |
| `POST` | `/v1/auth/email/verify` | `{token}` |
| `POST` | `/v1/auth/email/resend` | |
| `GET` | `/v1/auth/oauth/{provider}/start?redirect_to=` | Redirects to Google, Microsoft, GitHub or OIDC |
| `GET` | `/v1/auth/oauth/{provider}/callback` | Redirects to `APP_URL/auth/callback?code=` |
| `POST` | `/v1/auth/oauth/exchange` | `{code, code_verifier?}` → access token, or `mfa_required`. The verifier is required for a code from a native sign-in |
| `POST` | `/v1/auth/app-attest/challenge` | `{}` → `{challenge}`, single use, five minutes. For the iPhone app's App Attest |
| `POST` | `/v1/auth/app-attest/keys` | `{key_id, attestation, challenge}` → `204`. Registers an attested key |

```sh
curl https://api.opendesk.sh/v1/auth/config
```

```json
{ "providers": [{ "id": "google", "name": "Google" }], "password_login": true, "signup_enabled": true, "turnstile_site_key": "0x4AAA..." }
```

`/v1/auth/config` also has `native: { app_attest, push, relay_url }` for the native apps: whether App Attest is accepted, and the push mode (`direct`, `relay` or `off`).

Login, registration and password reset are rate limited and protected by a Turnstile challenge when one is configured; a native iPhone client may send an App Attest proof instead. Native clients receive the refresh token in the body rather than a cookie: see [Native apps](/api/authentication/#native-apps). Login gives the same answer, in the same time, for an unknown email and a wrong password.

## Two-factor authentication

| Method | Path | Description |
| --- | --- | --- |
| `POST` | `/v1/auth/totp/setup` | → `{secret, otpauth_url}` |
| `POST` | `/v1/auth/totp/enable` | `{code}` → `{recovery_codes}` |
| `POST` | `/v1/auth/totp/disable` | `{code}` |

A code can be used only once. Recovery codes are single use.

## Your account

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/v1/me` | `{ user, memberships }` |
| `PATCH` | `/v1/me` | `{name?, avatar_url?}` |
| `POST` | `/v1/me/password` | `{current_password, new_password}` |
| `GET` | `/v1/me/sessions` | Your sessions, with `current` marking this one and `client` (`web`, `desktop` or `ios`) |
| `DELETE` | `/v1/me/sessions/{id}` | Revoke a session |
| `GET` | `/v1/me/devices` | Phones registered for push notifications |
| `POST` | `/v1/me/devices` | `{platform: "ios", token? or relay_token?, environment, app_version, name}`: register this phone, bound to the calling session |
| `PATCH` | `/v1/me/devices/{id}` | `{muted_types?}`: notification types this phone does not get, on top of your notification preferences |
| `DELETE` | `/v1/me/devices/{id}` | Remove a phone |
| `DELETE` | `/v1/me` | `{password?, code?}`: delete your account |

```ts
type User = { id: string; email: string; name: string; avatar_url: string; email_verified: boolean; totp_enabled: boolean; created_at: string };
type Membership = { workspace: Workspace; role: Role };
```

A device belongs to the session that registered it: signing out or revoking that session removes it. See [Push notifications](/self-hosting/push-notifications/).

## Notification preferences

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/v1/me/notification-preferences` | Your preferences in the workspace named by `OD-Workspace` |
| `PATCH` | `/v1/me/notification-preferences` | `{email_cadence?, digest_hour?, timezone?, paused_until?, types?: [{type, email?, push?, desktop?}]}` |
| `POST` | `/v1/me/notification-preferences/test` | Send yourself a sample notification email (5 an hour) |
| `POST` | `/v1/notifications/unsubscribe` | Public. `{token}` from a notification email: turns off that workspace's notification emails |

```ts
type NotificationPreferences = {
  email_cadence: "instant" | "hourly" | "daily" | "off"; digest_hour: number; timezone: string; paused_until: string | null;
  email: string; email_verified: boolean; delay_minutes: number;
  types: { type: string; group: "tickets" | "team" | "ai_billing" | "workspace" | "mail"; email: boolean; push: boolean; desktop: boolean;
    defaults: { email: boolean; push: boolean; desktop: boolean } }[];
};
```

Channels (`email`, `push` for your phones, `desktop` for the desktop app) are chosen per type and per workspace. The email schedule and the pause apply to your whole account. With `instant`, an unread notification is emailed after `delay_minutes`, together with anything else waiting for you in that workspace; a notification you read first is never emailed. Notification emails carry a one-click unsubscribe (RFC 8058).

Passwords are 12 to 128 characters and are refused if they appear in a list of breached passwords. Deleting your account is refused while you are the only owner of a workspace that has other members.
