# Sign-in and passwords

## Shortest password allowed

Settings > Sign-in > Passwords sets the shortest password HolyArrs takes. Only the owner can change it.

- Default: 4 characters. Allowed: 4 to 64. HolyArrs never goes below 4, whatever the setting says.
- It counts for every new password: the first-run owner, an invite being accepted, a password change on My account, and `holyarrs reset-owner`.
- Passwords people already have keep working after the minimum goes up; the new minimum applies the next time they change it.
- The sign-in, invite and My account pages read the minimum from the server (`min_password_length` in `GET /api/v1/auth/state`), so the hint under the password box always matches.
- The longest password taken is 1024 characters.

Short passwords are easy to guess. HolyArrs slows down repeated wrong sign-ins from the same place and for the same name, which helps, but it does not make a short password safe. When HolyArrs can be reached from outside your home, raise the minimum to 12 or more.

API: `GET /api/v1/auth/settings` shows `min_password_length`; `PUT /api/v1/auth/settings` with `{"min_password_length": 12}` changes it (owner only, 4 to 64, anything else answers 400).

## Skip sign-in on the local network

Settings > Sign-in > Local network lets devices on your own networks open HolyArrs as the owner without a password. It is off by default.

- It works only for the networks listed under the switch, one per line: a range like `192.168.1.0/24` or `fd00::/8`, or one address like `192.168.1.20`. Up to 32. Anything else answers 400 naming the entry.
- It cannot be turned on with an empty list, and the list cannot be emptied while it is on.
- Behind a reverse proxy, the address checked is the one the proxy forwards, and only when the proxy is in `HOLYARRS_TRUSTED_PROXIES`. When it is not, the card shows no address and says the proxy is not trusted.
- The card shows the address HolyArrs sees for your browser. Some setups (Docker Desktop, rootless Docker or Podman, Kubernetes NodePort) make every device show up as one gateway address such as `172.17.0.1` (or `172.18.0.1` and up for compose networks), `10.88.0.1`, `10.89.x.1`, `10.0.2.2` or `192.168.65.x`. Listing that address lets anyone who can reach HolyArrs in as the owner, so the card warns when a listed entry looks like one. Open the page from a second device: if it shows the same address, turn the switch off, then remove that entry.
- Making API keys, changing sign-in settings, reading or replacing secrets and restoring backups still need a password sign-in.
- Installs that had it on before the list existed were given the ranges it trusted then (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `fc00::/7`, loopback and link-local), so nothing broke on the upgrade. Narrow the list to your own network.

API: `GET /api/v1/auth/settings` shows `skip_login_local_network`, `local_networks`, `seen_address`, `gateway_networks` and `proxy_untrusted`; `PUT /api/v1/auth/settings` with `{"skip_login_local_network": true, "local_networks": ["192.168.1.0/24"]}` changes them (owner only).

## Single sign-on and trusted proxy sign-in

HolyArrs can offer an OpenID Connect provider or sign-in through a trusted reverse proxy. Both are optional. When enabled, each appears as an explicit button on the sign-in page; merely visiting HolyArrs never signs someone in from proxy headers.

A new outside identity joins as a member only when the owner has allowed new members for that provider. It never becomes an owner or admin automatically. HolyArrs identifies an OpenID Connect account by the provider's stable issuer and subject values. It does not join accounts because their email addresses match.

To add either method to an existing account:

1. Sign in to that HolyArrs account.
2. Open **My account**.
3. Confirm the account's HolyArrs password. If two-factor sign-in is on, enter a current app or backup code too.
4. Choose **Link single sign-on** or **Link trusted proxy sign-in**.

The local confirmation makes a fresh session before linking. A session created through another provider, an older session without sign-in assurance, or a local session whose confirmation is no longer recent must complete this step. Provider configuration cannot skip it.

OpenID Connect sends the browser to the configured provider and back to HolyArrs. HolyArrs checks the provider, audience, signature, expiry, callback state, nonce and proof key before making a session. A failed or expired callback leaves the person signed out and shows a safe error without provider tokens or raw provider responses.

Trusted proxy sign-in accepts identity headers only when the request comes directly from an explicitly trusted proxy address. Forwarded client-address headers do not make an untrusted peer trusted. Keep HolyArrs unreachable except through that proxy when this method is enabled.

Signing out ends the HolyArrs session. It does not sign out the provider's own browser session, so a provider may be able to sign the same person in again without asking for a password. Use the provider's sign-out control too when that matters on a shared device.

## Recovery

Keep a working local owner password even when everyone normally uses an outside provider. If the provider or proxy is unavailable, the owner can still sign in locally. If the owner password is lost, run `holyarrs reset-owner` on the machine that runs HolyArrs. It sets a new local owner name and password, turns off two-factor sign-in for that owner, and signs out every device and API key.

Outside-only accounts cannot make a local password or confirm local sign-in in this version. Create and keep the local owner first, then link its provider identity from My account. An existing local account keeps its password and recovery path after linking.

