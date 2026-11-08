# For app makers

You are building an app that talks to HolyArrs over MCP. This page lists what HolyArrs expects. For setup as a user, see [Connect AI apps](ai-connections.md).

AI apps are held back in this release. See [Held back](../help/held-back.md).

## Transport

MCP over Streamable HTTP. Send one JSON-RPC 2.0 message per `POST`. The answer is JSON in the same response. There is no event stream, and `GET` answers 405. Accepted protocol versions: `2025-11-25`, `2025-06-18`, `2025-03-26`. A message may be at most 64 KB.

## Addresses

| What | Address |
| --- | --- |
| AI address | `POST /api/v1/mcp` (also `/mcp`) |
| Protected resource metadata | `GET /.well-known/oauth-protected-resource/api/v1/mcp` |
| Sign-in server metadata | `GET /.well-known/oauth-authorization-server` |
| Register an app | `POST /oauth/register` |
| Start sign-in | `GET /oauth/authorize` |
| Get the key | `POST /oauth/token` |

Under a [URL base](../getting-started/url-base.md), every address starts with the base.

## Keys

Send the key as `Authorization: Bearer <key>` or `X-Api-Key: <key>`.

- No key: 401 with a `WWW-Authenticate` header that points to the protected resource metadata, scope `mcp`.
- Wrong or revoked key: 401 with `error="invalid_token"`.
- Key without the `mcp` scope: 403.
- AI apps switched off, valid AI key: 404.
- An `Origin` header naming another host: 403.

## Sign-in

- Registration is open. Send `client_name` and 1 to 10 `redirect_uris`. Each is `https`, `http` on a loopback address, or the app's own scheme. Apps are public, with no client secret.
- The flow is authorization code with PKCE `S256`, which is required. The only scope is `mcp`.
- The person signs in if needed, then presses "Allow" on `/ai`.
- The code works once and lasts 5 minutes.
- The `access_token` is the app's AI key. It does not expire and there is no refresh token. It lasts until someone revokes it.

## Limits

60 calls a minute and 5 requests an hour per key. Over that, the answer is 429 with `Retry-After` and JSON-RPC error `-32000`.
