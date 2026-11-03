# Connect AI coding tools

Code editors and terminal agents with an AI chat can look things up in HolyArrs and ask for titles for you. They talk to HolyArrs over MCP (the Model Context Protocol). This guide covers Cursor, VS Code with Copilot, the Copilot CLI, Windsurf, Zed, Cline, Roo Code, JetBrains AI Assistant, Warp, Goose, Raycast and OpenCode.

For what an AI app can and cannot do, see [AI connections](ai-connections.md). Claude, ChatGPT, Codex and Gemini have their own guides: [Claude](ai-claude.md), [ChatGPT and Codex](ai-chatgpt-codex.md) and [Gemini](ai-gemini.md). Perplexity is covered in [AI connections](ai-connections.md).

HolyArrs has the same setup in Settings > AI connections > "Connect an AI app", with your own address filled in, and your key when one is needed. Cursor and VS Code have their own buttons there. Windsurf, Zed, Cline, Roo Code and the Copilot CLI are under "More apps", and JetBrains AI Assistant and Warp share one entry there. Goose, Raycast and OpenCode are only on this page.

## What you need

- HolyArrs with AI connections turned on (next section).
- Your HolyArrs AI address. It is HolyArrs's own address followed by `/api/v1/mcp`, for example `http://10.10.25.25:18730/api/v1/mcp`. The short form `/mcp` works too. If HolyArrs runs under a [URL base](url-base.md) such as `/holyarrs`, the base comes first: `/holyarrs/api/v1/mcp`. Settings > AI connections shows the full address with a Copy button.
- The app itself, on a computer that can reach that address. These apps all connect from your own computer, so a home network or Tailscale address works.

| App | What it needs |
| --- | --- |
| Cursor | Version 0.48 or newer. No paid plan is listed for adding servers (not tried live on the free plan). |
| VS Code | GitHub Copilot signed in, with agent mode in Chat. A Copilot Business or Enterprise organisation can switch MCP servers off by policy. |
| GitHub Copilot CLI | A GitHub Copilot plan. |
| Windsurf (now Devin Desktop) | No paid plan listed. On Teams it needs a full seat, and an admin allow-list can block servers that are not on it. |
| Zed | Nothing extra. |
| Cline, Roo Code | Nothing extra (both are VS Code extensions). |
| JetBrains AI Assistant | An AI Assistant subscription (which tier: not tried live). |
| Warp | Local agents only. Agent use may need a Warp AI plan (not tried live). |
| Goose | Nothing extra. |
| Raycast | Raycast Pro. |
| OpenCode | Nothing extra (not tried live). |

## Turn on AI connections

AI connections are off until the owner turns them on. While they are off, an app that sends an AI key gets 404 "not found". An app with no key is still sent to the browser sign-in, so it can be allowed (and the owner can turn AI connections on) from there.

Open Settings > AI connections and switch on "Let AI apps connect". Only the owner can change it.

## Two ways to connect

Which way you use depends on the address you open HolyArrs on. The Settings page picks the right one for you.

| Your address | How the app gets in |
| --- | --- |
| `https://...`, or `localhost` on the same computer | **Browser sign-in.** The app opens HolyArrs in your browser. You sign in with your HolyArrs password if asked and press "Allow" on the "Connect an AI app" page. Nothing to paste. |
| Plain `http://` on your home network or Tailscale, such as `http://10.10.25.25:18730` | **AI key.** Many apps refuse browser sign-in on a plain http address, so the Settings page uses a key there. Press "Make a key for Cursor" (or VS Code) in the steps, or "Make an AI key" under "More apps"; the key is made, filled into the entries and shown only once. |

About browser sign-in:

- The app gets its own key. It shows under Settings > AI connections > "Connected apps and AI keys", named after the app, and "Revoke" there disconnects it.
- If AI connections are off, the owner can turn them on from the "Allow" page. Anyone else is told to ask the owner.

About AI keys:

