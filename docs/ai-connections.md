# Connect AI apps

AI apps such as Claude, ChatGPT, Codex, Gemini or Cursor can use HolyArrs for you. Once connected, you can ask them things like "what's downloading?" or "is Dune in the Library?", and ask them to request a movie or show. They talk to HolyArrs over MCP, the common way AI apps reach outside tools.

This page covers what every app shares: turning it on, the two ways to connect, which address you need, what an AI app may do, and what to do when it fails. The steps for each app are under [Pick your app](#pick-your-app).

## Turn on AI connections

AI connections are off until the owner turns them on. While they are off, an app that sends an AI key gets 404 "not found". An app with no key yet is still told to sign in, so it can reach the "Connect an AI app" page.

1. Open Settings > AI connections.
2. Switch on "Let AI apps connect".

Only the owner can flip this switch. If someone else starts connecting an app while it is off, the "Connect an AI app" page tells them to ask the owner. If the owner connects an app while it is off, pressing "Allow" on that page turns it on.

The same card lists the ground rules:

- An AI app acts as the person who connected it. That person's permissions, request limit and kid account limits apply.
- Before it asks for a title, the AI app has to check with you.
- Each app gets at most 60 calls a minute and 5 requests an hour.

## The AI address

Every app connects to one address: HolyArrs's own address followed by `/api/v1/mcp`, for example:

```
http://10.10.25.25:18730/api/v1/mcp
https://holyarrs.example.com/api/v1/mcp
```

The short form `/mcp` works too. If HolyArrs runs under a [URL base](url-base.md) such as `/holyarrs`, the base comes first: `/holyarrs/api/v1/mcp`.

Settings > AI connections shows the address with a "Copy" button. It builds the address from the one in your browser, so **open that page on the same address the AI app will use**. The steps it shows also change with that address (see below).

## Two ways to connect

**Browser sign-in.** You add the address to the AI app. The app opens HolyArrs in your browser, you sign in with your HolyArrs password if asked, and you press "Allow" on the "Connect an AI app" page. That page names the app, the person it will act as, what it may do and where you go back to afterwards. The app then gets its own AI key. You never copy or paste a key.

**AI key.** You make a key in HolyArrs and the app sends it with every call, in the header `Authorization: Bearer <key>` (`X-Api-Key: <key>` works too). In the guide under Settings > AI connections, pick your app and press "Make a key": HolyArrs makes a key named after the app, fills it into the command or config shown, and shows it only that once. Copy the command then.

Never put an AI key in a link, and never paste it into a chat with the AI. It belongs only in the app's command or config file.

An AI key reaches only the AI address and nothing else in HolyArrs. Ordinary HolyArrs [API keys](parts-and-safe-mode.md#api-keys-and-scopes) do not work on the AI address.

### Which way fits your address

Browser sign-in works only when HolyArrs is on `https`, or on the same computer as the app (`localhost`). Newer AI apps refuse to sign in to a plain `http` address on the network, such as `http://10.10.25.25:18730`; Claude Code and Gemini CLI are two that refuse.

Cloud apps are a separate case. The Claude app (web, desktop and phone connectors), ChatGPT, the Gemini app, Perplexity, Le Chat and Copilot Studio connect from their maker's servers on the internet, not from your computer. They cannot reach a home address at all, so they need HolyArrs on a public `https` address.

| You open HolyArrs on | Apps on your computer | Cloud apps |
| --- | --- | --- |
| A public `https` name (reverse proxy, Cloudflare Tunnel or Tailscale Funnel) | Browser sign-in | Browser sign-in |
| A Tailscale `https` name that only your devices reach (Tailscale Serve) | Browser sign-in | Cannot reach it |
| `http://localhost:18730`, on the same computer as the app | Browser sign-in | Cannot reach it |
| A plain `http` home or Tailscale IP, such as `http://10.10.25.25:18730` | AI key | Cannot reach it |

The page under Settings > AI connections follows this table by itself, as far as it can tell from the address. On a plain `http` address it shows "Make a key" for apps on your computer, and for a cloud app it explains that the app cannot reach your address. It cannot tell whether an `https` name is open to the internet, so on a Tailscale Serve name or another home-only `https` name it still shows the cloud app steps; those apps will not reach it. One exception: Claude Desktop can still reach a home address through the `mcp-remote` local bridge, which needs Node.js (see [Pick your app](#pick-your-app)).

## Get an https or public address

You only need this for browser sign-in on another computer, or for cloud apps. Pick one:

- **Reverse proxy with a domain.** Caddy, Traefik, nginx-proxy-manager or similar, with a certificate, in front of HolyArrs. Cloud apps can reach it if the domain is open to the internet. See [Host names and reverse proxies](install.md#host-names-and-reverse-proxies) and [Running under a path](url-base.md).
- **Cloudflare Tunnel.** A `cloudflared` connector in your network gives HolyArrs a public `https` name without opening a port on your router. Cloud apps can reach it. (not tried live)
- **Tailscale Serve.** Gives HolyArrs an `https://<machine>.<tailnet>.ts.net` name with a real certificate, reachable only from your own Tailscale devices. Enough for browser sign-in in apps on those devices; cloud apps cannot reach it. For example `tailscale serve --bg 18730`. (not tried live)
- **Tailscale Funnel.** Opens that same `ts.net` name to the internet, so cloud apps can reach it. For example `tailscale funnel --bg 18730`. (not tried live)

Whichever you use, tell HolyArrs about it:

- Add the new name to `HOLYARRS_ALLOWED_HOSTS` (IP addresses and `localhost` need no entry).
- Add the address the proxy or tunnel connects from to `HOLYARRS_TRUSTED_PROXIES`, and make sure it sends `X-Forwarded-Proto: https`. Without this, HolyArrs does not know it is being reached over `https` and hands AI apps `http` sign-in addresses, which they refuse.

Cloud apps must reach the AI address and the sign-in addresses (see [For app makers](#for-app-makers)) without a login wall in front. A login screen such as Cloudflare Access in front of HolyArrs blocks them. (Perplexity is the one exception: it can pass Cloudflare Access with a service token.)

A public address puts the HolyArrs sign-in page on the internet. Use a strong password and [two-factor sign-in](users-and-roles.md#set-up-two-factor-sign-in).

## Pick your app

The full step-by-step guides are on the website: [holyarrs.coderluii.dev/docs/ai](https://holyarrs.coderluii.dev/docs/ai/). Settings > AI connections shows the same table with your own address and key filled in: pick a row to see that app's steps.

| App | Where to add HolyArrs | On a plain `http` home address |
| --- | --- | --- |
| Claude app (web, desktop, phone, Cowork) | Customize > Connectors > + Add > Add custom connector | Cannot reach it; needs a public `https` address |
| ChatGPT | Plugins > + > Add custom MCP server | Cannot reach it |
| Gemini app | Connected Apps > + Custom, with a client ID from Settings > AI connections | Cannot reach it |
| Grok | grok.com/connectors > New Connector > Custom | Cannot reach it |
| Perplexity | Plugins > Add custom > Add MCP connector > Remote | Cannot reach it |
| Claude Code | `claude mcp add` | AI key |
| Codex (terminal, IDE extension, ChatGPT desktop app) | `codex mcp add` | AI key |
| Cursor | Add to Cursor, or `~/.cursor/mcp.json` | AI key |
| VS Code with Copilot | MCP Servers > Add Server, or Add to VS Code | AI key |
| Gemini CLI | `gemini mcp add` | AI key |
| Claude Desktop at home | Settings > Developer > Edit Config, through the `mcp-remote` bridge (needs Node.js) | AI key |
| Open WebUI, LM Studio, Ollama (through ollmcp) | Each app's MCP settings | AI key |

Ollama runs the model but does not add MCP servers itself. Use it through an app that can, such as Open WebUI or ollmcp.

### Any other app

Any app that can add a remote (Streamable HTTP) MCP server works. Give it the AI address, then either sign in when asked or add the AI key as a header. Many use this shape:

```json
{
  "mcpServers": {
    "holyarrs": {
      "url": "http://10.10.25.25:18730/api/v1/mcp",
      "headers": {
        "Authorization": "Bearer <your AI key>"
      }
    }
  }
}
```

On a browser sign-in address, leave out `headers`.

## Check that it worked

Each app's steps end with its own check, such as `claude mcp list`. On the HolyArrs side, the app shows under Settings > AI connections > "Connected apps", with "last used" filled in after its first call.

Then try the first question, the same in every app:

> "What's downloading in HolyArrs?"

Settings > AI connections also has "Or let your agent do it": a short prompt you paste into Claude Code, Codex, Cursor or another agent so it adds HolyArrs itself. On a plain `http` address, after "Make a key", the prompt holds that app's key, so paste it only into your own agent.

## What AI apps can do

An AI app gets a small set of tools. Which ones it sees depends on the person it acts as and on which parts are on.

| Tool | What it does | Shown when |
| --- | --- | --- |
| `search_titles` | Finds movies and shows from a plain description, like "space movie from 2014". Shows what is in the Library and what could be asked for. | Movies, Shows or Music is on |
| `library_lookup` | Checks whether a movie, show or artist is in the Library, by name. | Movies, Shows or Music is on |
| `calendar` | What comes out in the next days: movies and episodes of shows in the Library, 1 to 60 days ahead (14 if the app does not say). | Movies, Shows or Music is on |
| `downloads` | What is downloading now and how far along it is. | Always |
| `my_asks` | Your own requests, their state, and how many more you may ask for. | Requests is on |
| `health` | Whether HolyArrs and the apps it uses are working. | The person may manage the Library |
| `ask_for_title` | Asks for a movie or a show for you, after you agree. | Requests is on and the person may ask for titles |

All tools except `ask_for_title` only read. Long lists are cut to 25 items and long texts are shortened.

### Asking for a title

Asking takes two steps. The AI app first gets a summary (name, year, a short description, and what will happen) and a one-time confirm token. It must show you that summary, and only after you agree call again with the token. The token works once, only for that same title, and lasts 5 minutes.

HolyArrs cannot see your chat, so this check relies on the AI app showing you the summary. Many apps also ask you before they use any tool.

A request made by an AI app is the same as one you make yourself: it counts against your request limit, and it follows your auto-approve setting. Approving and turning down requests stays with people, on the Requests page.

### What AI apps cannot do

- Approve or turn down requests.
- Change settings, people or keys.
- See other people's requests.
- Delete, rename or move anything, or start searches and downloads directly. (Once a request it made is approved, HolyArrs searches and downloads as for any request.)
- Do more than the person who connected them may do.

### Limits

Per app (per AI key): 60 calls a minute and 5 requests an hour. Over the limit, the app gets "too many calls from this key; try again in N seconds".

### History

Under Downloads > History, set Who to "AI apps" to see what AI apps did. Each request an AI app made shows as "By AI", with the app's name and the person it acted for. A request HolyArrs refused shows as "Not done", with the reason. An app that connects through browser sign-in also leaves an "AI app <name> signed in" row. That row belongs to the person who pressed "Allow", so it shows under Who "You" or "Members", not under "AI apps".

Look-ups (searches, the calendar, downloads) are not written to History. The "last used" time on the app's key shows when it was last active.

## Manage and revoke connected apps

Settings > AI connections > "Connected apps" lists every AI key: the app's name, the last four characters of the key, when it was made and when it was last used.

- Each app that connected through browser sign-in has its own key here, named after the app.
- To disconnect an app, press "Revoke". It stops working at once. To use it again, sign in from the app again or make a new key.
- "Make a key by hand" makes an AI key with a name you choose, for an app the guide does not list.
- The owner sees and can revoke every AI key, including ones made when another person's app signed in. Others with the permission to change settings see their own keys.
- Making, listing or revoking AI keys, and allowing an app through browser sign-in, all need the permission to change settings (the owner has it) and a recent sign-in with your password. People without it ask the owner to connect the app for them.
- A key stops working when the account it belongs to is turned off.

## Troubleshooting

| What you see | Why | What to do |
| --- | --- | --- |
| 404 or "not found" | AI connections are off, or the address is wrong. | The owner switches on "Let AI apps connect". Check the address ends in `/api/v1/mcp`. |
| 401, "sign in", "needs authentication" or "API key not accepted" | The app has no key yet, its key was revoked, or the account is turned off. | Sign in again from the app (each guide shows how), or make a new key and put it in place of the old one. |
| 403 "this API key's scope does not cover this route; make a key with the mcp scope" | The app sent an ordinary API key. | Make an AI key; ordinary API keys do not work on the AI address. |
| A cloud app says it cannot reach the server | It connects from the internet and cannot reach a home address. | Give HolyArrs a public `https` address (see above), or use an app that runs on your computer. |
| Sign-in is refused or fails on a plain `http` address | Claude Code, Gemini CLI and other newer apps refuse sign-in to plain `http` on the network. | Use "Make a key" on that address, or reach HolyArrs over `https` or `localhost`. |
| Sign-in fails behind a reverse proxy or tunnel | HolyArrs does not know it is on `https`, so it hands out `http` sign-in addresses. | Add the proxy to `HOLYARRS_TRUSTED_PROXIES` and have it send `X-Forwarded-Proto: https`. |
| Signing in on the new name says "request from another site or host name refused" | The public name is not on the allow-list. | Add it to `HOLYARRS_ALLOWED_HOSTS`. |
| "Could not connect the AI app": "This AI app is not registered with HolyArrs." | The app's sign-in details no longer match. | Remove HolyArrs in the AI app and add it again. |
| "Sign in with your password to manage AI keys." | You signed in another way, a while ago, or lack the permission to change settings. | Sign out and sign in with your password, or ask the owner. |
| "AI connections are off. Ask the owner to turn them on, then try again." | Shown on the "Connect an AI app" page. | The owner turns them on, then you start again from the AI app. |
| "too many calls from this key; try again in N seconds" | The app hit its limit. | Wait that long. |
| "that confirm_token is unknown, used or expired" | More than 5 minutes passed, or the token was already used. | Ask the AI app again; it gets a new summary and token. |
| A tool such as `ask_for_title` is missing | The person may not ask for titles, or Requests is off. | Check the person's permissions and the parts in Settings > Parts and controls. |
| The app never uses HolyArrs | The model may be too small to use tools well. | Use a model made for tool calls; the website's Local AI guide lists some. |

## For app makers

The AI address speaks MCP over Streamable HTTP: one JSON-RPC 2.0 message per `POST`, answered with JSON in the same response. There is no event stream; `GET` answers 405. Protocol versions `2025-11-25`, `2025-06-18` and `2025-03-26` are accepted.

| What | Address |
| --- | --- |
| AI address | `POST /api/v1/mcp` (also `POST /mcp`) |
| Protected resource metadata | `GET /.well-known/oauth-protected-resource/api/v1/mcp` (also `/.well-known/oauth-protected-resource` and `/.well-known/oauth-protected-resource/mcp`) |
| Sign-in server metadata | `GET /.well-known/oauth-authorization-server` |
| Register an app | `POST /oauth/register`, or an https `client_id` pointing to a client ID metadata document (public address, up to 5 KB, no redirects) |
| Start sign-in | `GET /oauth/authorize` |
| Get the key | `POST /oauth/token` |

- **Keys.** Send the AI key as `Authorization: Bearer <key>` or `X-Api-Key: <key>`. No key gives 401 with `WWW-Authenticate: Bearer resource_metadata="<origin>/.well-known/oauth-protected-resource/api/v1/mcp", scope="mcp"`. A wrong or revoked key gives 401 with `error="invalid_token"`. A key without the `mcp` scope gives 403. While AI connections are off, a call with a valid AI key gets 404 (a call with no key still gets the 401 above). A request whose `Origin` header names another host gets 403.
- **Registration** is open: send `client_name` and 1 to 10 `redirect_uris`, each `https`, `http` on `localhost` or a loopback IP, or the app's own scheme (such as `cursor://`). Apps are public: no client secret, `token_endpoint_auth_method` is `none`.
- **Sign-in** is the authorization code flow with PKCE `S256`, which is required. The only scope is `mcp`. HolyArrs sends the person to its sign-in page if needed, then to `/ai-connect`, where they press "Allow". On a loopback return address the port may differ from the registered one.
- **Token.** `POST /oauth/token` takes a form with `grant_type=authorization_code`, `code`, `code_verifier`, `client_id` and `redirect_uri`. The code works once and lasts 5 minutes. The answer's `access_token` is the app's own AI key, `token_type` `Bearer`, scope `mcp`. It does not expire and there is no refresh token; it lasts until someone revokes it.
- **Limits.** Over the limit, the answer is 429 with `Retry-After` and JSON-RPC error `-32000`. A message may be at most 64 KB.
- Under a [URL base](url-base.md), every address above starts with the base. Apps look for the sign-in server details at the root of the host first (`/.well-known/oauth-authorization-server/<base>`), so the proxy must pass that address on to HolyArrs, or use an AI key instead.
