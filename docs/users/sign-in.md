---
title: Sign-in and passwords
description: Set the shortest password, skip sign-in at home, and add single sign-on or trusted proxy sign-in.
level: Setup
sidebar: { order: 1 }
---

Decide how people sign in: password length, a home-network shortcut, and outside sign-in. Only the owner changes these, in **Settings > Sign-in and security**.

## Set the shortest password

1. Open **Settings > Sign-in and security > Passwords**.
2. Type a number from 4 to 64. The default is 8.
3. Select **Save**. You should see the new number stay after a reload.

The minimum applies to every new password: the first owner, accepted invites, changes on **My account** and `holyarrs reset-owner`. Existing passwords keep working until their next change. The longest password is 1024 characters.

:::caution
HolyArrs slows repeated wrong sign-ins, but a short password is still easy to guess. If HolyArrs is reachable from outside your home, use 12 or more.
:::

## Skip sign-in on my local network

Devices on your home network open HolyArrs as the owner, with no password. Off by default.

1. Open **Settings > Sign-in and security > Local network**.
2. List your networks, one per line, up to 32. Use a range (`192.168.1.0/24`, `fd00::/8`) or one address (`192.168.1.20`).
3. Turn the switch on and save. You should see your own address on the card.

The switch cannot be on with an empty list, or while any [Kid account](kids.md) exists. Making API keys, changing sign-in settings, reading or replacing secrets and restoring backups still ask for a password.

### Check the address first

Some setups make every device look like one gateway address: Docker Desktop, rootless Docker or Podman, and Kubernetes NodePort. Typical ones are `172.17.0.1`, `172.18.0.1` and up, `10.88.0.1`, `10.89.x.1`, `10.0.2.2` and `192.168.65.x`.

List one of those and anyone who reaches HolyArrs gets in as the owner. The card warns when an entry looks like one. Open the page from a second device: if it shows the same address, turn the switch off and remove that entry.

Behind a reverse proxy, HolyArrs reads the forwarded address only from proxies in `HOLYARRS_TRUSTED_PROXIES`. Otherwise the card shows no address and says the proxy is not trusted.

## Single sign-on

One OpenID Connect provider can sign people in. It shows as a button on the sign-in page.

1. Open **Settings > Sign-in and security > Single sign-on**.
2. Type the provider's issuer and select **Test**. No secret is sent for the test.
3. Type the client id and client secret. Tick **Public client** if the provider uses no secret.
4. Copy the **Callback address** and register that exact address at the provider.
5. Type your password, plus a two-factor code if you use one, and select **Save**. Every change asks for this.

Then link your own account: sign in with your password, open **My account**, under **Linked sign-ins** select **Link**, confirm your password, then select **Single sign-on**.

:::tip
Keep a working owner password. If the provider is down, password sign-in still works.
:::

## Choices

### Let new people join through single sign-on

Off by default. On, anyone the provider signs in gets an account, so HolyArrs asks you to confirm. Newcomers join as members who can only ask. Single sign-on never makes an owner or admin.

### Change the issuer or client id

You type the secret again. Old links are removed and everyone who used single sign-on is signed out. They link again from **My account**. Turning single sign-on off also signs them out.

### Unlink a sign-in

- Your own: **My account > Linked sign-ins > Unlink**. It needs a recent password confirmation, and is refused if it is your only way in.
- Anyone's: the owner uses **Settings > Users and roles**. If it is the person's only way in, HolyArrs asks once more.

Unlinking signs out the devices that used it. If joining is open, the same person can come back as a new member. Turn joining off to keep them out.

## If it does not work

**Symptom:** HolyArrs asks for your password again.
**Cause:** changes to accounts, keys and settings need a password confirmation from the last 10 minutes, plus two-factor if set. Outside sign-in and the local network shortcut do not count.
**Fix:** confirm with your password. Signing out and ending devices never ask.

