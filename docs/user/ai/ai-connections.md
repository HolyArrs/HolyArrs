# Connect AI apps

You want to ask an AI app things like "what's downloading?" or "is Dune in the Library?", or have it request a movie for you. This page shows you how to connect it to HolyArrs.

AI apps are held back in this release. You only see them when the `ai_apps` feature is switched on. See [Held back](../help/held-back.md).

Apps that work include Claude, ChatGPT, Codex, Gemini, Cursor and VS Code. They reach HolyArrs over MCP, the usual way these apps talk to outside services.

## Steps

1. Open Settings > AI apps.
2. Switch on "Let AI apps connect". Only the owner can do this.
3. Open that page on the same address your AI app will use. The page builds the AI address and the steps from the address in your browser.
4. Under "Connect an AI app", pick your app. Follow the steps it shows.
5. Check that it worked. Your app shows under "Linked AI apps" with a "last used" time after its first call. Then ask it: "What's downloading in HolyArrs?"

There is also "Or let your agent do it". It gives you a short prompt to paste into a coding agent so it adds HolyArrs by itself. If that prompt holds a key, paste it only into your own agent.

## The AI address

Every app uses the same address: your HolyArrs address followed by `/api/v1/mcp`.

```
http://your-server:6262/api/v1/mcp
```

`/mcp` also works. If HolyArrs runs under a [URL base](../getting-started/url-base.md) like `/holyarrs`, put the base first: `/holyarrs/api/v1/mcp`.

## Two ways to connect

**Browser sign-in.** You add the address to the app. The app opens HolyArrs in your browser. You sign in if asked, then press "Allow" on the "Connect an AI app" page. That page shows the app's name, who it will act as and what it may do. The app gets its own key. You never copy one.

**AI key.** You press "Make a key" next to your app. HolyArrs makes a key named after the app and puts it in the command or config shown. You see the key only once, so copy it then. The app sends it as `Authorization: Bearer <key>`. `X-Api-Key: <key>` works too.

Keep an AI key out of links and out of chats with the AI. It belongs only in the app's config. An AI key only works on the AI address. Ordinary [API keys](../help/parts-and-safe-mode.md#api-keys-and-scopes) do not work there.

### Which way fits your address

Browser sign-in needs `https`, or the app running on the same computer as HolyArrs (`localhost`). Many newer apps refuse to sign in to a plain `http` address on your network. Use an AI key there.

Some apps run in the cloud: the Claude app, ChatGPT, the Gemini app, Grok and Perplexity. They connect from their maker's servers, so they can't reach a home address. They need HolyArrs on a public `https` address.

| You open HolyArrs on | Apps on your computer | Cloud apps |
| --- | --- | --- |
| A public `https` name | Browser sign-in | Browser sign-in |
| An `https` name only your devices reach | Browser sign-in | Can't reach it |
| `localhost`, same computer as the app | Browser sign-in | Can't reach it |
| A plain `http` address like `http://your-server:6262` | AI key | Can't reach it |

The settings page follows this table as far as it can tell from your address. It can't tell if an `https` name is open to the internet, so it may show cloud steps that won't work on a home-only name.

Claude Desktop can still reach a home address through the `mcp-remote` bridge, which needs Node.js.

## Get an https or public address

You only need this for cloud apps, or for browser sign-in from another computer. Common choices:

- A reverse proxy with a domain and a certificate. See [Install](../getting-started/install.md).
- Cloudflare Tunnel. Gives a public `https` name without opening a port.
- Tailscale Serve. Gives an `https` name only your own devices reach.
- Tailscale Funnel. Opens that name to the internet.

Then tell HolyArrs about it:

- Add the new name to `HOLYARRS_ALLOWED_HOSTS`.
- Add the proxy's address to `HOLYARRS_TRUSTED_PROXIES`, and make sure it sends `X-Forwarded-Proto: https`. Without this, HolyArrs hands out `http` sign-in addresses and apps refuse them.

