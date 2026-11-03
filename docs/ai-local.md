# Connect a local AI

You can run an AI model on your own computer and let it look things up in HolyArrs and ask for titles for you. The model, the chat app and HolyArrs all stay on your network. They talk to HolyArrs over MCP (the Model Context Protocol).

This guide covers:

| App | What it is | How it connects |
| --- | --- | --- |
| [Ollama](#ollama) | Runs the model. It cannot connect to HolyArrs by itself, so you pair it with one of the three below. | Through a chat app |
| [Open WebUI](#open-webui) | A web chat for Ollama | AI key, or browser sign-in on https |
| [ollmcp](#ollmcp) | A terminal chat for Ollama | AI key |
| [Goose](#goose) | A desktop and terminal agent that can use Ollama | AI key, or browser sign-in on https |
| [LM Studio](#lm-studio) | Runs the model and has its own chat | AI key |
| [Jan](#jan) | Runs the model and has its own chat | Browser sign-in, or the local bridge |
| [AnythingLLM](#anythingllm) | Desktop chat with agents | AI key |
| [Msty Studio](#msty-studio) | Desktop chat | AI key (not tried live) |
| [Any app that only runs local commands](#apps-that-only-run-local-commands-the-mcp-remote-bridge) | | The `mcp-remote` bridge |

For what the AI can and cannot do once it is connected, see [AI connections](ai-connections.md). For coding tools (Cursor, VS Code, Zed and others), see [Connect AI coding tools](ai-coding-tools.md).

## Before you start

### What you need

- HolyArrs with AI connections turned on (next section).
- Your HolyArrs AI address. It is HolyArrs's own address followed by `/api/v1/mcp`, for example `http://10.10.25.25:18730/api/v1/mcp`. The short form `/mcp` works too. If HolyArrs runs under a [URL base](url-base.md) such as `/holyarrs`, the base comes first: `/holyarrs/api/v1/mcp`. Settings > AI connections shows the full address with a Copy button.
- A chat app that can add a remote MCP server over Streamable HTTP, or the [bridge](#apps-that-only-run-local-commands-the-mcp-remote-bridge) for one that cannot.
- A model that is good at tool calls. See [Pick a model](#pick-a-model-that-handles-tool-calls).

| App | Version |
| --- | --- |
| Open WebUI | v0.6.31 or newer |
| LM Studio | 0.3.17 or newer |
| AnythingLLM | Desktop v1.8.0 or newer |
| Jan, Msty Studio, Goose, ollmcp | Not tried live: use a current version |
| The `mcp-remote` bridge | [Node.js](https://nodejs.org) installed, so that `npx` works in a terminal |

Whether any of these apps needs a paid plan to add a server was not tried live.

### Turn on AI connections

AI connections are off until the owner turns them on. While they are off, the AI address answers 404 "not found".

1. Open Settings > AI connections.
2. Switch on "Let AI apps connect".

Only the owner can change this switch.

### The two ways to connect

Which way you use depends on the address the app reaches HolyArrs on.

| Your address | How the app gets in |
| --- | --- |
| Plain `http://` on your network, like `http://10.10.25.25:18730` or a Tailscale `100.x` address | **AI key.** This is the normal way for local AI. You make an AI key in HolyArrs and put it in the app's settings. |
| `https://...`, or `localhost` on the same computer | **Browser sign-in**, for apps that support it (Open WebUI, Jan, Goose). The app opens HolyArrs in your browser. You sign in with your HolyArrs password and press "Allow" on the "Connect an AI app" page. Nothing to paste. The other apps use an AI key here too. |

Most apps refuse browser sign-in to a plain http address that is not on the same computer, so on a home address use a key.

Either way the app ends up with its own key. It is listed under Settings > AI connections > "Connected apps and AI keys", with when it was last used. Press "Revoke" there to disconnect the app.

### Make an AI key

1. Open Settings > AI connections.
2. On a plain http address: under "Connect an AI app", pick "More apps" and press "Make an AI key". The key is filled into the entries on that page.
3. On an https address that button is not shown, because apps sign in there. For an app that still needs a key (LM Studio, AnythingLLM, Msty Studio, ollmcp), or to give the key the app's name, open "Make a key by hand" under "Connected apps and AI keys", type the app's name (for example `LM Studio`) and press "Make an AI key".
4. Copy the key straight away. It is shown only once. If you lose it, revoke it and make a new one.

About AI keys:

- An AI key works only on the AI address. It cannot sign in to HolyArrs or use [the API for other apps](api.md).
- The app sends it in the header `Authorization: Bearer <your AI key>`. The header `X-Api-Key: <your AI key>` works too.
- Never put a key in a web link or in a chat with the AI. It goes only in the app's settings.
- Config files keep the key as plain text. Keep them out of shared folders and out of git.

In the entries below, the address and `<your AI key>` are examples. Copy the real ones from the Settings page.

## Everything stays on your network

With a local model, the whole conversation stays at home:

- The model runs on your computer.
- The chat app talks to HolyArrs over your network, or over Tailscale.
- Your questions, HolyArrs's answers and your key do not go to an AI company.

Two things still leave your network, the same as without AI:

- A title search goes to HolyArrs's metadata service, like a search in the app. See [Metadata privacy](metadata-privacy.md).
- If you pick a model that runs on someone else's servers (for example an Ollama model whose name ends in `-cloud`), your chat and HolyArrs's answers go to that service.

Because the app connects from your own computer, a home address works. You do not need a public https address, unlike the cloud apps in [Connect Claude](ai-claude.md) or [Connect ChatGPT and Codex](ai-chatgpt-codex.md).

## Pick a model that handles tool calls

Connecting HolyArrs does not make a model better at using tools. A model that is weak at tool calls may ignore HolyArrs, send wrong values, or give up halfway through a question that needs two look-ups.

Pick a model built for tool calls, and give it room:

| Model family | Notes |
| --- | --- |
| Qwen3 or Qwen3.5, 7B or larger | Listed as tool-capable by Ollama and ollmcp. A good first choice. |
| gpt-oss 20B | ollmcp lists it as a `-cloud` model, which runs on Ollama's servers. The local version was not tried. |
| Llama 3.2 | Listed by ollmcp. |
| Mistral | Listed by ollmcp. |
| Gemma 4 | Listed by ollmcp. |

These were not tried live with HolyArrs. Below about 7B, expect questions that need more than one look-up to fail often.

Tips:

- Give the model a large enough context window. Tool lists and answers use part of it.
- HolyArrs offers only seven tools, so it adds little to the context. In Open WebUI you can narrow it further with the "Function Name Filter List".
- Start your question with "Use HolyArrs". It helps the model pick the HolyArrs tools.

## Ollama

Ollama runs models. Its own guides do not describe connecting it to an MCP server, so pair it with a chat app that does and pick your Ollama model there:

- [Open WebUI](#open-webui) if you want a chat in the browser.
- [ollmcp](#ollmcp) if you want a chat in the terminal.
- [Goose](#goose) if you want an agent on the desktop or in the terminal.

LM Studio, Jan, AnythingLLM and Msty Studio can also run models themselves, so they do not need Ollama.

## Open WebUI

Only an Open WebUI admin can add an MCP server. Open WebUI calls HolyArrs from the computer it runs on, not from your browser, so the address must work from there.

1. Open the Admin Panel > Settings > "Integrations", then the "External Tool Servers" list (some versions call it "External Tools").
2. Press "+ Add Connection".
3. Fill in the form:

| Field | Value |
| --- | --- |
| Type | "MCP (Streamable HTTP)". Do not pick OpenAPI. |
| Server URL | your AI address, for example `http://10.10.25.25:18730/api/v1/mcp` |
| Auth | "Bearer" |
| Key | your AI key, without the word `Bearer` |

4. Save.

If the list is greyed out, Open WebUI was started with `ENABLE_TOOL_SERVERS=False`. Start it without that setting.

**Open WebUI in Docker.** Inside a container, `localhost` means the container, not your computer. Use HolyArrs's network address, for example `http://10.10.25.25:18730/api/v1/mcp`. A host name such as `host.docker.internal` also works with an AI key.

**Use it in a chat.** Turn on the HolyArrs tools for the chat from the chat's tools menu (other guides name it "+" > "Integrations" > "Tools"; not tried live). Leave function calling on "Native", the default since v0.10.0. Native needs a model with tool-call support; only switch to "Legacy" if your model has none.

**Look only.** To let the model look things up but never ask for titles, keep `ask_for_title` out of the tools the "Function Name Filter List" lets through (not tried live: check afterwards that it no longer shows). Another way: connect with the key of a person who may not ask for titles. HolyArrs then leaves `ask_for_title` out by itself.

**Browser sign-in on https (not tried live).** Pick "OAuth 2.1 (DCR)" as Auth instead of "Bearer" and leave Key empty. Open WebUI itself must also be on https, or opened as `localhost`: HolyArrs only sends a sign-in back to an https address or to the same computer. Then:

- Set a fixed `WEBUI_SECRET_KEY` for Open WebUI. Without it the sign-in is lost when Open WebUI restarts.
- Turn the HolyArrs tools on per chat. Do not make them default tools on a model, because the sign-in cannot run in the middle of a chat answer.

## ollmcp

ollmcp (mcp-client-for-ollama) is a terminal chat that uses your Ollama models. Install it as its project page describes.

Save this as `holyarrs.json`:

```json
{
  "mcpServers": {
    "holyarrs": {
      "type": "streamable_http",
      "url": "http://10.10.25.25:18730/api/v1/mcp",
      "headers": {
        "Authorization": "Bearer <your AI key>"
      }
    }
  }
}
```

Then start a chat with your model:

```
ollmcp -j holyarrs.json -m qwen3.5
```

Or save the server once instead of using a file (not tried live):

```
ollmcp mcp add --transport http holyarrs http://10.10.25.25:18730/api/v1/mcp --header "Authorization: Bearer <your AI key>"
```

On the main `ollmcp` command, `-H` sets the Ollama host. It is not a header.

ollmcp also lists models whose name ends in `-cloud`. Those run on Ollama's servers, not yours.

## Goose

Set Ollama as Goose's provider in `goose configure`. Then add HolyArrs:

1. In a terminal, run `goose configure`, pick "Add Extension", then "Remote Extension (Streamable HTTP)".
2. URI: your AI address, for example `http://10.10.25.25:18730/api/v1/mcp`.
3. Timeout: the default is fine.
4. Custom headers: on a plain http address, say yes and add `Authorization` with the value `Bearer <your AI key>` (not tried live). On https, say no; Goose starts the browser sign-in when it first connects.

In the Goose desktop app it is Extensions > "Add custom extension". Which label the remote type has there was not tried live. More detail is in [Connect AI coding tools](ai-coding-tools.md#goose).

## LM Studio

LM Studio keeps its servers in `mcp.json`:

- Windows: `%USERPROFILE%\.lmstudio\mcp.json`
- macOS and Linux: `~/.lmstudio/mcp.json`

To open it from the app, go to the right sidebar's "Program" tab and pick "Install", then "Edit mcp.json".

Add this entry. It is the same as the "LM Studio, JetBrains AI, Warp and others" entry under "More apps" on the Settings page:

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

If the file already has other servers, add only the `"holyarrs": { ... }` part inside its `mcpServers`.

Use the key even on https. LM Studio's own guide does not describe browser sign-in. On an https address the Settings page leaves `headers` out of this entry, so add it yourself with a key from "Make a key by hand".

When the model calls a HolyArrs tool, LM Studio shows what it is about to send. Allow it once, or always. You can change that later in App Settings > "Tools & Integrations".

## Jan

1. Open Settings > "MCP Servers" and press "+ Add MCP Server".

**On https, or with HolyArrs on the same computer:**

2. Set Transport to "HTTP" and paste your AI address.
3. On the server's "Auth" line press "Sign in". HolyArrs opens in your browser: sign in if asked, then press "Allow". Jan waits five minutes for this.

**On a plain http address on your network:** Jan's guide does not say where to put a key header, and whether its browser sign-in accepts plain http was not tried live. Use the [bridge](#apps-that-only-run-local-commands-the-mcp-remote-bridge) instead (not tried live):

2. Set Transport to "STDIO".
3. Command: `npx`
4. Args, one per line:

```
-y
mcp-remote@latest
http://10.10.25.25:18730/api/v1/mcp
--transport
http-only
--allow-http
--header
Authorization:${AUTH_HEADER}
```

5. Env: `AUTH_HEADER` = `Bearer <your AI key>`.

"Clear authentication" on the server removes a stored sign-in. By default Jan cuts off a HolyArrs call that takes longer than 30 seconds.

## AnythingLLM

AnythingLLM keeps its servers in `plugins/anythingllm_mcp_servers.json` inside its storage folder. The file is made the first time you open "Agent Skills". Where the storage folder is on each system was not tried live.

Add this entry:

```json
{
  "mcpServers": {
    "holyarrs": {
      "type": "streamable",
      "url": "http://10.10.25.25:18730/api/v1/mcp",
      "headers": {
        "Authorization": "Bearer <your AI key>"
      }
    }
  }
}
```

- Keep `"type": "streamable"`. Without it, AnythingLLM assumes an older kind of server and cannot reach HolyArrs.
- Do not add a `command` line next to `url`.

Open "Agent Skills" to load the server. Its status, logs and a reload button are there. In a chat, start your message with `@agent` so AnythingLLM can use the tools.

## Msty Studio

1. Open Toolbox > Tools and press "Add New Tool".
2. Pick the type "HTTP".
3. "MCP Server URL": your AI address.
4. "Headers": `Authorization` with the value `Bearer <your AI key>`. Msty's guide does not show the exact format of this field, so this was not tried live.
5. Give it the name `HolyArrs` and press "Add".

Try it in the "Tool Console". Msty Studio on the web works only while it is linked to Msty Studio on your desktop.

## Apps that only run local commands: the mcp-remote bridge

Some apps can only start an MCP server as a command on your computer (stdio). The `mcp-remote` bridge is that command: it runs on your computer and passes everything on to HolyArrs's address. It needs [Node.js](https://nodejs.org).

Most apps take it in this form. It is the same as the "Claude Desktop (local bridge)" entry under "More apps" on the Settings page:

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

What the parts do:

- `-y` lets `npx` fetch the bridge without asking.
- `--transport http-only` makes it use Streamable HTTP, which is what HolyArrs answers.
- `--allow-http` lets it use a plain http address. Keep it only for addresses on your own network or Tailscale. On an `https://` address, leave it out.
- `Authorization:${AUTH_HEADER}` has no space after the colon on purpose: some apps on Windows split arguments at spaces. The key itself sits in `env`.

If an app has separate Command, Args and Env fields, put `npx` in Command, each item of `args` on its own line in Args, and `AUTH_HEADER` = `Bearer <your AI key>` in Env.

Restart the app after you change its settings.

## Check that it worked

1. In the app, look at the HolyArrs server's tool list. It shows up to seven tools: `search_titles`, `library_lookup`, `calendar`, `downloads`, `my_asks`, `health` and `ask_for_title`. Some are left out when the person cannot use them: `health` needs the permission to manage the Library, and `ask_for_title` needs Requests on and the permission to ask for titles.
2. In HolyArrs, open Settings > AI connections > "Connected apps and AI keys". The app's key shows "last used" instead of "never used" once the app has made a call.
3. Ask the first question below. Look-ups are not written to History. Titles an AI app asks for are: under Downloads > History, set Who to "AI apps".

## Try this first

Ask your model:

> "Use HolyArrs: what's downloading right now, and what's coming out this week?"

A good answer names what is in Downloads and lists titles from your calendar. If the model answers without calling HolyArrs, see [Troubleshooting](#troubleshooting).

## What the AI can and cannot do

An AI app acts as you, with your permissions and request limits.

- It can look things up: search for titles, check your Library, the calendar, Downloads, your own requests and HolyArrs's health.
- It can ask for a movie or show. Before it does, it has to show you what it will ask for and wait for your yes. Its first call only gets a summary back; nothing is asked for until it calls again after you agree.
- It cannot approve or turn down requests, change settings, or see other people's requests.
- Each app or key gets at most 60 calls a minute and 5 requests an hour.

The full list is in [AI connections](ai-connections.md).

## Troubleshooting

| What you see | What to do |
| --- | --- |
| 404 or "not found" | AI connections are off: turn on "Let AI apps connect". Or the address is wrong: it must end in `/api/v1/mcp`, with your URL base in front if you use one. |
| 401 "sign in: add HolyArrs in your AI app and allow it in the browser" | The app sent no key. Check the header name is `Authorization` and the value starts with `Bearer `. In Open WebUI, check Auth is "Bearer" and Key is filled in. |
| 401 "API key not accepted" | The key is wrong, revoked, or was copied with a piece missing. Make a new one. |
| 403 "use an AI key (Settings > AI connections)" | The app sent a normal HolyArrs API key. Only AI keys work here. |
| Cannot connect at all from Open WebUI or another app in Docker | `localhost` inside a container is the container. Use HolyArrs's network address. |
| The app will not sign in to an `http://` address | Expected on a plain http address. Use an AI key, or the bridge. |
| AnythingLLM shows the server but it never connects | `"type": "streamable"` is missing from its entry. |
| The bridge says plain http is not allowed | Add `--allow-http`, for an address on your own network only. |
| The bridge fails on Windows with a broken header | Keep `Authorization:${AUTH_HEADER}` with no space, and the `Bearer` value in `env`. |
| The tools show up, but the model never uses them | Start the question with "Use HolyArrs". Check the tools are turned on for this chat. Try a larger model built for tool calls. In Open WebUI, keep function calling on "Native". |
| The model sends wrong values or stops halfway | The model is too small or its context window too short. Pick a larger model or a larger context. |
| "too many calls from this key" | The app hit 60 calls a minute or 5 requests an hour. Wait the seconds it says. |
| It worked, then stopped | Someone revoked the app's key, or AI connections were turned off. Check both on the AI connections page. |