## Setting up single sign-on in Settings

The owner can set up OpenID Connect in **Settings → Sign-in → Single sign-on** instead of the environment:

1. Type the provider's issuer and choose **Test**. HolyArrs checks that an OpenID Connect provider answers there; it never sends a secret for the test.
2. Type the client id and the client secret, or tick **Public client** when the provider expects none.
3. Copy the **Callback address** and register that exact address at the provider.
4. Type your HolyArrs password (and a two-factor code when it is on) and choose **Save**. Every change needs it.

Rules that protect the setup:

- The client secret is kept encrypted and never shown again. Leave the field empty to keep it.
- A saved secret never goes to a new issuer or client id. Changing either needs the secret typed again.
- Changing the issuer or client id removes the links made through the old one and signs out everyone who used single sign-on. They link again from My account.
- Letting new people join needs a confirmation, because everyone the provider signs in could then make an account.
- Turning single sign-on off signs out the sessions it made.
- When `HOLYARRS_OIDC_ENABLED` is set at all, even to `false`, the environment owns single sign-on and Settings only shows it.
- Trusted proxy sign-in is set only in the environment. The addresses it trusts decide who may sign in as anyone, so a browser never changes them.
- `HOLYARRS_OIDC_ALLOW_LOCAL_NETWORK` and `HOLYARRS_OIDC_ALLOW_LOOPBACK_HTTP` also apply to single sign-on set up in Settings.

## Unlinking

A linked single sign-on or trusted proxy sign-in can be removed:

- **My account → Linked sign-ins → Unlink** removes your own. It needs a recent password confirmation, and it is refused when it is your only way to sign in.
- **Settings → Users** shows each person's linked sign-ins; the owner can unlink them. When it is their only way in, HolyArrs asks once more.

Unlinking signs out the sessions that sign-in made. When new people may join through that provider, the same person could still sign in again as a new member; turn joining off to keep them out.

## Deployment settings

Set these before starting HolyArrs. Both methods are off unless explicitly enabled. Boolean values are `true` or `false`. Invalid enabled configuration prevents startup; a provider outage leaves local sign-in available.

| Variable | Meaning |
| --- | --- |
| `HOLYARRS_OIDC_ENABLED` | Enable OpenID Connect. Default `false`. |
| `HOLYARRS_OIDC_ISSUER` | Exact HTTPS issuer published by the provider. |
| `HOLYARRS_OIDC_CLIENT_ID` | Registered application client id. |
| `HOLYARRS_OIDC_CLIENT_SECRET_FILE` | Optional file holding the client secret, at most 4096 bytes. Omit for a public client. Keep it outside tracked files. |
| `HOLYARRS_OIDC_REDIRECT_URL` | Exact browser-facing HTTPS callback: the app origin, its URL base if any, then `/api/v1/auth/oidc/callback`. Register this same value at the provider. |
| `HOLYARRS_OIDC_ALLOW_JOIN` | Allow unlinked identities to make member accounts with only the ability to ask. Default `false`. |
| `HOLYARRS_OIDC_ALLOW_LOCAL_NETWORK` | Permit provider endpoints on private networks. Default `false`; cloud metadata and unsafe destination addresses remain blocked. |
| `HOLYARRS_OIDC_ALLOW_LOOPBACK_HTTP` | Allow HTTP on loopback for isolated testing only. Default `false`. |
| `HOLYARRS_FORWARD_AUTH_ENABLED` | Enable trusted proxy sign-in. Default `false`. |
| `HOLYARRS_FORWARD_AUTH_ISSUER` | A stable HTTPS identifier for this proxy identity source. Changing it makes a different identity source. |
| `HOLYARRS_FORWARD_AUTH_TRUSTED_PEERS` | Comma-separated direct peer IP addresses or CIDRs. Required; an all-address range is refused. |
| `HOLYARRS_FORWARD_AUTH_ALLOW_JOIN` | Allow unlinked proxy identities to make member accounts with only the ability to ask. Default `false`. |

The OIDC provider must support authorization code sign-in with PKCE S256 and RS256 identity tokens. Discovery, authorization, token and signing-key endpoints must share the issuer's origin. HolyArrs does not request refresh tokens or use provider roles or groups. It supports one configured OIDC provider and one proxy identity source.

The proxy must remove incoming `Remote-User` and `Remote-Name` headers and write its own values after authenticating the person. `Remote-User` must contain one stable, nonempty subject, at most 255 bytes. Optional `Remote-Name` is a display name of at most 200 bytes. Repeated, comma-joined or control-character values are refused. Role headers are ignored. Allow only the proxy to reach the app listener: trusting a proxy address also trusts identities it supplies.

Changes that manage accounts, credentials, settings or app controls require a local password confirmation from the last ten minutes for browser sessions. Two-factor sign-in, when enrolled, is part of that confirmation. Outside sign-in and local-network bypass do not count as local confirmation. API keys retain their existing scopes. Logout and device-session revocation remain available without confirmation.