Cloud apps must reach HolyArrs with no login screen in front. A public address also puts your sign-in page on the internet, so use a strong password and [two-factor sign-in](../../users/roles.md#set-up-two-factor-sign-in).

## Pick your app

The full guide for each app is on [HolyArrs.com](https://holyarrs.com/docs/ai/). Settings > AI apps shows the same list with your address filled in.

| App | Where you add HolyArrs | On a plain `http` address |
| --- | --- | --- |
| Claude app | Connectors, add custom connector | Can't reach it |
| ChatGPT | Add custom MCP server | Can't reach it |
| Gemini app | Custom connected app | Can't reach it |
| Grok | Connectors, custom | Can't reach it |
| Perplexity | Add MCP connector, remote | Can't reach it |
| Claude Code | `claude mcp add` | AI key |
| Codex | `codex mcp add` | AI key |
| Cursor | `~/.cursor/mcp.json` | AI key |
| VS Code | MCP Servers, Add Server | AI key |
| Gemini CLI | `gemini mcp add` | AI key |
| Claude Desktop at home | Developer config, through `mcp-remote` | AI key |
| Open WebUI, LM Studio | The app's MCP settings | AI key |

Any other app that can add a remote MCP server works too. Many use this shape:

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

## Settings explained

### Let AI apps connect

The main switch. While it is off, an app with an AI key gets "not found". An app without a key is still sent to sign in, so the "Connect an AI app" page can tell you to ask the owner. If the owner presses "Allow" while it is off, that turns it on.

### Who the app acts as

An AI app acts as the person who connected it. That person's permissions, request limit and kid account limits apply. It can never do more than they can.

### What the app can see and do

| Tool | What it does | Shown when |
| --- | --- | --- |
| `search_titles` | Finds movies and shows from plain words | Movies, Shows or Music is on |
| `library_lookup` | Checks if a movie, show or artist is in the Library | Movies, Shows or Music is on |
| `calendar` | What comes out soon, 1 to 60 days ahead (14 by default) | Movies, Shows or Music is on |
| `downloads` | What is downloading and how far along | Always |
| `my_asks` | Your own requests and how many more you may make | Requests is on |
| `health` | Whether HolyArrs and its apps are working | The person may manage the Library |
| `ask_for_title` | Requests a movie or show, after you agree | Requests is on and the person may request |

Only `ask_for_title` changes anything. Lists are cut to 25 items.

Requesting takes two calls. First the app gets a summary and a confirm token. It must show you the summary. Only after you agree does it call again with the token. The token works once, for that title only, for 5 minutes. HolyArrs can't see your chat, so this depends on the app showing you the summary.

A request from an AI app counts like your own. It uses your request limit and follows your auto-approve setting.

An AI app can't approve or turn down requests, change settings, people or keys, see other people's requests, or delete or move files.

### Limits

Each AI key gets 60 calls a minute and 5 requests an hour. Over that, the app is told how many seconds to wait.

### Linked AI apps

This card lists every AI key: the name, the last four characters, when it was made and when it was last used.

- Press "Revoke" to disconnect an app. It stops at once.
- "Make a key by hand" makes a key with a name you choose, for apps not in the list.
- The owner sees every AI key. Others who may change settings see their own.
- Making, listing or revoking keys needs the permission to change settings and a recent sign-in with your password.
- A key stops working when its account is turned off.

### History

In Downloads > History, set Who to "AI apps" to see what AI apps did. Requests show as "By AI", with the app and the person. Refused requests show as "Not done", with the reason. Look-ups are not written to History.

## Troubleshooting

| What you see | What to do |
| --- | --- |
| 404 or "not found" | Switch on "Let AI apps connect". Check the address ends in `/api/v1/mcp`. |
| 401 or "sign in" | The key is missing, revoked or its account is off. Sign in again from the app, or make a new key. |
| 403 about the `mcp` scope | The app sent an ordinary API key. Make an AI key. |
| A cloud app can't reach the server | Give HolyArrs a public `https` address, or use an app on your computer. |
| Sign-in refused on a plain `http` address | Use "Make a key", or reach HolyArrs over `https` or `localhost`. |
| Sign-in fails behind a proxy or tunnel | Add the proxy to `HOLYARRS_TRUSTED_PROXIES` and send `X-Forwarded-Proto: https`. |
| "request from another site or host name refused" | Add the name to `HOLYARRS_ALLOWED_HOSTS`. |
| "This AI app is not registered with HolyArrs." | Remove HolyArrs from the app and add it again. |
| "Sign in with your password to manage AI keys." | Sign out and back in with your password, or ask the owner. |
| "too many calls from this key" | Wait the seconds shown. |
| "that confirm_token is unknown, used or expired" | Ask the app again. It gets a new token. |
| A tool is missing | Check the person's permissions and which parts are on. |
| The app never uses HolyArrs | The model may be too small for tools. Try a bigger one. |

For the technical details, see [For app makers](for-app-makers.md).
