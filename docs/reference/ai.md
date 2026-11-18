---
title: AI apps
description: The MCP plugin: let AI apps reach HolyArrs over MCP, AI keys, and what app makers need to know.
level: Expert
sidebar: { order: 4 }
---

Ask an AI app "what's downloading?" or have it request a movie, over MCP.

:::caution
AI apps are held back in this release. You see them only when the `ai_apps` feature is switched on.
:::

## Turn on the MCP plugin

HolyArrs has no per-app guides. The MCP plugin is the whole feature: turn it on, copy the address and give it to your AI app.

1. Open **Settings > AI apps** (`/settings/ai`). This is where the MCP plugin is turned on and AI keys are made.
2. Switch on **Let AI apps connect**. Only the owner can.
3. Copy the **AI address** and add it in your AI app as a remote MCP server (Streamable HTTP).
4. The app signs in through your browser and you click **Allow**, or you make an AI key under **Linked AI apps > Make a key by hand** and the app sends it as a header.

The AI address is your HolyArrs address plus `/api/v1/mcp`, such as `http://your-server:6262/api/v1/mcp`. `/mcp` works too. Under a URL base, put the base first.

## Choices

### Browser sign-in or AI key

| Way | How | Needs |
|---|---|---|
| Browser sign-in | Add the address; the app opens HolyArrs, you click **Allow**. The app gets its own key. | `https`, or the app on the same computer (`localhost`) |
| AI key | Under **Linked AI apps**, open **Make a key by hand**, copy the key once into the app's config. | Any address |

| You open HolyArrs on | Apps on your computer | Cloud apps (Claude app, ChatGPT, Gemini app, Grok, Perplexity) |
|---|---|---|
| A public `https` name | Browser sign-in | Browser sign-in |
| An `https` name only your devices reach | Browser sign-in | Can't reach it |
| `localhost` | Browser sign-in | Can't reach it |
| Plain `http`, such as `http://your-server:6262` | AI key | Can't reach it |

Cloud apps can't reach a home-only `https` name. Claude Desktop can reach a home address through the `mcp-remote` bridge, which needs Node.js.

Keep an AI key out of links and chats; it belongs in the app's config. AI keys work only on the AI address, and ordinary API keys don't work there.

### Getting an https address

For cloud apps or browser sign-in from another computer: a reverse proxy with a certificate, Cloudflare Tunnel (public, no open port), Tailscale Serve (your devices only) or Tailscale Funnel (public). Then:

- Add the name to `HOLYARRS_ALLOWED_HOSTS`.
- Add the proxy to `HOLYARRS_TRUSTED_PROXIES` and send `X-Forwarded-Proto: https`. Without it, HolyArrs hands out `http` sign-in addresses and apps refuse them.

:::caution
Cloud apps need HolyArrs with no login screen in front, which puts your sign-in page on the internet. Use a strong password and two-factor sign-in.
:::

### What an app can do

It acts as the person who connected it, with their permissions, request limit and kid account limits.

| Tool | Does | Shown when |
|---|---|---|
| `search_titles` | Finds movies and shows from plain words | Movies, TV shows or Music is on |
| `library_lookup` | Checks if a title is in the Library | Movies, TV shows or Music is on |
| `calendar` | What comes out in 1 to 60 days (default 14) | Movies, TV shows or Music is on |
| `downloads` | What is downloading and how far | Always |
| `my_asks` | Your requests and how many more you may make | Requests is on |
| `health` | Whether HolyArrs and its apps work | The person may manage the Library |
| `ask_for_title` | Requests a movie or show, after you agree | Requests is on and the person may request |

Only `ask_for_title` changes anything. It counts as your own request. An app can't approve requests, change settings, people or keys, see others' requests, or delete or move files.

### What each AI key may do

Each AI key also has seven tick boxes, a second limit on top of the tools above. They're set when the key is made; the owner or an admin can change them later with **Change** next to the key, and the change counts from the key's next call. The key still can't do more than the person who made it.

| Tick box | Lets the app | On for new keys |
|---|---|---|
| See library and activity | Look at your library, calendar, downloads and history | Yes |
| Add and request | Add titles, ask for titles, follow and report | Yes |
| Control downloads | Start searches, pause or move downloads, rename and import files | Yes |
| Change settings | Change downloaders, sources, folders, alerts and other settings | Yes |
| Delete | Remove or delete anything | No |
| Admin | Users, sign-in, keys, backups, updates, restarts and logs | No |
| Approve requests | Approve or decline pending requests, without full Admin (Admin can do this too) | No |

When an app tries something its key doesn't allow, it gets a message naming the tick box, such as "This key can't Delete."

## If it does not work

