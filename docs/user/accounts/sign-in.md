# Sign-in and passwords

You want to decide how people sign in to HolyArrs: how long passwords must be,
whether devices at home can skip the password, and whether people can use an
outside sign-in such as single sign-on. Only the owner changes these, under
**Settings > Sign-in and security**.

## Set the shortest password

1. Open **Settings > Sign-in and security > Passwords**.
2. Type a number from 4 to 64. The default is 8.
3. Save.

The minimum applies to every new password: the first owner, accepted invites,
password changes on My account, and `holyarrs reset-owner`. Passwords people
already have keep working. The new minimum applies the next time they change
it. The longest password is 1024 characters.

Short passwords are easy to guess. HolyArrs slows down repeated wrong
sign-ins, but that does not make a short password safe. If HolyArrs can be
reached from outside your home, use 12 or more.

## Skip sign-in on the local network

This lets devices on your home network open HolyArrs as the owner without a
password. It is off by default.

1. Open **Settings > Sign-in and security > Local network**.
2. List your networks, one per line. Use a range like `192.168.1.0/24` or
   `fd00::/8`, or one address like `192.168.1.20`. Up to 32 entries.
3. Turn the switch on and save.

The switch cannot be on with an empty list. Making API keys, changing sign-in
settings, reading or replacing secrets and restoring backups still need a
password.

You cannot turn this on while any Kid account exists. See
[Kid accounts](kid-accounts.md).

### Check the address first

The card shows the address HolyArrs sees for your browser. Some setups make
every device look like one gateway address. This happens with Docker Desktop,
rootless Docker or Podman, and Kubernetes NodePort. Typical addresses are
`172.17.0.1`, `172.18.0.1` and up, `10.88.0.1`, `10.89.x.1`, `10.0.2.2` or
`192.168.65.x`.

If you list such an address, anyone who can reach HolyArrs gets in as the
owner. The card warns when an entry looks like one. To check, open the page
from a second device. If it shows the same address, turn the switch off and
remove that entry.

Behind a reverse proxy, HolyArrs checks the address the proxy forwards, but
only when the proxy is listed in `HOLYARRS_TRUSTED_PROXIES`. If it is not, the
card shows no address and says the proxy is not trusted.

## Single sign-on

HolyArrs can let people sign in through one OpenID Connect provider. It shows
as a button on the sign-in page.

### Set it up in Settings

1. Open **Settings > Sign-in and security > Single sign-on**.
2. Type the provider's issuer and select **Test**. No secret is sent for the
   test.
3. Type the client id and client secret. Tick **Public client** if the
   provider uses no secret.
4. Copy the **Callback address** and register that exact address at the
   provider.
5. Type your password, and a two-factor code if you use one, then select
   **Save**. Every change asks for this.

What to know:

- The secret is stored encrypted and never shown again. Leave the field empty
  to keep it.
- Changing the issuer or client id needs the secret typed again. It also
  removes the old links and signs out everyone who used single sign-on. They
  link again from My account.
- Letting new people join asks you to confirm, because anyone the provider
  signs in could then make an account. Newcomers join as members who can only
  ask.
- Turning single sign-on off signs out everyone who signed in with it.
- If `HOLYARRS_OIDC_ENABLED` is set at all, even to `false`, the environment
  controls single sign-on and Settings only shows it.

HolyArrs never makes someone an owner or admin through single sign-on. It
matches accounts by the provider's own account id, never by email.

### Link it to your account

1. Sign in to HolyArrs with your password.
2. Open **My account**.
3. Confirm your password, and a two-factor code if you use one.
4. Select **Link single sign-on** or **Link trusted proxy sign-in**.

### Unlink

- **My account > Linked sign-ins > Unlink** removes your own link. It needs a
  recent password confirmation, and it is refused if it is your only way in.
- On **Settings > Users and roles**, the owner can unlink anyone. If it is their only
  way in, HolyArrs asks once more.

Unlinking signs out the devices that used it. If joining is open, the same
person can come back as a new member. Turn joining off to keep them out.

## Trusted proxy sign-in

If a reverse proxy already signs people in, HolyArrs can accept the name it
passes along. This is set only in the environment, never in the browser. It
shows as a button on the sign-in page. Visiting HolyArrs never signs anyone in
from proxy headers alone.

The proxy must remove incoming `Remote-User` and `Remote-Name` headers and set
its own after sign-in. `Remote-User` holds one stable name, at most 255
bytes. `Remote-Name` is an optional display name, at most 200 bytes. Repeated,
comma-joined or control-character values are refused. Role headers are
ignored.

Keep HolyArrs reachable only through that proxy. Trusting a proxy address
means trusting every name it sends.

## Environment settings

Set these before you start HolyArrs. Both methods are off unless you turn
them on. A bad setup stops HolyArrs from starting. A provider outage leaves
password sign-in working.

| Variable | Meaning |
| --- | --- |
| `HOLYARRS_OIDC_ENABLED` | Turn on single sign-on. Default `false`. |
| `HOLYARRS_OIDC_ISSUER` | The provider's exact HTTPS issuer. |
| `HOLYARRS_OIDC_CLIENT_ID` | The client id you registered. |
| `HOLYARRS_OIDC_CLIENT_SECRET_FILE` | Optional file holding the client secret, at most 4096 bytes. Leave out for a public client. |
| `HOLYARRS_OIDC_REDIRECT_URL` | The exact HTTPS callback: your address, its URL base if any, then `/api/v1/auth/oidc/callback`. Register the same value at the provider. |
| `HOLYARRS_OIDC_ALLOW_JOIN` | Let new people join as members who can only ask. Default `false`. |
| `HOLYARRS_OIDC_ALLOW_LOCAL_NETWORK` | Allow a provider on your private network. Default `false`. |
| `HOLYARRS_OIDC_ALLOW_LOOPBACK_HTTP` | Allow plain HTTP on loopback, for testing only. Default `false`. |
| `HOLYARRS_FORWARD_AUTH_ENABLED` | Turn on trusted proxy sign-in. Default `false`. |
| `HOLYARRS_FORWARD_AUTH_ISSUER` | A stable HTTPS name for this proxy. Changing it counts as a new source. |
| `HOLYARRS_FORWARD_AUTH_TRUSTED_PEERS` | Comma-separated proxy addresses or ranges. Required. An all-address range is refused. |
| `HOLYARRS_FORWARD_AUTH_ALLOW_JOIN` | Let new proxy names join as members who can only ask. Default `false`. |

The two `ALLOW_LOCAL_NETWORK` and `ALLOW_LOOPBACK_HTTP` settings also apply
when you set up single sign-on in Settings.

The provider must support authorization code sign-in with PKCE S256 and RS256
tokens, with all its endpoints on the issuer's origin. HolyArrs does not use
provider roles or groups.

## Troubleshooting

**The provider is down.** Sign in with your password. Always keep a working
owner password, even if everyone normally uses single sign-on.

**The owner password is lost.** Run `holyarrs reset-owner` on the machine
that runs HolyArrs. See [Signed-in devices and passwords](sessions.md).

**Signing out did not sign me out of the provider.** HolyArrs ends only its
own sign-in. On a shared device, sign out at the provider too.

**HolyArrs asks for my password again.** Changes to accounts, keys and
settings need a password confirmation from the last ten minutes, plus
two-factor if you use it. Outside sign-in and the local network shortcut do
not count. Signing out and ending devices never ask.

**An account made through single sign-on cannot set a password.** That is not
possible in this version. Make the local owner first, then link single
sign-on to it from My account.

**A callback failed.** You stay signed out and see a short error. Check that
the callback address at the provider matches exactly.
