# Connect ChatGPT and Codex

This guide connects HolyArrs to two OpenAI apps:

| App | Works on a home address (`http://10.10.25.25:18730`) | Works on a public https address |
| --- | --- | --- |
| [ChatGPT](#chatgpt) (chatgpt.com) | No | Yes, with browser sign-in |
| [Codex](#codex) (the terminal app, the IDE extension and the Codex app) | Yes, with an AI key | Yes, with browser sign-in |

For what the AI can and cannot do once it is connected, see [AI connections](ai-connections.md).

## Before you start

### What you need

- **ChatGPT:** a plan that lets you add your own MCP server. ChatGPT Business, Enterprise and Edu can. Pro may be able to. Plus is unclear and Free is not listed (not tried live). On Business only admins and owners can add one. On Enterprise and Edu an admin has to give you the "Create plugins with MCPs" permission first; it is off by default.
- **ChatGPT also needs** HolyArrs on a public https address. See [ChatGPT](#chatgpt) for why.
- **Codex:** any current version. Codex itself needs a sign-in with a ChatGPT plan that includes Codex, or an OpenAI API key. Adding HolyArrs needs no extra plan, beta switch or setting.
- **For an AI key** (Codex on a home address): making, listing and revoking AI keys needs the "Change settings" permission, which the owner has, and a sign-in with your password. Browser sign-in works without that permission.

### Turn on AI connections

AI connections are off until the owner turns them on. While they are off, an app that already has a key gets 404 "not found" from the AI address.

1. Open Settings > AI connections.
2. Switch on "Let AI apps connect".

Only the owner can change this switch. Under "Connect an AI app" the page shows your AI address, for example `http://10.10.25.25:18730/api/v1/mcp`. Copy it from there: it already includes your [URL base](url-base.md) if you use one.

### The two ways to connect

Which way you use depends on the address you open HolyArrs on.

| Your address | How the app connects |
| --- | --- |
| `https://...`, or `localhost` on the same computer | **Browser sign-in.** The app opens HolyArrs in your browser. You sign in with your HolyArrs password and press "Allow" on the "Connect an AI app" page. Nothing to paste. |
| Plain `http://` on your network, like `http://10.10.25.25:18730` or a Tailscale `100.x` address | **AI key.** You make an AI key in HolyArrs and put it in the app's settings. Only Codex can use this way; ChatGPT cannot reach a home address at all. |

Either way the app ends up with its own key. It is listed under Settings > AI connections > "Connected apps and AI keys", with when it was last used. Press "Revoke" there to disconnect the app. A key from browser sign-in is named after the app and lasts until you revoke it.

If AI connections are still off when you reach the "Allow" page, the page tells you. The owner can turn them on right there by pressing "Allow".

Never put a key in a link or paste it into a chat with the AI.

## ChatGPT

ChatGPT connects to HolyArrs from OpenAI's servers, not from your computer. So HolyArrs needs a public https address that the internet can reach, for example:

- your reverse proxy with a domain name,
- Cloudflare Tunnel,
- Tailscale Funnel.

A home address like `http://10.10.25.25:18730`, or a Tailscale address only your own devices can reach, does not work. On such an address the HolyArrs guide says so instead of showing steps.

ChatGPT only signs in through the browser (OAuth). It has no place for a key or a header, so AI keys do not work with ChatGPT.

OpenAI also offers a Secure MCP Tunnel for servers that are not public. The tunnel does not carry the sign-in step itself, so HolyArrs sign-in through it may fail; it is not tried live.

### Add HolyArrs

1. Open HolyArrs on its public https address, then Settings > AI connections. Pick ChatGPT and copy the address.
2. On chatgpt.com open Plugins. The "Open ChatGPT" button in the HolyArrs guide goes to `https://chatgpt.com/plugins`.
   - On some plans this lives under Settings > "Apps and Connectors" instead. If you see a "Developer mode" switch there, turn it on.
3. Press "+", then "Add custom MCP server".
4. Name it HolyArrs. Under "Connection", paste the address into "Server URL", for example `https://holyarrs.example.com/api/v1/mcp`.
5. For sign-in, choose "OAuth". Leave the client ID and secret fields empty.
6. Read the warning and press "I understand and want to continue", then "Create as a plugin".
7. Find HolyArrs among your plugins and install it. HolyArrs opens in your browser. Sign in if asked, then press "Allow".

Adding the server works on the web only. Whether HolyArrs then shows in the ChatGPT phone app is not tried live.

### Use it in a chat

Type `@HolyArrs` in the message box and pick it, then ask your question. The "Ask ChatGPT" button in the HolyArrs guide opens a new chat with the first question below already typed in; type `@HolyArrs` in front of it if ChatGPT does not use it on its own.

Looking things up runs straight away. Asking for a title is an action, so ChatGPT asks you before it runs it. You can let it remember your choice for the rest of that chat. HolyArrs adds its own check on top: the AI must show you what it will ask for before it can make the request.

### After HolyArrs updates

When a HolyArrs update adds or changes AI tools, ChatGPT keeps the old list until you refresh it. Open the HolyArrs plugin's page in ChatGPT, press "Refresh", then start a new chat. The same page lets you switch single tools on or off.

### Check that it worked

The HolyArrs plugin page lists its tools: `search_titles`, `library_lookup`, `calendar`, `downloads`, `my_asks`, `health` and `ask_for_title`. A tool is left out when you may not use it: `health` needs the "Run the Library" permission, `my_asks` needs the Requests part on, and `ask_for_title` also needs the "Ask for titles" permission. `search_titles`, `library_lookup` and `calendar` need Movies, Shows or Music on.

In HolyArrs, a new entry named after the app (the name ChatGPT gives itself) shows under "Connected apps and AI keys" with a recent "last used" time.

## Codex

The Codex terminal app (CLI), the IDE extension and the Codex app all read the same settings file, `~/.codex/config.toml` (Windows: `%USERPROFILE%\.codex\config.toml`). Add HolyArrs once and all three have it.

Codex runs on your own computer, so it can reach a home address as long as that computer can.

### On an https address: browser sign-in

1. Run this in a terminal, with your own address:

   ```
   codex mcp add holyarrs --url https://holyarrs.example.com/api/v1/mcp
   ```

2. Codex usually opens the browser by itself. If it does not, or later when you need to sign in again, run:

   ```
   codex mcp login holyarrs
   ```

   Tried live on 2026-11-03 against HolyArrs on `localhost`: `codex mcp add` found the sign-in by itself, the browser showed the HolyArrs Allow page, and after "Allow" Codex said it was logged in; `codex mcp list` showed the server with OAuth.

3. HolyArrs opens in your browser. Sign in if asked, then press "Allow".

You can also let Codex do this for you. The guide in Settings shows a short text under "Or let Codex set itself up". Paste it into Codex:

```
Set up the HolyArrs MCP server for me in Codex:
1. Add a remote MCP server named "holyarrs" at https://holyarrs.example.com/api/v1/mcp (Streamable HTTP), for all my projects.
2. Start its sign-in; I will sign in to HolyArrs in the browser and press Allow.
3. Tell me when its tools show up.
```

The text holds no key, so it is safe to paste into the chat.

### On a home address: an AI key

`codex mcp add` has no option for a header, so the key goes into the settings file.

1. In Settings > AI connections, pick Codex and press "Make a key for Codex". The key is filled into the block below and shown only this once.
2. Copy the block the guide shows and add it to `~/.codex/config.toml` (Windows: `%USERPROFILE%\.codex\config.toml`). It looks like this, with your key in place of `<your AI key>`:

   ```toml
   [mcp_servers.holyarrs]
   url = "http://10.10.25.25:18730/api/v1/mcp"
   http_headers = { "Authorization" = "Bearer <your AI key>" }
   ```

3. Restart Codex. In the IDE extension or the Codex app, restart the extension or the app.

HolyArrs also accepts the key in an `X-Api-Key` header. The block above uses `Authorization: Bearer`, which Codex treats as a key and so skips browser sign-in.

### Keep the key out of the settings file

The block above stores the key in plain text. To keep it in an environment variable instead, use `bearer_token_env_var`:

```toml
[mcp_servers.holyarrs]
url = "http://10.10.25.25:18730/api/v1/mcp"
bearer_token_env_var = "HOLYARRS_AI_KEY"
```

Or let Codex write that entry for you:

```
codex mcp add holyarrs --url http://10.10.25.25:18730/api/v1/mcp --bearer-token-env-var HOLYARRS_AI_KEY
```

Then set the variable:

- **Windows:** `setx HOLYARRS_AI_KEY "<your AI key>"`, then close and reopen the terminal, VS Code or the Codex app.
- **macOS and Linux:** add `export HOLYARRS_AI_KEY='<your AI key>'` to your shell profile and open a new terminal.

Codex reads the variable when it starts, so a running Codex does not see a new value.

### Other ways to add it

- **IDE extension and Codex app:** open the gear menu (in the app: Settings), then MCP servers > Add server. Enter the name `holyarrs`, choose "Streamable HTTP", paste the address and save. Restart the extension or the app. On an https address, press "Authenticate" when Codex asks you to sign in. These steps add no key, so on a home address use the file instead.
- **Only for one project:** a `.codex/config.toml` inside a project folder takes the same block, but Codex loads it only for projects you marked as trusted.
- **WSL:** Codex inside WSL reads its own `~/.codex/config.toml`, separate from the Windows one. Add HolyArrs there too.

Old guides on the web tell you to add `experimental_use_rmcp_client` or `rmcp_client` to the file. Current Codex does not need either.

### Check that it worked

```
codex mcp list
```

holyarrs should be listed with its address and how it signs in. If it says a sign-in is needed, run `codex mcp login holyarrs`. Inside Codex, type `/mcp` to see the servers it loaded.

## Try it

Ask this first. It is the same question in every guide:

> Use HolyArrs: what's downloading right now, and what's coming out this week?

The AI should use the `downloads` and `calendar` tools and answer from your HolyArrs.

## What ChatGPT and Codex can and cannot do

The app acts as the person who connected it, with that person's permissions and request limits.

- **It can look things up:** search for titles, check the Library, read the calendar, see downloads, see your own requests, and check health.
- **It can ask for a movie or show,** but only after checking with you. HolyArrs first gives the AI a summary to show you. The AI must ask you, then call again to make the request.
- **It cannot** approve or turn down requests, change settings, or see other people's requests. Approving requests stays with people, on the Requests page.
- **Limits per app:** at most 60 calls a minute and 5 requests an hour.
- **Its actions show in History:** every title it asks for, every action that fails, and the browser sign-in itself. Look-ups are not written to History. Pick "AI apps" as who to see only the AI events.

The full list is in [AI connections](ai-connections.md).

## Troubleshooting

| What you see | What to do |
| --- | --- |
| "not found" or 404 | AI connections are off. The owner turns on "Let AI apps connect" in Settings > AI connections. Also check the address ends in `/api/v1/mcp`, with your URL base in front if you use one. |
| ChatGPT cannot reach the server, or will not create the plugin | The address is not public https, or something in front of HolyArrs blocks OpenAI's servers: a login wall such as Cloudflare Access, a country block, or a firewall. |
| ChatGPT has no "Add custom MCP server" | Your plan or your workspace role does not allow it. On Business ask an admin or owner; on Enterprise and Edu ask an admin for the "Create plugins with MCPs" permission. |
| ChatGPT does not use HolyArrs in a chat | Type `@HolyArrs` and pick it before your question. |
| ChatGPT misses a new tool, or shows an old one | Open the HolyArrs plugin's page, press "Refresh" and start a new chat. |
| ChatGPT stops working after you pressed "Revoke" in HolyArrs | That removed its key. Sign in again from the HolyArrs plugin in ChatGPT, or remove the plugin and add it again (not tried live). |
| Codex sign-in fails, or never opens the browser, on an `http://` address | The HolyArrs guide only offers browser sign-in on https or `localhost`. Use an AI key in `config.toml` instead, or put HolyArrs on https. |
| "use an AI key (Settings > AI connections)" | The key is a normal HolyArrs API key. Only AI keys work here. Make one in Settings > AI connections. |
| "API key not accepted" | The key was revoked or copied wrong. Make a new one and update `config.toml` or the variable. |
| "Sign in with your password to manage AI keys." when making a key | You opened HolyArrs without a password (Settings > Sign-in > Local network), or you lack the "Change settings" permission. Sign in with your password, or ask the owner. Allowing an app through browser sign-in needs the same permission. |
| "too many calls from this key; try again in ... seconds" | The look-up or action limit was reached. Wait the time shown. |
| Codex with `bearer_token_env_var` is refused | Codex started before the variable was set. On Windows, `setx` reaches only newly started programs: close and reopen the terminal, VS Code or the Codex app. |
| Codex shows holyarrs as failed when it starts | HolyArrs took too long to answer. Add `startup_timeout_sec = 30` under `[mcp_servers.holyarrs]` (the default is 10). |
| Codex ignores the key and asks you to sign in, or shows "sign in: add HolyArrs in your AI app and allow it in the browser" | The `[mcp_servers.holyarrs]` block is missing `http_headers` or `bearer_token_env_var`, or it sits in a different `config.toml` (WSL, a project file, or a moved `CODEX_HOME`). |
| An app you no longer use is still listed | Press "Revoke" next to it under "Connected apps and AI keys". |