| Symptom | Fix |
|---|---|
| 404 or "not found" | Switch on **Let AI apps connect**. Check the address ends in `/api/v1/mcp`. |
| 401 or "sign in" | Key missing, revoked or its account is off. Sign in again from the app, or make a new key. |
| 403 about the `mcp` scope | The app sent an ordinary API key. Make an AI key. |
| Sign-in refused on plain `http` | Use an AI key from **Make a key by hand**, or reach HolyArrs over `https` or `localhost`. |
| Sign-in fails behind a proxy | Set `HOLYARRS_TRUSTED_PROXIES` and send `X-Forwarded-Proto: https`. |
| "request from another site or host name refused" | Add the name to `HOLYARRS_ALLOWED_HOSTS`. |
| "This AI app is not registered with HolyArrs." | Remove HolyArrs from the app and add it again. |
| "Sign in with your password to manage AI keys." | Sign out and back in with your password, or ask the owner. |
| A tool is missing | Check the person's permissions and which parts are on. |
| "too many calls from this key" | Wait the seconds shown. |
| "that confirm_token is unknown, used or expired" | Ask the app again. |
| The app never uses HolyArrs | The model may be too small for tools. Try a bigger one. |

## For experts

<details class="ha-more"><summary>Linked AI apps and History</summary>

- The card lists each AI key: name, last 4 characters, made and last used.
- **Revoke** stops an app at once. **Make a key by hand** names a new key.
- What an AI key may do: in **Settings > AI apps** (`/settings/ai`), card **Linked AI apps**, open **Make a key by hand**. Seven tick boxes: **See library and activity**, **Add and request**, **Control downloads**, **Change settings**, **Delete**, **Admin (users, keys, backups, updates)** and **Approve requests**. The first four start ticked. The list shows each key's boxes in short form, such as *Can: See · Add · Downloads · Settings · Approve*. The owner and admins can change a key's boxes with **Change** next to it (`PUT /api/v1/auth/keys/{id}` with `mcp_groups`, signed in, not with a key); its scopes stay fixed.
- The owner sees every AI key; others who may change settings see their own. Managing keys needs that permission and a recent password sign-in.
- A key stops working when its account is turned off.
- While **Let AI apps connect** is off, a keyed app gets not found; an app without a key is still sent to sign in. The owner clicking **Allow** turns it on.
- In **Downloads > History**, set **Who** to **AI apps**. Requests show as **By AI** with the app and person; refused ones as **Not done** with the reason. Look-ups are not written.

</details>

<details class="ha-more"><summary>Config shape for other apps</summary>

```json
{
  "mcpServers": {
    "holyarrs": {
      "url": "http://your-server:6262/api/v1/mcp",
      "headers": { "Authorization": "Bearer <your AI key>" }
    }
  }
}
```

With browser sign-in, leave out `headers`.

</details>

<details class="ha-more"><summary>For app makers: transport and addresses</summary>

MCP over Streamable HTTP: one JSON-RPC 2.0 message per `POST`, answered as JSON in the same response. No event stream; `GET` answers 405. Protocol versions `2025-11-25`, `2025-06-18`, `2025-03-26`. Messages up to 64 KB.

| What | Address |
|---|---|
| AI address | `POST /api/v1/mcp` (also `/mcp`) |
| Protected resource metadata | `GET /.well-known/oauth-protected-resource/api/v1/mcp` |
| Sign-in server metadata | `GET /.well-known/oauth-authorization-server` |
| Register an app | `POST /oauth/register` |
| Start sign-in | `GET /oauth/authorize` |
| Get the key | `POST /oauth/token` |

Under a URL base, every address starts with the base.

</details>

<details class="ha-more"><summary>For app makers: keys, sign-in and limits</summary>

Send `Authorization: Bearer <key>` or `X-Api-Key: <key>`.

| Case | Answer |
|---|---|
| No key | 401, `WWW-Authenticate` pointing to the protected resource metadata, scope `mcp` |
| Wrong or revoked key | 401, `error="invalid_token"` |
| Key without `mcp` scope | 403 |
| AI apps off, valid AI key | 404 |
| `Origin` naming another host | 403 |
| Over 60 calls a minute or 5 requests an hour per key | 429 with `Retry-After`, JSON-RPC error `-32000` |

- Registration is open: `client_name` and 1 to 10 `redirect_uris`, each `https`, `http` on loopback, or the app's own scheme. Apps are public, with no client secret.
- Authorization code with PKCE `S256` (required). The only scope is `mcp`. The person clicks **Allow** on `/ai`.
- The code works once, for 5 minutes. The `access_token` is the app's AI key: no expiry, no refresh token, valid until revoked.
- `ask_for_title` takes 2 calls: the first returns a summary and a confirm token the app must show you; the second sends the token. It works once, for that title, for 5 minutes. Lists are cut to 25 items.

</details>

## Related

- [Reverse proxy and URL base](../run/url-base/)
- [Parts and safe mode](../run/parts-safe-mode/)
- [Sign-in](../users/sign-in/)
