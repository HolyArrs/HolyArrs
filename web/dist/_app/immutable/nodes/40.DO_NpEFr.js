import{$i as e,Ai as t,Ci as n,Ei as r,Ii as i,Ki as a,Kn as o,Li as s,Mi as c,Mr as l,Ni as u,Oi as d,Qi as f,Ri as p,Vi as m,Vn as h,Vr as g,Xi as ee,Xn as _,ba as v,ci as y,ea as b,er as x,fa as S,ia as C,ji as w,la as T,li as te,na as E,oa as D,p as ne,pa as O,qi as k,qn as A,r as j,ri as re,si as ie,ta as M,wi as ae,ya as N,yi as oe,zi as se,zr as P}from"../chunks/BC1N8Fe3.js";import{n as F}from"../chunks/BOvTqP14.js";import{n as I,o as ce,r as le,t as ue}from"../chunks/C9wD_Ehi.js";import{r as de}from"../chunks/ChlVjBUP.js";var L=[{slug:`claude-code`,name:`Claude Code`,family:`claude`,cloud:!1,needs:`Any Claude Code install; VS Code and JetBrains extensions share it.`,signin:[{text:`Run this in a terminal.`,code:`claude mcp add --transport http --scope user holyarrs {url}`},{text:`Start Claude Code, run /mcp, pick holyarrs, choose Authenticate.`},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Claude Code and press Make a key.`},{text:`Run this in a terminal.`,code:`claude mcp add --transport http --scope user holyarrs {url} --header "Authorization: Bearer {key}"`}],check:`claude mcp list shows holyarrs as Connected.`,note:`The key sits in plain text in ~/.claude.json.`,source:`live`},{slug:`claude-app`,name:`Claude app`,family:`claude`,cloud:!0,needs:`Public https HolyArrs; the Free plan allows one custom connector.`,signin:[{text:`In Claude on web or desktop, open Customize › Connectors, press + Add › Add custom connector.`},{text:`Name it HolyArrs, paste the address and press Continue.`,code:`{url}`},{text:`Check the details Claude found and press Continue.`},{text:`Keep Sign in now and Use Claude's published identity, then press Add.`},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[],check:`HolyArrs shows as connected under Customize › Connectors.`,note:`Add it on web or desktop; the phone app and Cowork can then use it too.`,source:`live`},{slug:`claude-desktop-bridge`,name:`Claude Desktop (home)`,family:`claude`,cloud:!1,needs:`Claude Desktop and Node.js, so npx runs.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick Claude Desktop (home) and press Make a key.`},{text:`In Claude Desktop, open Settings › Developer and press Edit config.`},{text:`Add the holyarrs entry under mcpServers in claude_desktop_config.json.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "command": "npx",
      "args": [
        "-y",
        "mcp-remote@latest",
        "{url}",
        "--transport",
        "http-only",
        "--allow-http",
        "--header",
        "Authorization:\${AUTH_HEADER}"
      ],
      "env": {
        "AUTH_HEADER": "Bearer {key}"
      }
    }
  }
}`},{text:`Quit Claude Desktop from the tray or menu bar, then reopen it.`}],check:`In a chat, press + › Connectors › Manage connectors; holyarrs is listed.`,note:"Keep Authorization:${AUTH_HEADER} without a space; Windows splits arguments at spaces.",source:`docs`},{slug:`chatgpt`,name:`ChatGPT`,family:`chatgpt-codex`,cloud:!0,needs:`Check your ChatGPT plan first: not every plan can add a custom MCP server.`,signin:[{text:`On chatgpt.com, open Plugins, press + and choose Add custom MCP server.`},{text:`Name it HolyArrs and paste the address into Server URL.`,code:`{url}`},{text:`Choose OAuth, leave client ID and secret empty, accept the warning, press Create as a plugin.`},{text:`Install the HolyArrs plugin. HolyArrs opens in your browser. Press Allow.`}],key:[],check:`Type @HolyArrs in a new chat and ask a question.`,note:`Any login in front of HolyArrs, such as Cloudflare Access, blocks ChatGPT.`,source:`docs`},{slug:`chatgpt-desktop`,name:`ChatGPT desktop app`,family:`chatgpt-codex`,cloud:!1,needs:`New ChatGPT desktop app (July 2026 or later), not ChatGPT Classic.`,signin:[{text:`Open Settings › MCP servers › Add server.`},{text:`Name it holyarrs, choose Streamable HTTP, paste the address and save.`,code:`{url}`},{text:`Restart the app. In MCP servers, press Authenticate next to holyarrs.`},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick ChatGPT desktop app and press Make a key.`},{text:`Add this block to ~/.codex/config.toml (Windows: %USERPROFILE%\\.codex\\config.toml).`,code:`[mcp_servers.holyarrs]
url = "{url}"
http_headers = { "Authorization" = "Bearer {key}" }`},{text:`Restart the ChatGPT app.`}],check:`Settings › MCP servers lists holyarrs.`,note:`Shares ~/.codex/config.toml with Codex; a server added in either shows in both.`,source:`docs`},{slug:`codex-cli`,name:`Codex CLI`,family:`chatgpt-codex`,cloud:!1,needs:`Current Codex, signed in with ChatGPT or an OpenAI API key.`,signin:[{text:`Run this in a terminal.`,code:`codex mcp add holyarrs --url {url}`},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Codex CLI and press Make a key.`},{text:`Add this block to ~/.codex/config.toml (Windows: %USERPROFILE%\\.codex\\config.toml).`,code:`[mcp_servers.holyarrs]
url = "{url}"
http_headers = { "Authorization" = "Bearer {key}" }`},{text:`Restart Codex.`}],check:`Run codex mcp list; holyarrs is listed.`,note:`If no browser opens, run codex mcp login holyarrs.`,source:`live`},{slug:`codex-ide`,name:`Codex IDE extension`,family:`chatgpt-codex`,cloud:!1,needs:`Current Codex IDE extension, signed in.`,signin:[{text:`Open the gear menu, then MCP servers › Add server.`},{text:`Name it holyarrs, choose Streamable HTTP, paste the address and save.`,code:`{url}`},{text:`Press Restart extension. In MCP servers, press Authenticate next to holyarrs.`},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Codex IDE extension and press Make a key.`},{text:`Add this block to ~/.codex/config.toml (Windows: %USERPROFILE%\\.codex\\config.toml).`,code:`[mcp_servers.holyarrs]
url = "{url}"
http_headers = { "Authorization" = "Bearer {key}" }`},{text:`Press Restart extension.`}],check:`Type /mcp in Codex; holyarrs is listed.`,note:`Codex inside WSL reads its own ~/.codex/config.toml.`,source:`docs`},{slug:`gemini-cli`,name:`Gemini CLI`,family:`gemini`,cloud:!1,needs:`Paid Gemini API key, Vertex AI, or Code Assist Standard/Enterprise license.`,signin:[{text:`Add HolyArrs for all folders.`,code:`gemini mcp add --scope user --transport http holyarrs {url}`},{text:`Start gemini and run /mcp auth holyarrs.`,code:`/mcp auth holyarrs`},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Gemini CLI and press Make a key.`},{text:`Add HolyArrs for all folders.`,code:`gemini mcp add --scope user --transport http --header "Authorization: Bearer {key}" holyarrs {url}`},{text:`Start gemini and run /mcp list.`,code:`/mcp list`}],check:`gemini mcp list shows holyarrs as connected.`,note:`Personal Google accounts can no longer sign in to Gemini CLI; Google points them to Antigravity.`,source:`docs`},{slug:`antigravity`,name:`Antigravity`,family:`gemini`,cloud:!1,needs:`Antigravity editor, app or CLI.`,signin:[{text:`Open the global MCP config file.`,code:`~/.gemini/config/mcp_config.json`},{text:`Add this inside mcpServers and save.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "serverUrl": "{url}"
    }
  }
}`},{text:`In Agent settings › Customizations, press Authenticate next to holyarrs.`},{text:`HolyArrs opens in your browser. Press Allow, then paste the code into Antigravity and submit it.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Antigravity and press Make a key.`},{text:`Open the global MCP config file.`,code:`~/.gemini/config/mcp_config.json`},{text:`Add this inside mcpServers and save.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "serverUrl": "{url}",
      "headers": {
        "Authorization": "Bearer {key}"
      }
    }
  }
}`},{text:`Restart Antigravity.`}],check:`Run /mcp in the Antigravity CLI; holyarrs shows connected.`,note:`Google does not say plain http works; if refused, use https and sign-in.`,source:`docs`},{slug:`gemini-code-assist`,name:`Gemini Code Assist`,family:`gemini`,cloud:!1,needs:`Standard or Enterprise license; extension in VS Code.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick Gemini Code Assist and press Make a key.`},{text:`Add this inside mcpServers in ~/.gemini/settings.json.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "httpUrl": "{url}",
      "headers": {
        "Authorization": "Bearer {key}"
      }
    }
  }
}`},{text:`Run Developer: Reload Window from the Command Palette.`}],check:`Type /mcp in Code Assist chat; holyarrs lists its tools.`,note:`JetBrains IDEs need the npx mcp-remote bridge, which needs Node.js.`,source:`docs`},{slug:`gemini-app`,name:`Gemini app`,family:`gemini`,cloud:!0,needs:`US, 18+, personal Google account, English, Gemini chat history on.`,signin:[{text:`In Gemini, open Connected Apps and press + Custom.`},{text:`Paste the address. Under Additional settings, press Copy redirect URI.`,code:`{url}`},{text:`In HolyArrs, open Settings › AI connections › Gemini app, paste it and press Make client ID.`},{text:`Paste that Client ID into Gemini, leave Client secret empty, press Next.`},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[],check:`In a chat, type @, pick HolyArrs and ask a question.`,note:`Gemini does not register itself, so it needs this client ID.`,source:`live`,clientIdHelper:!0},{slug:`vs-code-copilot`,name:`VS Code with Copilot`,family:`copilot`,cloud:!1,needs:`GitHub Copilot signed in; Business or Enterprise admins can block MCP servers.`,signin:[{text:`In VS Code, run MCP: Add Server from the Command Palette.`},{text:`Pick HTTP, paste this address and name it holyarrs.`,code:`{url}`},{text:`Choose Global, then Copilot Global.`},{text:`Run MCP: List Servers, pick holyarrs, choose Start. HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick VS Code with Copilot and press Make a key.`},{text:`Press Add to VS Code and confirm the install in VS Code.`},{text:`Or add this entry to ~/.copilot/mcp-config.json.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "type": "http",
      "url": "{url}",
      "headers": {
        "Authorization": "Bearer {key}"
      },
      "tools": ["*"]
    }
  }
}`}],check:`Run MCP: List Servers; holyarrs shows as running.`,note:`~/.copilot/mcp-config.json is shared with the Copilot CLI.`,source:`docs`},{slug:`copilot-cli`,name:`GitHub Copilot CLI`,family:`copilot`,cloud:!1,needs:`A GitHub Copilot plan.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick GitHub Copilot CLI and press Make a key.`},{text:`Run this in a terminal.`,code:`copilot mcp add --transport http --header "Authorization: Bearer {key}" holyarrs {url}`},{text:`Or type /mcp add: name holyarrs, type HTTP, URL {url}, this header; press Ctrl+S.`,code:`{"Authorization": "Bearer {key}"}`}],check:`Run copilot mcp get holyarrs, or type /mcp show holyarrs.`,note:`Browser sign-in for remote servers is undocumented; use a key.`,source:`docs`},{slug:`copilot-cloud-agent`,name:`GitHub Copilot cloud agent`,family:`copilot`,cloud:!0,needs:`Copilot cloud agent enabled on the repository; HolyArrs on public https.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick GitHub Copilot cloud agent and press Make a key.`},{text:`In the repository, add an Agents secret COPILOT_MCP_HOLYARRS_KEY holding the key.`},{text:`Open the repository's Settings › Copilot › MCP servers.`},{text:`Paste this into MCP configuration and press Save MCP configuration.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "type": "http",
      "url": "{url}",
      "headers": {
        "Authorization": "Bearer $COPILOT_MCP_HOLYARRS_KEY"
      },
      "tools": ["search_titles", "library_lookup", "calendar", "downloads", "my_asks", "health"]
    }
  }
}`}],check:`Give it a task asking what's downloading in HolyArrs.`,note:`It runs tools without asking, so ask_for_title is left out.`,source:`docs`},{slug:`m365-copilot`,name:`Microsoft 365 Copilot`,family:`copilot`,cloud:!0,needs:`Work account, Agents Toolkit 6.12+, admin allowing custom app upload.`,signin:[{text:`In VS Code's Microsoft 365 Agents Toolkit, select Create a New Agent/App › Declarative Agent.`},{text:`Select Add an Action › Start with an MCP Server and enter this address.`,code:`{url}`},{text:`Choose OAuth (with dynamic registration), name the agent, then select Lifecycle › Provision.`},{text:`In Microsoft 365 Copilot chat, pick the agent and press its sign-in button. HolyArrs opens in your browser. Press Allow.`}],key:[],check:`Ask the agent what's downloading; it answers from HolyArrs.`,note:`Unconfirmed whether it accepts sign-in without refresh tokens or client secrets.`,source:`docs`},{slug:`copilot-studio`,name:`Copilot Studio`,family:`copilot`,cloud:!0,needs:`Copilot Studio licence and environment; HolyArrs on public https.`,signin:[{text:`In your agent, open Tools › Add a tool › New tool › Model Context Protocol.`},{text:`Set Server name holyarrs, a Server description and this Server URL.`,code:`{url}`},{text:`Pick OAuth 2.0, type Dynamic discovery, then select Create and Next.`},{text:`Select Create a new connection, then Add to agent. HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Copilot Studio and press Make a key.`},{text:`In your agent, open Tools › Add a tool › New tool › Model Context Protocol.`},{text:`Set Server name holyarrs, a Server description and this Server URL.`,code:`{url}`},{text:`Pick API key, type Header, name X-Api-Key; connect with this key, then Add to agent.`,code:`{key}`}],check:`Ask the agent what's downloading; it answers from HolyArrs.`,note:`OAuth fit is unconfirmed (no refresh token); the key way avoids it.`,source:`docs`},{slug:`grok`,name:`Grok`,family:`chat-apps`,cloud:!0,needs:`Public https HolyArrs; on Business and Enterprise, an admin sets up connectors first.`,signin:[{text:`Open grok.com/connectors, press New Connector and pick Custom.`},{text:`Paste the address as the server URL.`,code:`{url}`},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[],check:`The connector shows as connected and lists its tools.`,note:`Grok's help names no sign-in types, so browser sign-in is unconfirmed.`,source:`live`},{slug:`perplexity`,name:`Perplexity`,family:`chat-apps`,cloud:!0,needs:`Pro, Max or Enterprise plan, and public https HolyArrs.`,signin:[{text:`Open Plugins, press Add custom, then Add MCP connector, then Remote.`},{text:`Set Name to HolyArrs and paste the address under MCP server URL.`,code:`{url}`},{text:`Under Advanced, set these, tick the risk box and press Add.`,code:`Authentication: OAuth
Transport: Streamable HTTP
Network access: Public`},{text:`Click the HolyArrs card. HolyArrs opens in your browser. Press Allow.`}],key:[],check:`Switch HolyArrs on under Sources; its card lists the tools.`,note:`On a client_secret error, set Authentication to API Key and paste an AI key (unconfirmed).`,source:`live`},{slug:`perplexity-mac-helper`,name:`Perplexity Mac helper`,family:`chat-apps`,cloud:!1,needs:`Mac App Store version of Perplexity, and Node.js.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick Perplexity Mac helper and press Make a key.`},{text:`In the Mac app, open Plugins and install PerplexityXPC when asked.`},{text:`Press Add custom; on the Simple tab, set Server Name to holyarrs and paste this into Command.`,code:`npx -y mcp-remote {url} --allow-http --transport http-only --header "Authorization: Bearer {key}"`},{text:`Press Save, then switch HolyArrs on under Sources.`}],check:`The server status reads Running.`,note:`Keep the command on one line.`,source:`docs`},{slug:`le-chat`,name:`Mistral Le Chat`,family:`chat-apps`,cloud:!0,needs:`Public https HolyArrs. On team plans, an admin adds connectors.`,signin:[{text:`Open Connectors, press + Add Connector, then the Custom MCP Connector tab.`},{text:`Fill in the name and server address.`,code:`Connector name: holyarrs
Server URL: {url}`},{text:`Press Connect. HolyArrs opens in your browser. Press Allow.`}],key:[],check:`Under My Connectors, the HolyArrs card lists its tools in Functions.`,note:`Mistral's docs now file connectors under Vibe Work, so labels may differ.`,source:`docs`},{slug:`manus`,name:`Manus`,family:`chat-apps`,cloud:!0,needs:`Public https HolyArrs.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick Manus and press Make a key.`},{text:`In Manus, open Connectors, press Create and choose an MCP server by URL.`},{text:`Enter the address, and the key as a bearer token under Authentication.`,code:`Server URL: {url}
Token: {key}`},{text:`Press Test connection.`}],check:`Test connection lists the HolyArrs tools.`,note:`Older help puts this under Settings › Integrations › Custom MCP Servers › Add Server.`,source:`docs`},{slug:`genspark`,name:`Genspark`,family:`chat-apps`,cloud:!0,needs:`Public https HolyArrs. Work accounts follow the admin's custom MCP policy.`,signin:[{text:`Open Skills, then Connectors.`},{text:`Add a custom MCP server and paste the address as MCP URL.`,code:`{url}`},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[],check:`The connector shows as connected and lists its tools.`,note:`Whether Genspark can sign in or send a key is unconfirmed.`,source:`second-hand`},{slug:`huggingchat`,name:`HuggingChat`,family:`chat-apps`,cloud:!0,needs:`Public https HolyArrs, and a model that supports tools.`,signin:[{text:`In the chat box, press +, pick MCP Servers, then Add Server.`},{text:`Type a name and paste the address.`,code:`{url}`},{text:`Run Health Check. HolyArrs opens in your browser. Press Allow.`}],key:[],check:`Health Check passes; switch HolyArrs on in MCP Servers.`,note:`No key field and sign-in is unconfirmed, so this may not work yet.`,source:`docs`},{slug:`qwen-desktop`,name:`Qwen Chat desktop app`,family:`chat-apps`,cloud:!1,needs:`Qwen Chat desktop app on Mac or Windows, and Node.js 20+.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick Qwen Chat desktop app and press Make a key.`},{text:`In a chat, open the MCP option, then MCP settings › Add MCP.`},{text:`Choose the JSON form, paste this and press Save and Enable.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "command": "npx",
      "args": ["-y", "mcp-remote@latest", "{url}", "--transport", "http-only", "--allow-http", "--header", "Authorization:\${HOLYARRS_AUTH}"],
      "env": { "HOLYARRS_AUTH": "Bearer {key}" }
    }
  }
}`}],check:`A green mark shows next to holyarrs.`,note:`Quick Add has no key field; use the JSON form.`,source:`second-hand`},{slug:`cursor`,name:`Cursor`,family:`code-editors`,cloud:!1,needs:``,signin:[{text:`Open ~/.cursor/mcp.json.`},{text:`Add this entry and save:`,code:`{
  "mcpServers": {
    "holyarrs": {
      "url": "{url}"
    }
  }
}`},{text:`Under Customize, click Needs login next to holyarrs. HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Cursor and press Make a key.`},{text:`Open ~/.cursor/mcp.json.`},{text:`Add this entry and save:`,code:`{
  "mcpServers": {
    "holyarrs": {
      "url": "{url}",
      "headers": {
        "Authorization": "Bearer {key}"
      }
    }
  }
}`}],check:`Customize lists holyarrs; errors show in Output › MCP Logs.`,note:`The Needs login label is second-hand; Cursor's docs do not name it.`,source:`docs`},{slug:`windsurf`,name:`Windsurf`,family:`code-editors`,cloud:!1,needs:`Teams plans need a full seat; an admin allow-list can block it.`,signin:[{text:`Open ~/.config/devin/mcp_config.json (Windows: %APPDATA%\\devin\\mcp_config.json).`},{text:`Add this entry and save:`,code:`{
  "mcpServers": {
    "holyarrs": {
      "url": "{url}"
    }
  }
}`},{text:`Under Customize › MCPs, press Authenticate next to holyarrs. HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Windsurf and press Make a key.`},{text:`Open ~/.config/devin/mcp_config.json (Windows: %APPDATA%\\devin\\mcp_config.json).`},{text:`Add this entry and save:`,code:`{
  "mcpServers": {
    "holyarrs": {
      "url": "{url}",
      "headers": {
        "Authorization": "Bearer {key}"
      }
    }
  }
}`}],check:`Customize › MCPs lists holyarrs, switched on.`,note:`Older Windsurf builds read ~/.codeium/windsurf/mcp_config.json instead (second-hand).`,source:`docs`},{slug:`zed`,name:`Zed`,family:`code-editors`,cloud:!1,needs:``,signin:[{text:`Run zed: open settings file from the command palette.`},{text:`Add context_servers inside your settings and save:`,code:`{
  "context_servers": {
    "holyarrs": {
      "url": "{url}"
    }
  }
}`},{text:`Zed starts the sign-in. HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Zed and press Make a key.`},{text:`Run zed: open settings file from the command palette.`},{text:`Add context_servers inside your settings and save:`,code:`{
  "context_servers": {
    "holyarrs": {
      "url": "{url}",
      "headers": {
        "Authorization": "Bearer {key}"
      }
    }
  }
}`}],check:`holyarrs shows a green dot and "Server is active".`,note:``,source:`docs`},{slug:`jetbrains-ai`,name:`JetBrains AI Assistant`,family:`code-editors`,cloud:!1,needs:`AI Assistant installed; a company admin can block self-added servers.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick JetBrains AI Assistant and press Make a key.`},{text:`Open Settings › Tools › AI Assistant › Model Context Protocol (MCP) and press Add.`},{text:`Pick HTTP (Streamable HTTP), paste this entry, then press OK and Apply:`,code:`{
  "mcpServers": {
    "holyarrs": {
      "url": "{url}",
      "headers": {
        "Authorization": "Bearer {key}"
      }
    }
  }
}`}],check:`The Status column shows holyarrs; its icon lists the tools.`,note:`JetBrains documents only url; whether it sends headers is unconfirmed.`,source:`docs`},{slug:`kiro`,name:`Kiro`,family:`code-editors`,cloud:!1,needs:`An https HolyArrs address, or Kiro on the HolyArrs computer.`,signin:[{text:`In Kiro Settings, search MCP and switch on MCP support.`},{text:`From the Command Palette, run Kiro: Open user MCP config (JSON).`},{text:`Add this entry and save:`,code:`{
  "mcpServers": {
    "holyarrs": {
      "url": "{url}"
    }
  }
}`},{text:`Kiro starts the sign-in. HolyArrs opens in your browser. Press Allow.`}],key:[],check:`Kiro's MCP panel lists holyarrs with its tools.`,note:`Kiro refuses plain http network addresses, so there is no key way.`,source:`docs`},{slug:`cline`,name:`Cline`,family:`vscode-add-ons`,cloud:!1,needs:``,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick Cline and press Make a key.`},{text:`In Cline, click the MCP Servers icon, then Configure › Configure MCP Servers.`},{text:`Add this to cline_mcp_settings.json and save.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "type": "streamableHttp",
      "url": "{url}",
      "headers": {
        "Authorization": "Bearer {key}"
      }
    }
  }
}`}],check:`holyarrs shows in Cline's MCP server list without an error.`,note:`Keep "type": "streamableHttp"; without it Cline falls back to SSE and fails.`,source:`docs`},{slug:`continue`,name:`Continue`,family:`vscode-add-ons`,cloud:!1,needs:``,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick Continue and press Make a key.`},{text:`Open ~/.continue/config.yaml from the gear next to Local Config.`},{text:`Add this entry under mcpServers and save.`,code:`mcpServers:
  - name: HolyArrs
    type: streamable-http
    url: {url}
    requestOptions:
      headers:
        Authorization: Bearer {key}`},{text:`Switch the chat to agent mode.`}],check:`HolyArrs shows in Continue's MCP list without an error.`,note:`Some versions may not send the key header; if HolyArrs answers "sign in", use Cline.`,source:`docs`},{slug:`kilo-code`,name:`Kilo Code`,family:`vscode-add-ons`,cloud:!1,needs:``,signin:[{text:`Add this under mcp in ~/.config/kilo/kilo.jsonc and save.`,code:`{
  "mcp": {
    "holyarrs": {
      "type": "remote",
      "url": "{url}"
    }
  }
}`},{text:`Click the notice that holyarrs needs sign-in.`},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Kilo Code and press Make a key.`},{text:`Add this under mcp in ~/.config/kilo/kilo.jsonc and save.`,code:`{
  "mcp": {
    "holyarrs": {
      "type": "remote",
      "url": "{url}",
      "headers": {
        "Authorization": "Bearer {key}"
      }
    }
  }
}`}],check:`holyarrs shows in Kilo Code's MCP server list without an error.`,note:`If Kilo Code tries sign-in on plain http, add "oauth": false to the entry.`,source:`docs`},{slug:`opencode`,name:`OpenCode`,family:`terminal-tools`,cloud:!1,needs:``,signin:[{text:`Add this to ~/.config/opencode/opencode.json.`,code:`{
  "mcp": {
    "holyarrs": {
      "type": "remote",
      "url": "{url}"
    }
  }
}`},{text:`Run this to sign in.`,code:`opencode mcp auth holyarrs`},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick OpenCode and press Make a key.`},{text:`Add this to ~/.config/opencode/opencode.json.`,code:`{
  "mcp": {
    "holyarrs": {
      "type": "remote",
      "url": "{url}",
      "oauth": false,
      "headers": {
        "Authorization": "Bearer {key}"
      }
    }
  }
}`},{text:`Restart OpenCode.`}],check:`opencode mcp list shows holyarrs and its sign-in state.`,note:`A project can use its own opencode.json instead.`,source:`docs`},{slug:`warp`,name:`Warp`,family:`terminal-tools`,cloud:!1,needs:``,signin:[{text:`In Warp, open Settings › Agents › MCP servers and press + Add.`},{text:`Pick URL and paste this.`,code:`{
  "holyarrs": {
    "url": "{url}"
  }
}`},{text:`When the server starts, HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Warp and press Make a key.`},{text:`In Warp, open Settings › Agents › MCP servers and press + Add.`},{text:`Pick URL and paste this.`,code:`{
  "holyarrs": {
    "url": "{url}",
    "headers": {
      "Authorization": "Bearer {key}"
    }
  }
}`}],check:`holyarrs is listed under Settings › Agents › MCP servers.`,note:`Covers local agents only; cloud agents set up separately. Sign-in is per computer.`,source:`docs`},{slug:`amp`,name:`Amp`,family:`terminal-tools`,cloud:!1,needs:``,signin:[{text:`Add HolyArrs.`,code:`amp mcp add holyarrs {url}`},{text:`Start Amp. HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Amp and press Make a key.`},{text:`Add the amp.mcpServers block to ~/.config/amp/settings.json.`,code:`{
  "amp.mcpServers": {
    "holyarrs": {
      "url": "{url}",
      "headers": {
        "Authorization": "Bearer {key}"
      }
    }
  }
}`},{text:`Restart Amp.`}],check:`amp mcp doctor shows holyarrs and its state.`,note:`In a project's .amp/settings.json, run amp mcp approve holyarrs first.`,source:`docs`},{slug:`pi`,name:`Pi`,family:`terminal-tools`,cloud:!1,needs:`Pi 0.99.0 or newer.`,signin:[{text:`Add this to ~/.pi/agent/mcp.json.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "url": "{url}"
    }
  }
}`},{text:`Run this, or /mcp login holyarrs inside Pi.`,code:`pi mcp login holyarrs`},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Pi and press Make a key.`},{text:`Add this to ~/.pi/agent/mcp.json.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "url": "{url}",
      "headers": {
        "Authorization": "Bearer {key}"
      }
    }
  }
}`},{text:`Restart Pi.`}],check:`Type /mcp inside Pi; holyarrs is listed.`,note:``,source:`docs`},{slug:`mistral-vibe`,name:`Mistral Vibe CLI`,family:`terminal-tools`,cloud:!1,needs:``,signin:[{text:`Add HolyArrs.`,code:`vibe mcp add holyarrs --url {url}`},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Mistral Vibe CLI and press Make a key.`},{text:`Put the key in HOLYARRS_AI_KEY.`,code:`export HOLYARRS_AI_KEY="{key}"`},{text:`Add this to ~/.vibe/config.toml.`,code:`[[mcp_servers]]
name = "holyarrs"
transport = "http"
url = "{url}"

[mcp_servers.auth]
type = "static"
api_key_env = "HOLYARRS_AI_KEY"
api_key_header = "Authorization"
api_key_format = "Bearer {token}"`},{text:`Start Vibe from that shell.`}],check:`/mcp holyarrs inside Vibe lists the HolyArrs tools.`,note:`If no browser opens, run /mcp login holyarrs inside Vibe.`,source:`docs`},{slug:`kimi-code`,name:`Kimi Code CLI`,family:`terminal-tools`,cloud:!1,needs:``,signin:[{text:`Add this to ~/.kimi-code/mcp.json.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "url": "{url}"
    }
  }
}`},{text:`Inside Kimi Code, run this.`,code:`/mcp-config login holyarrs`},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Kimi Code CLI and press Make a key.`},{text:`Add this to ~/.kimi-code/mcp.json.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "url": "{url}",
      "headers": {
        "Authorization": "Bearer {key}"
      }
    }
  }
}`},{text:`Restart Kimi Code.`}],check:`/mcp inside Kimi Code shows holyarrs connected.`,note:``,source:`docs`},{slug:`devin`,name:`Devin`,family:`terminal-tools`,cloud:!0,needs:`Public https HolyArrs without a login wall; Manage MCP Servers permission.`,signin:[{text:`In Devin, open Customize › MCPs, press Add MCP, then Add custom MCP.`},{text:`Fill in these fields and press Save.`,code:`Server Name: HolyArrs
Transport:   HTTP
Server URL:  {url}
Auth:        OAuth
Access:      Personal`},{text:`Press Connect. HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Devin and press Make a key.`},{text:`In Devin, open Customize › MCPs, press Add MCP, then Add custom MCP.`},{text:`Fill in these fields and press Save.`,code:`Server Name:  HolyArrs
Transport:    HTTP
Server URL:   {url}
Auth:         Auth Header
Header name:  Authorization
Header value: Bearer {key}`}],check:`Press Test tools; it lists the HolyArrs tools.`,note:`Access Organization shares one sign-in: everyone acts as whoever pressed Allow.`,source:`docs`},{slug:`open-webui`,name:`Open WebUI`,family:`self-hosted-chat`,cloud:!1,needs:`Open WebUI v0.6.31 or newer and an Open WebUI admin account.`,signin:[{text:`In Open WebUI, open Admin Panel › Settings › Integrations and press + Add Connection.`},{text:`Set Type MCP (Streamable HTTP), Auth OAuth 2.1, Server URL {url}; leave Key empty.`,code:`Server URL: {url}`},{text:`Give Open WebUI a fixed secret so the sign-in survives restarts.`,code:`WEBUI_SECRET_KEY=<long random text you keep>`},{text:`Turn on HolyArrs in a chat's tools menu. HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Open WebUI and press Make a key.`},{text:`In Open WebUI, open Admin Panel › Settings › Integrations and press + Add Connection.`},{text:`Set Type MCP (Streamable HTTP), Auth Bearer, fill in these fields and save.`,code:`Server URL: {url}
Key: {key}`},{text:`Turn on HolyArrs in a chat's tools menu.`}],check:`A new chat's tools menu lists the HolyArrs tools.`,note:`In Docker, localhost is the container; use HolyArrs's network IP.`,source:`docs`},{slug:`librechat`,name:`LibreChat`,family:`self-hosted-chat`,cloud:!1,needs:`A LibreChat role that may add MCP servers.`,signin:[{text:`Open MCP Settings › Add MCP, paste {url}, pick Transport Streamable HTTP.`,code:`{url}`},{text:`Under Authentication pick OAuth; leave Client ID and Client Secret empty.`},{text:`Tick I trust this application and press Create.`},{text:`Connect the server. HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick LibreChat and press Make a key.`},{text:`On a home address, allow HolyArrs in librechat.yaml and restart LibreChat.`,code:`mcpSettings:
  allowedAddresses:
    - "<HolyArrs IP>:<port>"`},{text:`Open MCP Settings › Add MCP, paste {url}, pick Transport Streamable HTTP.`,code:`{url}`},{text:`Set Authentication API Key, Header Format Bearer, enter {key}, tick I trust this application, press Create.`,code:`{key}`}],check:`The message box's + › MCP Servers lists HolyArrs.`,note:`Use allowedAddresses; allowedDomains blocks every unlisted server.`,source:`docs`},{slug:`lobehub`,name:`LobeHub`,family:`self-hosted-chat`,cloud:!1,needs:`The LobeHub desktop app when HolyArrs is on a home address.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick LobeHub and press Make a key.`},{text:`In LobeHub, open Settings › Connectors and add an MCP server by hand.`},{text:`Choose HTTP, enter {url}, set auth API Key, enter {key}, save.`,code:`URL: {url}
API Key: {key}`},{text:`Open Agent Settings › Connectors, select HolyArrs and save.`}],check:`Settings › Connectors shows HolyArrs as connected.`,note:`No API Key field? Add the header Authorization: Bearer followed by your key instead.`,source:`docs`},{slug:`lm-studio`,name:`LM Studio`,family:`desktop-apps`,cloud:!1,needs:`LM Studio 0.3.17 or newer; 0.4.10 or newer for browser sign-in.`,signin:[{text:`In LM Studio, open the Program tab, pick Install, then Edit mcp.json.`},{text:`Add this entry and save.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "url": "{url}"
    }
  }
}`},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick LM Studio and press Make a key.`},{text:`In LM Studio, open the Program tab, pick Install, then Edit mcp.json.`},{text:`Add this entry and save.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "url": "{url}",
      "headers": {
        "Authorization": "Bearer {key}"
      }
    }
  }
}`}],check:`holyarrs lists its tools in LM Studio.`,note:`If mcp.json lists other servers, add holyarrs beside them.`,source:`docs`},{slug:`jan`,name:`Jan`,family:`desktop-apps`,cloud:!1,needs:`Node.js for the key way.`,signin:[{text:`In Jan, open Settings › MCP Servers and press + Add MCP Server.`},{text:`Set Transport to HTTP and paste this address.`,code:`{url}`},{text:`Press Sign in on the Auth line. HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Jan and press Make a key.`},{text:`In Jan, open Settings › MCP Servers and press + Add MCP Server.`},{text:`Set Transport to STDIO, Command to npx, and these Args, one per line.`,code:`-y
mcp-remote@latest
{url}
--transport
http-only
--allow-http
--header
Authorization:\${AUTH_HEADER}`},{text:`In Env, set AUTH_HEADER to this value.`,code:`Bearer {key}`}],check:`holyarrs lists its tools in Jan.`,note:`Jan stops a tool call after 30 seconds by default.`,source:`docs`},{slug:`anythingllm`,name:`AnythingLLM`,family:`desktop-apps`,cloud:!1,needs:`AnythingLLM Desktop v1.8.0 or newer.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick AnythingLLM and press Make a key.`},{text:`In AnythingLLM, open Agent Skills once to create the servers file.`},{text:`Add this entry to plugins/anythingllm_mcp_servers.json in its storage folder.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "type": "streamable",
      "url": "{url}",
      "headers": {
        "Authorization": "Bearer {key}"
      }
    }
  }
}`},{text:`Open Agent Skills again to load holyarrs.`}],check:`holyarrs shows as running under Agent Skills.`,note:`Start chat messages with @agent so the model can use tools.`,source:`docs`},{slug:`cherry-studio`,name:`Cherry Studio`,family:`desktop-apps`,cloud:!1,needs:``,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick Cherry Studio and press Make a key.`},{text:`In Cherry Studio, open Settings › MCP › MCP Servers, press Add, choose Import from JSON.`},{text:`Paste this entry and save.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "type": "streamableHttp",
      "url": "{url}",
      "headers": {
        "Authorization": "Bearer {key}"
      }
    }
  }
}`},{text:`Switch holyarrs on in your assistant's MCP settings.`}],check:`holyarrs shows as running in MCP Servers.`,note:`Without "type": "streamableHttp", Cherry Studio treats the entry as a program.`,source:`second-hand`},{slug:`msty-studio`,name:`Msty Studio`,family:`desktop-apps`,cloud:!1,needs:``,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick Msty Studio and press Make a key.`},{text:`In Msty Studio, open Toolbox › Tools, press Add New Tool and pick HTTP.`},{text:`Name it HolyArrs and paste this into MCP Server URL.`,code:`{url}`},{text:`In Headers, add Authorization with this value, then press Add.`,code:`Bearer {key}`}],check:`Run a HolyArrs tool in the Tool Console.`,note:`Msty's docs do not show the Headers field format.`,source:`docs`},{slug:`5ire`,name:`5ire`,family:`desktop-apps`,cloud:!1,needs:`5ire v0.11.0 or newer, and Node.js.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick 5ire and press Make a key.`},{text:`In 5ire, open Tools and press New.`},{text:`Fill in command, arguments and environment from this entry.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "command": "npx",
      "args": [
        "-y",
        "mcp-remote@latest",
        "{url}",
        "--transport",
        "http-only",
        "--allow-http",
        "--header",
        "Authorization:\${AUTH_HEADER}"
      ],
      "env": {
        "AUTH_HEADER": "Bearer {key}"
      }
    }
  }
}`},{text:`Switch the tool on.`}],check:``,note:`Drop --allow-http when HolyArrs is on https.`,source:`second-hand`},{slug:`hermes-agent`,name:`Hermes Agent`,family:`local-agents`,cloud:!1,needs:``,signin:[{text:`Add this to ~/.hermes/config.yaml.`,code:`mcp_servers:
  holyarrs:
    url: "{url}"
    auth: oauth`},{text:`Start the sign-in.`,code:`hermes mcp login holyarrs`},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Hermes Agent and press Make a key.`},{text:`Add this to ~/.hermes/config.yaml.`,code:`mcp_servers:
  holyarrs:
    url: "{url}"
    headers:
      Authorization: "Bearer {key}"`},{text:`Run /reload-mcp in a running session, or start a new one.`,code:`/reload-mcp`}],check:`hermes mcp test holyarrs lists the HolyArrs tools.`,note:`Tools show as mcp_holyarrs_<tool>; hide one under tools: exclude.`,source:`docs`},{slug:`goose`,name:`Goose`,family:`local-agents`,cloud:!1,needs:``,signin:[{text:`Run goose configure, then Add Extension › Remote Extension (Streamable HTTP).`,code:`goose configure`},{text:`Name it holyarrs, enter this URI, keep the default timeout, skip custom headers.`,code:`{url}`},{text:`Run goose session. HolyArrs opens in your browser. Press Allow.`,code:`goose session`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Goose and press Make a key.`},{text:`Run goose configure, then Add Extension › Remote Extension (Streamable HTTP).`,code:`goose configure`},{text:`Name it holyarrs, enter this URI, keep the default timeout.`,code:`{url}`},{text:`Add a custom header named Authorization with this value.`,code:`Bearer {key}`}],check:`Goose's extension list shows holyarrs.`,note:`Desktop app: Extensions › Add custom extension, same URI and header.`,source:`docs`},{slug:`ollmcp`,name:`ollmcp`,family:`local-agents`,cloud:!1,needs:`Python 3.11 or newer; Ollama running with a pulled model.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick ollmcp and press Make a key.`},{text:`Install ollmcp.`,code:`uv tool install --upgrade ollmcp`},{text:`Save this as holyarrs.json.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "type": "streamable_http",
      "url": "{url}",
      "headers": {
        "Authorization": "Bearer {key}"
      }
    }
  }
}`},{text:`Start a chat with your Ollama model.`,code:`ollmcp -j holyarrs.json -m qwen3.5`}],check:`Type /tools in the chat; HolyArrs tools are listed.`,note:`-H on the ollmcp command sets the Ollama host, not a header.`,source:`live`},{slug:`llama-cpp`,name:`llama.cpp web UI`,family:`local-runners`,cloud:!1,needs:`Node.js for npx. llama.cpp MCP support is experimental.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick llama.cpp web UI and press Make a key.`},{text:`Save this as mcp.json in the folder you start llama-server from.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "command": "npx",
      "args": [
        "-y",
        "mcp-remote@latest",
        "{url}",
        "--transport",
        "http-only",
        "--allow-http",
        "--header",
        "Authorization:\${AUTH_HEADER}"
      ],
      "env": {
        "AUTH_HEADER": "Bearer {key}"
      }
    }
  }
}`},{text:`Start llama-server with the file.`,code:`llama-server -m <your model>.gguf --mcp-servers-config mcp.json`},{text:`Open the llama.cpp web UI.`}],check:`The HolyArrs tools show in the web UI.`,note:`On an https address, drop --allow-http.`,source:`docs`},{slug:`localai`,name:`LocalAI`,family:`local-runners`,cloud:!1,needs:``,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick LocalAI and press Make a key.`},{text:`Add this mcp block to your model's YAML config.`,code:`mcp:
  remote: |
    {
      "mcpServers": {
        "holyarrs": {
          "url": "{url}",
          "token": "{key}"
        }
      }
    }`},{text:`Reload the model.`}],check:`In chat, MCP > Servers lists holyarrs with no errors.`,note:`In the model editor, paste only the inner mcpServers JSON into Remote MCP Servers.`,source:`docs`},{slug:`text-generation-webui`,name:`text-generation-webui`,family:`local-runners`,cloud:!1,needs:``,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick text-generation-webui and press Make a key.`},{text:`In the Model tab, load a model with tool-call support.`},{text:`In the chat sidebar, open MCP servers and add this line.`,code:`{url},Authorization: Bearer {key}`}],check:`Ask "Use HolyArrs: what is downloading?"; a tool runs.`,note:`Tool calls run without asking you first.`,source:`docs`},{slug:`koboldcpp`,name:`KoboldCpp`,family:`local-runners`,cloud:!1,needs:`KoboldCpp 1.106 or newer, and Node.js for npx.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick KoboldCpp and press Make a key.`},{text:`Save this as mcp.json.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "command": "npx",
      "args": [
        "-y",
        "mcp-remote@latest",
        "{url}",
        "--transport",
        "http-only",
        "--allow-http",
        "--header",
        "Authorization:\${AUTH_HEADER}"
      ],
      "env": {
        "AUTH_HEADER": "Bearer {key}"
      }
    }
  }
}`},{text:`Start KoboldCpp with the file.`,code:`koboldcpp --model <your model>.gguf --mcpfile mcp.json`},{text:`In KoboldAI Lite settings, turn on tool calling and fetch the tools.`}],check:`No MCP errors at startup; HolyArrs tools listed in Lite.`,note:`Turn on Manually Approve Tools, or every tool call runs unasked.`,source:`docs`},{slug:`page-assist`,name:`Page Assist`,family:`local-runners`,cloud:!1,needs:``,signin:[{text:`Set up OAuth as Page Assist's MCP guide describes.`},{text:`In Page Assist's MCP settings, add {url} with OAuth 2.1.`},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Page Assist and press Make a key.`},{text:`In Page Assist's MCP settings, add {url} with Bearer token {key}.`},{text:`Turn on Require approval before running MCP tools.`}],check:`The key shows "last used" in Settings › AI connections.`,note:`It calls from your browser, so HolyArrs may refuse it as another site.`,source:`docs`}];function fe(e){return(e.scopes??[]).includes(`mcp`)}function pe(e,t=``){return e.replace(/\/+$/,``)+t.replace(/\/+$/,``)+`/api/v1/mcp`}var R=`<your AI key>`;function me(e){try{let t=new URL(e);return t.protocol===`https:`||[`localhost`,`127.0.0.1`,`[::1]`].includes(t.hostname)}catch{return!1}}function he(e){try{let t=new URL(e),n=t.hostname;return t.protocol!==`https:`||n===`localhost`||n.startsWith(`[`)||/^(10|127)\./.test(n)||/^192\.168\./.test(n)||/^172\.(1[6-9]|2\d|3[01])\./.test(n)||/^100\.(6[4-9]|[7-9]\d|1[01]\d|12[0-7])\./.test(n)||n.endsWith(`.local`)||n.endsWith(`.lan`)||n.endsWith(`.home.arpa`)}catch{return!0}}var z=`https://holyarrs.coderluii.dev/docs/ai/`,B=e=>`Bearer ${e.key??R}`,V=e=>JSON.stringify(e,null,2);function ge(e,t){return[`Set up the HolyArrs MCP server for me in ${e}:`,`1. Add a remote MCP server named "holyarrs" at ${t} (Streamable HTTP), for all my projects.`,`2. Start its sign-in; I will sign in to HolyArrs in the browser and press Allow.`,`3. Tell me when its tools show up.`].join(`
`)}var H=(e,t)=>e.replaceAll(`{url}`,t.url).replaceAll(`{key}`,t.key??R);function U(e){return{id:e.slug,name:e.name,guide:e.slug+`/`,cloud:e.cloud,signIn:e.signin.length>0,needs:e.needs||void 0,note:e.note||void 0,check:e.check||void 0,untried:e.source!==`live`,agent:[`claude-code`,`codex-cli`,`gemini-cli`].includes(e.slug),clientIdHelper:e.clientIdHelper,steps:t=>(t.signIn&&e.signin.length?e.signin:e.key.length?e.key:e.signin).filter(e=>!e.text.startsWith(`In HolyArrs, open Settings`)).map(e=>({text:H(e.text,t),...e.code?{code:H(e.code,t)}:{}}))}}var W={id:`any-app`,name:`Any other app`,guide:`other-apps/`,steps:e=>[{text:`Add a remote MCP server named holyarrs, type Streamable HTTP, with this address.`,code:e.url},e.signIn?{text:`When the app asks, sign in. HolyArrs opens in your browser. Press Allow.`}:{text:`Have the app send this header.`,code:`Authorization: ${B(e)}`},{text:`Apps with a settings file mostly take this form.`,code:V({mcpServers:{holyarrs:e.signIn?{url:e.url}:{url:e.url,headers:{Authorization:B(e)}}}})},{text:`App runs only local programs? Use this bridge (needs Node.js).`,code:`npx -y mcp-remote@latest ${e.url} --transport http-only${e.url.startsWith(`http:`)?` --allow-http`:``} --header "Authorization: ${B(e)}"`}]},G=e=>L.filter(t=>t.family===e).map(U),K=[{id:`claude`,name:`Claude`,apps:G(`claude`)},{id:`chatgpt-codex`,name:`ChatGPT & Codex`,apps:G(`chatgpt-codex`)},{id:`gemini`,name:`Gemini`,apps:G(`gemini`)},{id:`copilot`,name:`Copilot`,apps:G(`copilot`)},{id:`chat-apps`,name:`Grok & more`,apps:G(`chat-apps`)},{id:`code-editors`,name:`Code editors`,apps:G(`code-editors`)},{id:`vscode-add-ons`,name:`VS Code add-ons`,apps:G(`vscode-add-ons`)},{id:`terminal-tools`,name:`Terminal tools`,apps:G(`terminal-tools`)},{id:`self-hosted-chat`,name:`Self-hosted chat`,apps:G(`self-hosted-chat`),local:!0},{id:`desktop-apps`,name:`Desktop AI`,apps:G(`desktop-apps`),local:!0},{id:`local-agents`,name:`Local agents`,apps:G(`local-agents`),local:!0},{id:`local-runners`,name:`Local runners`,apps:G(`local-runners`),local:!0},{id:`any-app`,name:`Any other app`,apps:[W]}];function _e(e){return K.find(t=>t.apps.some(t=>t.id===e))??K.find(t=>t.id===e)??K[0]}var ve=u(`<p class="hint">Only the owner can turn this on or off.</p>`),ye=u(`<p class="small muted" style="margin: var(--s-2) 0 var(--s-3)">AI assistants like Claude, ChatGPT or Cursor can check your library, calendar and downloads, and ask for titles for you. They can only do what you
			allow, and every request they make shows in History.</p> <label class="switch"><input type="checkbox"/> <span class="sw" aria-hidden="true"></span> <span>Let AI apps connect</span></label> <!> <ul class="facts small muted svelte-i6k0zr"><li>An AI app acts as you: your permissions and request limits apply.</li> <li>Before it asks for a title, the AI app has to check with you. Approving requests stays with people, on the Requests page.</li> <li>At most 60 calls a minute and 5 requests an hour per app.</li></ul>`,1),be=u(`On this address apps use an AI key. Press <b>Make a key</b> in the steps and it is filled in for you.`,1),xe=u(`<p class="notice warn">Turn on AI connections above first, or the app gets "not found".</p>`),Se=u(`<div class="apps sub svelte-i6k0zr" role="group"></div>`),Ce=u(`<p class="notice" data-testid="ai-unreachable"> <!></p> <a class="xs guide-link svelte-i6k0zr" target="_blank" rel="noopener noreferrer">How to reach HolyArrs from outside your home</a>`,1),we=u(`<p class="small muted needs svelte-i6k0zr"> </p>`),Te=u(`<div class="keyrow svelte-i6k0zr"><!> <span class="xs muted">It is filled into the steps below and shown only now.</span></div>`),Ee=u(`<code> </code>`),De=u(`<div class="codebox svelte-i6k0zr"><pre class="svelte-i6k0zr"> </pre> <!></div>`),Oe=u(`<li class="svelte-i6k0zr"><span class="num svelte-i6k0zr" aria-hidden="true"></span> <div class="grow"><p class="st svelte-i6k0zr"></p> <!></div></li>`),ke=u(`<p class="notice warn"> </p>`),Ae=u(`<form class="client-id svelte-i6k0zr" data-testid="ai-client-id"><label class="small" for="ai-redirect"> </label> <div class="keyrow svelte-i6k0zr"><input id="ai-redirect" class="input grow" placeholder="https://…" autocomplete="off"/> <!></div> <!> <!></form>`),je=u(`<button type="button" class="linkish small svelte-i6k0zr"> </button> <!>`,1),Me=u(`<p class="xs muted"><b>Check:</b> </p>`),Ne=u(`<p class="xs muted"> </p>`),Pe=u(`<!> <!> <ol class="steps svelte-i6k0zr"></ol> <!> <!> <!> <!> <!>`,1),Fe=u(`· <a class="guide-link svelte-i6k0zr" target="_blank" rel="noopener noreferrer">Pick a local model</a>`,1),Ie=u(`<div class="addr-row svelte-i6k0zr"><code class="addr grow svelte-i6k0zr" data-testid="ai-address"> </code> <!></div> <p class="small muted mode svelte-i6k0zr" data-testid="ai-mode"><!></p> <!> <div class="apps svelte-i6k0zr" role="group" aria-label="Kind of AI app"></div> <!> <div class="guide svelte-i6k0zr" data-testid="ai-guide"><!> <p class="xs links svelte-i6k0zr"><a class="guide-link svelte-i6k0zr" target="_blank" rel="noopener noreferrer" data-testid="ai-full-guide"> </a> <!> · <a class="guide-link svelte-i6k0zr" target="_blank" rel="noopener noreferrer">All AI guides</a></p></div> <div class="try svelte-i6k0zr"><div class="xs muted">Try this first</div> <p class="small svelte-i6k0zr"> </p></div>`,1),Le=u(`<div class="fresh svelte-i6k0zr" role="status"><p class="small"><b> </b>: copy this key now. It is shown only once.</p> <div class="row svelte-i6k0zr"><code class="key grow svelte-i6k0zr"> </code> <!> <!></div></div>`),Re=u(`<p class="small muted">No AI apps yet.</p>`),ze=u(`<li><div class="grow"><div class="name"> <span class="mono xs muted"> </span></div> <div class="xs muted"> </div></div> <!></li>`),Be=u(`<ul class="rows"></ul>`),Ve=u(`<!> <!> <details class="more svelte-i6k0zr" style="margin-top: var(--s-4)"><summary class="svelte-i6k0zr">Make a key by hand</summary> <form class="row svelte-i6k0zr"><label class="sr-only" for="ai-key-label">Key name</label> <input id="ai-key-label" class="input grow" style="max-width: 320px" placeholder="Which AI app? (Claude Desktop)" maxlength="64"/> <!></form></details>`,1),He=u(`<p class="small muted" style="margin: var(--s-2) 0 var(--s-4)">Each app that signs in gets its own key here. A key works only for AI connections; revoke it to disconnect the app.</p> <!>`,1),Ue=u(`<!> <!> <!>`,1),We=u(`<!> <!>`,1);function q(s,u){O(u,!0);let L=D(!1),R=D(``),B=D(!1),V=D(!1),H=D(E([])),U=D(``),W=D(``),G=D(!1),q=D(null),Ge=D(E(P.url.searchParams.get(`app`)??K[0].apps[0].id)),J=D(!1),Ke=T(()=>h.s.role===`owner`),Y=T(()=>typeof window>`u`?`/api/v1/mcp`:pe(window.location.origin,g())),qe=T(()=>m(H).filter(fe)),Je=T(()=>me(m(Y))),Ye=T(()=>({url:m(Y),key:m(q)?.app===m(Ge)?m(q).key:void 0,signIn:m(Je)})),X=T(()=>_e(m(Ge))),Z=T(()=>m(X).apps.find(e=>e.id===m(Ge))??m(X).apps[0]),Xe=T(()=>!!m(Z)?.cloud&&he(m(Y))),Ze=T(()=>!m(Z).cloud&&(!m(Je)||m(Z).signIn===!1)),Qe=T(()=>m(Y).replace(/^https?:\/\//,``).split(`/`)[0]);function $e(e){C(Ge,e,!0),C(J,!1);let t=new URL(P.url.href);t.searchParams.set(`app`,e),l(t,P.state)}function et(e){return e instanceof o&&e.status===403?`Sign in with your password to manage AI keys.`:_(e)}async function tt(){C(R,``);try{C(B,(await A.get(`/settings/ai`)).enabled,!0)}catch(e){C(R,_(e),!0)}try{C(H,(await A.get(`/auth/keys`)).keys,!0),C(U,``)}catch(e){C(U,et(e),!0)}C(L,!0)}d(tt);async function nt(e){if(m(V))return;let t=m(B);C(B,e,!0),C(V,!0);try{C(B,(await A.put(`/settings/ai`,{enabled:e})).enabled,!0),x.push(m(B)?`AI connections are on.`:`AI connections are off.`,`ok`)}catch(e){C(B,t,!0),x.push(_(e),`fail`)}finally{C(V,!1)}}async function rt(e){e.preventDefault(),m(W).trim()&&await it(m(W).trim()),C(W,``)}async function it(e,t){C(G,!0);try{let n=await A.post(`/auth/keys`,{label:e,scopes:[`mcp`]});C(q,{key:n.key,label:n.label,app:t},!0),C(H,(await A.get(`/auth/keys`)).keys,!0)}catch(e){x.push(et(e),`fail`)}finally{C(G,!1)}}async function at(e){if(confirm(`Revoke the AI key "${e.label}"? The AI app using it stops working.`))try{await A.del(`/auth/keys/${e.id}`),C(H,m(H).filter(t=>t.id!==e.id),!0),x.push(`AI key revoked.`,`ok`)}catch(e){x.push(et(e),`fail`)}}let ot=D(``),Q=D(``),st=D(``);async function ct(e){e.preventDefault(),C(st,``),C(Q,``);try{let e=await fetch(`${g()}/oauth/register`,{method:`POST`,headers:{"Content-Type":`application/json`},body:JSON.stringify({client_name:m(Z).name,redirect_uris:[m(ot).trim()]})}),t=await e.json();if(!e.ok)throw Error(t.error_description??`HolyArrs did not accept that address.`);C(Q,t.client_id,!0)}catch(e){C(st,_(e),!0)}}async function $(e){try{await navigator.clipboard.writeText(e),x.push(`Copied.`,`ok`)}catch{x.push(`Copy did not work here; select the text and copy it by hand.`,`fail`)}}var lt=We();oe(`i6k0zr`,e=>{a(()=>{ee.title=`AI connections · HolyArrs`})});var ut=e(lt);I(ut,{title:`AI connections`});var dt=M(ut,2),ft=e=>{ne(e,{rows:3})},pt=e=>{ue(e,{hasData:!1,get message(){return m(R)},onretry:tt,busy:!1})},mt=a=>{var o=Ue(),s=e(o);F(s,{id:`ai-switch`,title:`AI connections`,children:(t,n)=>{var i=ye(),a=M(e(i),2),o=f(a);ie(o),N(4),v(a);var s=M(a,2),c=e=>{var t=ve();w(e,t)};r(s,e=>{m(Ke)||e(c)}),N(2),k(()=>{te(o,m(B)),o.disabled=!m(Ke)||m(V),y(o,`aria-busy`,m(V))}),p(`change`,o,e=>nt(e.currentTarget.checked)),w(t,i)},$$slots:{default:!0}});var l=M(s,2);F(l,{id:`ai-connect`,title:`Connect an AI app`,children:(a,o)=>{var s=Ie(),l=e(s),u=f(l),d=b(u,!0),h=M(u,2);j(h,{icon:`copy`,size:`sm`,type:`button`,onclick:()=>$(m(Y)),children:(e,t)=>{N();var n=i(`Copy`);w(e,n)},$$slots:{default:!0}}),v(l);var g=M(l,2),ee=f(g),_=e=>{var t=i(`Apps sign in through your browser with your HolyArrs password. Nothing to paste.`);w(e,t)},x=e=>{var t=be();N(2),w(e,t)};r(ee,e=>{m(Je)?e(_):e(x,-1)}),v(g);var S=M(g,2),te=e=>{var t=xe();w(e,t)};r(S,e=>{m(B)||e(te)});var E=M(S,2);n(E,21,()=>K,e=>e.id,(e,t)=>{{let n=T(()=>m(X).id===m(t).id);de(e,{get label(){return m(t).name},get selected(){return m(n)},onclick:()=>$e(m(t).apps[0].id)})}}),v(E);var D=M(E,2),ne=e=>{var t=Se();n(t,21,()=>m(X).apps,e=>e.id,(e,t)=>{{let n=T(()=>m(Z).id===m(t).id);de(e,{get label(){return m(t).name},get selected(){return m(n)},onclick:()=>$e(m(t).id)})}}),v(t),k(()=>y(t,`aria-label`,`${m(X).name??``} app`)),w(e,t)};r(D,e=>{m(X).apps.length>1&&e(ne)});var O=M(D,2),A=f(O),oe=n=>{var a=Ce(),o=e(a),s=f(o),c=M(s),l=e=>{var t=i(`On this network, pick "Claude Desktop (home)" instead.`);w(e,t)};r(c,e=>{m(X).id===`claude`&&e(l)}),v(o);var u=M(o,2);k(()=>{t(s,`${m(Z).name??``} connects from the internet, so it cannot reach ${m(Qe)??``}. It works once HolyArrs has a public https address (your reverse proxy,
					Cloudflare Tunnel or Tailscale Funnel); open this page on that address for the steps. `),y(u,`href`,z+`reach-holyarrs/`)}),w(n,a)},P=a=>{var o=Pe(),s=e(o),l=e=>{var n=we(),r=b(n,!0);k(()=>t(r,m(Z).needs)),w(e,n)};r(s,e=>{m(Z).needs&&e(l)});var u=M(s,2),d=e=>{var n=Te(),r=f(n);{let e=T(()=>m(G)||!!m(U));j(r,{icon:`key`,variant:`primary`,type:`button`,onclick:()=>it(m(Z).name,m(Z).id),get disabled(){return m(e)},get"aria-busy"(){return m(G)},children:(e,n)=>{N();var r=i();k(()=>t(r,`Make a key for ${m(Z).name??``}`)),w(e,r)},$$slots:{default:!0}})}N(2),v(n),w(e,n)};r(u,e=>{m(Ze)&&m(q)?.app!==m(Z).id&&e(d)});var h=M(u,2);n(h,21,()=>m(Z).steps(m(Ye)),ae,(a,o,s)=>{var l=Oe(),u=f(l);u.textContent=s+1;var d=M(u,2),p=f(d);n(p,21,()=>m(o).text.split("`"),ae,(n,a,o)=>{var s=c(),l=e(s),u=e=>{var n=Ee(),r=b(n,!0);k(()=>t(r,m(a))),w(e,n)},d=e=>{var n=i();k(()=>t(n,m(a))),w(e,n)};r(l,e=>{o%2?e(u):e(d,-1)}),w(n,s)}),v(p);var h=M(p,2),g=e=>{var n=De(),r=f(n),a=b(r,!0),s=M(r,2);j(s,{icon:`copy`,size:`sm`,type:`button`,onclick:()=>$(m(o).code??``),children:(e,t)=>{N();var n=i(`Copy`);w(e,n)},$$slots:{default:!0}}),v(n),k(()=>t(a,m(o).code)),w(e,n)};r(h,e=>{m(o).code&&e(g)}),v(d),v(l),w(a,l)}),v(h);var g=M(h,2),ee=e=>{var n=Ae(),a=f(n),o=b(a),s=M(a,2),c=f(s);ie(c);var l=M(c,2);{let e=T(()=>!m(ot).trim());j(l,{type:`submit`,get disabled(){return m(e)},children:(e,t)=>{N();var n=i(`Make client ID`);w(e,n)},$$slots:{default:!0}})}v(s);var u=M(s,2),d=e=>{var n=ke(),r=b(n,!0);k(()=>t(r,m(st))),w(e,n)};r(u,e=>{m(st)&&e(d)});var p=M(u,2),h=e=>{var n=De(),r=f(n),a=b(r,!0),o=M(r,2);j(o,{icon:`copy`,size:`sm`,type:`button`,onclick:()=>$(m(Q)),children:(e,t)=>{N();var n=i(`Copy`);w(e,n)},$$slots:{default:!0}}),v(n),k(()=>t(a,m(Q))),w(e,n)};r(p,e=>{m(Q)&&e(h)}),v(n),k(()=>t(o,`${m(Z).name??``}'s redirect URI`)),se(`submit`,n,ct),re(c,()=>m(ot),e=>C(ot,e)),w(e,n)};r(g,e=>{m(Z).clientIdHelper&&e(ee)});var _=M(g,2),x=n=>{var a=je(),o=e(a),s=b(o),c=M(o,2),l=e=>{var n=De(),r=f(n),a=b(r,!0),o=M(r,2);j(o,{icon:`copy`,size:`sm`,type:`button`,onclick:()=>$(ge(m(Z).name,m(Y))),children:(e,t)=>{N();var n=i(`Copy`);w(e,n)},$$slots:{default:!0}}),v(n),k(e=>t(a,e),[()=>ge(m(Z).name,m(Y))]),w(e,n)};r(c,e=>{m(J)&&e(l)}),k(()=>{y(o,`aria-expanded`,m(J)),t(s,`Or let ${m(Z).name??``} set itself up`)}),p(`click`,o,()=>C(J,!m(J))),w(n,a)};r(_,e=>{m(Z).agent&&m(Je)&&e(x)});var S=M(_,2),te=e=>{var n=Me(),r=M(f(n));v(n),k(()=>t(r,` ${m(Z).check??``}`)),w(e,n)};r(S,e=>{m(Z).check&&e(te)});var E=M(S,2),D=e=>{var n=Ne(),r=b(n,!0);k(()=>t(r,m(Z).note)),w(e,n)};r(E,e=>{m(Z).note&&e(D)});var ne=M(E,2),O=e=>{var n=Ne(),r=b(n);k(()=>t(r,`Not tested with HolyArrs yet: these steps follow ${m(Z).name??``}'s docs.`)),w(e,n)};r(ne,e=>{m(Z).untried&&e(O)}),w(a,o)};r(A,e=>{m(Xe)?e(oe):e(P,-1)});var F=M(A,2),I=f(F),ce=b(I),le=M(I,2),ue=t=>{var n=Fe(),r=M(e(n));k(()=>y(r,`href`,z+`local-models/`)),w(t,n)};r(le,e=>{m(X).local&&e(ue)});var L=M(le,2);v(F),v(O);var fe=M(O,2),pe=M(f(fe),2),R=b(pe);v(fe),k(()=>{t(d,m(Y)),y(I,`href`,z+m(Z).guide),t(ce,`Guide: ${m(Z).name??``}`),y(L,`href`,z),t(R,`"Use HolyArrs: what's downloading right now, and what's coming out this week?"`)}),w(a,s)},$$slots:{default:!0}});var u=M(l,2);F(u,{id:`ai-keys`,title:`Connected apps and AI keys`,children:(a,o)=>{var s=He(),c=M(e(s),2),l=e=>{var n=ke(),r=b(n,!0);k(()=>t(r,m(U))),w(e,n)},u=a=>{var o=Ve(),s=e(o),c=e=>{var n=Le(),r=f(n),a=f(r),o=b(a,!0);N(),v(r);var s=M(r,2),c=f(s),l=b(c,!0),u=M(c,2);j(u,{icon:`copy`,type:`button`,onclick:()=>m(q)&&$(m(q).key),children:(e,t)=>{N();var n=i(`Copy`);w(e,n)},$$slots:{default:!0}});var d=M(u,2);j(d,{variant:`plain`,type:`button`,onclick:()=>C(q,null),children:(e,t)=>{N();var n=i(`Done`);w(e,n)},$$slots:{default:!0}}),v(s),v(n),k(()=>{t(o,m(q).label),t(l,m(q).key)}),w(e,n)};r(s,e=>{m(q)&&e(c)});var l=M(s,2),u=e=>{var t=Re();w(e,t)},d=e=>{var r=Be();n(r,21,()=>m(qe),e=>e.id,(e,n)=>{var r=ze(),a=f(r),o=f(a),s=f(o),c=M(s),l=b(c,!0);v(o);var u=M(o,2),d=b(u);v(a);var p=M(a,2);j(p,{variant:`destructive`,type:`button`,onclick:()=>at(m(n)),children:(e,t)=>{N();var n=i(`Revoke`);w(e,n)},$$slots:{default:!0}}),v(r),k((e,r)=>{t(s,`${m(n).label??``} `),t(l,m(n).hint),t(d,`Made ${e??``} · ${r??``}`)},[()=>ce(m(n).created_at),()=>m(n).last_used_at?`last used ${le(m(n).last_used_at)}`:`never used`]),w(e,r)}),v(r),w(e,r)};r(l,e=>{m(qe).length===0?e(u):e(d,-1)});var p=M(l,2),h=M(f(p),2),g=M(f(h),2);ie(g);var ee=M(g,2);{let e=T(()=>m(G)||!m(W).trim());j(ee,{icon:`key`,type:`submit`,get disabled(){return m(e)},get"aria-busy"(){return m(G)},children:(e,t)=>{N();var n=i(`Make an AI key`);w(e,n)},$$slots:{default:!0}})}v(h),v(p),se(`submit`,h,rt),re(g,()=>m(W),e=>C(W,e)),w(a,o)};r(c,e=>{m(U)?e(l):e(u,-1)}),w(a,s)},$$slots:{default:!0}}),w(a,o)};r(dt,e=>{m(L)?m(R)?e(pt,1):e(mt,-1):e(ft)}),w(s,lt),S()}s([`change`,`click`]);export{q as component};