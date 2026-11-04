# Security

## Reporting a problem

Please report security problems privately by email to
**holyarrs@coderluii.dev**. Do not open a public issue for them.

Tell us what you found, how to reproduce it, the HolyArrs version (the
`version` field of `/api/v1/health`, or the image tag) and how it is run
(Docker, bare binary, reverse proxy). We answer within 7 days, keep you
informed while we fix it, and credit you in the release notes unless you ask
us not to.

## Supported versions

Before v1, only the newest release gets security fixes. After v1, the newest
minor release does.

## How HolyArrs protects your setup

- **Sign-in on every page and API route**, except `/healthz` and `/login`.
  The first run needs a one-time setup code from the server log or
  `setup-code.txt` in the config folder (mode 0600); there is no default
  password. Because the code is printed to the log, finish the first-run
  setup before you send logs anywhere else (a log service, a bug report).
- **Passwords** are hashed with argon2id. Sessions use HttpOnly, SameSite=Lax
  cookies (Secure behind https; X-Forwarded-Proto counts only from a trusted
  proxy), rotate their id, and end after 7 idle days or
  30 days at most. Failed sign-ins back off, up to 15 minutes.
- **Changes need a CSRF token** and a same-origin request, and the Host header
  must be an IP address, `localhost` or a name listed in
  `HOLYARRS_ALLOWED_HOSTS`, which stops DNS rebinding.
- **API keys** are shown once and stored only as hashes.
- **Skip sign-in on the local network** is off by default. It applies only to
  the networks the owner lists (up to 32, like `192.168.1.0/24`), and cannot be
  turned on with an empty list. Installs that had it on before the list
  existed got the private, loopback and link-local ranges once, so narrow
  them. It never applies behind a proxy that is not in
  `HOLYARRS_TRUSTED_PROXIES`, and such requests cannot make API keys, change
  sign-in settings, change secrets or restore backups.
- **Secrets** (Source keys, Downloader passwords, the TMDB token) are encrypted
  at rest with AES-256-GCM. The master key lives in `holyarrs.key` (mode 0600)
  or the file named by `HOLYARRS_MASTER_KEY_FILE`, never in the database or in
  backups. API answers show secrets masked.
- **Sources cannot reach your local network** (loopback, private, link-local
  and CGNAT addresses) unless you allow it for that Source. The check runs on
  every connection, after DNS and on redirects.
- **Images:** none is published yet. Release images will be built for amd64,
  arm64 and armv7, signed with cosign (keyless, through GitHub's OIDC
  identity) and published with an SBOM.

## Checking an image

Once a release image is published, check it with:

    cosign verify ghcr.io/coderluii/holyarrs:<tag> \
      --certificate-identity-regexp 'https://github.com/CoderLuii/holyarrs/.github/workflows/release.yml@refs/tags/.*' \
      --certificate-oidc-issuer https://token.actions.githubusercontent.com

    cosign verify-attestation --type spdxjson ghcr.io/coderluii/holyarrs:<tag> \
      --certificate-identity-regexp 'https://github.com/CoderLuii/holyarrs/.github/workflows/release.yml@refs/tags/.*' \
      --certificate-oidc-issuer https://token.actions.githubusercontent.com