- An AI key works only on the AI address. It cannot sign in to HolyArrs or use [the API for other apps](api.md).
- The app sends it in the header `Authorization: Bearer <your AI key>`. The header `X-Api-Key: <your AI key>` works too.
- It is shown once. If you lose it, revoke it and make a new one.
- The "More apps" button names its key "AI app". To give a key the app's own name, or to make one for an app the Settings page does not list, use "Make a key by hand" under "Connected apps and AI keys".
- Never paste a key into a web link or into a chat with the AI. The "Add to Cursor" and "Add to VS Code" buttons are the one exception: they open the app on your computer directly and do not pass through a website.
- Config files keep the key as plain text. Do not commit a project config file that holds a key to git.

In the entries below, the address and `<your AI key>` are examples. Copy the real ones from the Settings page.

## Cursor

1. In HolyArrs, open Settings > AI connections and pick "Cursor". On a plain http address, press "Make a key for Cursor" first.
2. Press "Add to Cursor". Cursor opens its install screen with HolyArrs filled in. Or put this in `~/.cursor/mcp.json` yourself (on Windows that should be `%USERPROFILE%\.cursor\mcp.json`; not tried live):

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

   On an https address, leave out the `headers` part:

   ```json
   {
     "mcpServers": {
       "holyarrs": {
         "url": "https://holyarrs.example.com/api/v1/mcp"
       }
     }
   }
   ```

3. On https, Cursor shows "Needs login" next to holyarrs. Click it, sign in to HolyArrs in the browser if asked, then press "Allow".

**Check it:** in Cursor Settings, MCP, holyarrs has a green dot.

A project can have its own `.cursor/mcp.json` too. If that file holds a key, keep it out of git.

## VS Code with GitHub Copilot

