# Connect Claude

This guide connects HolyArrs to Claude in three ways:

| Claude | Works on a home address (`http://10.10.25.25:18730`) | Works on a public https address |
| --- | --- | --- |
| [Claude Code](#claude-code) (the terminal app and its VS Code and JetBrains extensions) | Yes, with an AI key | Yes, with browser sign-in |
| [The Claude app](#the-claude-app-web-desktop-and-phone) (claude.ai, the desktop app, the phone app) | No | Yes, with browser sign-in |
| [Claude Desktop through the local bridge](#claude-desktop-on-a-home-address) | Yes, with an AI key | Yes, with an AI key |

For what the AI can and cannot do once it is connected, see [AI connections](ai-connections.md).

## Before you start

### What you need

- **Claude Code:** any install. Adding your own server needs no particular plan.
- **The Claude app:** custom connectors work on every plan. The Free plan allows one custom connector. On Team and Enterprise plans only an owner can add one (see [Team and Enterprise](#team-and-enterprise)).
- **Claude Desktop through the bridge:** the Claude desktop app on Windows, macOS or Linux, and [Node.js](https://nodejs.org) installed, so that `npx` works in a terminal.

### Turn on AI connections

AI connections are off until the owner turns them on. While they are off, the AI address answers 404 "not found".

1. Open Settings > AI connections. HolyArrs may ask for your password again before it shows the page.
2. Switch on "Let AI apps connect".

Only the owner can change this switch. Under "Connect an AI app" the page shows your AI address, for example `http://10.10.25.25:18730/api/v1/mcp`. Copy it from there: it already includes your [URL base](url-base.md) if you use one.

### The two ways to connect

Which way you use depends on the address you open HolyArrs on.

| Your address | How Claude connects |
| --- | --- |
| `https://...`, or `localhost` on the same computer | **Browser sign-in.** Claude opens HolyArrs in your browser. You sign in with your HolyArrs password and press "Allow" on the "Connect an AI app" page. Nothing to paste. |
| Plain `http://` on your network, like `http://10.10.25.25:18730` or a Tailscale `100.x` address | **AI key.** Claude Code refuses browser sign-in to a plain http address that is not this computer. You make an AI key in HolyArrs and put it in Claude's settings. |

Either way the app ends up with its own key. It is listed under Settings > AI connections > "Connected apps and AI keys", with when it was last used. Press "Revoke" there to disconnect the app.

An AI key works only on the AI address. It cannot sign in to HolyArrs or use the rest of the API. Keep it out of links, and never paste it into a chat with the AI.

**Make a key.** On an address where browser sign-in does not work, Settings > AI connections shows a key button under "Connect an AI app":

- Pick Claude Code and press "Make a key for Claude Code". The key is named "Claude Code".
- For Claude Desktop, pick "More apps" and press "Make an AI key". This key is named "AI app".

HolyArrs fills the key into the command or file shown and shows it only this once, so copy the command or file then. To choose the name yourself, or to make a key on an https address (for the bridge), use "Make a key by hand" under "Connected apps and AI keys".

Making AI keys needs the permission to change settings (the owner has it) and a recent sign-in with your password. Allowing an app through browser sign-in needs the same permission.

## Claude Code

Claude Code's VS Code and JetBrains extensions read the same settings as the terminal app, so you add HolyArrs once. The commands below add it for all your projects (`--scope user`).

### On an https address: browser sign-in

1. Run this in a terminal, with your own address:

   ```
   claude mcp add --transport http --scope user holyarrs https://holyarrs.example.com/api/v1/mcp
   ```

2. Start Claude Code and type `/mcp`. Pick holyarrs and choose Authenticate. You can also run `claude mcp login holyarrs` from the terminal instead. Tried live on 2026-11-03 against HolyArrs on `localhost`: Claude Code registered itself, the browser showed the HolyArrs Allow page, and after "Allow" it said the tools were available; HolyArrs listed a key named after it.
3. HolyArrs opens in your browser. Sign in if asked, then press "Allow".

If AI connections are still off, the "Allow" page tells you. The owner can turn them on right there: pressing "Allow" switches them on.

You can also let Claude Code do the first two steps. The guide in Settings shows a short text under "Or let Claude Code set itself up". Paste it into Claude Code:

```
Set up the HolyArrs MCP server for me in Claude Code:
1. Add a remote MCP server named "holyarrs" at https://holyarrs.example.com/api/v1/mcp (Streamable HTTP), for all my projects.
2. Start its sign-in; I will sign in to HolyArrs in the browser and press Allow.
3. Tell me when its tools show up.
```

The text holds no key, so it is safe to paste into the chat.

### On a home address: an AI key

1. In Settings > AI connections, pick Claude Code and press "Make a key for Claude Code".
2. Copy the command the guide shows and run it in a terminal. It looks like this, with your key in place of `<your AI key>`:

   ```
   claude mcp add --transport http --scope user holyarrs http://10.10.25.25:18730/api/v1/mcp --header "Authorization: Bearer <your AI key>"
   ```

3. Start Claude Code and type `/mcp`. holyarrs shows as connected.

This works on a plain http home address: tried live with Claude Code on 2026-11-03, where `claude mcp list` showed holyarrs as connected.

HolyArrs also accepts the key in an `X-Api-Key` header. The guide uses `Authorization: Bearer`, the usual form for AI apps.

### Keep the key out of the settings file

The command above stores the key in plain text in `~/.claude.json` (on Windows `%USERPROFILE%\.claude.json`). If you would rather keep it in an environment variable, use a `.mcp.json` file in your project folder instead of the command:

```json
{
  "mcpServers": {
    "holyarrs": {
      "type": "http",
      "url": "http://10.10.25.25:18730/api/v1/mcp",
      "headers": {
        "Authorization": "Bearer ${HOLYARRS_AI_KEY}"
      }
    }
  }
}
```

- Claude Code fills in `${HOLYARRS_AI_KEY}` each time it connects. Set that variable in your shell profile (macOS and Linux) or with `setx HOLYARRS_AI_KEY "<your AI key>"` (Windows, then open a new terminal). If the variable is not set, the text `${HOLYARRS_AI_KEY}` is sent as it is and HolyArrs answers "API key not accepted".
- Keep `"type": "http"`. Without it Claude Code treats the entry as a local program and fails.
- Do not use `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN` as the variable: Claude Code leaves those empty in server settings. A name of your own, like `HOLYARRS_AI_KEY`, is safe.
- A `.mcp.json` file applies to that project only. The first time you start `claude` there, it asks you to approve the server.
- Do not write `$HOLYARRS_AI_KEY` into the `claude mcp add` command. Your shell fills it in before Claude Code sees it, so the real key ends up in the file anyway.

### Check that it worked

```
claude mcp list
```

holyarrs should say Connected. "Needs authentication" means the sign-in step is still open. "Failed to connect" is covered under [Troubleshooting](#troubleshooting).

## The Claude app: web, desktop and phone

The Claude app connects to HolyArrs from Anthropic's servers, not from your computer. Even the desktop and phone apps do this. So HolyArrs needs a public https address that the internet can reach, for example:

- your reverse proxy with a domain name,
- Cloudflare Tunnel,
- Tailscale Funnel (not tried live).

Tell HolyArrs about that address: add the name to `HOLYARRS_ALLOWED_HOSTS`, add the proxy or tunnel to `HOLYARRS_TRUSTED_PROXIES`, and have it send `X-Forwarded-Proto: https`. Without the last two, HolyArrs hands Claude `http` sign-in addresses and the sign-in fails. See [Get an https or public address](ai-connections.md#get-an-https-or-public-address).

A home address like `http://10.10.25.25:18730`, or a Tailscale address only your own devices can reach, does not work. On such an address the HolyArrs guide says so instead of showing steps. To use Claude Desktop on your home network, see [Claude Desktop on a home address](#claude-desktop-on-a-home-address).

### Add the connector

1. Open HolyArrs on its public https address, then Settings > AI connections. Pick "Claude app" and copy the address.
2. In Claude (web or desktop) open Customize > Connectors and choose "Add custom connector" (the "+" button). Older Claude versions have this under Settings > Connectors. The "Open Claude connectors" button in the HolyArrs guide opens that page on claude.ai.
3. Name it HolyArrs and paste the address, for example `https://holyarrs.example.com/api/v1/mcp`.
4. For sign-in, pick "Sign in now" and leave the client choice as it is: HolyArrs registers Claude by itself. Add the connector.
5. Press Connect. HolyArrs opens in your browser. Sign in if asked, then press "Allow".

Adding the connector from the phone app is not described by Claude (not tried live). Add it on the web or the desktop app; it then shows on your phone too, on the same Claude account.

If you also use Claude Code signed in with the same Claude subscription, the connector can show up there as well.

### Team and Enterprise

On these plans an owner adds the connector once for everyone, under Organization settings > Connectors. Each person then finds HolyArrs, marked as custom, in their own Customize > Connectors list and presses Connect, which signs them in to HolyArrs as themselves.

### Use it in a chat

Each chat picks which connectors it uses. In the message box, press "+", open Connectors and switch HolyArrs on. The first time Claude uses a HolyArrs tool, it asks you to allow it once or always.

The "Ask Claude" button in the HolyArrs guide opens a new chat with the first question below already typed in.

### Check that it worked

The HolyArrs connector shows as connected under Customize > Connectors (older versions: Settings > Connectors), and its tools are listed under it.

## Claude Desktop on a home address

On a home address, Claude Desktop can still reach HolyArrs through a small local bridge called `mcp-remote`. The bridge runs on your computer, so it can reach your network. It needs Node.js.

1. In Settings > AI connections, pick "More apps" and press "Make an AI key". Open "Claude Desktop (local bridge)" and copy the entry. Your key is already in it.
2. In Claude Desktop open Settings, then Developer, and press Edit config. This opens `claude_desktop_config.json`:

   | System | File |
   | --- | --- |
   | Windows | `%APPDATA%\Claude\claude_desktop_config.json` |
   | macOS | `~/Library/Application Support/Claude/claude_desktop_config.json` |
   | Linux | `~/.config/Claude/claude_desktop_config.json` |

3. Paste the entry. It looks like this, with your address and key:

   ```json
   {
     "mcpServers": {
       "holyarrs": {
         "command": "npx",
         "args": [
           "-y",
           "mcp-remote@latest",
           "http://10.10.25.25:18730/api/v1/mcp",
           "--transport",
           "http-only",
           "--allow-http",
           "--header",
           "Authorization:${AUTH_HEADER}"
         ],
         "env": {
           "AUTH_HEADER": "Bearer <your AI key>"
         }
       }
     }
   }
   ```

   If the file already has other servers under `mcpServers`, add the `holyarrs` part next to them instead of replacing the file.

4. Quit Claude Desktop fully and start it again. Closing the window is not enough. On Windows, quit it from the icon by the clock. On macOS, quit it from the menu bar.

About the entry:

- `--allow-http` lets the bridge use a plain http address. It is left out when your address starts with `https://`.
- `Authorization:${AUTH_HEADER}` has no space after the colon on purpose. Claude Desktop on Windows splits arguments at spaces, so the full `Bearer <key>` value sits in `env` instead.
- The key is stored in plain text in this file. Revoke it in HolyArrs if the file is ever shared.
- On an https address the "Make an AI key" button is not shown and the entry holds `<your AI key>`. Make a key with "Make a key by hand" and put it in place of `<your AI key>`.

### Check that it worked

Open a new chat and open the tools menu in the message box: holyarrs should be listed with its tools (not tried live). If it is missing, see [Troubleshooting](#troubleshooting).

## Try it

Ask this first. It is the same question in every guide:

> Use HolyArrs: what's downloading right now, and what's coming out this week?

Claude should use the `downloads` and `calendar` tools and answer from your HolyArrs.

## What Claude can and cannot do

Claude acts as the person who connected it, with that person's permissions and request limits.

- **It can look things up:** search for titles, check the Library, read the calendar, see downloads and see your own requests. People who may manage the Library can also check health.
- **It can ask for a movie or show,** if you may ask for titles, but only after checking with you. HolyArrs first gives Claude a summary to show you. Claude must ask you, then call again within 5 minutes to make the request.
- **It cannot** approve or turn down requests, change settings, or see other people's requests. Approving requests stays with people, on the Requests page.
- **Limits per app:** at most 60 calls a minute and 5 requests an hour.
- **Every request it makes shows in History,** with the app and its key name. Look-ups are not written to History; the key's "last used" time shows when the app was last active.

The full list is in [AI connections](ai-connections.md#what-ai-apps-can-do).

## Troubleshooting

| What you see | What to do |
| --- | --- |
| "not found" or 404 | AI connections are off. The owner turns on "Let AI apps connect" in Settings > AI connections. Also check the address ends in `/api/v1/mcp`, with your URL base in front if you use one. |
| "sign in: add HolyArrs in your AI app and allow it in the browser" | Claude sent no key. On a home address, check the `--header` part of the command (or the `headers` in `.mcp.json`) is there. On https, sign in through `/mcp` > Authenticate. |
| Claude Code sign-in fails, or never opens the browser, on an `http://` address | Claude Code only signs in through the browser on https or `localhost`. Use an AI key instead, or put HolyArrs on https. |
| Sign-in fails behind a reverse proxy or tunnel | HolyArrs does not know it is on https. Add the proxy to `HOLYARRS_TRUSTED_PROXIES` and have it send `X-Forwarded-Proto: https`. |
| "use an AI key (Settings > AI connections)" | The key is a normal HolyArrs API key. Only AI keys work here. Make one in Settings > AI connections. |
| "API key not accepted" | The key was revoked or copied wrong. Make a new one and update the command or file. |
| "Sign in with your password to manage AI keys." when making a key | Making keys needs a recent sign-in with your password and the permission to change settings. Sign out and sign in again with your password, then press the button again. Without the permission, ask the owner. |
| The "Connect an AI app" page asks "Confirm it's you first." | Allowing an app makes it a key, so HolyArrs asks for your password (and a two-factor code when that is on) if you have not typed it in the last 10 minutes. Type it, press "Confirm", then "Allow". |
| "Could not connect the AI app" | Remove HolyArrs in Claude and add it again. |
| "too many calls from this key; try again in ... seconds" | The look-up or action limit was reached. Wait the time shown. |
| The Claude app says it cannot reach the server | The address is not public https, or something in front of HolyArrs blocks Anthropic's servers: a login wall such as Cloudflare Access, a country block, or a firewall. Anthropic connects from `160.79.104.0/21` and `153.61.208.0/20`. |
| Claude Desktop does not list holyarrs | Check Node.js is installed (`npx --version` in a terminal), then quit Claude Desktop fully and start it again. Claude Desktop writes the bridge's log to `mcp-server-holyarrs.log` in its logs folder: `%APPDATA%\Claude\logs` on Windows, `~/Library/Logs/Claude` on macOS, `~/.config/Claude/logs` on Linux. |
| Claude Desktop on Windows ignores your edits | Some Windows installs read the file from `%LOCALAPPDATA%\Packages\Claude_pzs8sxrjxfjjc\LocalCache\Roaming\Claude\claude_desktop_config.json` instead (not tried live). Put the entry there too. |
| `claude mcp list` says "Needs authentication" | Type `/mcp` in Claude Code, pick holyarrs and choose Authenticate, or run `claude mcp login holyarrs`. |
| An app you no longer use is still listed | Press "Revoke" next to it under "Connected apps and AI keys". |
