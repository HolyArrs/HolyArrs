// Setup for every AI app, the same words as the website guides
// (holyarrs.coderluii.dev/docs/ai/<slug>/). Copied from the site repo's
// dev/ai-clients.json; keep the two in step. {url} and {key} are filled in.

export type AppStep = { text: string; code?: string };

export type AppData = {
  slug: string;
  name: string;
  family: string;
  /** Connects from the maker's servers: needs a public https address. */
  cloud: boolean;
  needs: string;
  /** Steps on https or this computer (browser sign-in). */
  signin: AppStep[];
  /** Steps on a plain http home address (AI key). */
  key: AppStep[];
  check: string;
  note: string;
  /** live: tried with HolyArrs; docs or second-hand: not yet. */
  source: string;
};

export const APP_DATA: AppData[] = [
  {
    slug: "claude-code",
    name: "Claude Code",
    family: "claude",
    cloud: false,
    needs:
      "Any Claude Code install; VS Code and JetBrains extensions share it.",
    signin: [
      {
        text: "Run this in a terminal.",
        code: "claude mcp add --transport http --scope user holyarrs {url}",
      },
      {
        text: "Start Claude Code, run /mcp, pick holyarrs, choose Authenticate.",
      },
      { text: "HolyArrs opens in your browser. Press Allow." },
    ],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Claude Code and press Make a key.",
      },
      {
        text: "Run this in a terminal.",
        code: 'claude mcp add --transport http --scope user holyarrs {url} --header "Authorization: Bearer {key}"',
      },
    ],
    check: "claude mcp list shows holyarrs as Connected.",
    note: "The key sits in plain text in ~/.claude.json.",
    source: "live",
  },
  {
    slug: "claude-app",
    name: "Claude app",
    family: "claude",
    cloud: true,
    needs: "Public https HolyArrs; the Free plan allows one custom connector.",
    signin: [
      {
        text: "In Claude on web or desktop, open Customize › Connectors › Add custom connector.",
      },
      { text: "Name it HolyArrs and paste the address.", code: "{url}" },
      { text: "Press Connect. HolyArrs opens in your browser. Press Allow." },
    ],
    key: [],
    check: "HolyArrs shows as connected under Customize › Connectors.",
    note: "Add it on web or desktop; the phone app then lists it.",
    source: "docs",
  },
  {
    slug: "claude-desktop-bridge",
    name: "Claude Desktop (home)",
    family: "claude",
    cloud: false,
    needs: "Claude Desktop and Node.js, so npx runs.",
    signin: [],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Claude Desktop (home) and press Make a key.",
      },
      {
        text: "In Claude Desktop, open Settings › Developer and press Edit config.",
      },
      {
        text: "Add the holyarrs entry under mcpServers in claude_desktop_config.json.",
        code: '{\n  "mcpServers": {\n    "holyarrs": {\n      "command": "npx",\n      "args": [\n        "-y",\n        "mcp-remote@latest",\n        "{url}",\n        "--transport",\n        "http-only",\n        "--allow-http",\n        "--header",\n        "Authorization:${AUTH_HEADER}"\n      ],\n      "env": {\n        "AUTH_HEADER": "Bearer {key}"\n      }\n    }\n  }\n}',
      },
      {
        text: "Quit Claude Desktop from the tray or menu bar, then reopen it.",
      },
    ],
    check: "holyarrs appears in a new chat's tools menu.",
    note: "Keep Authorization:${AUTH_HEADER} without a space; Windows splits arguments at spaces.",
    source: "docs",
  },
  {
    slug: "chatgpt",
    name: "ChatGPT",
    family: "chatgpt-codex",
    cloud: true,
    needs:
      "Business, Enterprise or Edu plan; Pro reportedly allows look-ups only.",
    signin: [
      {
        text: "On chatgpt.com, open Plugins, press + and choose Add custom MCP server.",
      },
      {
        text: "Name it HolyArrs and paste the address into Server URL.",
        code: "{url}",
      },
      {
        text: "Choose OAuth, leave client ID and secret empty, accept the warning, press Create as a plugin.",
      },
      {
        text: "Install the HolyArrs plugin. HolyArrs opens in your browser. Press Allow.",
      },
    ],
    key: [],
    check: "Type @HolyArrs in a new chat and ask a question.",
    note: "Any login in front of HolyArrs, such as Cloudflare Access, blocks ChatGPT.",
    source: "docs",
  },
  {
    slug: "chatgpt-desktop",
    name: "ChatGPT desktop app",
    family: "chatgpt-codex",
    cloud: false,
    needs: "New ChatGPT desktop app (July 2026 or later), not ChatGPT Classic.",
    signin: [
      { text: "Open Settings › MCP servers › Add server." },
      {
        text: "Name it holyarrs, choose Streamable HTTP, paste the address and save.",
        code: "{url}",
      },
      { text: "Restart the app. HolyArrs opens in your browser. Press Allow." },
    ],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick ChatGPT desktop app and press Make a key.",
      },
      {
        text: "Add this block to ~/.codex/config.toml (Windows: %USERPROFILE%\\.codex\\config.toml).",
        code: '[mcp_servers.holyarrs]\nurl = "{url}"\nhttp_headers = { "Authorization" = "Bearer {key}" }',
      },
      { text: "Restart the ChatGPT app." },
    ],
    check: "Settings › MCP servers lists holyarrs.",
    note: "Shares ~/.codex/config.toml with Codex; a server added in either shows in both.",
    source: "docs",
  },
  {
    slug: "codex-cli",
    name: "Codex CLI",
    family: "chatgpt-codex",
    cloud: false,
    needs: "Current Codex, signed in with ChatGPT or an OpenAI API key.",
    signin: [
      {
        text: "Run this in a terminal.",
        code: "codex mcp add holyarrs --url {url}",
      },
      { text: "HolyArrs opens in your browser. Press Allow." },
    ],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Codex CLI and press Make a key.",
      },
      {
        text: "Add this block to ~/.codex/config.toml (Windows: %USERPROFILE%\\.codex\\config.toml).",
        code: '[mcp_servers.holyarrs]\nurl = "{url}"\nhttp_headers = { "Authorization" = "Bearer {key}" }',
      },
      { text: "Restart Codex." },
    ],
    check: "Run codex mcp list; holyarrs is listed.",
    note: "If no browser opens, run codex mcp login holyarrs.",
    source: "live",
  },
  {
    slug: "codex-ide",
    name: "Codex IDE extension",
    family: "chatgpt-codex",
    cloud: false,
    needs: "Current Codex IDE extension, signed in.",
    signin: [
      { text: "Open the gear menu, then MCP servers › Add server." },
      {
        text: "Name it holyarrs, choose Streamable HTTP, paste the address and save.",
        code: "{url}",
      },
      {
        text: "Restart the extension. HolyArrs opens in your browser. Press Allow.",
      },
    ],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Codex IDE extension and press Make a key.",
      },
      {
        text: "Add this block to ~/.codex/config.toml (Windows: %USERPROFILE%\\.codex\\config.toml).",
        code: '[mcp_servers.holyarrs]\nurl = "{url}"\nhttp_headers = { "Authorization" = "Bearer {key}" }',
      },
      { text: "Restart the extension." },
    ],
    check: "Type /mcp in Codex; holyarrs is listed.",
    note: "Codex inside WSL reads its own ~/.codex/config.toml.",
    source: "docs",
  },
  {
    slug: "gemini-cli",
    name: "Gemini CLI",
    family: "gemini",
    cloud: false,
    needs:
      "Paid Gemini API key, Vertex AI, or Code Assist Standard/Enterprise license.",
    signin: [
      {
        text: "Add HolyArrs for all folders.",
        code: "gemini mcp add --scope user --transport http holyarrs {url}",
      },
      {
        text: "Start gemini and run /mcp auth holyarrs.",
        code: "/mcp auth holyarrs",
      },
      { text: "HolyArrs opens in your browser. Press Allow." },
    ],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Gemini CLI and press Make a key.",
      },
      {
        text: "Add HolyArrs for all folders.",
        code: 'gemini mcp add --scope user --transport http --header "Authorization: Bearer {key}" holyarrs {url}',
      },
      { text: "Start gemini and run /mcp list.", code: "/mcp list" },
    ],
    check: "gemini mcp list shows holyarrs as connected.",
    note: 'Sign-in to a private https name (like .ts.net) needs "oauth": { "allowPrivateNetwork": true } in the holyarrs entry.',
    source: "docs",
  },
  {
    slug: "antigravity",
    name: "Antigravity",
    family: "gemini",
    cloud: false,
    needs: "Antigravity editor, app or CLI.",
    signin: [
      {
        text: "Open the global MCP config file.",
        code: "~/.gemini/config/mcp_config.json",
      },
      {
        text: "Add this inside mcpServers and save.",
        code: '{\n  "mcpServers": {\n    "holyarrs": {\n      "serverUrl": "{url}"\n    }\n  }\n}',
      },
      {
        text: "In Agent settings › Customizations, press Authenticate next to holyarrs.",
      },
      {
        text: "HolyArrs opens in your browser. Press Allow, then paste the code into Antigravity and submit it.",
      },
    ],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Antigravity and press Make a key.",
      },
      {
        text: "Open the global MCP config file.",
        code: "~/.gemini/config/mcp_config.json",
      },
      {
        text: "Add this inside mcpServers and save.",
        code: '{\n  "mcpServers": {\n    "holyarrs": {\n      "serverUrl": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
      },
      { text: "Restart Antigravity." },
    ],
    check: "Run /mcp in the Antigravity CLI; holyarrs shows connected.",
    note: "Google does not say plain http works; if refused, use https and sign-in.",
    source: "docs",
  },
  {
    slug: "gemini-code-assist",
    name: "Gemini Code Assist",
    family: "gemini",
    cloud: false,
    needs: "Standard or Enterprise license; extension in VS Code.",
    signin: [],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Gemini Code Assist and press Make a key.",
      },
      {
        text: "Add this inside mcpServers in ~/.gemini/settings.json.",
        code: '{\n  "mcpServers": {\n    "holyarrs": {\n      "httpUrl": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
      },
      { text: "Run Developer: Reload Window from the Command Palette." },
    ],
    check: "Type /mcp in Code Assist chat; holyarrs lists its tools.",
    note: "JetBrains IDEs need the npx mcp-remote bridge, which needs Node.js.",
    source: "docs",
  },
  {
    slug: "gemini-app",
    name: "Gemini app",
    family: "gemini",
    cloud: true,
    needs: "US, 18+, personal Google account, English, Gemini chat history on.",
    signin: [
      {
        text: "On a computer, open gemini.google.com › Settings › Connected Apps › Custom apps.",
      },
      {
        text: "Add a custom app, paste the address and press Next.",
        code: "{url}",
      },
      { text: "HolyArrs opens in your browser. Press Allow." },
    ],
    key: [],
    check: "HolyArrs shows under Custom apps, on web and mobile.",
    note: "Leave Advanced features closed; HolyArrs registers Gemini automatically.",
    source: "docs",
  },
  {
    slug: "vs-code-copilot",
    name: "VS Code with Copilot",
    family: "copilot",
    cloud: false,
    needs:
      "GitHub Copilot signed in; Business or Enterprise admins can block MCP servers.",
    signin: [
      { text: "In VS Code, run MCP: Add Server from the Command Palette." },
      {
        text: "Pick HTTP, paste this address and name it holyarrs.",
        code: "{url}",
      },
      { text: "Choose Global, then Copilot Global." },
      {
        text: "Run MCP: List Servers, pick holyarrs, choose Start. HolyArrs opens in your browser. Press Allow.",
      },
    ],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick VS Code with Copilot and press Make a key.",
      },
      { text: "Press Add to VS Code and confirm the install in VS Code." },
      {
        text: "Or add this entry to ~/.copilot/mcp-config.json.",
        code: '{\n  "mcpServers": {\n    "holyarrs": {\n      "type": "http",\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      },\n      "tools": ["*"]\n    }\n  }\n}',
      },
    ],
    check: "Run MCP: List Servers; holyarrs shows as running.",
    note: "~/.copilot/mcp-config.json is shared with the Copilot CLI.",
    source: "docs",
  },
  {
    slug: "copilot-cli",
    name: "GitHub Copilot CLI",
    family: "copilot",
    cloud: false,
    needs: "A GitHub Copilot plan.",
    signin: [],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick GitHub Copilot CLI and press Make a key.",
      },
      {
        text: "Run this in a terminal.",
        code: 'copilot mcp add --transport http --header "Authorization: Bearer {key}" holyarrs {url}',
      },
      {
        text: "Or type /mcp add: name holyarrs, type HTTP, URL {url}, this header; press Ctrl+S.",
        code: '{"Authorization": "Bearer {key}"}',
      },
    ],
    check: "Run copilot mcp get holyarrs, or type /mcp show holyarrs.",
    note: "Browser sign-in for remote servers is undocumented; use a key.",
    source: "docs",
  },
  {
    slug: "copilot-cloud-agent",
    name: "GitHub Copilot cloud agent",
    family: "copilot",
    cloud: true,
    needs:
      "Copilot cloud agent enabled on the repository; HolyArrs on public https.",
    signin: [],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick GitHub Copilot cloud agent and press Make a key.",
      },
      {
        text: "In the repository, add an Agents secret COPILOT_MCP_HOLYARRS_KEY holding the key.",
      },
      { text: "Open the repository's Settings › Copilot › MCP servers." },
      {
        text: "Paste this into MCP configuration and press Save MCP configuration.",
        code: '{\n  "mcpServers": {\n    "holyarrs": {\n      "type": "http",\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer $COPILOT_MCP_HOLYARRS_KEY"\n      },\n      "tools": ["search_titles", "library_lookup", "calendar", "downloads", "my_asks", "health"]\n    }\n  }\n}',
      },
    ],
    check: "Give it a task asking what's downloading in HolyArrs.",
    note: "It runs tools without asking, so ask_for_title is left out.",
    source: "docs",
  },
  {
    slug: "m365-copilot",
    name: "Microsoft 365 Copilot",
    family: "copilot",
    cloud: true,
    needs:
      "Work account, Agents Toolkit 6.12+, admin allowing custom app upload.",
    signin: [
      {
        text: "In VS Code's Microsoft 365 Agents Toolkit, select Create a New Agent/App › Declarative Agent.",
      },
      {
        text: "Select Add an Action › Start with an MCP Server and enter this address.",
        code: "{url}",
      },
      {
        text: "Choose OAuth (with dynamic registration), name the agent, then select Lifecycle › Provision.",
      },
      {
        text: "In Microsoft 365 Copilot chat, pick the agent and press its sign-in button. HolyArrs opens in your browser. Press Allow.",
      },
    ],
    key: [],
    check: "Ask the agent what's downloading; it answers from HolyArrs.",
    note: "Unconfirmed whether it accepts sign-in without refresh tokens or client secrets.",
    source: "docs",
  },
  {
    slug: "copilot-studio",
    name: "Copilot Studio",
    family: "copilot",
    cloud: true,
    needs: "Copilot Studio licence and environment; HolyArrs on public https.",
    signin: [
      {
        text: "In your agent, open Tools › Add a tool › New tool › Model Context Protocol.",
      },
      {
        text: "Set Server name holyarrs, a Server description and this Server URL.",
        code: "{url}",
      },
      {
        text: "Pick OAuth 2.0, type Dynamic discovery, then select Create and Next.",
      },
      {
        text: "Select Create a new connection, then Add to agent. HolyArrs opens in your browser. Press Allow.",
      },
    ],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Copilot Studio and press Make a key.",
      },
      {
        text: "In your agent, open Tools › Add a tool › New tool › Model Context Protocol.",
      },
      {
        text: "Set Server name holyarrs, a Server description and this Server URL.",
        code: "{url}",
      },
      {
        text: "Pick API key, type Header, name X-Api-Key; connect with this key, then Add to agent.",
        code: "{key}",
      },
    ],
    check: "Ask the agent what's downloading; it answers from HolyArrs.",
    note: "OAuth fit is unconfirmed (no refresh token); the key way avoids it.",
    source: "docs",
  },
  {
    slug: "grok",
    name: "Grok",
    family: "chat-apps",
    cloud: true,
    needs:
      "Public https HolyArrs; on Business and Enterprise, an admin sets up connectors first.",
    signin: [
      {
        text: "Open grok.com/connectors, press New Connector and pick Custom.",
      },
      { text: "Paste the address as the server URL.", code: "{url}" },
      { text: "HolyArrs opens in your browser. Press Allow." },
    ],
    key: [],
    check: "The connector shows as connected and lists its tools.",
    note: "Grok's help names no sign-in types, so browser sign-in is unconfirmed.",
    source: "docs",
  },
  {
    slug: "perplexity",
    name: "Perplexity",
    family: "chat-apps",
    cloud: true,
    needs: "Pro, Max or Enterprise plan, and public https HolyArrs.",
    signin: [
      {
        text: "Open Plugins, press Add custom, then Add MCP connector, then Remote.",
      },
      {
        text: "Set Name to HolyArrs and paste the address under MCP server URL.",
        code: "{url}",
      },
      {
        text: "Under Advanced, set these, tick the risk box and press Add.",
        code: "Authentication: OAuth\nTransport: Streamable HTTP\nNetwork access: Public",
      },
      {
        text: "Click the HolyArrs card. HolyArrs opens in your browser. Press Allow.",
      },
    ],
    key: [],
    check: "Switch HolyArrs on under Sources; its card lists the tools.",
    note: "On a client_secret error, set Authentication to API Key and paste an AI key (unconfirmed).",
    source: "docs",
  },
  {
    slug: "perplexity-mac-helper",
    name: "Perplexity Mac helper",
    family: "chat-apps",
    cloud: false,
    needs: "Mac App Store version of Perplexity, and Node.js.",
    signin: [],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Perplexity Mac helper and press Make a key.",
      },
      {
        text: "In the Mac app, open Plugins and install PerplexityXPC when asked.",
      },
      {
        text: "Press Add custom; on the Simple tab, set Server Name to holyarrs and paste this into Command.",
        code: 'npx -y mcp-remote {url} --allow-http --transport http-only --header "Authorization: Bearer {key}"',
      },
      { text: "Press Save, then switch HolyArrs on under Sources." },
    ],
    check: "The server status reads Running.",
    note: "Keep the command on one line.",
    source: "docs",
  },
  {
    slug: "le-chat",
    name: "Mistral Le Chat",
    family: "chat-apps",
    cloud: true,
    needs: "Public https HolyArrs. On team plans, an admin adds connectors.",
    signin: [
      {
        text: "Open Connectors, press + Add Connector, then the Custom MCP Connector tab.",
      },
      {
        text: "Fill in the name and server address.",
        code: "Connector name: holyarrs\nServer URL: {url}",
      },
      { text: "Press Connect. HolyArrs opens in your browser. Press Allow." },
    ],
    key: [],
    check:
      "Under My Connectors, the HolyArrs card lists its tools in Functions.",
    note: "Mistral's docs now file connectors under Vibe Work, so labels may differ.",
    source: "docs",
  },
  {
    slug: "manus",
    name: "Manus",
    family: "chat-apps",
    cloud: true,
    needs: "Public https HolyArrs.",
    signin: [],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Manus and press Make a key.",
      },
      {
        text: "In Manus, open Connectors, press Create and choose an MCP server by URL.",
      },
      {
        text: "Enter the address, and the key as a bearer token under Authentication.",
        code: "Server URL: {url}\nToken: {key}",
      },
      { text: "Press Test connection." },
    ],
    check: "Test connection lists the HolyArrs tools.",
    note: "Older help puts this under Settings › Integrations › Custom MCP Servers › Add Server.",
    source: "docs",
  },
  {
    slug: "genspark",
    name: "Genspark",
    family: "chat-apps",
    cloud: true,
    needs:
      "Public https HolyArrs. Work accounts follow the admin's custom MCP policy.",
    signin: [
      { text: "Open Skills, then Connectors." },
      {
        text: "Add a custom MCP server and paste the address as MCP URL.",
        code: "{url}",
      },
      { text: "HolyArrs opens in your browser. Press Allow." },
    ],
    key: [],
    check: "The connector shows as connected and lists its tools.",
    note: "Whether Genspark can sign in or send a key is unconfirmed.",
    source: "second-hand",
  },
  {
    slug: "huggingchat",
    name: "HuggingChat",
    family: "chat-apps",
    cloud: true,
    needs: "Public https HolyArrs, and a model that supports tools.",
    signin: [
      { text: "In the chat box, press +, pick MCP Servers, then Add Server." },
      { text: "Type a name and paste the address.", code: "{url}" },
      {
        text: "Run Health Check. HolyArrs opens in your browser. Press Allow.",
      },
    ],
    key: [],
    check: "Health Check passes; switch HolyArrs on in MCP Servers.",
    note: "No key field and sign-in is unconfirmed, so this may not work yet.",
    source: "docs",
  },
  {
    slug: "qwen-desktop",
    name: "Qwen Chat desktop app",
    family: "chat-apps",
    cloud: false,
    needs: "Qwen Chat desktop app on Mac or Windows, and Node.js 20+.",
    signin: [],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Qwen Chat desktop app and press Make a key.",
      },
      { text: "In a chat, open the MCP option, then MCP settings › Add MCP." },
      {
        text: "Choose the JSON form, paste this and press Save and Enable.",
        code: '{\n  "mcpServers": {\n    "holyarrs": {\n      "command": "npx",\n      "args": ["-y", "mcp-remote@latest", "{url}", "--transport", "http-only", "--allow-http", "--header", "Authorization:${HOLYARRS_AUTH}"],\n      "env": { "HOLYARRS_AUTH": "Bearer {key}" }\n    }\n  }\n}',
      },
    ],
    check: "A green mark shows next to holyarrs.",
    note: "Quick Add has no key field; use the JSON form.",
    source: "second-hand",
  },
  {
    slug: "cursor",
    name: "Cursor",
    family: "code-editors",
    cloud: false,
    needs: "",
    signin: [
      { text: "Open ~/.cursor/mcp.json." },
      {
        text: "Add this entry and save:",
        code: '{\n  "mcpServers": {\n    "holyarrs": {\n      "url": "{url}"\n    }\n  }\n}',
      },
      {
        text: "Under Customize, click Needs login next to holyarrs. HolyArrs opens in your browser. Press Allow.",
      },
    ],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Cursor and press Make a key.",
      },
      { text: "Open ~/.cursor/mcp.json." },
      {
        text: "Add this entry and save:",
        code: '{\n  "mcpServers": {\n    "holyarrs": {\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
      },
    ],
    check: "Customize lists holyarrs; errors show in Output › MCP Logs.",
    note: "The Needs login label is second-hand; Cursor's docs do not name it.",
    source: "docs",
  },
  {
    slug: "windsurf",
    name: "Windsurf",
    family: "code-editors",
    cloud: false,
    needs: "Teams plans need a full seat; an admin allow-list can block it.",
    signin: [
      {
        text: "Open ~/.config/devin/mcp_config.json (Windows: %APPDATA%\\devin\\mcp_config.json).",
      },
      {
        text: "Add this entry and save:",
        code: '{\n  "mcpServers": {\n    "holyarrs": {\n      "url": "{url}"\n    }\n  }\n}',
      },
      {
        text: "Under Customize › MCPs, press Authenticate next to holyarrs. HolyArrs opens in your browser. Press Allow.",
      },
    ],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Windsurf and press Make a key.",
      },
      {
        text: "Open ~/.config/devin/mcp_config.json (Windows: %APPDATA%\\devin\\mcp_config.json).",
      },
      {
        text: "Add this entry and save:",
        code: '{\n  "mcpServers": {\n    "holyarrs": {\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
      },
    ],
    check: "Customize › MCPs lists holyarrs, switched on.",
    note: "Older Windsurf builds read ~/.codeium/windsurf/mcp_config.json instead (second-hand).",
    source: "docs",
  },
  {
    slug: "zed",
    name: "Zed",
    family: "code-editors",
    cloud: false,
    needs: "",
    signin: [
      { text: "Run zed: open settings file from the command palette." },
      {
        text: "Add context_servers inside your settings and save:",
        code: '{\n  "context_servers": {\n    "holyarrs": {\n      "url": "{url}"\n    }\n  }\n}',
      },
      {
        text: "Zed starts the sign-in. HolyArrs opens in your browser. Press Allow.",
      },
    ],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Zed and press Make a key.",
      },
      { text: "Run zed: open settings file from the command palette." },
      {
        text: "Add context_servers inside your settings and save:",
        code: '{\n  "context_servers": {\n    "holyarrs": {\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
      },
    ],
    check: 'holyarrs shows a green dot and "Server is active".',
    note: "",
    source: "docs",
  },
  {
    slug: "jetbrains-ai",
    name: "JetBrains AI Assistant",
    family: "code-editors",
    cloud: false,
    needs:
      "AI Assistant installed; a company admin can block self-added servers.",
    signin: [],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick JetBrains AI Assistant and press Make a key.",
      },
      {
        text: "Open Settings › Tools › AI Assistant › Model Context Protocol (MCP) and press Add.",
      },
      {
        text: "Pick HTTP (Streamable HTTP), paste this entry, then press OK and Apply:",
        code: '{\n  "mcpServers": {\n    "holyarrs": {\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
      },
    ],
    check: "The Status column shows holyarrs; its icon lists the tools.",
    note: "JetBrains documents only url; whether it sends headers is unconfirmed.",
    source: "docs",
  },
  {
    slug: "kiro",
    name: "Kiro",
    family: "code-editors",
    cloud: false,
    needs: "An https HolyArrs address, or Kiro on the HolyArrs computer.",
    signin: [
      { text: "In Kiro Settings, search MCP and switch on MCP support." },
      {
        text: "From the Command Palette, run Kiro: Open user MCP config (JSON).",
      },
      {
        text: "Add this entry and save:",
        code: '{\n  "mcpServers": {\n    "holyarrs": {\n      "url": "{url}"\n    }\n  }\n}',
      },
      {
        text: "Kiro starts the sign-in. HolyArrs opens in your browser. Press Allow.",
      },
    ],
    key: [],
    check: "Kiro's MCP panel lists holyarrs with its tools.",
    note: "Kiro refuses plain http network addresses, so there is no key way.",
    source: "docs",
  },
  {
    slug: "cline",
    name: "Cline",
    family: "vscode-add-ons",
    cloud: false,
    needs: "",
    signin: [],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Cline and press Make a key.",
      },
      {
        text: "In Cline, click the MCP Servers icon, then Configure › Configure MCP Servers.",
      },
      {
        text: "Add this to cline_mcp_settings.json and save.",
        code: '{\n  "mcpServers": {\n    "holyarrs": {\n      "type": "streamableHttp",\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
      },
    ],
    check: "holyarrs shows in Cline's MCP server list without an error.",
    note: 'Keep "type": "streamableHttp"; without it Cline falls back to SSE and fails.',
    source: "docs",
  },
  {
    slug: "continue",
    name: "Continue",
    family: "vscode-add-ons",
    cloud: false,
    needs: "",
    signin: [],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Continue and press Make a key.",
      },
      {
        text: "Open ~/.continue/config.yaml from the gear next to Local Config.",
      },
      {
        text: "Add this entry under mcpServers and save.",
        code: "mcpServers:\n  - name: HolyArrs\n    type: streamable-http\n    url: {url}\n    requestOptions:\n      headers:\n        Authorization: Bearer {key}",
      },
      { text: "Switch the chat to agent mode." },
    ],
    check: "HolyArrs shows in Continue's MCP list without an error.",
    note: 'Some versions may not send the key header; if HolyArrs answers "sign in", use Cline.',
    source: "docs",
  },
  {
    slug: "kilo-code",
    name: "Kilo Code",
    family: "vscode-add-ons",
    cloud: false,
    needs: "",
    signin: [
      {
        text: "Add this under mcp in ~/.config/kilo/kilo.jsonc and save.",
        code: '{\n  "mcp": {\n    "holyarrs": {\n      "type": "remote",\n      "url": "{url}"\n    }\n  }\n}',
      },
      { text: "Click the notice that holyarrs needs sign-in." },
      { text: "HolyArrs opens in your browser. Press Allow." },
    ],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Kilo Code and press Make a key.",
      },
      {
        text: "Add this under mcp in ~/.config/kilo/kilo.jsonc and save.",
        code: '{\n  "mcp": {\n    "holyarrs": {\n      "type": "remote",\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
      },
    ],
    check: "holyarrs shows in Kilo Code's MCP server list without an error.",
    note: 'If Kilo Code tries sign-in on plain http, add "oauth": false to the entry.',
    source: "docs",
  },
  {
    slug: "opencode",
    name: "OpenCode",
    family: "terminal-tools",
    cloud: false,
    needs: "",
    signin: [
      {
        text: "Add this to ~/.config/opencode/opencode.json.",
        code: '{\n  "mcp": {\n    "holyarrs": {\n      "type": "remote",\n      "url": "{url}"\n    }\n  }\n}',
      },
      { text: "Run this to sign in.", code: "opencode mcp auth holyarrs" },
      { text: "HolyArrs opens in your browser. Press Allow." },
    ],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick OpenCode and press Make a key.",
      },
      {
        text: "Add this to ~/.config/opencode/opencode.json.",
        code: '{\n  "mcp": {\n    "holyarrs": {\n      "type": "remote",\n      "url": "{url}",\n      "oauth": false,\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
      },
      { text: "Restart OpenCode." },
    ],
    check: "opencode mcp list shows holyarrs and its sign-in state.",
    note: "A project can use its own opencode.json instead.",
    source: "docs",
  },
  {
    slug: "warp",
    name: "Warp",
    family: "terminal-tools",
    cloud: false,
    needs: "",
    signin: [
      {
        text: "In Warp, open Settings › Agents › MCP servers and press + Add.",
      },
      {
        text: "Pick URL and paste this.",
        code: '{\n  "holyarrs": {\n    "url": "{url}"\n  }\n}',
      },
      {
        text: "When the server starts, HolyArrs opens in your browser. Press Allow.",
      },
    ],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Warp and press Make a key.",
      },
      {
        text: "In Warp, open Settings › Agents › MCP servers and press + Add.",
      },
      {
        text: "Pick URL and paste this.",
        code: '{\n  "holyarrs": {\n    "url": "{url}",\n    "headers": {\n      "Authorization": "Bearer {key}"\n    }\n  }\n}',
      },
    ],
    check: "holyarrs is listed under Settings › Agents › MCP servers.",
    note: "Covers local agents only; cloud agents set up separately. Sign-in is per computer.",
    source: "docs",
  },
  {
    slug: "amp",
    name: "Amp",
    family: "terminal-tools",
    cloud: false,
    needs: "",
    signin: [
      { text: "Add HolyArrs.", code: "amp mcp add holyarrs {url}" },
      { text: "Start Amp. HolyArrs opens in your browser. Press Allow." },
    ],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Amp and press Make a key.",
      },
      {
        text: "Add the amp.mcpServers block to ~/.config/amp/settings.json.",
        code: '{\n  "amp.mcpServers": {\n    "holyarrs": {\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
      },
      { text: "Restart Amp." },
    ],
    check: "amp mcp doctor shows holyarrs and its state.",
    note: "In a project's .amp/settings.json, run amp mcp approve holyarrs first.",
    source: "docs",
  },
  {
    slug: "pi",
    name: "Pi",
    family: "terminal-tools",
    cloud: false,
    needs: "Pi 0.99.0 or newer.",
    signin: [
      {
        text: "Add this to ~/.pi/agent/mcp.json.",
        code: '{\n  "mcpServers": {\n    "holyarrs": {\n      "url": "{url}"\n    }\n  }\n}',
      },
      {
        text: "Run this, or /mcp login holyarrs inside Pi.",
        code: "pi mcp login holyarrs",
      },
      { text: "HolyArrs opens in your browser. Press Allow." },
    ],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Pi and press Make a key.",
      },
      {
        text: "Add this to ~/.pi/agent/mcp.json.",
        code: '{\n  "mcpServers": {\n    "holyarrs": {\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
      },
      { text: "Restart Pi." },
    ],
    check: "Type /mcp inside Pi; holyarrs is listed.",
    note: "",
    source: "docs",
  },
  {
    slug: "mistral-vibe",
    name: "Mistral Vibe CLI",
    family: "terminal-tools",
    cloud: false,
    needs: "",
    signin: [
      { text: "Add HolyArrs.", code: "vibe mcp add holyarrs --url {url}" },
      { text: "HolyArrs opens in your browser. Press Allow." },
    ],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Mistral Vibe CLI and press Make a key.",
      },
      {
        text: "Put the key in HOLYARRS_AI_KEY.",
        code: 'export HOLYARRS_AI_KEY="{key}"',
      },
      {
        text: "Add this to ~/.vibe/config.toml.",
        code: '[[mcp_servers]]\nname = "holyarrs"\ntransport = "http"\nurl = "{url}"\n\n[mcp_servers.auth]\ntype = "static"\napi_key_env = "HOLYARRS_AI_KEY"\napi_key_header = "Authorization"\napi_key_format = "Bearer {token}"',
      },
      { text: "Start Vibe from that shell." },
    ],
    check: "/mcp holyarrs inside Vibe lists the HolyArrs tools.",
    note: "If no browser opens, run /mcp login holyarrs inside Vibe.",
    source: "docs",
  },
  {
    slug: "kimi-code",
    name: "Kimi Code CLI",
    family: "terminal-tools",
    cloud: false,
    needs: "",
    signin: [
      {
        text: "Add this to ~/.kimi-code/mcp.json.",
        code: '{\n  "mcpServers": {\n    "holyarrs": {\n      "url": "{url}"\n    }\n  }\n}',
      },
      {
        text: "Inside Kimi Code, run this.",
        code: "/mcp-config login holyarrs",
      },
      { text: "HolyArrs opens in your browser. Press Allow." },
    ],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Kimi Code CLI and press Make a key.",
      },
      {
        text: "Add this to ~/.kimi-code/mcp.json.",
        code: '{\n  "mcpServers": {\n    "holyarrs": {\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
      },
      { text: "Restart Kimi Code." },
    ],
    check: "/mcp inside Kimi Code shows holyarrs connected.",
    note: "",
    source: "docs",
  },
  {
    slug: "devin",
    name: "Devin",
    family: "terminal-tools",
    cloud: true,
    needs:
      "Public https HolyArrs without a login wall; Manage MCP Servers permission.",
    signin: [
      {
        text: "In Devin, open Customize › MCPs, press Add MCP, then Add custom MCP.",
      },
      {
        text: "Fill in these fields and press Save.",
        code: "Server Name: HolyArrs\nTransport:   HTTP\nServer URL:  {url}\nAuth:        OAuth\nAccess:      Personal",
      },
      { text: "Press Connect. HolyArrs opens in your browser. Press Allow." },
    ],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Devin and press Make a key.",
      },
      {
        text: "In Devin, open Customize › MCPs, press Add MCP, then Add custom MCP.",
      },
      {
        text: "Fill in these fields and press Save.",
        code: "Server Name:  HolyArrs\nTransport:    HTTP\nServer URL:   {url}\nAuth:         Auth Header\nHeader name:  Authorization\nHeader value: Bearer {key}",
      },
    ],
    check: "Press Test tools; it lists the HolyArrs tools.",
    note: "Access Organization shares one sign-in: everyone acts as whoever pressed Allow.",
    source: "docs",
  },
  {
    slug: "open-webui",
    name: "Open WebUI",
    family: "self-hosted-chat",
    cloud: false,
    needs: "Open WebUI v0.6.31 or newer and an Open WebUI admin account.",
    signin: [
      {
        text: "In Open WebUI, open Admin Panel › Settings › Integrations and press + Add Connection.",
      },
      {
        text: "Set Type MCP (Streamable HTTP), Auth OAuth 2.1, Server URL {url}; leave Key empty.",
        code: "Server URL: {url}",
      },
      {
        text: "Give Open WebUI a fixed secret so the sign-in survives restarts.",
        code: "WEBUI_SECRET_KEY=<long random text you keep>",
      },
      {
        text: "Turn on HolyArrs in a chat's tools menu. HolyArrs opens in your browser. Press Allow.",
      },
    ],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Open WebUI and press Make a key.",
      },
      {
        text: "In Open WebUI, open Admin Panel › Settings › Integrations and press + Add Connection.",
      },
      {
        text: "Set Type MCP (Streamable HTTP), Auth Bearer, fill in these fields and save.",
        code: "Server URL: {url}\nKey: {key}",
      },
      { text: "Turn on HolyArrs in a chat's tools menu." },
    ],
    check: "A new chat's tools menu lists the HolyArrs tools.",
    note: "In Docker, localhost is the container; use HolyArrs's network IP.",
    source: "docs",
  },
  {
    slug: "librechat",
    name: "LibreChat",
    family: "self-hosted-chat",
    cloud: false,
    needs: "A LibreChat role that may add MCP servers.",
    signin: [
      {
        text: "Open MCP Settings › Add MCP, paste {url}, pick Transport Streamable HTTP.",
        code: "{url}",
      },
      {
        text: "Under Authentication pick OAuth; leave Client ID and Client Secret empty.",
      },
      { text: "Tick I trust this application and press Create." },
      {
        text: "Connect the server. HolyArrs opens in your browser. Press Allow.",
      },
    ],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick LibreChat and press Make a key.",
      },
      {
        text: "On a home address, allow HolyArrs in librechat.yaml and restart LibreChat.",
        code: 'mcpSettings:\n  allowedAddresses:\n    - "<HolyArrs IP>:<port>"',
      },
      {
        text: "Open MCP Settings › Add MCP, paste {url}, pick Transport Streamable HTTP.",
        code: "{url}",
      },
      {
        text: "Set Authentication API Key, Header Format Bearer, enter {key}, tick I trust this application, press Create.",
        code: "{key}",
      },
    ],
    check: "The message box's + › MCP Servers lists HolyArrs.",
    note: "Use allowedAddresses; allowedDomains blocks every unlisted server.",
    source: "docs",
  },
  {
    slug: "lobehub",
    name: "LobeHub",
    family: "self-hosted-chat",
    cloud: false,
    needs: "The LobeHub desktop app when HolyArrs is on a home address.",
    signin: [],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick LobeHub and press Make a key.",
      },
      {
        text: "In LobeHub, open Settings › Connectors and add an MCP server by hand.",
      },
      {
        text: "Choose HTTP, enter {url}, set auth API Key, enter {key}, save.",
        code: "URL: {url}\nAPI Key: {key}",
      },
      { text: "Open Agent Settings › Connectors, select HolyArrs and save." },
    ],
    check: "Settings › Connectors shows HolyArrs as connected.",
    note: "No API Key field? Add the header Authorization: Bearer followed by your key instead.",
    source: "docs",
  },
  {
    slug: "lm-studio",
    name: "LM Studio",
    family: "desktop-apps",
    cloud: false,
    needs: "LM Studio 0.3.17 or newer; 0.4.10 or newer for browser sign-in.",
    signin: [
      {
        text: "In LM Studio, open the Program tab, pick Install, then Edit mcp.json.",
      },
      {
        text: "Add this entry and save.",
        code: '{\n  "mcpServers": {\n    "holyarrs": {\n      "url": "{url}"\n    }\n  }\n}',
      },
      { text: "HolyArrs opens in your browser. Press Allow." },
    ],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick LM Studio and press Make a key.",
      },
      {
        text: "In LM Studio, open the Program tab, pick Install, then Edit mcp.json.",
      },
      {
        text: "Add this entry and save.",
        code: '{\n  "mcpServers": {\n    "holyarrs": {\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
      },
    ],
    check: "holyarrs lists its tools in LM Studio.",
    note: "If mcp.json lists other servers, add holyarrs beside them.",
    source: "docs",
  },
  {
    slug: "jan",
    name: "Jan",
    family: "desktop-apps",
    cloud: false,
    needs: "Node.js for the key way.",
    signin: [
      {
        text: "In Jan, open Settings › MCP Servers and press + Add MCP Server.",
      },
      { text: "Set Transport to HTTP and paste this address.", code: "{url}" },
      {
        text: "Press Sign in on the Auth line. HolyArrs opens in your browser. Press Allow.",
      },
    ],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Jan and press Make a key.",
      },
      {
        text: "In Jan, open Settings › MCP Servers and press + Add MCP Server.",
      },
      {
        text: "Set Transport to STDIO, Command to npx, and these Args, one per line.",
        code: "-y\nmcp-remote@latest\n{url}\n--transport\nhttp-only\n--allow-http\n--header\nAuthorization:${AUTH_HEADER}",
      },
      { text: "In Env, set AUTH_HEADER to this value.", code: "Bearer {key}" },
    ],
    check: "holyarrs lists its tools in Jan.",
    note: "Jan stops a tool call after 30 seconds by default.",
    source: "docs",
  },
  {
    slug: "anythingllm",
    name: "AnythingLLM",
    family: "desktop-apps",
    cloud: false,
    needs: "AnythingLLM Desktop v1.8.0 or newer.",
    signin: [],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick AnythingLLM and press Make a key.",
      },
      {
        text: "In AnythingLLM, open Agent Skills once to create the servers file.",
      },
      {
        text: "Add this entry to plugins/anythingllm_mcp_servers.json in its storage folder.",
        code: '{\n  "mcpServers": {\n    "holyarrs": {\n      "type": "streamable",\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
      },
      { text: "Open Agent Skills again to load holyarrs." },
    ],
    check: "holyarrs shows as running under Agent Skills.",
    note: "Start chat messages with @agent so the model can use tools.",
    source: "docs",
  },
  {
    slug: "cherry-studio",
    name: "Cherry Studio",
    family: "desktop-apps",
    cloud: false,
    needs: "",
    signin: [],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Cherry Studio and press Make a key.",
      },
      {
        text: "In Cherry Studio, open Settings › MCP › MCP Servers, press Add, choose Import from JSON.",
      },
      {
        text: "Paste this entry and save.",
        code: '{\n  "mcpServers": {\n    "holyarrs": {\n      "type": "streamableHttp",\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
      },
      { text: "Switch holyarrs on in your assistant's MCP settings." },
    ],
    check: "holyarrs shows as running in MCP Servers.",
    note: 'Without "type": "streamableHttp", Cherry Studio treats the entry as a program.',
    source: "second-hand",
  },
  {
    slug: "msty-studio",
    name: "Msty Studio",
    family: "desktop-apps",
    cloud: false,
    needs: "",
    signin: [],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Msty Studio and press Make a key.",
      },
      {
        text: "In Msty Studio, open Toolbox › Tools, press Add New Tool and pick HTTP.",
      },
      {
        text: "Name it HolyArrs and paste this into MCP Server URL.",
        code: "{url}",
      },
      {
        text: "In Headers, add Authorization with this value, then press Add.",
        code: "Bearer {key}",
      },
    ],
    check: "Run a HolyArrs tool in the Tool Console.",
    note: "Msty's docs do not show the Headers field format.",
    source: "docs",
  },
  {
    slug: "5ire",
    name: "5ire",
    family: "desktop-apps",
    cloud: false,
    needs: "5ire v0.11.0 or newer, and Node.js.",
    signin: [],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick 5ire and press Make a key.",
      },
      { text: "In 5ire, open Tools and press New." },
      {
        text: "Fill in command, arguments and environment from this entry.",
        code: '{\n  "mcpServers": {\n    "holyarrs": {\n      "command": "npx",\n      "args": [\n        "-y",\n        "mcp-remote@latest",\n        "{url}",\n        "--transport",\n        "http-only",\n        "--allow-http",\n        "--header",\n        "Authorization:${AUTH_HEADER}"\n      ],\n      "env": {\n        "AUTH_HEADER": "Bearer {key}"\n      }\n    }\n  }\n}',
      },
      { text: "Switch the tool on." },
    ],
    check: "",
    note: "Drop --allow-http when HolyArrs is on https.",
    source: "second-hand",
  },
  {
    slug: "hermes-agent",
    name: "Hermes Agent",
    family: "local-agents",
    cloud: false,
    needs: "",
    signin: [
      {
        text: "Add this to ~/.hermes/config.yaml.",
        code: 'mcp_servers:\n  holyarrs:\n    url: "{url}"\n    auth: oauth',
      },
      { text: "Start the sign-in.", code: "hermes mcp login holyarrs" },
      { text: "HolyArrs opens in your browser. Press Allow." },
    ],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Hermes Agent and press Make a key.",
      },
      {
        text: "Add this to ~/.hermes/config.yaml.",
        code: 'mcp_servers:\n  holyarrs:\n    url: "{url}"\n    headers:\n      Authorization: "Bearer {key}"',
      },
      {
        text: "Run /reload-mcp in a running session, or start a new one.",
        code: "/reload-mcp",
      },
    ],
    check: "hermes mcp test holyarrs lists the HolyArrs tools.",
    note: "Tools show as mcp_holyarrs_<tool>; hide one under tools: exclude.",
    source: "docs",
  },
  {
    slug: "goose",
    name: "Goose",
    family: "local-agents",
    cloud: false,
    needs: "",
    signin: [
      {
        text: "Run goose configure, then Add Extension › Remote Extension (Streamable HTTP).",
        code: "goose configure",
      },
      {
        text: "Name it holyarrs, enter this URI, keep the default timeout, skip custom headers.",
        code: "{url}",
      },
      {
        text: "Run goose session. HolyArrs opens in your browser. Press Allow.",
        code: "goose session",
      },
    ],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Goose and press Make a key.",
      },
      {
        text: "Run goose configure, then Add Extension › Remote Extension (Streamable HTTP).",
        code: "goose configure",
      },
      {
        text: "Name it holyarrs, enter this URI, keep the default timeout.",
        code: "{url}",
      },
      {
        text: "Add a custom header named Authorization with this value.",
        code: "Bearer {key}",
      },
    ],
    check: "Goose's extension list shows holyarrs.",
    note: "Desktop app: Extensions › Add custom extension, same URI and header.",
    source: "docs",
  },
  {
    slug: "ollmcp",
    name: "ollmcp",
    family: "local-agents",
    cloud: false,
    needs: "Python 3.11 or newer; Ollama running with a pulled model.",
    signin: [],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick ollmcp and press Make a key.",
      },
      { text: "Install ollmcp.", code: "uv tool install --upgrade ollmcp" },
      {
        text: "Save this as holyarrs.json.",
        code: '{\n  "mcpServers": {\n    "holyarrs": {\n      "type": "streamable_http",\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
      },
      {
        text: "Start a chat with your Ollama model.",
        code: "ollmcp -j holyarrs.json -m qwen3.5",
      },
    ],
    check: "Type /tools in the chat; HolyArrs tools are listed.",
    note: "-H on the ollmcp command sets the Ollama host, not a header.",
    source: "docs",
  },
  {
    slug: "llama-cpp",
    name: "llama.cpp web UI",
    family: "local-runners",
    cloud: false,
    needs: "Node.js for npx. llama.cpp MCP support is experimental.",
    signin: [],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick llama.cpp web UI and press Make a key.",
      },
      {
        text: "Save this as mcp.json in the folder you start llama-server from.",
        code: '{\n  "mcpServers": {\n    "holyarrs": {\n      "command": "npx",\n      "args": [\n        "-y",\n        "mcp-remote@latest",\n        "{url}",\n        "--transport",\n        "http-only",\n        "--allow-http",\n        "--header",\n        "Authorization:${AUTH_HEADER}"\n      ],\n      "env": {\n        "AUTH_HEADER": "Bearer {key}"\n      }\n    }\n  }\n}',
      },
      {
        text: "Start llama-server with the file.",
        code: "llama-server -m <your model>.gguf --mcp-servers-config mcp.json",
      },
      { text: "Open the llama.cpp web UI." },
    ],
    check: "The HolyArrs tools show in the web UI.",
    note: "On an https address, drop --allow-http.",
    source: "docs",
  },
  {
    slug: "localai",
    name: "LocalAI",
    family: "local-runners",
    cloud: false,
    needs: "",
    signin: [],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick LocalAI and press Make a key.",
      },
      {
        text: "Add this mcp block to your model's YAML config.",
        code: 'mcp:\n  remote: |\n    {\n      "mcpServers": {\n        "holyarrs": {\n          "url": "{url}",\n          "token": "{key}"\n        }\n      }\n    }',
      },
      { text: "Reload the model." },
    ],
    check: "In chat, MCP > Servers lists holyarrs with no errors.",
    note: "In the model editor, paste only the inner mcpServers JSON into Remote MCP Servers.",
    source: "docs",
  },
  {
    slug: "text-generation-webui",
    name: "text-generation-webui",
    family: "local-runners",
    cloud: false,
    needs: "",
    signin: [],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick text-generation-webui and press Make a key.",
      },
      { text: "In the Model tab, load a model with tool-call support." },
      {
        text: "In the chat sidebar, open MCP servers and add this line.",
        code: "{url},Authorization: Bearer {key}",
      },
    ],
    check: 'Ask "Use HolyArrs: what is downloading?"; a tool runs.',
    note: "Tool calls run without asking you first.",
    source: "docs",
  },
  {
    slug: "koboldcpp",
    name: "KoboldCpp",
    family: "local-runners",
    cloud: false,
    needs: "KoboldCpp 1.106 or newer, and Node.js for npx.",
    signin: [],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick KoboldCpp and press Make a key.",
      },
      {
        text: "Save this as mcp.json.",
        code: '{\n  "mcpServers": {\n    "holyarrs": {\n      "command": "npx",\n      "args": [\n        "-y",\n        "mcp-remote@latest",\n        "{url}",\n        "--transport",\n        "http-only",\n        "--allow-http",\n        "--header",\n        "Authorization:${AUTH_HEADER}"\n      ],\n      "env": {\n        "AUTH_HEADER": "Bearer {key}"\n      }\n    }\n  }\n}',
      },
      {
        text: "Start KoboldCpp with the file.",
        code: "koboldcpp --model <your model>.gguf --mcpfile mcp.json",
      },
      {
        text: "In KoboldAI Lite settings, turn on tool calling and fetch the tools.",
      },
    ],
    check: "No MCP errors at startup; HolyArrs tools listed in Lite.",
    note: "Turn on Manually Approve Tools, or every tool call runs unasked.",
    source: "docs",
  },
  {
    slug: "page-assist",
    name: "Page Assist",
    family: "local-runners",
    cloud: false,
    needs: "",
    signin: [
      { text: "Set up OAuth as Page Assist's MCP guide describes." },
      { text: "In Page Assist's MCP settings, add {url} with OAuth 2.1." },
      { text: "HolyArrs opens in your browser. Press Allow." },
    ],
    key: [
      {
        text: "In HolyArrs, open Settings › AI connections, pick Page Assist and press Make a key.",
      },
      {
        text: "In Page Assist's MCP settings, add {url} with Bearer token {key}.",
      },
      { text: "Turn on Require approval before running MCP tools." },
    ],
    check: 'The key shows "last used" in Settings › AI connections.',
    note: "It calls from your browser, so HolyArrs may refuse it as another site.",
    source: "docs",
  },
];
