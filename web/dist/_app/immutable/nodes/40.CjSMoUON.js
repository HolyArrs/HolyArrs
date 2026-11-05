import{Ai as e,Bi as t,Ca as n,Ci as r,Da as i,Di as a,Ea as o,Fi as s,Gi as c,Gn as l,Gr as u,Hi as d,Ii as f,Ir as p,Ki as m,Li as h,Mi as g,Oi as _,Pi as v,Ui as ee,Ur as y,Vi as te,Xi as b,Xn as x,Zi as S,Zn as C,_a as ne,a as re,aa as w,ca as T,di as ie,ea as ae,er as E,fi as D,ga as oe,ia as O,ir as k,na as A,oa as se,p as ce,pa as j,pi as le,r as M,ra as N,ri as P,si as ue,ua as F,wa as I,yi as de}from"../chunks/qvJYZCxF.js";import{n as fe}from"../chunks/xI-i6TPB.js";import{n as pe,o as me,r as he,t as ge}from"../chunks/Bsoaas_m.js";import{t as _e}from"../chunks/Bvs60lja.js";var ve=[{slug:`claude-code`,name:`Claude Code`,cloud:!1,needs:`Any Claude Code install. The VS Code and JetBrains extensions share it.`,signin:[{text:`Run this in a terminal.`,code:`claude mcp add --transport http --scope user holyarrs {url}`},{text:`Start claude, run /mcp, pick holyarrs and choose Authenticate. If no browser opens, run claude mcp login holyarrs in a terminal.`,bold:[`Authenticate`]},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Claude Code and press Make a key.`},{text:`Run this in a terminal.`,code:`claude mcp add --transport http --scope user holyarrs {url} --header "Authorization: Bearer {key}"`}],check:`claude mcp list shows holyarrs as Connected.`,note:`Claude Code only sends a sign-in to https or localhost addresses. On plain http it stops with "Refusing to send credentials to non-https token endpoint". If you see that, use an AI key. The key sits in plain text in ~/.claude.json.`,page:`claude-code`,table:{group:`agents`,order:1,where:"`claude mcp add`"},tested:`2026-11-03`,agent:!0},{slug:`claude-app`,name:`Claude app`,cloud:!0,needs:`HolyArrs at a public https address. The Free plan allows one custom connector.`,signin:[{text:`In Claude on the web or desktop, open Customize › Connectors, press + and pick Add custom connector.`,bold:[`Customize › Connectors`,`+`,`Add custom connector`],link:{text:`Open Connectors in Claude`,href:`https://claude.ai/customize/connectors`}},{text:`Name it HolyArrs, paste the address and press Continue. Check what Claude found and press Continue again. Keep Sign in now and Use Claude's published identity, then press Add.`,code:`{url}`,bold:[`Continue`,`Sign in now`,`Use Claude's published identity`,`Add`]},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[],check:`HolyArrs shows as connected under Customize › Connectors.`,note:`Add it on the web or desktop, and the phone app and Cowork can use it too.`,page:`claude-app`,table:{group:`chat`,order:1,where:`Customize › Connectors › Add custom connector`},tested:`2026-11-03`},{slug:`claude-desktop-bridge`,name:`Claude Desktop (home)`,cloud:!1,needs:`Claude Desktop and Node.js, so npx runs.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick Claude Desktop at home and press Make a key.`},{text:`From the Claude menu (Windows: the app menu), open Settings… › Developer and press Edit Config. Add the holyarrs entry under mcpServers, beside any others.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "command": "npx",
      "args": [
        "-y",
        "mcp-remote",
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
}`,bold:[`Settings… › Developer`,`Edit Config`]},{text:`Fully quit Claude Desktop from the tray or menu bar, then open it again.`}],check:`In a chat, press + › Connectors and look for HolyArrs.`,note:"Keep Authorization:${AUTH_HEADER} without a space, because Windows splits arguments at spaces. Logs: %APPDATA%\\Claude\\logs\\mcp*.log (macOS: ~/Library/Logs/Claude/). mcp-remote is not pinned to a tested version yet.",page:`claude-desktop-home`,table:{group:`computer`,order:1,label:`Claude Desktop at home`,where:`Settings › Developer › Edit Config (mcp-remote bridge)`},tested:null,agent:!0},{slug:`chatgpt`,name:`ChatGPT`,cloud:!0,needs:`HolyArrs at a public https address, and your plan or workspace has to allow custom MCP servers.`,signin:[{text:`On chatgpt.com, open Plugins, press + and pick Add custom MCP server.`,bold:[`Plugins`,`+`,`Add custom MCP server`],link:{text:`Open Plugins in ChatGPT`,href:`https://chatgpt.com/plugins`}},{text:`Name it HolyArrs. Under Connection, choose Server URL and paste the address.`,code:`{url}`,bold:[`Server URL`]},{text:`Under Authentication, pick OAuth and leave client ID and secret empty, because HolyArrs registers ChatGPT on its own. Accept the warning and press Create as a plugin.`,bold:[`OAuth`,`Create as a plugin`]},{text:`Install the HolyArrs plugin. HolyArrs opens in your browser. Press Allow.`}],key:[],check:`Type @, pick HolyArrs and ask a question.`,note:`Any login in front of HolyArrs, such as Cloudflare Access, blocks ChatGPT. After a HolyArrs update, open the plugin and press Refresh. The Tunnel option (Secure MCP Tunnel) could reach a home address, but it isn't tested with HolyArrs yet.`,page:`chatgpt`,table:{group:`chat`,order:2,where:`Plugins › + › Add custom MCP server`},tested:null},{slug:`chatgpt-desktop`,name:`ChatGPT desktop app`,cloud:!1,needs:`The new ChatGPT desktop app, not ChatGPT Classic.`,signin:[{text:`Open Settings › MCP servers › Add server.`,bold:[`Settings › MCP servers › Add server`]},{text:`Name it holyarrs, choose Streamable HTTP, paste the address and save.`,code:`{url}`,bold:[`Streamable HTTP`]},{text:`Restart the app. In MCP servers, press Authenticate next to holyarrs.`,bold:[`Authenticate`]},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick ChatGPT desktop app and press Make a key.`},{text:`Add this block to ~/.codex/config.toml (Windows: %USERPROFILE%\\.codex\\config.toml).`,code:`[mcp_servers.holyarrs]
url = "{url}"
http_headers = { "Authorization" = "Bearer {key}" }`},{text:`Restart the ChatGPT app.`}],check:`Settings › MCP servers lists holyarrs.`,note:`It shares ~/.codex/config.toml with Codex, so a server added in either one shows in both.`,page:`codex`,tested:null,agent:!0},{slug:`codex-cli`,name:`Codex CLI`,cloud:!1,needs:`Current Codex, signed in with ChatGPT or an OpenAI API key.`,signin:[{text:`Run this in a terminal.`,code:`codex mcp add holyarrs --url {url}`},{text:`Sign in. On a computer without a browser, add --no-browser and paste the redirect URL back.`,code:`codex mcp login holyarrs`},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Codex and press Make a key.`},{text:`Save the key in an environment variable named HOLYARRS_AI_KEY.`},{text:`Add this block to ~/.codex/config.toml (Windows: %USERPROFILE%\\.codex\\config.toml).`,code:`[mcp_servers.holyarrs]
url = "{url}"
bearer_token_env_var = "HOLYARRS_AI_KEY"`},{text:`Restart Codex.`}],check:`Run codex mcp list and look for holyarrs.`,note:`If no browser opens, run codex mcp login holyarrs.`,page:`codex`,table:{group:`agents`,order:2,label:`Codex`,where:"`codex mcp add` (CLI, IDE extension and ChatGPT desktop)"},tested:`2026-11-03`,agent:!0},{slug:`codex-ide`,name:`Codex IDE extension`,cloud:!1,needs:`Current Codex IDE extension, signed in.`,signin:[{text:`Open the gear menu, then MCP servers › Add server.`,bold:[`MCP servers › Add server`]},{text:`Name it holyarrs, choose Streamable HTTP, paste the address and save.`,code:`{url}`,bold:[`Streamable HTTP`]},{text:`Press Restart extension. In MCP servers, press Authenticate next to holyarrs.`,bold:[`Restart extension`,`Authenticate`]},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Codex IDE extension and press Make a key.`},{text:`Add this block to ~/.codex/config.toml (Windows: %USERPROFILE%\\.codex\\config.toml).`,code:`[mcp_servers.holyarrs]
url = "{url}"
http_headers = { "Authorization" = "Bearer {key}" }`},{text:`Press Restart extension.`}],check:`Type /mcp in Codex and look for holyarrs.`,note:`Codex inside WSL reads its own ~/.codex/config.toml.`,page:`codex`,tested:null,agent:!0},{slug:`gemini-cli`,name:`Gemini CLI`,cloud:!1,needs:`Paid Gemini API key, Vertex AI, or Code Assist Standard/Enterprise license.`,signin:[{text:`Add HolyArrs for all folders.`,code:`gemini mcp add --scope user --transport http holyarrs {url}`},{text:`Start gemini and run /mcp auth holyarrs.`,code:`/mcp auth holyarrs`},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Gemini CLI and press Make a key.`},{text:`Add HolyArrs for all folders.`,code:`gemini mcp add --scope user --transport http --header "Authorization: Bearer {key}" holyarrs {url}`},{text:`Start gemini and run /mcp list.`,code:`/mcp list`}],check:`gemini mcp list shows holyarrs as connected.`,note:`Personal Google accounts can no longer sign in to Gemini CLI. Google points them to Antigravity instead.`,page:null,table:{group:`agents`,order:5,where:"`gemini mcp add --scope user --transport http holyarrs <address>`"},tested:`2026-11-03`,agent:!0},{slug:`antigravity`,name:`Antigravity`,cloud:!1,needs:`Antigravity editor, app or CLI.`,signin:[{text:`Open the global MCP config file.`,code:`~/.gemini/config/mcp_config.json`},{text:`Add this inside mcpServers and save.`,code:`{
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
}`},{text:`Restart Antigravity.`}],check:`Run /mcp in the Antigravity CLI; holyarrs shows connected.`,note:`Google does not say plain http works; if refused, use https and sign-in.`,page:null,tested:null},{slug:`gemini-code-assist`,name:`Gemini Code Assist`,cloud:!1,needs:`Standard or Enterprise license; extension in VS Code.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick Gemini Code Assist and press Make a key.`},{text:`Add this inside mcpServers in ~/.gemini/settings.json.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "httpUrl": "{url}",
      "headers": {
        "Authorization": "Bearer {key}"
      }
    }
  }
}`},{text:`Run Developer: Reload Window from the Command Palette.`}],check:`Type /mcp in Code Assist chat; holyarrs lists its tools.`,note:`JetBrains IDEs need the npx mcp-remote bridge, which needs Node.js.`,page:null,tested:null},{slug:`gemini-app`,name:`Gemini app`,cloud:!0,needs:`US, 18+, personal Google account, Gemini in English, Gemini chat history on, on a computer browser.`,signin:[{text:`On gemini.google.com, open Settings › Connected Apps (or Personal Intelligence › Connected Apps). Under Custom apps, press Add a custom app (it may show as + Custom) and paste the address.`,code:`{url}`,bold:[`Settings › Connected Apps`,`Personal Intelligence › Connected Apps`,`Add a custom app`,`+ Custom`]},{text:`Under Show more next to Advanced features (or Additional settings), copy the redirect URI.`,bold:[`Show more`,`Advanced features`,`Additional settings`]},{text:`In HolyArrs, open Settings › AI connections › Gemini app, paste it and press Make client ID.`},{text:`Paste the client ID into Gemini, leave the client secret empty and press Next. HolyArrs opens in your browser. Press Allow.`,bold:[`Next`]}],key:[],check:`In a chat, type @, pick HolyArrs and ask a question.`,note:`Gemini does not register itself, so it needs this client ID.`,page:`gemini-app`,table:{group:`chat`,order:3,where:`Settings › Connected Apps › Add a custom app (needs a client ID)`},tested:`2026-11-03`,clientIdHelper:!0},{slug:`vs-code-copilot`,name:`VS Code with Copilot`,cloud:!1,needs:`GitHub Copilot signed in. Business or Enterprise admins can block MCP servers.`,signin:[{text:`In VS Code, run Chat: Open Customizations › MCP Servers › Add Server, or MCP: Add Server from the Command Palette.`,bold:[`Chat: Open Customizations › MCP Servers › Add Server`,`MCP: Add Server`]},{text:`Pick HTTP, paste this address and name it holyarrs.`,code:`{url}`},{text:`Choose Global, then Copilot Global.`},{text:`Run MCP: List Servers, pick holyarrs and choose Start. HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick VS Code with Copilot and press Make a key.`},{text:`Press Add to VS Code and confirm the install in VS Code.`},{text:`Or add this entry to ~/.copilot/mcp-config.json.`,code:`{
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
}`}],check:`Run MCP: List Servers and check that holyarrs is running.`,note:`~/.copilot/mcp-config.json is shared with the Copilot CLI. VS Code's own mcp.json uses servers; the Copilot file uses mcpServers. Don't copy between them.`,page:null,table:{group:`agents`,order:4,where:`Chat: Open Customizations › MCP Servers › Add Server`,deeplink:`vscode`},tested:null,agent:!0},{slug:`copilot-cli`,name:`GitHub Copilot CLI`,cloud:!1,needs:`A GitHub Copilot plan.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick GitHub Copilot CLI and press Make a key.`},{text:`Run this in a terminal.`,code:`copilot mcp add --transport http --header "Authorization: Bearer {key}" holyarrs {url}`},{text:`Or type /mcp add: name holyarrs, type HTTP, URL {url}, this header; press Ctrl+S.`,code:`{"Authorization": "Bearer {key}"}`}],check:`Run copilot mcp get holyarrs, or type /mcp show holyarrs.`,note:`Browser sign-in for remote servers is undocumented; use a key.`,page:null,tested:null},{slug:`copilot-cloud-agent`,name:`GitHub Copilot cloud agent`,cloud:!0,needs:`Copilot cloud agent enabled on the repository; HolyArrs on public https.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick GitHub Copilot cloud agent and press Make a key.`},{text:`In the repository, add an Agents secret COPILOT_MCP_HOLYARRS_KEY holding the key.`},{text:`Open the repository's Settings › Copilot › MCP servers.`},{text:`Paste this into MCP configuration and press Save MCP configuration.`,code:`{
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
}`}],check:`Give it a task asking what's downloading in HolyArrs.`,note:`It runs tools without asking, so ask_for_title is left out.`,page:null,tested:null},{slug:`m365-copilot`,name:`Microsoft 365 Copilot`,cloud:!0,needs:`Work account, Agents Toolkit 6.12+, admin allowing custom app upload.`,signin:[{text:`In VS Code's Microsoft 365 Agents Toolkit, select Create a New Agent/App › Declarative Agent.`},{text:`Select Add an Action › Start with an MCP Server and enter this address.`,code:`{url}`},{text:`Choose OAuth (with dynamic registration), name the agent, then select Lifecycle › Provision.`},{text:`In Microsoft 365 Copilot chat, pick the agent and press its sign-in button. HolyArrs opens in your browser. Press Allow.`}],key:[],check:`Ask the agent what's downloading; it answers from HolyArrs.`,note:`Unconfirmed whether it accepts sign-in without refresh tokens or client secrets.`,page:null,tested:null},{slug:`copilot-studio`,name:`Copilot Studio`,cloud:!0,needs:`Copilot Studio licence and environment; HolyArrs on public https.`,signin:[{text:`In your agent, open Tools › Add a tool › New tool › Model Context Protocol.`},{text:`Set Server name holyarrs, a Server description and this Server URL.`,code:`{url}`},{text:`Pick OAuth 2.0, type Dynamic discovery, then select Create and Next.`},{text:`Select Create a new connection, then Add to agent. HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Copilot Studio and press Make a key.`},{text:`In your agent, open Tools › Add a tool › New tool › Model Context Protocol.`},{text:`Set Server name holyarrs, a Server description and this Server URL.`,code:`{url}`},{text:`Pick API key, type Header, name X-Api-Key; connect with this key, then Add to agent.`,code:`{key}`}],check:`Ask the agent what's downloading; it answers from HolyArrs.`,note:`OAuth fit is unconfirmed (no refresh token); the key way avoids it.`,page:null,tested:null},{slug:`grok`,name:`Grok`,cloud:!0,needs:`HolyArrs at a public https address. On Business and Enterprise, an admin sets up connectors first.`,signin:[{text:`Open grok.com/connectors, press New Connector and pick Custom.`},{text:`Paste the address as the server URL.`,code:`{url}`},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[],check:`The connector shows as connected and lists its tools.`,note:`Grok rejects home and private addresses (10.x, 192.168.x, 127.0.0.1). Grok's help names no sign-in types, so browser sign-in is unconfirmed.`,page:null,table:{group:`chat`,order:4,where:`grok.com/connectors › New Connector › Custom`},tested:`2026-11-03`},{slug:`perplexity`,name:`Perplexity`,cloud:!0,needs:`HolyArrs at a public https address, and your plan has to allow custom connectors.`,signin:[{text:`Open Plugins (or Customize › Connectors), press Add custom, then Add MCP connector, then Remote.`,bold:[`Plugins`,`Customize › Connectors`,`Add custom`,`Add MCP connector`,`Remote`]},{text:`Set Name to HolyArrs and paste the address under MCP server URL.`,code:`{url}`},{text:`Under Advanced, set these, tick the risk box and press Add.`,code:`Authentication: OAuth
Transport: Streamable HTTP
Network access: Public`},{text:`Click the HolyArrs card. HolyArrs opens in your browser. Press Allow.`}],key:[],check:`Switch HolyArrs on under Sources. Its card lists the tools.`,note:`On a client_secret error, set Authentication to API Key and paste an AI key (unconfirmed). Network access has a Cloudflare Access option for HolyArrs behind Access.`,page:null,table:{group:`chat`,order:5,where:`Plugins › Add custom › Add MCP connector › Remote`},tested:`2026-11-03`},{slug:`perplexity-mac-helper`,name:`Perplexity Mac helper`,cloud:!1,needs:`Mac App Store version of Perplexity, and Node.js.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick Perplexity Mac helper and press Make a key.`},{text:`In the Mac app, open Plugins and install PerplexityXPC when asked.`},{text:`Press Add custom; on the Simple tab, set Server Name to holyarrs and paste this into Command.`,code:`npx -y mcp-remote {url} --allow-http --transport http-only --header "Authorization: Bearer {key}"`},{text:`Press Save, then switch HolyArrs on under Sources.`}],check:`The server status reads Running.`,note:`Keep the command on one line.`,page:null,tested:null},{slug:`le-chat`,name:`Mistral Le Chat`,cloud:!0,needs:`Public https HolyArrs. On team plans, an admin adds connectors.`,signin:[{text:`Open Connectors, press + Add Connector, then the Custom MCP Connector tab.`},{text:`Fill in the name and server address.`,code:`Connector name: holyarrs
Server URL: {url}`},{text:`Press Connect. HolyArrs opens in your browser. Press Allow.`}],key:[],check:`Under My Connectors, the HolyArrs card lists its tools in Functions.`,note:`Mistral's docs now file connectors under Vibe Work, so labels may differ.`,page:null,tested:null},{slug:`manus`,name:`Manus`,cloud:!0,needs:`Public https HolyArrs.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick Manus and press Make a key.`},{text:`In Manus, open Connectors, press Create and choose an MCP server by URL.`},{text:`Enter the address, and the key as a bearer token under Authentication.`,code:`Server URL: {url}
Token: {key}`},{text:`Press Test connection.`}],check:`Test connection lists the HolyArrs tools.`,note:`Older help puts this under Settings › Integrations › Custom MCP Servers › Add Server.`,page:null,tested:null},{slug:`genspark`,name:`Genspark`,cloud:!0,needs:`Public https HolyArrs. Work accounts follow the admin's custom MCP policy.`,signin:[{text:`Open Skills, then Connectors.`},{text:`Add a custom MCP server and paste the address as MCP URL.`,code:`{url}`},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[],check:`The connector shows as connected and lists its tools.`,note:`Whether Genspark can sign in or send a key is unconfirmed.`,page:null,tested:null},{slug:`huggingchat`,name:`HuggingChat`,cloud:!0,needs:`Public https HolyArrs, and a model that supports tools.`,signin:[{text:`In the chat box, press +, pick MCP Servers, then Add Server.`},{text:`Type a name and paste the address.`,code:`{url}`},{text:`Run Health Check. HolyArrs opens in your browser. Press Allow.`}],key:[],check:`Health Check passes; switch HolyArrs on in MCP Servers.`,note:`No key field and sign-in is unconfirmed, so this may not work yet.`,page:null,tested:null},{slug:`qwen-desktop`,name:`Qwen Chat desktop app`,cloud:!1,needs:`Qwen Chat desktop app on Mac or Windows, and Node.js 20+.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick Qwen Chat desktop app and press Make a key.`},{text:`In a chat, open the MCP option, then MCP settings › Add MCP.`},{text:`Choose the JSON form, paste this and press Save and Enable.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "command": "npx",
      "args": ["-y", "mcp-remote@latest", "{url}", "--transport", "http-only", "--allow-http", "--header", "Authorization:\${HOLYARRS_AUTH}"],
      "env": { "HOLYARRS_AUTH": "Bearer {key}" }
    }
  }
}`}],check:`A green mark shows next to holyarrs.`,note:`Quick Add has no key field; use the JSON form.`,page:null,tested:null},{slug:`cursor`,name:`Cursor`,cloud:!1,needs:``,signin:[{text:`Press Add to Cursor. Or open ~/.cursor/mcp.json and add this entry beside any others.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "url": "{url}"
    }
  }
}`,bold:[`Add to Cursor`],deeplink:`cursor`},{text:`In Cursor, find holyarrs under Customize (older builds: Settings › Tools & MCP) and press Needs login (some builds say Needs authentication or Connect).`,bold:[`Customize`,`Settings › Tools & MCP`,`Needs login`,`Needs authentication`,`Connect`]},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Cursor and press Make a key.`},{text:`Open ~/.cursor/mcp.json and add this entry beside any others.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "url": "{url}",
      "headers": {
        "Authorization": "Bearer {key}"
      }
    }
  }
}`}],check:`holyarrs shows its tools under Customize.`,note:`If Connect does nothing, open the sign-in link from Output › MCP Logs.`,page:`cursor`,table:{group:`agents`,order:3,where:"Add to Cursor, or `~/.cursor/mcp.json`"},tested:null,agent:!0},{slug:`windsurf`,name:`Windsurf`,cloud:!1,needs:`Teams plans need a full seat; an admin allow-list can block it.`,signin:[{text:`Open ~/.config/devin/mcp_config.json (Windows: %APPDATA%\\devin\\mcp_config.json).`},{text:`Add this entry and save:`,code:`{
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
}`}],check:`Customize › MCPs lists holyarrs, switched on.`,note:`Older Windsurf builds read ~/.codeium/windsurf/mcp_config.json instead (second-hand).`,page:null,tested:null},{slug:`zed`,name:`Zed`,cloud:!1,needs:``,signin:[{text:`Run zed: open settings file from the command palette.`},{text:`Add context_servers inside your settings and save:`,code:`{
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
}`}],check:`holyarrs shows a green dot and "Server is active".`,note:``,page:null,tested:null},{slug:`jetbrains-ai`,name:`JetBrains AI Assistant`,cloud:!1,needs:`AI Assistant installed; a company admin can block self-added servers.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick JetBrains AI Assistant and press Make a key.`},{text:`Open Settings › Tools › AI Assistant › Model Context Protocol (MCP) and press Add.`},{text:`Pick HTTP (Streamable HTTP), paste this entry, then press OK and Apply:`,code:`{
  "mcpServers": {
    "holyarrs": {
      "url": "{url}",
      "headers": {
        "Authorization": "Bearer {key}"
      }
    }
  }
}`}],check:`The Status column shows holyarrs; its icon lists the tools.`,note:`JetBrains documents only url; whether it sends headers is unconfirmed.`,page:null,tested:null},{slug:`kiro`,name:`Kiro`,cloud:!1,needs:`An https HolyArrs address, or Kiro on the HolyArrs computer.`,signin:[{text:`In Kiro Settings, search MCP and switch on MCP support.`},{text:`From the Command Palette, run Kiro: Open user MCP config (JSON).`},{text:`Add this entry and save:`,code:`{
  "mcpServers": {
    "holyarrs": {
      "url": "{url}"
    }
  }
}`},{text:`Kiro starts the sign-in. HolyArrs opens in your browser. Press Allow.`}],key:[],check:`Kiro's MCP panel lists holyarrs with its tools.`,note:`Kiro refuses plain http network addresses, so there is no key way.`,page:null,tested:null},{slug:`cline`,name:`Cline`,cloud:!1,needs:``,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick Cline and press Make a key.`},{text:`In Cline, click the MCP Servers icon, then Configure › Configure MCP Servers.`},{text:`Add this to cline_mcp_settings.json and save.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "type": "streamableHttp",
      "url": "{url}",
      "headers": {
        "Authorization": "Bearer {key}"
      }
    }
  }
}`}],check:`holyarrs shows in Cline's MCP server list without an error.`,note:`Keep "type": "streamableHttp"; without it Cline falls back to SSE and fails.`,page:null,tested:null},{slug:`continue`,name:`Continue`,cloud:!1,needs:``,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick Continue and press Make a key.`},{text:`Open ~/.continue/config.yaml from the gear next to Local Config.`},{text:`Add this entry under mcpServers and save.`,code:`mcpServers:
  - name: HolyArrs
    type: streamable-http
    url: {url}
    requestOptions:
      headers:
        Authorization: Bearer {key}`},{text:`Switch the chat to agent mode.`}],check:`HolyArrs shows in Continue's MCP list without an error.`,note:`Some versions may not send the key header; if HolyArrs answers "sign in", use Cline.`,page:null,tested:null},{slug:`kilo-code`,name:`Kilo Code`,cloud:!1,needs:``,signin:[{text:`Add this under mcp in ~/.config/kilo/kilo.jsonc and save.`,code:`{
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
}`}],check:`holyarrs shows in Kilo Code's MCP server list without an error.`,note:`If Kilo Code tries sign-in on plain http, add "oauth": false to the entry.`,page:null,tested:null},{slug:`opencode`,name:`OpenCode`,cloud:!1,needs:``,signin:[{text:`Add this to ~/.config/opencode/opencode.json.`,code:`{
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
}`},{text:`Restart OpenCode.`}],check:`opencode mcp list shows holyarrs and its sign-in state.`,note:`A project can use its own opencode.json instead.`,page:null,tested:null},{slug:`warp`,name:`Warp`,cloud:!1,needs:``,signin:[{text:`In Warp, open Settings › Agents › MCP servers and press + Add.`},{text:`Pick URL and paste this.`,code:`{
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
}`}],check:`holyarrs is listed under Settings › Agents › MCP servers.`,note:`Covers local agents only; cloud agents set up separately. Sign-in is per computer.`,page:null,tested:null},{slug:`amp`,name:`Amp`,cloud:!1,needs:``,signin:[{text:`Add HolyArrs.`,code:`amp mcp add holyarrs {url}`},{text:`Start Amp. HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Amp and press Make a key.`},{text:`Add the amp.mcpServers block to ~/.config/amp/settings.json.`,code:`{
  "amp.mcpServers": {
    "holyarrs": {
      "url": "{url}",
      "headers": {
        "Authorization": "Bearer {key}"
      }
    }
  }
}`},{text:`Restart Amp.`}],check:`amp mcp doctor shows holyarrs and its state.`,note:`In a project's .amp/settings.json, run amp mcp approve holyarrs first.`,page:null,tested:null},{slug:`pi`,name:`Pi`,cloud:!1,needs:`Pi 0.99.0 or newer.`,signin:[{text:`Add this to ~/.pi/agent/mcp.json.`,code:`{
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
}`},{text:`Restart Pi.`}],check:`Type /mcp inside Pi; holyarrs is listed.`,note:``,page:null,tested:null},{slug:`mistral-vibe`,name:`Mistral Vibe CLI`,cloud:!1,needs:``,signin:[{text:`Add HolyArrs.`,code:`vibe mcp add holyarrs --url {url}`},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Mistral Vibe CLI and press Make a key.`},{text:`Put the key in HOLYARRS_AI_KEY.`,code:`export HOLYARRS_AI_KEY="{key}"`},{text:`Add this to ~/.vibe/config.toml.`,code:`[[mcp_servers]]
name = "holyarrs"
transport = "http"
url = "{url}"

[mcp_servers.auth]
type = "static"
api_key_env = "HOLYARRS_AI_KEY"
api_key_header = "Authorization"
api_key_format = "Bearer {token}"`},{text:`Start Vibe from that shell.`}],check:`/mcp holyarrs inside Vibe lists the HolyArrs tools.`,note:`If no browser opens, run /mcp login holyarrs inside Vibe.`,page:null,tested:null},{slug:`kimi-code`,name:`Kimi Code CLI`,cloud:!1,needs:``,signin:[{text:`Add this to ~/.kimi-code/mcp.json.`,code:`{
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
}`},{text:`Restart Kimi Code.`}],check:`/mcp inside Kimi Code shows holyarrs connected.`,note:``,page:null,tested:null},{slug:`devin`,name:`Devin`,cloud:!0,needs:`Public https HolyArrs without a login wall; Manage MCP Servers permission.`,signin:[{text:`In Devin, open Customize › MCPs, press Add MCP, then Add custom MCP.`},{text:`Fill in these fields and press Save.`,code:`Server Name: HolyArrs
Transport:   HTTP
Server URL:  {url}
Auth:        OAuth
Access:      Personal`},{text:`Press Connect. HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Devin and press Make a key.`},{text:`In Devin, open Customize › MCPs, press Add MCP, then Add custom MCP.`},{text:`Fill in these fields and press Save.`,code:`Server Name:  HolyArrs
Transport:    HTTP
Server URL:   {url}
Auth:         Auth Header
Header name:  Authorization
Header value: Bearer {key}`}],check:`Press Test tools; it lists the HolyArrs tools.`,note:`Access Organization shares one sign-in: everyone acts as whoever pressed Allow.`,page:null,tested:null},{slug:`open-webui`,name:`Open WebUI`,cloud:!1,needs:`Open WebUI v0.6.31 or newer and an Open WebUI admin account.`,signin:[{text:`In Open WebUI, open Admin Panel › Settings › Integrations and press + Add Connection.`,bold:[`Admin Panel › Settings › Integrations`,`+ Add Connection`]},{text:`Set Type to MCP (Streamable HTTP) and paste the address as Server URL. Set Auth to OAuth 2.1, press Register Client, then Save.`,code:`{url}`,bold:[`MCP (Streamable HTTP)`,`Server URL`,`OAuth 2.1`,`Register Client`,`Save`]},{text:`Set a fixed secret so the sign-in survives Open WebUI restarts.`,code:`WEBUI_SECRET_KEY=<long random text you keep>`},{text:`In a chat, open + › Integrations › Tools and turn HolyArrs on. HolyArrs opens in your browser. Press Allow.`,bold:[`+ › Integrations › Tools`]}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Open WebUI and press Make a key.`},{text:`In Open WebUI, open Admin Panel › Settings › Integrations and press + Add Connection.`,bold:[`Admin Panel › Settings › Integrations`,`+ Add Connection`]},{text:`Set Type to MCP (Streamable HTTP), Auth to Bearer, fill in these two fields and save.`,code:`Server URL: {url}
Key: {key}`,bold:[`MCP (Streamable HTTP)`,`Bearer`]},{text:`In a chat, open + › Integrations › Tools and turn HolyArrs on.`,bold:[`+ › Integrations › Tools`]}],check:`A new chat's tools menu lists the HolyArrs tools.`,note:`In Docker, localhost is the Open WebUI container. Use HolyArrs's network address or host.docker.internal.`,page:`local-ai`,table:{group:`computer`,order:2,label:`Open WebUI, LM Studio, Ollama`,where:`Each app's MCP settings, with an AI key`},tested:null},{slug:`librechat`,name:`LibreChat`,cloud:!1,needs:`A LibreChat role that may add MCP servers.`,signin:[{text:`Open MCP Settings › Add MCP, paste {url}, pick Transport Streamable HTTP.`,code:`{url}`},{text:`Under Authentication pick OAuth; leave Client ID and Client Secret empty.`},{text:`Tick I trust this application and press Create.`},{text:`Connect the server. HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick LibreChat and press Make a key.`},{text:`On a home address, allow HolyArrs in librechat.yaml and restart LibreChat.`,code:`mcpSettings:
  allowedAddresses:
    - "<HolyArrs IP>:<port>"`},{text:`Open MCP Settings › Add MCP, paste {url}, pick Transport Streamable HTTP.`,code:`{url}`},{text:`Set Authentication API Key, Header Format Bearer, enter {key}, tick I trust this application, press Create.`,code:`{key}`}],check:`The message box's + › MCP Servers lists HolyArrs.`,note:`Use allowedAddresses; allowedDomains blocks every unlisted server.`,page:null,tested:null},{slug:`lobehub`,name:`LobeHub`,cloud:!1,needs:`The LobeHub desktop app when HolyArrs is on a home address.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick LobeHub and press Make a key.`},{text:`In LobeHub, open Settings › Connectors and add an MCP server by hand.`},{text:`Choose HTTP, enter {url}, set auth API Key, enter {key}, save.`,code:`URL: {url}
API Key: {key}`},{text:`Open Agent Settings › Connectors, select HolyArrs and save.`}],check:`Settings › Connectors shows HolyArrs as connected.`,note:`No API Key field? Add the header Authorization: Bearer followed by your key instead.`,page:null,tested:null},{slug:`lm-studio`,name:`LM Studio`,cloud:!1,needs:`LM Studio 0.3.17 or newer, and 0.4.10 or newer for browser sign-in (0.4.12 on Windows).`,signin:[{text:`In LM Studio, open the Program tab, pick Install, then Edit mcp.json.`,bold:[`Program`,`Install`,`Edit mcp.json`]},{text:`Add this entry and save.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "url": "{url}"
    }
  }
}`},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick LM Studio and press Make a key.`},{text:`In LM Studio, open the Program tab, pick Install, then Edit mcp.json.`,bold:[`Program`,`Install`,`Edit mcp.json`]},{text:`Add this entry and save.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "url": "{url}",
      "headers": {
        "Authorization": "Bearer {key}"
      }
    }
  }
}`}],check:`holyarrs lists its tools in LM Studio.`,note:`If mcp.json lists other servers, add holyarrs beside them.`,page:`local-ai`,tested:null,agent:!0},{slug:`jan`,name:`Jan`,cloud:!1,needs:`Node.js for the key way.`,signin:[{text:`In Jan, open Settings › MCP Servers and press + Add MCP Server.`},{text:`Set Transport to HTTP and paste this address.`,code:`{url}`},{text:`Press Sign in on the Auth line. HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Jan and press Make a key.`},{text:`In Jan, open Settings › MCP Servers and press + Add MCP Server.`},{text:`Set Transport to STDIO, Command to npx, and these Args, one per line.`,code:`-y
mcp-remote@latest
{url}
--transport
http-only
--allow-http
--header
Authorization:\${AUTH_HEADER}`},{text:`In Env, set AUTH_HEADER to this value.`,code:`Bearer {key}`}],check:`holyarrs lists its tools in Jan.`,note:`Jan stops a tool call after 30 seconds by default.`,page:null,tested:null},{slug:`anythingllm`,name:`AnythingLLM`,cloud:!1,needs:`AnythingLLM Desktop v1.8.0 or newer.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick AnythingLLM and press Make a key.`},{text:`In AnythingLLM, open Agent Skills once to create the servers file.`},{text:`Add this entry to plugins/anythingllm_mcp_servers.json in its storage folder.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "type": "streamable",
      "url": "{url}",
      "headers": {
        "Authorization": "Bearer {key}"
      }
    }
  }
}`},{text:`Open Agent Skills again to load holyarrs.`}],check:`holyarrs shows as running under Agent Skills.`,note:`Start chat messages with @agent so the model can use tools.`,page:null,tested:null},{slug:`cherry-studio`,name:`Cherry Studio`,cloud:!1,needs:``,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick Cherry Studio and press Make a key.`},{text:`In Cherry Studio, open Settings › MCP › MCP Servers, press Add, choose Import from JSON.`},{text:`Paste this entry and save.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "type": "streamableHttp",
      "url": "{url}",
      "headers": {
        "Authorization": "Bearer {key}"
      }
    }
  }
}`},{text:`Switch holyarrs on in your assistant's MCP settings.`}],check:`holyarrs shows as running in MCP Servers.`,note:`Without "type": "streamableHttp", Cherry Studio treats the entry as a program.`,page:null,tested:null},{slug:`msty-studio`,name:`Msty Studio`,cloud:!1,needs:``,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick Msty Studio and press Make a key.`},{text:`In Msty Studio, open Toolbox › Tools, press Add New Tool and pick HTTP.`},{text:`Name it HolyArrs and paste this into MCP Server URL.`,code:`{url}`},{text:`In Headers, add Authorization with this value, then press Add.`,code:`Bearer {key}`}],check:`Run a HolyArrs tool in the Tool Console.`,note:`Msty's docs do not show the Headers field format.`,page:null,tested:null},{slug:`5ire`,name:`5ire`,cloud:!1,needs:`5ire v0.11.0 or newer, and Node.js.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick 5ire and press Make a key.`},{text:`In 5ire, open Tools and press New.`},{text:`Fill in command, arguments and environment from this entry.`,code:`{
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
}`},{text:`Switch the tool on.`}],check:``,note:`Drop --allow-http when HolyArrs is on https.`,page:null,tested:null},{slug:`hermes-agent`,name:`Hermes Agent`,cloud:!1,needs:``,signin:[{text:`Add this to ~/.hermes/config.yaml.`,code:`mcp_servers:
  holyarrs:
    url: "{url}"
    auth: oauth`},{text:`Start the sign-in.`,code:`hermes mcp login holyarrs`},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Hermes Agent and press Make a key.`},{text:`Add this to ~/.hermes/config.yaml.`,code:`mcp_servers:
  holyarrs:
    url: "{url}"
    headers:
      Authorization: "Bearer {key}"`},{text:`Run /reload-mcp in a running session, or start a new one.`,code:`/reload-mcp`}],check:`hermes mcp test holyarrs lists the HolyArrs tools.`,note:`Tools show as mcp_holyarrs_<tool>; hide one under tools: exclude.`,page:null,tested:null},{slug:`goose`,name:`Goose`,cloud:!1,needs:``,signin:[{text:`Run goose configure, then Add Extension › Remote Extension (Streamable HTTP).`,code:`goose configure`},{text:`Name it holyarrs, enter this URI, keep the default timeout, skip custom headers.`,code:`{url}`},{text:`Run goose session. HolyArrs opens in your browser. Press Allow.`,code:`goose session`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Goose and press Make a key.`},{text:`Run goose configure, then Add Extension › Remote Extension (Streamable HTTP).`,code:`goose configure`},{text:`Name it holyarrs, enter this URI, keep the default timeout.`,code:`{url}`},{text:`Add a custom header named Authorization with this value.`,code:`Bearer {key}`}],check:`Goose's extension list shows holyarrs.`,note:`Desktop app: Extensions › Add custom extension, same URI and header.`,page:null,tested:null},{slug:`ollmcp`,name:`ollmcp`,cloud:!1,needs:`Python 3.11 or newer, and Ollama running with a model pulled.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick ollmcp and press Make a key.`},{text:`Install ollmcp.`,code:`uv tool install --upgrade ollmcp`},{text:`Save this as holyarrs.json.`,code:`{
  "mcpServers": {
    "holyarrs": {
      "type": "streamable_http",
      "url": "{url}",
      "headers": {
        "Authorization": "Bearer {key}"
      }
    }
  }
}`},{text:`Start a chat with your Ollama model.`,code:`ollmcp -j holyarrs.json -m qwen3:8b`}],check:`Type /tools in the chat to see the HolyArrs tools.`,note:`-H on the ollmcp command sets the Ollama host, not a header.`,page:`local-ai`,tested:`2026-11-03`,agent:!0},{slug:`llama-cpp`,name:`llama.cpp web UI`,cloud:!1,needs:`Node.js for npx. llama.cpp MCP support is experimental.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick llama.cpp web UI and press Make a key.`},{text:`Save this as mcp.json in the folder you start llama-server from.`,code:`{
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
}`},{text:`Start llama-server with the file.`,code:`llama-server -m <your model>.gguf --mcp-servers-config mcp.json`},{text:`Open the llama.cpp web UI.`}],check:`The HolyArrs tools show in the web UI.`,note:`On an https address, drop --allow-http.`,page:null,tested:null},{slug:`localai`,name:`LocalAI`,cloud:!1,needs:``,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick LocalAI and press Make a key.`},{text:`Add this mcp block to your model's YAML config.`,code:`mcp:
  remote: |
    {
      "mcpServers": {
        "holyarrs": {
          "url": "{url}",
          "token": "{key}"
        }
      }
    }`},{text:`Reload the model.`}],check:`In chat, MCP > Servers lists holyarrs with no errors.`,note:`In the model editor, paste only the inner mcpServers JSON into Remote MCP Servers.`,page:null,tested:null},{slug:`text-generation-webui`,name:`text-generation-webui`,cloud:!1,needs:``,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick text-generation-webui and press Make a key.`},{text:`In the Model tab, load a model with tool-call support.`},{text:`In the chat sidebar, open MCP servers and add this line.`,code:`{url},Authorization: Bearer {key}`}],check:`Ask "Use HolyArrs: what is downloading?"; a tool runs.`,note:`Tool calls run without asking you first.`,page:null,tested:null},{slug:`koboldcpp`,name:`KoboldCpp`,cloud:!1,needs:`KoboldCpp 1.106 or newer, and Node.js for npx.`,signin:[],key:[{text:`In HolyArrs, open Settings › AI connections, pick KoboldCpp and press Make a key.`},{text:`Save this as mcp.json.`,code:`{
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
}`},{text:`Start KoboldCpp with the file.`,code:`koboldcpp --model <your model>.gguf --mcpfile mcp.json`},{text:`In KoboldAI Lite settings, turn on tool calling and fetch the tools.`}],check:`No MCP errors at startup; HolyArrs tools listed in Lite.`,note:`Turn on Manually Approve Tools, or every tool call runs unasked.`,page:null,tested:null},{slug:`page-assist`,name:`Page Assist`,cloud:!1,needs:``,signin:[{text:`Set up OAuth as Page Assist's MCP guide describes.`},{text:`In Page Assist's MCP settings, add {url} with OAuth 2.1.`},{text:`HolyArrs opens in your browser. Press Allow.`}],key:[{text:`In HolyArrs, open Settings › AI connections, pick Page Assist and press Make a key.`},{text:`In Page Assist's MCP settings, add {url} with Bearer token {key}.`},{text:`Turn on Require approval before running MCP tools.`}],check:`The key shows "last used" in Settings › AI connections.`,note:`It calls from your browser, so HolyArrs may refuse it as another site.`,page:null,tested:null}];function ye(e){return(e.scopes??[]).includes(`mcp`)}function be(e,t=``){return e.replace(/\/+$/,``)+t.replace(/\/+$/,``)+`/api/v1/mcp`}var L=`<your AI key>`;function xe(e){try{let t=new URL(e);return t.protocol===`https:`||[`localhost`,`127.0.0.1`,`[::1]`].includes(t.hostname)}catch{return!1}}function Se(e){try{let t=new URL(e),n=t.hostname;return t.protocol!==`https:`||n===`localhost`||n.startsWith(`[`)||/^(10|127)\./.test(n)||/^192\.168\./.test(n)||/^172\.(1[6-9]|2\d|3[01])\./.test(n)||/^100\.(6[4-9]|[7-9]\d|1[01]\d|12[0-7])\./.test(n)||n.endsWith(`.local`)||n.endsWith(`.lan`)||n.endsWith(`.home.arpa`)}catch{return!0}}var Ce=`https://holyarrs.coderluii.dev/docs/ai/`,we=`What's downloading in HolyArrs?`;function Te(e){return e?`${Ce}${e}/`:`${Ce}#apps`}function Ee(e,t={}){let n=`Read ${Ce}ai.md and connect yourself to my HolyArrs at ${e}.`,r=`Only add the HolyArrs MCP server. Ask me before you change anything else.`;return t.key?`${n} Use this AI key: ${t.key}. ${r}`:t.home?`${n} ${r} I'll give you an AI key when you ask.`:`${n} ${r}`}function De(e){return e.split("`").map((e,t)=>t%2?{text:e,code:!0}:{text:e}).filter(e=>e.text)}var R=[`Settings › AI connections`,`Make client ID`,`Make a key`,`Allow`],z=/(~\/[\w./-]+|%\w+%\\[\w.\\*]+|(?<![\w-])--[a-z][a-z-]+|\b[A-Z][A-Z0-9]*_[A-Z0-9_]+\b|\bclaude mcp login holyarrs\b|(?<![\w/])\/mcp\b|\b[\w-]+\.(?:json|toml)\b|\bholyarrs\.json\b)/g,B=e=>e.replace(/[.*+?^${}()|[\]\\]/g,`\\$&`);function V(e,t=[]){let n=[...new Set([...t,...R])].sort((e,t)=>t.length-e.length),r=[],i=e;for(let e of n)i=i.replace(RegExp(`(?<![\\w\\u0001])${B(e)}(?![\\w\\u0001])`,`g`),()=>`\u0001${r.push(e)-1}\u0001`);return i=i.replace(z,e=>e.endsWith(`.`)?"`"+e.slice(0,-1)+"`.":"`"+e+"`"),De(i).flatMap(e=>e.code?[e]:e.text.split(/\u0001(\d+)\u0001/).map((e,t)=>t%2?{text:r[Number(e)],b:!0}:{text:e}).filter(e=>e.text))}function Oe(e,t){if(e===`cursor`){let e=new TextEncoder().encode(JSON.stringify({url:t}));return`cursor://anysphere.cursor-deeplink/mcp/install?name=holyarrs&config=${btoa(String.fromCharCode(...e))}`}return`vscode:mcp/install?${encodeURIComponent(JSON.stringify({name:`holyarrs`,type:`http`,url:t}))}`}var ke={cursor:`Add to Cursor`,vscode:`Add to VS Code`},H=(e,t)=>e.replaceAll(`{url}`,t.url).replaceAll(`{key}`,t.key??L);function U(e,t,n){return{text:H(e.text,t),parts:V(e.text,e.bold).map(e=>({...e,text:H(e.text,t)})),...e.code?{code:H(e.code,t)}:{},...e.link?{link:e.link}:{},...e.deeplink&&n?{deeplink:e.deeplink}:{}}}function W(e){return{id:e.slug,name:e.name,cloud:e.cloud,signIn:e.signin.length>0,needs:e.needs||void 0,note:e.note||void 0,check:e.check||void 0,tested:e.tested,clientIdHelper:e.clientIdHelper,steps:t=>{let n=t.signIn&&e.signin.length>0;return(n?e.signin:e.key.length?e.key:e.signin).filter(e=>!e.text.startsWith(`In HolyArrs`)).map(e=>U(e,t,n))}}}var G=e=>`Bearer ${e.key??L}`,Ae=e=>JSON.stringify(e,null,2),K=e=>({text:e,parts:V(e)}),je={id:`any-app`,name:`Any other app`,signIn:!0,steps:e=>[{...K(`Add a remote MCP server named holyarrs, type Streamable HTTP, with this address.`),code:e.url},e.signIn?K(`When the app asks, sign in. HolyArrs opens in your browser. Press Allow.`):{...K(`Have the app send this header.`),code:`Authorization: ${G(e)}`},{...K(`Apps with a settings file mostly take this form.`),code:Ae({mcpServers:{holyarrs:e.signIn?{url:e.url}:{url:e.url,headers:{Authorization:G(e)}}}})},{...K(`App runs only local programs? Use this bridge (needs Node.js).`),code:`npx -y mcp-remote@latest ${e.url} --transport http-only${e.url.startsWith(`http:`)?` --allow-http`:``} --header "Authorization: ${G(e)}"`}]},Me=[[`chat`,`Chat apps`],[`agents`,`Agents and editors`],[`computer`,`On your computer`]],Ne=/^Add to (Cursor|VS Code), or /,Pe=[...Me.flatMap(([e])=>ve.filter(t=>t.table?.group===e).sort((e,t)=>e.table.order-t.table.order).map(t=>{let n=t.table.where.match(Ne),r=t.table.deeplink??(n?n[1]===`Cursor`?`cursor`:`vscode`:void 0);return{id:t.slug,name:t.table.label??t.name,group:e,where:n?t.table.where.slice(n[0].length):t.table.where,guide:Te(t.page),page:t.page,...r?{deeplink:r}:{},app:W(t)}})),{id:`any-app`,name:`Any other app`,group:`computer`,where:`Its remote MCP server settings, with Streamable HTTP`,guide:`${Ce}#any-other-app`,page:null,app:je}];function Fe(e){if(!e)return;let t=Pe.find(t=>t.id===e)??(e===`any-other-app`?Pe.at(-1):void 0);if(t)return{id:t.id,row:t,app:t.app};let n=ve.find(t=>t.slug===e),r=Pe.find(t=>t.page&&t.page===(n?.page??e));return n?{id:n.slug,row:r,app:W(n)}:r?{id:r.id,row:r,app:r.app}:void 0}function Ie(e){return e.page?[e.app,...ve.filter(t=>t.page===e.page&&t.slug!==e.id).map(W)]:[e.app]}function Le(e,t,n=!0){if(!n&&e.where.includes(`<address>`)){let n=e.where.replaceAll("`",``).split(` `).slice(0,2).join(` `),r=e.app.steps({url:t,signIn:!1}).find(e=>e.code?.startsWith(n+` `))?.code;if(r)return"`"+r+"`"}return e.where.replaceAll(`<address>`,t)}var Re=[`Jan`,`Feb`,`Mar`,`Apr`,`May`,`Jun`,`Jul`,`Aug`,`Sep`,`Oct`,`Nov`,`Dec`];function ze(e){if(e.id===`any-app`)return``;if(e.tested){let[t,n,r]=e.tested.split(`-`).map(Number);return`Tested with HolyArrs on ${r} ${Re[n-1]} ${t}`}return`Not tested with HolyArrs yet. These steps follow ${e.name}'s own help pages.`}var Be=(n,r=o)=>{var i=f(),l=N(i);a(l,17,r,_,(n,r)=>{var i=f(),a=N(i),o=e=>{var t=Ve(),n=O(t,!0);S(()=>v(n,c(r).text)),s(e,t)},l=e=>{var t=q(),n=O(t,!0);S(()=>v(n,c(r).text)),s(e,t)},u=e=>{var n=t();S(()=>v(n,c(r).text)),s(e,n)};e(a,e=>{c(r).code?e(o):c(r).b?e(l,1):e(u,-1)}),s(n,i)}),s(n,i)},Ve=h(`<code class="svelte-i6k0zr"> </code>`),q=h(`<b> </b>`),He=h(`<div class="fresh svelte-i6k0zr" role="status"><p class="small svelte-i6k0zr"><b>Copy this key now</b></p> <p class="small svelte-i6k0zr"> </p> <div class="fresh-row svelte-i6k0zr"><code class="key grow svelte-i6k0zr"> </code> <!> <!></div></div>`),Ue=h(`<div class="which svelte-i6k0zr" role="group" aria-label="Which app"></div>`),We=h(`<p class="notice svelte-i6k0zr" data-testid="ai-unreachable">Needs a public https address.</p> <a class="small guide-link svelte-i6k0zr" target="_blank" rel="noopener noreferrer">Only on your home network?</a>`,1),Ge=h(`<p class="small muted svelte-i6k0zr"> </p>`),Ke=h(`<div class="keyrow svelte-i6k0zr"><!> <span class="xs muted"> </span></div>`),qe=h(`<p class="st svelte-i6k0zr"><a class="guide-link svelte-i6k0zr" target="_blank" rel="noopener noreferrer"> </a></p>`),Je=h(`<p class="st svelte-i6k0zr"><!></p>`),Ye=h(`<div class="codebox svelte-i6k0zr"><pre class="svelte-i6k0zr"> </pre> <!></div>`),Xe=h(`<li class="svelte-i6k0zr"><span class="num svelte-i6k0zr" aria-hidden="true"></span> <div class="grow"><p class="st svelte-i6k0zr"><!></p> <!> <!> <!></div></li>`),Ze=h(`<p class="notice warn svelte-i6k0zr" role="alert"> </p>`),Qe=h(`<div class="xs muted">Client ID</div> <div class="codebox svelte-i6k0zr"><pre class="svelte-i6k0zr"> </pre> <!></div> <p class="small svelte-i6k0zr">Paste it into Gemini, leave the client secret empty and press <b>Next</b>.</p>`,1),$e=h(`<form class="client-id svelte-i6k0zr" data-testid="ai-client-id" novalidate=""><p class="small svelte-i6k0zr">Gemini doesn't register itself, so HolyArrs makes a client ID for it.</p> <label class="label" for="ai-redirect">Gemini's redirect URI</label> <div class="keyrow svelte-i6k0zr"><input id="ai-redirect" name="redirect_uri" type="url" inputmode="url" spellcheck="false" class="input grow" placeholder="https://…" autocomplete="off" aria-describedby="ai-redirect-hint"/> <!></div> <p class="hint svelte-i6k0zr" id="ai-redirect-hint">In Gemini, open <b>Show more</b> next to <b>Advanced features</b> and copy the redirect URI.</p> <!> <!></form>`),et=h(`<p class="xs muted svelte-i6k0zr" data-testid="ai-tested"> </p>`),tt=h(`<p class="xs muted svelte-i6k0zr"><b>Check:</b> </p>`),nt=h(`<p class="xs muted svelte-i6k0zr"> </p>`),rt=h(`<!> <!> <ol class="steps svelte-i6k0zr"></ol> <!> <!> <!> <!>`,1),it=h(`<div class="panel svelte-i6k0zr" id="ai-app-panel" data-testid="ai-guide"><h4 class="h3 svelte-i6k0zr" tabindex="-1"> </h4> <!> <!></div>`),at=h(`<p class="hint">Only the owner can turn this on or off.</p>`),ot=h(`<p class="small muted" style="margin: var(--s-2) 0 var(--s-3)">AI apps like Claude, ChatGPT or Cursor can check your library, calendar and downloads, and ask for titles for you. They can only do what you allow,
			and every request they make shows in History.</p> <label class="switch"><input type="checkbox"/> <span class="sw" aria-hidden="true"></span> <span>Let AI apps connect</span></label> <!> <ul class="facts small muted svelte-i6k0zr"><li>An AI app acts as you, so your permissions and request limits apply.</li> <li>Before it asks for a title, the AI app checks with you. Approving requests stays with people, on the Requests page.</li> <li>At most 60 calls a minute and 5 requests an hour per app.</li></ul>`,1),st=h(`<p class="notice warn svelte-i6k0zr"><span>Turn on <b>Let AI apps connect</b> above first. Until then, AI apps get "not found".</span></p>`),ct=h(`This is a plain-http address, so most apps won't sign in through the browser. Use an AI key: press <b>Make a key</b> next to your app below.`,1),lt=h(`HolyArrs opens in your browser. Sign in if it asks, then press <b>Allow</b>.`,1),ut=h(`Paste the AI key from <b>Make a key</b> where your app asks for a header or key.`,1),dt=h(`, or <!>`,1),ft=h(`<!><!>`,1),pt=h(`<tr class="panel-row svelte-i6k0zr"><td colspan="3" class="svelte-i6k0zr"><!></td></tr>`),mt=h(`<tr><th scope="row" class="app svelte-i6k0zr"><button type="button" class="pick svelte-i6k0zr"> </button></th><td class="where svelte-i6k0zr"><!></td><td class="guide svelte-i6k0zr"><a class="guide-link svelte-i6k0zr" target="_blank" rel="noopener noreferrer">Guide</a></td></tr> <!>`,1),ht=h(`<tbody class="svelte-i6k0zr"><tr class="group svelte-i6k0zr"><th scope="rowgroup" colspan="3" class="svelte-i6k0zr"> </th></tr><!></tbody>`),gt=h(`<p class="xs muted svelte-i6k0zr">This prompt has your key in it. Paste it only into your own agent.</p>`),_t=h(`<p class="xs muted svelte-i6k0zr">Press <b>Make a key</b> first to put your key in the prompt.</p>`),vt=h(`<div class="small addr-label svelte-i6k0zr">AI address</div> <div class="addr-row svelte-i6k0zr"><code class="addr grow svelte-i6k0zr" data-testid="ai-address"> </code> <!></div> <p class="xs muted">This address shows an error in a browser. That's normal, because only AI apps use it.</p> <!> <p class="small muted mode svelte-i6k0zr" data-testid="ai-mode"><!></p> <section class="part svelte-i6k0zr" aria-labelledby="ai-steps-h"><h3 class="h3 svelte-i6k0zr" id="ai-steps-h">Connect in 3 steps</h3> <ol class="steps svelte-i6k0zr" data-testid="ai-steps"><li class="svelte-i6k0zr"><span class="num svelte-i6k0zr" aria-hidden="true">1</span><p class="st svelte-i6k0zr">In your AI app, add a custom connector or MCP server.</p></li> <li class="svelte-i6k0zr"><span class="num svelte-i6k0zr" aria-hidden="true">2</span><p class="st svelte-i6k0zr">Paste your AI address.</p></li> <li class="svelte-i6k0zr"><span class="num svelte-i6k0zr" aria-hidden="true">3</span> <p class="st svelte-i6k0zr"><!></p></li></ol> <p class="small muted svelte-i6k0zr">Claude, ChatGPT, Gemini, Grok and Perplexity connect from the cloud, so they need HolyArrs at a public https address.</p> <a class="small guide-link svelte-i6k0zr" target="_blank" rel="noopener noreferrer">Only on your home network?</a></section> <section class="part svelte-i6k0zr" aria-labelledby="ai-apps-h"><h3 class="h3 svelte-i6k0zr" id="ai-apps-h">Where to add HolyArrs</h3> <table class="apps svelte-i6k0zr" data-testid="ai-table" aria-labelledby="ai-apps-h"><thead class="svelte-i6k0zr"><tr><th scope="col" class="svelte-i6k0zr">App</th><th scope="col" class="svelte-i6k0zr">Where to add it</th><th scope="col" class="guide-h svelte-i6k0zr"><span class="sr-only">Guide</span></th></tr></thead><!></table> <!> <a class="small guide-link svelte-i6k0zr" target="_blank" rel="noopener noreferrer">All AI guides</a></section> <section class="part svelte-i6k0zr" aria-labelledby="ai-agent-h"><h3 class="h3 svelte-i6k0zr" id="ai-agent-h">Or let your agent do it</h3> <p class="small svelte-i6k0zr">Using Claude Code, Codex, Cursor or another agent? Paste this into it.</p> <div class="codebox svelte-i6k0zr"><pre data-testid="ai-prompt" class="svelte-i6k0zr"> </pre> <!></div> <!></section> <section class="part svelte-i6k0zr" aria-labelledby="ai-check-h" data-testid="ai-check"><h3 class="h3 svelte-i6k0zr" id="ai-check-h">Check it works</h3> <p class="small svelte-i6k0zr">Ask your AI:</p> <div class="codebox svelte-i6k0zr"><pre class="svelte-i6k0zr"> </pre> <!></div> <p class="xs muted svelte-i6k0zr">If it lists your downloads, or says nothing is downloading, you're connected.</p></section>`,1),yt=h(`<p class="notice warn svelte-i6k0zr"> </p>`),bt=h(`<p class="small muted">No AI apps yet. Connect one above and it shows here.</p>`),xt=h(`<li><div class="grow"><div class="name"> <span class="mono xs muted"> </span></div> <div class="xs muted"> </div></div> <!></li>`),St=h(`<ul class="rows"></ul>`),Ct=h(`<!> <!> <details class="more svelte-i6k0zr" style="margin-top: var(--s-4)"><summary class="svelte-i6k0zr">Make a key by hand</summary> <form class="row"><label class="sr-only" for="ai-key-label">Key name</label> <input id="ai-key-label" class="input grow" style="max-width: 320px" placeholder="Which AI app? (for example Claude Desktop)" maxlength="64"/> <!></form></details>`,1),wt=h(`<p class="small muted" style="margin: var(--s-2) 0 var(--s-4)">Each app that signs in or uses a key shows here. Revoke it to disconnect the app.</p> <!>`,1),Tt=h(`<div class="confirm-actions svelte-i6k0zr"><!> <!></div>`),Et=h(`<!> <!> <!> <!>`,1),Dt=h(`<!> <!>`,1);function J(o,h){ne(h,!0);let te=r=>{var i=f(),a=N(i),o=e=>{var r=He(),i=w(A(r),2),a=O(i),o=w(i,2),l=A(o),u=O(l,!0),d=w(l,2);M(d,{icon:`copy`,type:`button`,onclick:()=>c(W)&&Zt(c(W).key),children:(e,r)=>{n();var i=t(`Copy`);s(e,i)},$$slots:{default:!0}});var f=w(d,2);M(f,{variant:`plain`,type:`button`,onclick:Kt,children:(e,r)=>{n();var i=t(`Done`);s(e,i)},$$slots:{default:!0}}),I(o),I(r),P(r,e=>T(Re,e),()=>c(Re)),S(()=>{v(a,`${c(W).label??``}: it's shown only once. If you lose it, make a new one.`),v(u,c(W).key)}),s(e,r)};e(a,e=>{c(W)&&e(o)}),s(r,i)},ve=r=>{var i=f(),o=N(i),l=r=>{var i=it(),o=A(i),l=O(o,!0);P(o,e=>T(Ne,e),()=>c(Ne));var u=w(o,2),d=e=>{var t=Ue();a(t,21,()=>Ie(c(G).row),e=>e.id,(e,t)=>{{let n=j(()=>c(t).id===c(Z).id);_e(e,{get label(){return c(t).name},get selected(){return c(n)},onclick:()=>zt(c(t))})}}),I(t),s(e,t)},f=j(()=>c(G).row&&Ie(c(G).row).length>1);e(u,e=>{c(f)&&e(d)});var p=w(u,2),m=e=>{var t=We(),n=w(N(t),2);S(()=>D(n,`href`,`https://holyarrs.coderluii.dev/docs/ai/#home-network`)),s(e,t)},h=r=>{var i=rt(),o=N(i),l=e=>{var t=Ge(),n=O(t,!0);S(()=>v(n,c(Z).needs)),s(e,t)};e(o,e=>{c(Z).needs&&e(l)});var u=w(o,2),d=e=>{var r=Ke(),i=A(r);{let e=j(()=>c(U)||!!c(V));M(i,{icon:`key`,variant:`primary`,type:`button`,onclick:()=>Gt(c(Z).name,c(Z).id),get disabled(){return c(e)},get"aria-busy"(){return c(U)},get"aria-label"(){return`Make a key for ${c(Z).name??``}`},children:(e,r)=>{n();var i=t();S(()=>v(i,c(U)?`Making a key…`:`Make a key`)),s(e,i)},$$slots:{default:!0}})}var a=w(i,2),o=O(a,!0);I(r),S(()=>v(o,c(V)||`It's filled into the steps and the agent prompt, and shown only now.`)),s(e,r)},f=e=>{te(e)};e(u,e=>{c(It)&&c(W)?.app!==c(Z).id?e(d):c(W)&&c(W).app===c(Z).id&&e(f,1)});var p=w(u,2);a(p,21,()=>c(Z).steps(c(Pt)),_,(r,i,a)=>{var o=Xe(),l=A(o);l.textContent=a+1;var u=w(l,2),d=A(u);Be(A(d),()=>c(i).parts),I(d);var f=w(d,2),p=e=>{var t=qe(),n=A(t),r=O(n,!0);I(t),S(()=>{D(n,`href`,c(i).link.href),v(r,c(i).link.text)}),s(e,t)};e(f,e=>{c(i).link&&e(p)});var m=w(f,2),h=e=>{var r=Je(),a=A(r);{let e=j(()=>Oe(c(i).deeplink,c(Y)));M(a,{size:`sm`,get href(){return c(e)},children:(e,r)=>{n();var a=t();S(()=>v(a,ke[c(i).deeplink])),s(e,a)},$$slots:{default:!0}})}I(r),s(e,r)};e(m,e=>{c(i).deeplink&&e(h)});var g=w(m,2),_=e=>{var r=Ye(),a=A(r),o=O(a,!0),l=w(a,2);{let e=j(()=>Qt(c(i).code));M(l,{icon:`copy`,size:`sm`,type:`button`,get"aria-label"(){return c(e)},onclick:()=>Zt(c(i).code??``),children:(e,r)=>{n();var i=t(`Copy`);s(e,i)},$$slots:{default:!0}})}I(r),S(()=>v(o,c(i).code)),s(e,r)};e(g,e=>{c(i).code&&e(_)}),I(u),I(o),s(r,o)}),I(p);var m=w(p,2),h=r=>{var i=$e(),a=w(A(i),4),o=A(a);ie(o);var l=w(o,2);M(l,{type:`submit`,children:(e,r)=>{n();var i=t(`Make client ID`);s(e,i)},$$slots:{default:!0}}),I(a);var u=w(a,4),d=e=>{var t=Ze(),n=O(t,!0);S(()=>v(n,c($))),s(e,t)};e(u,e=>{c($)&&e(d)});var f=w(u,2),p=e=>{var r=Qe(),i=w(N(r),2),a=A(i),o=O(a,!0),l=w(a,2);M(l,{icon:`copy`,size:`sm`,type:`button`,"aria-label":`Copy client ID`,onclick:()=>Zt(c(Q)),children:(e,r)=>{n();var i=t(`Copy`);s(e,i)},$$slots:{default:!0}}),I(i),n(2),S(()=>v(o,c(Q))),s(e,r)};e(f,e=>{c(Q)&&e(p)}),I(i),ee(`submit`,i,Xt),ue(o,()=>c(Yt),e=>T(Yt,e)),s(r,i)};e(m,e=>{c(Z).clientIdHelper&&e(h)});var g=w(m,2),y=e=>{var t=et(),n=O(t,!0);S(e=>v(n,e),[()=>ze(c(Z))]),s(e,t)},b=j(()=>ze(c(Z)));e(g,e=>{c(b)&&e(y)});var x=w(g,2),C=e=>{var t=tt(),n=w(A(t));I(t),S(()=>v(n,` ${c(Z).check??``}`)),s(e,t)};e(x,e=>{c(Z).check&&e(C)});var ne=w(x,2),re=e=>{var t=nt(),n=O(t,!0);S(()=>v(n,c(Z).note)),s(e,t)};e(ne,e=>{c(Z).note&&e(re)}),s(r,i)};e(p,e=>{c(Ft)?e(m):e(h,-1)}),I(i),P(i,e=>T(je,e),()=>c(je)),S(()=>v(l,c(Z).name)),s(r,i)};e(o,e=>{c(G)&&c(Z)&&e(l)}),s(r,i)},L=F(!1),Te=F(``),R=F(!1),z=F(!1),B=F(se([])),V=F(``),H=F(``),U=F(!1),W=F(null),G=F(se(Fe(y.url.searchParams.get(`app`)))),Ae=`(max-width: 767px)`,K=F(se(typeof window<`u`&&window.matchMedia(Ae).matches)),je=F(void 0),Ne=F(void 0),Re=F(void 0),Ve=F(void 0),q=F(null),J=F(!1),Ot=F(!1),kt=j(()=>l.s.role===`owner`),At=j(()=>typeof window>`u`?``:(window.location.origin+u()).replace(/\/+$/,``)),Y=j(()=>typeof window>`u`?`/api/v1/mcp`:be(window.location.origin,u())),jt=j(()=>c(B).filter(ye)),X=j(()=>xe(c(Y))),Mt=j(()=>Se(c(Y))),Z=j(()=>c(G)?.app),Nt=j(()=>c(Z)&&c(W)?.app===c(Z).id?c(W).key:void 0),Pt=j(()=>({url:c(Y),key:c(Nt),signIn:c(X)})),Ft=j(()=>!!c(Z)?.cloud&&c(Mt)),It=j(()=>!!c(Z)&&!c(Z).cloud&&(!c(X)||!c(Z).signIn)),Lt=j(()=>c(X)?void 0:c(Nt)),Rt=j(()=>Ee(c(At),{key:c(Lt),home:!c(X)}));g(()=>{let e=window.matchMedia(Ae);T(K,e.matches,!0);let t=()=>T(K,e.matches,!0);return e.addEventListener(`change`,t),()=>e.removeEventListener(`change`,t)});function zt(e){if(!c(G)?.row||e.id===c(Z)?.id)return;T(G,{id:e.id,row:c(G).row,app:e},!0);let t=new URL(y.url.href);t.searchParams.set(`app`,e.id),p(t,y.state)}async function Bt(e){T(G,c(G)?.row?.id===e.id?void 0:{id:e.id,row:e,app:e.app},!0);let t=new URL(y.url.href);c(G)?t.searchParams.set(`app`,c(G).id):t.searchParams.delete(`app`),p(t,y.state),T(Yt,``),T(Q,``),T($,``),c(G)&&(await m(),c(Ne)?.focus({preventScroll:!0}),c(je)?.scrollIntoView({block:`nearest`}))}function Vt(e){return e instanceof x&&e.status===403?`Sign in with your password to manage AI keys.`:E(e)}async function Ht(){T(Te,``);try{T(R,(await C.get(`/settings/ai`)).enabled,!0)}catch(e){T(Te,E(e),!0)}try{T(B,(await C.get(`/auth/keys`)).keys,!0),T(V,``)}catch(e){T(V,Vt(e),!0)}let e=!c(L);T(L,!0),e&&c(G)&&(await m(),c(je)?.scrollIntoView({block:`nearest`}))}g(Ht);async function Ut(e){if(c(z))return;let t=c(R);T(R,e,!0),T(z,!0);try{T(R,(await C.put(`/settings/ai`,{enabled:e})).enabled,!0),k.push(c(R)?`AI connections are on.`:`AI connections are off.`,`ok`)}catch(e){T(R,t,!0),k.push(E(e),`fail`)}finally{T(z,!1)}}async function Wt(e){e.preventDefault(),c(H).trim()&&await Gt(c(H).trim()),T(H,``)}async function Gt(e,t){T(U,!0);try{let n=await C.post(`/auth/keys`,{label:e,scopes:[`mcp`]});T(W,{id:n.id,key:n.key,label:n.label,app:t},!0),T(B,(await C.get(`/auth/keys`)).keys,!0)}catch(e){k.push(Vt(e),`fail`)}finally{T(U,!1)}c(W)&&t&&(await m(),c(Re)?.querySelector(`button`)?.focus())}async function Kt(){let e=!!c(Z)&&c(W)?.app===c(Z).id;T(W,null),await m(),e?c(Ne)?.focus():c(Ve)?.focus()}function qt(e){T(q,e,!0),T(J,!0)}async function Jt(e){if(!c(Ot)){T(Ot,!0),T(J,!1);try{await C.del(`/auth/keys/${e.id}`),T(B,c(B).filter(t=>t.id!==e.id),!0),c(W)?.id===e.id&&T(W,null),k.push(`${e.label} revoked.`,`ok`)}catch(e){k.push(Vt(e),`fail`)}finally{T(Ot,!1)}}}let Yt=F(``),Q=F(``),$=F(``);async function Xt(e){if(e.preventDefault(),T($,``),T(Q,``),!c(Yt).trim())T($,`Paste Gemini's redirect URI first.`);else try{let e=await fetch(`${u()}/oauth/register`,{method:`POST`,headers:{"Content-Type":`application/json`},body:JSON.stringify({client_name:c(Z)?.name??`Gemini app`,redirect_uris:[c(Yt).trim()]})});if(e.status===400){T($,`HolyArrs didn't accept that address. Copy the redirect URI from Gemini again, starting with https://.`);return}if(!e.ok)throw Error(String(e.status));T(Q,(await e.json()).client_id,!0)}catch{T($,`HolyArrs didn't answer. Try again.`)}}async function Zt(e){try{await navigator.clipboard.writeText(e),k.push(`Copied.`,`ok`)}catch{k.push(`Couldn't copy here. Select the text and copy it yourself.`,`fail`)}}let Qt=e=>e.includes(`
`)?`Copy config`:e===c(Y)?`Copy AI address`:e.startsWith(`Authorization:`)?`Copy header`:`Copy command`;var $t=Dt();r(`i6k0zr`,e=>{b(()=>{ae.title=`AI connections · HolyArrs`})});var en=N($t);pe(en,{title:`AI connections`});var tn=w(en,2),nn=e=>{ce(e,{rows:3})},rn=e=>{ge(e,{hasData:!1,get message(){return c(Te)},onretry:Ht,busy:!1})},an=r=>{var o=Et(),l=N(o);fe(l,{id:`ai-switch`,title:`AI connections`,children:(t,r)=>{var i=ot(),a=w(N(i),2),o=A(a);ie(o),n(4),I(a);var l=w(a,2),u=e=>{var t=at();s(e,t)};e(l,e=>{c(kt)||e(u)}),n(2),S(()=>{le(o,c(R)),o.disabled=!c(kt)||c(z),D(o,`aria-busy`,c(z))}),d(`change`,o,e=>Ut(e.currentTarget.checked)),s(t,i)},$$slots:{default:!0}});var u=w(l,2);fe(u,{id:`ai-connect`,title:`Connect an AI app`,children:(r,o)=>{var l=vt(),u=w(N(l),2),f=A(u),p=O(f,!0),m=w(f,2);M(m,{icon:`copy`,size:`sm`,type:`button`,"aria-label":`Copy AI address`,onclick:()=>Zt(c(Y)),children:(e,r)=>{n();var i=t(`Copy`);s(e,i)},$$slots:{default:!0}}),I(u);var h=w(u,4),g=e=>{var t=st();s(e,t)};e(h,e=>{c(R)||e(g)});var _=w(h,2),ee=A(_),y=e=>{var n=t(`Apps sign in through your browser. You press Allow once.`);s(e,n)},te=e=>{var t=ct();n(2),s(e,t)};e(ee,e=>{c(X)?e(y):e(te,-1)}),I(_);var b=w(_,2),x=w(A(b),2),C=w(A(x),4),ne=w(A(C),2),re=A(ne),T=e=>{var t=lt();n(2),s(e,t)},ie=e=>{var t=ut();n(2),s(e,t)};e(re,e=>{c(X)?e(T):e(ie,-1)}),I(ne),I(C),I(x);var ae=w(x,4);I(b);var E=w(b,2),oe=w(A(E),2),k=w(A(oe));a(k,17,()=>Me,([e,t])=>e,(r,o)=>{var l=j(()=>i(c(o),2));let u=()=>c(l)[0],f=()=>c(l)[1];var p=ht(),m=A(p),h=A(m),g=O(h,!0);I(m);var _=w(m);a(_,17,()=>Pe.filter(e=>e.group===u()),e=>e.id,(r,i)=>{let a=j(()=>!!c(i).app.cloud&&c(Mt)),o=j(()=>c(G)?.row?.id===c(i).id);var l=mt(),u=N(l);let f;var p=A(u),m=A(p),h=O(m,!0);I(p);var g=w(p),_=A(g),ee=e=>{var n=t(`Needs a public https address.`);s(e,n)},y=r=>{var a=ft(),o=N(a);{let e=j(()=>De(Le(c(i),c(Y),c(X))));Be(o,()=>c(e))}var l=w(o),u=e=>{var r=dt(),a=w(N(r));{let e=j(()=>Oe(c(i).deeplink,c(Y)));M(a,{size:`sm`,get href(){return c(e)},children:(e,r)=>{n();var a=t();S(()=>v(a,ke[c(i).deeplink])),s(e,a)},$$slots:{default:!0}})}s(e,r)};e(l,e=>{c(i).deeplink&&c(X)&&e(u)}),s(r,a)};e(_,e=>{c(a)?e(ee):e(y,-1)}),I(g);var te=w(g),b=O(te);I(u);var x=w(u,2),C=e=>{var t=pt(),n=A(t),r=A(n);ve(r),I(n),I(t),s(e,t)};e(x,e=>{c(K)&&c(o)&&e(C)}),S(()=>{f=de(u,1,`app-row svelte-i6k0zr`,null,f,{on:c(o),off:c(a)}),D(m,`aria-expanded`,c(o)),D(m,`aria-controls`,c(o)?`ai-app-panel`:void 0),v(h,c(i).name),D(b,`href`,c(i).guide),D(b,`aria-label`,`Guide for ${c(i).name??``}`)}),d(`click`,m,()=>Bt(c(i))),s(r,l)}),I(p),S(()=>v(g,f())),s(r,p)}),I(oe);var se=w(oe,2),ce=e=>{ve(e)};e(se,e=>{(!c(K)||!c(G)?.row)&&e(ce)});var le=w(se,2);I(E);var P=w(E,2),ue=w(A(P),4),F=A(ue),fe=O(F,!0),pe=w(F,2);M(pe,{icon:`copy`,size:`sm`,type:`button`,onclick:()=>Zt(c(Rt)),children:(e,r)=>{n();var i=t(`Copy prompt`);s(e,i)},$$slots:{default:!0}}),I(ue);var me=w(ue,2),he=e=>{var t=gt();s(e,t)},ge=e=>{var t=_t();s(e,t)};e(me,e=>{c(Lt)?e(he):c(X)||e(ge,1)}),I(P);var _e=w(P,2),ye=w(A(_e),4),be=A(ye),L=O(be,!0),xe=w(be,2);M(xe,{icon:`copy`,size:`sm`,type:`button`,onclick:()=>Zt(we),children:(e,r)=>{n();var i=t(`Copy question`);s(e,i)},$$slots:{default:!0}}),I(ye),n(2),I(_e),S(()=>{v(p,c(Y)),D(ae,`href`,`https://holyarrs.coderluii.dev/docs/ai/#home-network`),D(le,`href`,Ce),v(fe,c(Rt)),v(L,we)}),s(r,l)},$$slots:{default:!0}});var f=w(u,2);fe(f,{id:`ai-keys`,title:`Connected apps`,children:(r,i)=>{var o=wt(),l=w(N(o),2),u=e=>{var t=yt(),n=O(t,!0);S(()=>v(n,c(V))),s(e,t)},d=r=>{var i=Ct(),o=N(i),l=e=>{te(e)};e(o,e=>{c(W)&&!(c(Z)&&c(W).app===c(Z).id)&&e(l)});var u=w(o,2),d=e=>{var t=bt();s(e,t)},f=e=>{var r=St();a(r,21,()=>c(jt),e=>e.id,(e,r)=>{var i=xt(),a=A(i),o=A(a),l=A(o),u=w(l),d=O(u,!0);I(o);var f=w(o,2),p=O(f);I(a);var m=w(a,2);M(m,{variant:`destructive`,type:`button`,onclick:()=>qt(c(r)),children:(e,r)=>{n();var i=t(`Revoke`);s(e,i)},$$slots:{default:!0}}),I(i),S((e,t)=>{v(l,`${c(r).label??``} `),v(d,c(r).hint),v(p,`Made ${e??``} · ${t??``}`)},[()=>me(c(r).created_at),()=>c(r).last_used_at?`last used ${he(c(r).last_used_at)}`:`never used`]),s(e,i)}),I(r),s(e,r)};e(u,e=>{c(jt).length===0?e(d):e(f,-1)});var p=w(u,2),m=A(p);P(m,e=>T(Ve,e),()=>c(Ve));var h=w(m,2),g=w(A(h),2);ie(g);var _=w(g,2);{let e=j(()=>c(U)||!c(H).trim());M(_,{icon:`key`,type:`submit`,get disabled(){return c(e)},get"aria-busy"(){return c(U)},children:(e,r)=>{n();var i=t(`Make an AI key`);s(e,i)},$$slots:{default:!0}})}I(h),I(p),ee(`submit`,h,Wt),ue(g,()=>c(H),e=>T(H,e)),s(r,i)};e(l,e=>{c(V)?e(u):e(d,-1)}),s(r,o)},$$slots:{default:!0}});var p=w(f,2);{let e=j(()=>c(q)?`Revoke ${c(q).label}?`:`Revoke`);re(p,{size:`sm`,get title(){return c(e)},description:`It stops working until you connect it again.`,get open(){return c(J)},set open(e){T(J,e,!0)},children:(e,r)=>{var i=Tt(),a=A(i);M(a,{variant:`plain`,type:`button`,onclick:()=>T(J,!1),children:(e,r)=>{n();var i=t(`Keep it`);s(e,i)},$$slots:{default:!0}});var o=w(a,2);M(o,{variant:`destructive`,type:`button`,get disabled(){return c(Ot)},onclick:()=>c(q)&&void Jt(c(q)),children:(e,r)=>{n();var i=t(`Revoke`);s(e,i)},$$slots:{default:!0}}),I(i),s(e,i)},$$slots:{default:!0}})}s(r,o)};e(tn,e=>{c(L)?c(Te)?e(rn,1):e(an,-1):e(nn)}),s(o,$t),oe()}te([`change`,`click`]);export{J as component};