**Symptom:** a callback failed with a short error, and you stay signed out.
**Cause:** the callback address at the provider differs from HolyArrs.
**Fix:** copy **Callback address** again and paste it exactly at the provider.

**Symptom:** an account made through single sign-on cannot set a password.
**Cause:** not possible in this version.
**Fix:** make the local owner first, then link single sign-on to it.

**Symptom:** signing out of HolyArrs left you signed in at the provider.
**Cause:** HolyArrs ends only its own sign-in.
**Fix:** on a shared device, sign out at the provider too.

Lost the owner password? See [Sessions and passwords](sessions.md#forgot-your-password).

## For experts

<details class="ha-more"><summary>Trusted proxy sign-in</summary>

If a reverse proxy already signs people in, HolyArrs accepts the name it passes. Set it only in the environment. It shows as a button on the sign-in page; a visit never signs anyone in from headers alone.

- The proxy strips incoming `Remote-User` and `Remote-Name` and sets its own after sign-in.
- `Remote-User`: one stable name, at most 255 bytes. `Remote-Name`: optional display name, at most 200 bytes.
- Repeated, comma-joined or control-character values are refused. Role headers are ignored.

Keep HolyArrs reachable only through that proxy. Trusting a proxy address means trusting every name it sends. Link it to an account from **My account > Link trusted proxy sign-in**.

</details>

<details class="ha-more"><summary>Environment variables</summary>

Set these before start. Both methods are off unless turned on. A bad setup stops HolyArrs from starting; a provider outage leaves password sign-in working. If `HOLYARRS_OIDC_ENABLED` is set at all, even to `false`, the environment owns single sign-on and Settings only shows it.

| Variable | What it does | Default |
| --- | --- | --- |
| `HOLYARRS_OIDC_ENABLED` | Turn on single sign-on | `false` |
| `HOLYARRS_OIDC_ISSUER` | Provider's exact HTTPS issuer | none |
| `HOLYARRS_OIDC_CLIENT_ID` | Registered client id | none |
| `HOLYARRS_OIDC_CLIENT_SECRET_FILE` | File holding the client secret, at most 4096 bytes. Leave out for a public client | none |
| `HOLYARRS_OIDC_REDIRECT_URL` | Exact HTTPS callback: your address, URL base if any, then `/api/v1/auth/oidc/callback` | none |
| `HOLYARRS_OIDC_ALLOW_JOIN` | New people join as members who can only ask | `false` |
| `HOLYARRS_OIDC_ALLOW_LOCAL_NETWORK` | Allow a provider on your private network | `false` |
| `HOLYARRS_OIDC_ALLOW_LOOPBACK_HTTP` | Allow plain HTTP on loopback, for testing | `false` |
| `HOLYARRS_FORWARD_AUTH_ENABLED` | Turn on trusted proxy sign-in | `false` |
| `HOLYARRS_FORWARD_AUTH_ISSUER` | Stable HTTPS name for this proxy. A new value counts as a new source | none |
| `HOLYARRS_FORWARD_AUTH_TRUSTED_PEERS` | Comma-separated proxy addresses or ranges. Required. An all-address range is refused | none |
| `HOLYARRS_FORWARD_AUTH_ALLOW_JOIN` | New proxy names join as members who can only ask | `false` |

The two `ALLOW_LOCAL_NETWORK` and `ALLOW_LOOPBACK_HTTP` settings also apply to single sign-on set up in Settings.

</details>

<details class="ha-more"><summary>How single sign-on works</summary>

- The provider must support authorization code sign-in with PKCE S256 and RS256 tokens, with every endpoint on the issuer's origin.
- Accounts match by the provider's own account id, never by email.
- Provider roles and groups are not used.
- The client secret is stored encrypted and never shown again. Leave the field empty to keep it.

</details>

## Related

- [Users and roles](roles.md)
- [Sessions and passwords](sessions.md)
- [Kid accounts](kids.md)
- [URL base and reverse proxy](../run/url-base.md)