1. In HolyArrs, open Settings > AI connections and pick "VS Code". On a plain http address, press "Make a key for VS Code" first.
2. Press "Add to VS Code". VS Code opens its install screen with HolyArrs filled in, key included.

   Or add it by hand: open the Command Palette, run `MCP: Add Server`, pick HTTP, paste the address, enter `holyarrs` as the name and pick where to save it. The steps below assume your user `mcp.json`; the Copilot Global choice saves to the Copilot file instead (see [GitHub Copilot CLI](#github-copilot-cli)).
3. If you added it by hand on a plain http address, open its entry in `mcp.json` and add this header:

   ```json
   {
     "headers": {
       "Authorization": "Bearer <your AI key>"
     }
   }
   ```

   On https, start the server instead (`MCP: List Servers`, holyarrs, Start) and allow the sign-in in the browser.

**Check it:** Command Palette, `MCP: List Servers`. holyarrs is in the list with its state.

### Keep the key out of the file

VS Code can ask for the key once and keep it in its own secret store. Then `mcp.json` never holds it. Put this in your user `mcp.json` (Command Palette: `MCP: Open User Configuration`):

```json
{
  "inputs": [
    {
      "type": "promptString",
      "id": "holyarrs-key",
      "description": "HolyArrs AI key",
      "password": true
    }
  ],
  "servers": {
    "holyarrs": {
      "type": "http",
      "url": "http://10.10.25.25:18730/api/v1/mcp",
      "headers": {
        "Authorization": "Bearer ${input:holyarrs-key}"
      }
    }
  }
}
```

The first time the server starts, VS Code asks for the key. Paste the AI key there, not in the chat.

Note the top-level word: VS Code's own `mcp.json` uses `servers`, while the Copilot file below uses `mcpServers`. A wrong top-level word is a common reason a server does not show up.

### GitHub Copilot CLI

The Copilot CLI keeps its servers in `~/.copilot/mcp-config.json` (on Windows that should be `%USERPROFILE%\.copilot\mcp-config.json`; not tried live). Newer VS Code builds can read the same file for Copilot sessions. Add:

```json
{
  "mcpServers": {
    "holyarrs": {
      "type": "http",
      "url": "http://10.10.25.25:18730/api/v1/mcp",
      "headers": {
        "Authorization": "Bearer <your AI key>"
      }
    }
  }
}
```

On https, leave out the `headers` part and sign in when asked (the sign-in step in the Copilot CLI is not tried live).

Or run:

```
copilot mcp add --transport http --header "Authorization: Bearer <your AI key>" holyarrs http://10.10.25.25:18730/api/v1/mcp
```

This file does not understand the `${input:...}` form. To keep the key out of it, use an environment variable instead: `"Authorization": "Bearer ${HOLYARRS_AI_KEY}"`, with `HOLYARRS_AI_KEY` set on the computer that runs Copilot.

## Windsurf

Windsurf is now called Devin Desktop. Current builds read `~/.config/devin/mcp_config.json` (Windows: `%APPDATA%\devin\mcp_config.json`). Add:

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

You can also add it from the app: Customize, then the MCPs tab.

On https, leave out the `headers` part. Devin Desktop starts the browser sign-in when you add it; press "Allow".

Older Windsurf builds read `~/.codeium/windsurf/mcp_config.json` and name the address `serverUrl`. The "More apps" entry on the Settings page uses the Devin file and `url`; on an older build, put the same entry in the older file (whether it accepts `url` there is not tried live; if not, rename it to `serverUrl`).

## Zed

Open Settings > AI > MCP Servers > Add Server > Add Remote Server, or add this to Zed's `settings.json` (run `zed: open settings file`):

```json
{
  "context_servers": {
    "holyarrs": {
      "url": "http://10.10.25.25:18730/api/v1/mcp",
      "headers": {
        "Authorization": "Bearer <your AI key>"
      }
    }
  }
}
```

If your `settings.json` already has other settings, add only the `context_servers` part inside it.

On https, leave out the `headers` part. Zed then starts the browser sign-in by itself. With an `Authorization` header present, Zed uses the key and skips sign-in.

**Check it:** the server has a green dot with "Server is active".

## Cline

Cline always uses an AI key, on https too. On https, the Settings page has no "Make a key" button for it: use "Make a key by hand" under "Connected apps and AI keys" and name it Cline.

In Cline, open the MCP Servers icon, the Configure tab, then "Configure MCP Servers". That opens `cline_mcp_settings.json`. Add:

```json
{
  "mcpServers": {
    "holyarrs": {
      "type": "streamableHttp",
      "url": "http://10.10.25.25:18730/api/v1/mcp",
      "headers": {
        "Authorization": "Bearer <your AI key>"
      }
    }
  }
}
```

Keep `"type": "streamableHttp"`. Without it Cline tries an older way of connecting and fails.

The Cline command-line tool has the same thing as one command:

```
cline mcp add holyarrs http://10.10.25.25:18730/api/v1/mcp --transport http --header "Authorization: Bearer <your AI key>" --yes
```

## Roo Code

Roo Code also always uses an AI key; on https, make one by hand as for Cline.

Use "Edit Global MCP" to open `mcp_settings.json` (or "Edit Project MCP" for `.roo/mcp.json` in one project). Add:

```json
{
  "mcpServers": {
    "holyarrs": {
      "type": "streamable-http",
      "url": "http://10.10.25.25:18730/api/v1/mcp",
      "headers": {
        "Authorization": "Bearer <your AI key>"
      }
    }
  }
}
```

The type is spelled `streamable-http` here, with a dash, unlike Cline's `streamableHttp`.

## JetBrains AI Assistant

Open Settings | Tools | AI Assistant | Model Context Protocol (MCP) and press Add. In the dialog, pick HTTP as the connection type and paste:

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

Choose whether it is for all projects or only this one, then OK and Apply.

On https, leave out the `headers` part. Whether AI Assistant reads `headers`, and whether it offers browser sign-in, are both not tried live.

**Check it:** the Status column next to holyarrs.

## Warp

Open Settings > Agents > MCP servers (or "Open MCP Servers" from the Command Palette). Press + Add, pick the URL type and paste:

```json
{
  "holyarrs": {
    "url": "http://10.10.25.25:18730/api/v1/mcp",
    "headers": {
      "Authorization": "Bearer <your AI key>"
    }
  }
}
```

Use `url`, not `serverUrl`. The "Warp" entry under "More apps" on the Settings page is this same form.

On https, leave out the `headers` part. Warp opens the browser sign-in the first time the server starts. Warp keeps the sign-in per device, so each computer signs in once.

## Goose

In a terminal, run `goose configure`, pick "Add Extension", then "Remote Extension (Streamable HTTP)". Answer the prompts:

- URI: your AI address, for example `http://10.10.25.25:18730/api/v1/mcp`.
- Timeout: the default is fine.
- Custom headers: on a plain http address, say yes and add `Authorization` with the value `Bearer <your AI key>`. On https, say no; Goose starts the browser sign-in when it first connects.

In the Goose desktop app it is Extensions > "Add custom extension". Which label the remote type has there is not tried live.

Goose keeps extensions in `~/.config/goose/config.yaml` (Windows: `%APPDATA%\Block\goose\config\config.yaml`). Use `goose configure` rather than editing that file for the key; the exact name of the headers setting in the file is not tried live.

## Raycast

Run the "Install MCP Server" command and fill in the form:

| Field | Value |
| --- | --- |
| Name | HolyArrs |
| Transport | HTTP |
| URL | your AI address |
| HTTP Headers | on a plain http address: `Authorization` = `Bearer <your AI key>` |
| OAuth Type | on https: Dynamic |

Install it. On https, press Sign In when Raycast shows it, then "Allow" in HolyArrs.

## OpenCode

Not tried live. OpenCode is not in the Settings page's list, and the shared entry there does not fit it. Add this to `~/.config/opencode/opencode.json` (or `opencode.json` in a project):

```json
{
  "mcp": {
    "holyarrs": {
      "type": "remote",
      "url": "http://10.10.25.25:18730/api/v1/mcp",
      "headers": {
        "Authorization": "Bearer <your AI key>"
      }
    }
  }
}
```

On https, leave out the `headers` part and run `opencode mcp auth holyarrs` to sign in. Make the key by hand under "Connected apps and AI keys" and name it OpenCode.

## Other apps

Any app that can add a remote MCP server over Streamable HTTP can connect. Give it the AI address. On https, sign in when it asks. On a plain http address, give it an AI key in the `Authorization: Bearer` header. Under "More apps", the Settings page also has a shared entry for LM Studio and similar apps. For apps that run a model on your own computer, such as LM Studio, Open WebUI or Jan, see [Local AI apps](ai-local.md).

## Check that it works

1. Use the app's own check from its section above.
2. In HolyArrs, open Settings > AI connections > "Connected apps and AI keys". The app's key shows "last used" instead of "never used" once it has made a call.
3. Ask the first question below. Look-ups are not written to History. When the app asks for a title, that shows in History with "AI apps" as who did it.

## Try this first

Ask your app:

> "Use HolyArrs: what's downloading right now, and what's coming out this week?"

Starting with "Use HolyArrs" helps the app pick the HolyArrs tools over its own guesses.

## What the AI can and cannot do

An AI app acts as you, with your permissions and request limits.

- It can look things up: search for titles, check your Library, the calendar, Downloads and your own requests. If you may manage the Library, it can also check HolyArrs's health.
- It can ask for a movie or show. Before it does, it has to show you what it will ask for and wait for your yes.
- It cannot approve or turn down requests, change settings, or see other people's requests.
- Each app or key gets at most 60 calls a minute and 5 requests an hour.

The full list is in [AI connections](ai-connections.md).

## Troubleshooting

| What you see | What to do |
| --- | --- |
| 404 or "not found" | AI connections are off: turn on "Let AI apps connect". Or the address is wrong: it must end in `/api/v1/mcp`, with your URL base in front if you use one. |
| 401 "sign in: add HolyArrs in your AI app and allow it in the browser" | The app sent no key and has not signed in. On https, start the sign-in in the app (Cursor: "Needs login"). On plain http, add the key header. |
| 401 "API key not accepted" | The key is wrong, revoked, or was copied with a piece missing. Make a new one. |
| 403 "use an AI key (Settings > AI connections)" | The app sent a normal HolyArrs API key. Only AI keys work here. Make one on the AI connections page. |
| The app says sign-in failed, or will not sign in to an `http://` address | Expected on a plain http address. Use an AI key instead, or reach HolyArrs on https. |
| The server never shows up | Check the top-level word for your app (`mcpServers`, `servers`, `context_servers`, `mcp`) and the address key (`url` or `serverUrl`). Restart the app after editing the file. |
| "Add to Cursor" or "Add to VS Code" did nothing | Your browser may block app links. Copy the entry into the file by hand. |
| "too many calls from this key" | The app hit 60 calls a minute or 5 requests an hour. Wait the seconds it says. |
| The app answers without using HolyArrs | Start your question with "Use HolyArrs", and check the server is switched on in the app's MCP list. |
| It worked, then stopped | Someone revoked the app's key, or AI connections were turned off. Check both on the AI connections page. |
