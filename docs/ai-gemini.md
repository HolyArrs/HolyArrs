# Connect Gemini

Google ships Gemini in several apps, and each one connects to HolyArrs in its own way. This page covers all of them. For what AI apps can do in HolyArrs in general, see [AI connections](ai-connections.md).

| Gemini app | Runs on | Works with a home address like `http://10.10.25.25:18730` | Guide |
| --- | --- | --- | --- |
| Gemini CLI | your computer | yes, with an AI key | [Gemini CLI](#gemini-cli) |
| Antigravity (editor and CLI) | your computer | yes, with an AI key (not tried live) | [Antigravity](#antigravity) |
| Gemini Code Assist in VS Code or IntelliJ | your computer | yes, with an AI key (not tried live) | [Gemini Code Assist](#gemini-code-assist) |
| Gemini app (gemini.google.com, Android) | Google's servers | no: needs a public https address | [The Gemini app](#the-gemini-app) |
| Gemini Enterprise | Google's servers | no | [Gemini Enterprise](#gemini-enterprise) |

## What you need

- HolyArrs with AI connections turned on (next section). Only the owner can turn them on.
- A HolyArrs account. The AI app acts as you, so your permissions and request limits apply to it.
- For **Gemini CLI**: the `gemini` command, signed in with an account Google still serves. On 18 June 2026 Google stopped serving Gemini CLI to Google AI Pro and Ultra subscribers and to the free Gemini Code Assist for individuals, and moved them to Antigravity. Gemini CLI keeps working with a paid Gemini API key, a Vertex AI key, or a Gemini Code Assist Standard or Enterprise license. Whether a free AI Studio key still works is not confirmed. If your account is not one of those, use [Antigravity](#antigravity).
- For **Antigravity**: the Antigravity editor or CLI.
- For **Gemini Code Assist**: a Standard or Enterprise license, and the extension in VS Code or IntelliJ.
- For **the Gemini app**: see [The Gemini app](#the-gemini-app); it has its own rules about country, age and account type.

## Turn on AI connections

Open Settings > AI connections. Switch on "Let AI apps connect". While it is off, the AI address answers 404 "not found" even to a good AI key, and Gemini shows HolyArrs as not connected or failing.

The same page has the guide for each app under "Connect an AI app". Pick the "Gemini" chip. The address at the top of that card is the one to give Gemini. It is your HolyArrs address with `/api/v1/mcp` on the end, for example `http://10.10.25.25:18730/api/v1/mcp`. If HolyArrs runs under a [URL base](url-base.md), the page puts it in for you.

## Two ways to connect

Which way you use depends on the address you open HolyArrs on. The page tells you under the address.

| Your HolyArrs address | How Gemini signs in |
| --- | --- |
| `https://...`, or `localhost` on the same computer | **Browser sign-in.** Gemini opens HolyArrs in your browser. You sign in with your HolyArrs password and press "Allow" on the "Connect an AI app" page. Nothing to paste. |
| Plain `http://` on your network, like `http://10.10.25.25:18730` or a Tailscale `100.x` address | **AI key.** Gemini CLI refuses browser sign-in to a plain http address on the network. Press "Make a key for Gemini" in the guide: HolyArrs makes an AI key, fills it into the command or file shown, and shows it only once. |

About AI keys:

- An AI key works only on the AI address. It cannot open the rest of the HolyArrs API.
- Gemini sends it in the header `Authorization: Bearer <your AI key>`. The header `X-Api-Key` also works.
- Never put a key in a link, and never paste it into a chat with Gemini. Put it only in the command or file the guide shows.
- Every app that signs in, and every key you make, shows under Settings > AI connections > "Connected apps and AI keys". Press "Revoke" there to cut an app off.

To make a key, or to press "Allow", you must have signed in to HolyArrs with your password. If AI connections are off, the "Connect an AI app" page says so: for the owner, pressing "Allow" turns them on as well. Anyone else is asked to get the owner to turn them on first.

## Gemini CLI

`--scope user` makes HolyArrs available in every folder. Without it, Gemini adds the server only to the folder you are in. `--transport http` is needed too, or Gemini treats the address as a program to run.

### With browser sign-in (https or localhost)

1. Run this in a terminal, with your own address:

   ```
   gemini mcp add --scope user --transport http holyarrs https://holyarrs.example.com/api/v1/mcp
   ```

2. Start `gemini` and type `/mcp auth holyarrs`. HolyArrs opens in your browser: sign in if asked, then press "Allow".

The browser has to open on the same computer as Gemini CLI. Sign-in does not work over SSH without a desktop, or inside a container.

#### Let Gemini set itself up

With browser sign-in, the guide also has "Or let Gemini set itself up". It gives you a short request to paste into Gemini instead of running the command yourself:

```
Set up the HolyArrs MCP server for me in Gemini:
1. Add a remote MCP server named "holyarrs" at https://holyarrs.example.com/api/v1/mcp (Streamable HTTP), for all my projects.
2. Start its sign-in; I will sign in to HolyArrs in the browser and press Allow.
3. Tell me when its tools show up.
```

It holds no key, which is why the guide offers it only for browser sign-in.

### With an AI key (plain http on your network)

1. In Settings > AI connections, pick "Gemini" and press "Make a key for Gemini".
2. Copy the command the guide shows, with the key already in it, and run it in a terminal:

   ```
   gemini mcp add --scope user --transport http --header "Authorization: Bearer <your AI key>" holyarrs http://10.10.25.25:18730/api/v1/mcp
   ```

3. Start `gemini` and type `/mcp list`.

The command saves the entry, key included, in `~/.gemini/settings.json` (Windows: `%USERPROFILE%\.gemini\settings.json`), as `url` with `"type": "http"`. To keep the key out of that file, write `Bearer ${HOLYARRS_AI_KEY}` in the header there instead and set the `HOLYARRS_AI_KEY` environment variable before you start `gemini` (not tried live).

## Antigravity

Antigravity reads its servers from `~/.gemini/config/mcp_config.json` (Windows: `%USERPROFILE%\.gemini\config\mcp_config.json`). Whether the editor and the CLI share this one file has not been tried live. Create the file if it is not there. If it already lists servers, add the `holyarrs` entry next to them inside `mcpServers`.

With browser sign-in (https or localhost):

```json
{
  "mcpServers": {
    "holyarrs": {
      "serverUrl": "https://holyarrs.example.com/api/v1/mcp"
    }
  }
}
```

Antigravity should open HolyArrs in your browser the first time it connects; sign in and press "Allow" (not tried live).

With an AI key (plain http): press "Make a key for Gemini" first, then copy the entry from the guide:

```json
{
  "mcpServers": {
    "holyarrs": {
      "serverUrl": "http://10.10.25.25:18730/api/v1/mcp",
      "headers": {
        "Authorization": "Bearer <your AI key>"
      }
    }
  }
}
```

Use `serverUrl`, not `httpUrl`: Antigravity does not read `httpUrl`. If HolyArrs does not show up after you save the file, restart Antigravity (not tried live). A workspace can have its own file at `.agents/mcp_config.json` in that folder, if you want HolyArrs in one project only. Whether Antigravity accepts a plain http address has not been tried live; if it refuses, use an https address.

## Gemini Code Assist

Gemini Code Assist is the Gemini extension for VS Code and IntelliJ. Its agent mode can use HolyArrs.

### VS Code

Code Assist reads the same `~/.gemini/settings.json` file as Gemini CLI. If you already ran the Gemini CLI command above with `--scope user`, HolyArrs is in that file, but written as `url` with `"type": "http"`. Whether Code Assist reads that form has not been tried live; if HolyArrs does not show up, replace the entry with the one below. Otherwise add this entry inside `mcpServers` by hand.

For the key: on a plain http address, press "Make a key for Gemini" in the guide and copy the key from "Connected apps and AI keys", where it shows once. On an https address the Gemini guide has no key button, so open "Make a key by hand" under "Connected apps and AI keys", give the key a name and press "Make an AI key".

```json
{
  "mcpServers": {
    "holyarrs": {
      "httpUrl": "http://10.10.25.25:18730/api/v1/mcp",
      "headers": {
        "Authorization": "Bearer <your AI key>"
      }
    }
  }
}
```

Then run "Developer: Reload Window" from the Command Palette. In the Code Assist chat, `/mcp` lists the servers and their tools. Code Assist does not describe a browser sign-in for these servers, so use the key, even on https (not tried live).

### IntelliJ and other JetBrains IDEs

Code Assist in JetBrains IDEs reaches remote servers only through the `mcp-remote` bridge, which needs Node.js. Add the entry below to the `mcp.json` file Code Assist uses in the IDE's settings folder (not tried live). It is the same entry the guide shows for "Claude Desktop (local bridge)" under "More apps". On a plain http address, press "Make an AI key" there to get it filled in. On https there is no button there: make a key by hand (see [VS Code](#vs-code)) and put it after `Bearer` in place of `<your AI key>`.

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

Keep `Authorization:${AUTH_HEADER}` with no space after the colon. Some apps on Windows split arguments at spaces. `--allow-http` appears only for a plain http address.

## The Gemini app

This is Gemini on gemini.google.com and in the Android app. It can add your own MCP server as a "Custom apps" entry, with limits. None of this has been tried live with HolyArrs.

What is possible:

- Adding HolyArrs from gemini.google.com on a computer: Settings > Connected Apps > Custom apps, enter the HolyArrs AI address, press "Next", and follow the steps. HolyArrs opens for you to sign in and press "Allow".
- Using it on your phone afterwards. Setup works only on the web, but once added, it is there in the Android app too.

What is not possible:

- **A home address.** The Gemini app connects from Google's servers, not from your device, so it cannot reach `10.x`, `192.168.x` or a Tailscale `100.x` address. Google does not say which addresses it accepts; plan on a public https address: a reverse proxy with a domain, Cloudflare Tunnel, or Tailscale Funnel. Open Settings > AI connections on that public address and copy the address at the top of "Connect an AI app". The "Gemini" chip there covers Gemini CLI and Antigravity only, not the Gemini app.
- **An AI key.** Google documents no field for a fixed key header, so use browser sign-in. HolyArrs lets apps register themselves, so Gemini should not need the "Advanced features" box where Google asks for credentials from servers that do not.
- **Everyone.** Google offers custom apps only to people aged 18 or over, in the US, on a personal Google account (not work or school), with the Gemini app in English and Gemini activity history kept on.

If any of these rule you out, use Gemini CLI or Antigravity on your own computer instead.

## Gemini Enterprise

Gemini Enterprise also connects from Google's servers and needs a public https address with a certificate from a public authority. It offers two choices: no sign-in at all, or sign-in with a client ID and secret made ahead of time. HolyArrs always needs a sign-in or an AI key, has no screen for making a client ID and secret, and does not hand out client secrets, so Gemini Enterprise is not supported yet (not tried live).

## Check it worked

- **Gemini CLI:** run `gemini mcp list` in a terminal: `holyarrs` shows as connected. Inside `gemini`, `/mcp list` also shows its tools.
- **Antigravity and Code Assist:** their MCP list shows `holyarrs` with its tools (not tried live).
- **HolyArrs:** under Settings > AI connections > "Connected apps and AI keys", the app's key shows "last used" with a recent time.

## Try this first

Ask Gemini:

> "Use HolyArrs: what's downloading right now, and what's coming out this week?"

Gemini should call the `downloads` and `calendar` tools and answer from your own HolyArrs.

## What Gemini can and cannot do

Gemini gets the same tools as every AI app; [AI connections](ai-connections.md) has the full list.

- It can look things up: find titles, check the Library, read the calendar and Downloads, and see your own requests. If you may manage the Library, it can also check health. These only read.
- It can ask for a movie or show for you. HolyArrs makes it check with you first: Gemini has to show you what it will ask for, and only after you agree can it go ahead.
- It cannot approve or turn down requests, change settings, or see other people's requests.
- Limits per app or key: 60 calls a minute and 5 requests an hour.
- Every request it makes shows in History, with the app's name and yours, under Changes > Asks. Anything it tried that HolyArrs did not do shows under Problems > AI apps. Look-ups are not written to History.

## Troubleshooting

| What you see | What to do |
| --- | --- |
| 404 or "not found" | AI connections are off. The owner switches on "Let AI apps connect" in Settings > AI connections. Also check the address ends in `/api/v1/mcp`. |
| Gemini CLI refuses to sign in to an `http://` address | Expected: Gemini CLI signs in only on https or localhost. Use "Make a key for Gemini" and the key command. |
| Sign-in fails on an https name that points to your home network, like a Tailscale `.ts.net` name | Gemini CLI blocks sign-in to private addresses unless told otherwise. Add `"oauth": { "allowPrivateNetwork": true }` inside the `holyarrs` entry in `~/.gemini/settings.json` (not tried live), or use an AI key. |
| Gemini says the server needs authentication | Type `/mcp auth holyarrs` and allow it in the browser. If a revoked or old sign-in is the cause, the same command signs in again. |
| 401 | The key is wrong, revoked or missing. Make a new one with "Make a key for Gemini" and run the command again. |
| 403 "use an AI key (Settings > AI connections)" | The key is an ordinary API key. Only an AI key works here. |
| "too many calls from this key; try again in N seconds" | You hit 60 calls a minute or 5 requests an hour. Wait and ask again. |
| Gemini CLI tries to run the address as a program | `--transport http` is missing. Remove the server with `gemini mcp remove holyarrs --scope user` and add it again with the full command. |
| HolyArrs works in one folder only | `--scope user` was left out. In that folder run `gemini mcp remove holyarrs`, then add it again with `--scope user`. |
| "Make a key for Gemini" is greyed out, or "Sign in with your password to manage AI keys." | You reached HolyArrs without typing your password. Sign in with your password, then press it again. |
| The Gemini app cannot reach HolyArrs | It needs a public https address; see [The Gemini app](#the-gemini-app). |
| The "Allow" button is greyed out | AI connections are off and you are not the owner. Ask the owner to turn them on. |
