// Setup for the AI apps beyond the main tabs, one entry per app, matching
// the website guides (holyarrs.coderluii.dev/docs/ai/<family>/). {url} and
// {key} are filled in by ai-connect.ts. Keep in step with those pages.

export type AppData = {
  family: string;
  id: string;
  name: string;
  /** Connects from the maker's servers: needs a public https address. */
  cloud: boolean;
  /** Takes browser sign-in. */
  signIn: boolean;
  /** Takes an AI key in a header. */
  key: boolean;
  where: string;
  signInSnippet: string;
  keySnippet: string;
  steps: string;
  check: string;
  /** Steps from the maker's docs or a write-up, not from a run against HolyArrs. */
  secondHand: boolean;
};

export const APP_DATA: AppData[] = [
  {
    family: "gemini",
    id: "gemini-code-assist",
    name: "Gemini Code Assist (VS Code)",
    cloud: false,
    signIn: false,
    key: true,
    where: "~/.gemini/settings.json (shared with Gemini CLI)",
    signInSnippet: "",
    keySnippet:
      '{\n  "mcpServers": {\n    "holyarrs": {\n      "httpUrl": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
    steps:
      'Make an AI key: "Make a key for Gemini" on plain http, or "Make a key by hand" on https or localhost. Add the entry inside mcpServers in ~/.gemini/settings.json. Run "Developer: Reload Window" from the Command Palette.',
    check: "In the Code Assist chat, type /mcp to see HolyArrs and its tools.",
    secondHand: false,
  },
  {
    family: "gemini",
    id: "gemini-code-assist-jetbrains",
    name: "Gemini Code Assist (IntelliJ and other JetBrains IDEs)",
    cloud: false,
    signIn: false,
    key: true,
    where: "The mcp.json file Code Assist uses in the IDE's settings folder",
    signInSnippet: "",
    keySnippet:
      '{\n  "mcpServers": {\n    "holyarrs": {\n      "command": "npx",\n      "args": [\n        "-y",\n        "mcp-remote@latest",\n        "{url}",\n        "--transport",\n        "http-only",\n        "--allow-http",\n        "--header",\n        "Authorization:${AUTH_HEADER}"\n      ],\n      "env": {\n        "AUTH_HEADER": "Bearer {key}"\n      }\n    }\n  }\n}',
    steps:
      "Code Assist reaches remote servers in JetBrains IDEs only through the mcp-remote bridge, which needs Node.js. Add the entry to the mcp.json file Code Assist uses. Keep Authorization:${AUTH_HEADER} with no space, and leave out --allow-http on an https address.",
    check: "Its MCP list shows holyarrs with its tools.",
    secondHand: false,
  },
  {
    family: "gemini",
    id: "gemini-app",
    name: "Gemini app",
    cloud: true,
    signIn: true,
    key: false,
    where: "gemini.google.com > Settings > Connected Apps > Custom apps",
    signInSnippet: "",
    keySnippet: "",
    steps:
      'On a computer, open gemini.google.com (setup works only on the web). Go to Settings > Connected Apps > Custom apps, add a custom app, paste the AI address and press "Next". HolyArrs opens: sign in if asked and press "Allow"; it then shows in the Android app too.',
    check: "HolyArrs shows under Custom apps.",
    secondHand: false,
  },
  {
    family: "copilot",
    id: "copilot-cli",
    name: "GitHub Copilot CLI",
    cloud: false,
    signIn: false,
    key: true,
    where:
      "~/.copilot/mcp-config.json (Windows: %USERPROFILE%\\.copilot\\mcp-config.json), or /mcp add inside the CLI",
    signInSnippet: "",
    keySnippet:
      'copilot mcp add --transport http --header "Authorization: Bearer {key}" holyarrs {url}',
    steps:
      'Make the key in Settings > AI connections > "More apps" > "GitHub Copilot CLI" (plain http) or with "Make a key by hand" (https). Run the command, or type /mcp add inside the Copilot CLI: name holyarrs, type "HTTP/SSE", the AI address as URL, the header JSON in the HTTP headers field, tools *, then Ctrl+S.',
    check:
      "Type /mcp show holyarrs, or run copilot mcp get holyarrs in a terminal.",
    secondHand: false,
  },
  {
    family: "copilot",
    id: "copilot-cloud-agent",
    name: "GitHub Copilot cloud agent",
    cloud: true,
    signIn: false,
    key: true,
    where:
      "GitHub repository Settings > Copilot > MCP servers, with the key in an Agents secret named COPILOT_MCP_HOLYARRS_KEY",
    signInSnippet: "",
    keySnippet:
      '{\n  "mcpServers": {\n    "holyarrs": {\n      "type": "http",\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer $COPILOT_MCP_HOLYARRS_KEY"\n      },\n      "tools": ["search_titles", "library_lookup", "calendar", "downloads", "my_asks", "health"]\n    }\n  }\n}',
    steps:
      'Make an AI key by hand, named after the repository. In the repository add an Agents secret COPILOT_MCP_HOLYARRS_KEY with the key as its value. Paste the entry into "MCP configuration" and press "Save MCP configuration"; it leaves out ask_for_title because the agent uses tools without asking.',
    check: "Ask the first question and read the answer.",
    secondHand: false,
  },
  {
    family: "copilot",
    id: "m365-copilot",
    name: "Microsoft 365 Copilot",
    cloud: true,
    signIn: true,
    key: false,
    where:
      "VS Code > Microsoft 365 Agents Toolkit > Create a New Agent/App > Declarative Agent > Add an Action > Start with an MCP Server",
    signInSnippet: "",
    keySnippet: "",
    steps:
      'In the Microsoft 365 Agents Toolkit, create a Declarative Agent, add an action that starts with an MCP Server, enter the AI address and pick "OAuth (with dynamic registration)". Sign in under Accounts, then select "Provision" in the Lifecycle pane. In Microsoft 365 Copilot chat pick the agent, sign in to HolyArrs in the pop-up and press "Allow".',
    check: "Ask the first question and read the answer.",
    secondHand: false,
  },
  {
    family: "copilot",
    id: "copilot-studio",
    name: "Copilot Studio",
    cloud: true,
    signIn: true,
    key: true,
    where:
      "Your agent's Tools page > Add a tool > New tool > Model Context Protocol",
    signInSnippet: "",
    keySnippet: "",
    steps:
      'Fill in Server name (holyarrs), a Server description and the AI address as Server URL. For browser sign-in pick "OAuth 2.0" with type "Dynamic discovery"; for an AI key pick "API key", type "Header", header name X-Api-Key. Select "Create", create a new connection, sign in and press "Allow" if asked, then "Add to agent".',
    check: "Ask the first question and read the answer.",
    secondHand: false,
  },
  {
    family: "chat-apps",
    id: "grok",
    name: "Grok",
    cloud: true,
    signIn: true,
    key: false,
    where: "grok.com/connectors > New Connector > Custom",
    signInSnippet: "Server URL: {url}",
    keySnippet: "",
    steps:
      'Go to grok.com/connectors, press "New Connector" and pick "Custom". Paste the AI address as the server URL. Finish the sign-in Grok asks for: sign in to HolyArrs if asked and press "Allow" (which sign-in types Grok supports is unconfirmed).',
    check: "The HolyArrs connector shows as connected and lists its tools.",
    secondHand: false,
  },
  {
    family: "chat-apps",
    id: "perplexity",
    name: "Perplexity (web and desktop)",
    cloud: true,
    signIn: true,
    key: true,
    where:
      "Plugins > Add custom > Add MCP connector > Remote (older: Customize > Connectors or Settings > Connectors)",
    signInSnippet:
      "Name:           HolyArrs\nMCP server URL: {url}\nAuthentication: OAuth\nTransport:      Streamable HTTP\nNetwork access: Public",
    keySnippet: "",
    steps:
      'Press "Add custom", "Add MCP connector", "Remote"; name it HolyArrs, paste the AI address, and under "Advanced" set Authentication OAuth, Transport Streamable HTTP, Network access Public, then "Add". Click the HolyArrs card, sign in and press "Allow", then switch HolyArrs on in the thread or under "Sources". If sign-in fails with a client_secret error, set Authentication to "API Key" and paste an AI key (unconfirmed).',
    check: "The HolyArrs connector shows as connected and lists its tools.",
    secondHand: false,
  },
  {
    family: "chat-apps",
    id: "perplexity-mac-helper",
    name: "Perplexity Mac app (local helper)",
    cloud: false,
    signIn: false,
    key: true,
    where:
      'Mac app > Plugins > Add custom > "Simple" tab (Server Name and Command)',
    signInSnippet: "",
    keySnippet:
      'npx -y mcp-remote {url} --allow-http --transport http-only --header "Authorization: Bearer {key}"',
    steps:
      'Needs the Mac App Store version, Node.js and the "PerplexityXPC" helper. Press "Add custom", set "Server Name" to holyarrs and paste the command into "Command" on one line (leave out --allow-http on https), then "Save". Switch HolyArrs on under "Sources".',
    check: 'The status reads "Running".',
    secondHand: false,
  },
  {
    family: "chat-apps",
    id: "le-chat",
    name: "Mistral Le Chat",
    cloud: true,
    signIn: true,
    key: true,
    where: "Connectors > + Add Connector > Custom MCP Connector",
    signInSnippet: "Connector name: holyarrs\nServer URL:     {url}",
    keySnippet: "",
    steps:
      'Open "Connectors", press "+ Add Connector" and open the "Custom MCP Connector" tab. Set "Connector name" to holyarrs and paste the AI address under "Server URL". Press "Connect", sign in to HolyArrs if asked and press "Allow".',
    check: "The HolyArrs connector shows as connected and lists its tools.",
    secondHand: false,
  },
  {
    family: "chat-apps",
    id: "manus",
    name: "Manus",
    cloud: true,
    signIn: false,
    key: true,
    where:
      "Connectors > Create > add an MCP server by URL (or Settings > Integrations > Custom MCP Servers > Add Server)",
    signInSnippet: "",
    keySnippet: "Server URL: {url}\nToken:      {key}",
    steps:
      'Make an AI key by hand. Open "Connectors", press "Create" and pick the option to add an MCP server by URL. Fill in "Server name" and "Server URL", and enter the AI key under "Authentication" as a bearer token or API key.',
    check: 'Press "Test connection"; Manus should list the HolyArrs tools.',
    secondHand: false,
  },
  {
    family: "chat-apps",
    id: "genspark",
    name: "Genspark",
    cloud: true,
    signIn: false,
    key: false,
    where: "Skills > Connectors > add a custom MCP server",
    signInSnippet: "",
    keySnippet: "",
    steps:
      'Open "Skills", then "Connectors". Add a custom MCP server and paste the AI address as the MCP URL. Finish any sign-in it asks for; whether Genspark can sign in to HolyArrs or send an AI key is unconfirmed.',
    check: "The HolyArrs connector shows as connected and lists its tools.",
    secondHand: true,
  },
  {
    family: "chat-apps",
    id: "huggingchat",
    name: "HuggingChat",
    cloud: true,
    signIn: false,
    key: false,
    where: 'Chat box "+" > MCP Servers (also under Settings) > Add Server',
    signInSnippet: "",
    keySnippet: "",
    steps:
      'In the chat box press "+" and pick "MCP Servers", then "Add Server": type a name and paste the AI address. Switch HolyArrs on in the MCP Servers panel and use a model that supports tools. The form has no key field and HuggingChat sign-in is unconfirmed, so this may not work yet.',
    check: 'Run "Health Check".',
    secondHand: false,
  },
  {
    family: "chat-apps",
    id: "qwen-desktop",
    name: "Qwen Chat desktop app",
    cloud: false,
    signIn: false,
    key: true,
    where: "In a chat, the MCP option > MCP settings > Add MCP > add with JSON",
    signInSnippet: "",
    keySnippet:
      '{\n  "mcpServers": {\n    "holyarrs": {\n      "command": "npx",\n      "args": ["-y", "mcp-remote@latest", "{url}", "--transport", "http-only", "--allow-http", "--header", "Authorization:${HOLYARRS_AUTH}"],\n      "env": { "HOLYARRS_AUTH": "Bearer {key}" }\n    }\n  }\n}',
    steps:
      'Make an AI key by hand; needs Node.js 20 or newer. In a chat open the MCP option to reach "MCP settings", press "Add MCP" and choose to add it with JSON. Paste the config (leave out "--allow-http" on https) and press "Save and Enable".',
    check: "A green mark means it connected.",
    secondHand: true,
  },
  {
    family: "code-editors",
    id: "windsurf",
    name: "Windsurf / Devin Desktop",
    cloud: false,
    signIn: true,
    key: true,
    where:
      "~/.config/devin/mcp_config.json (Windows: %APPDATA%\\devin\\mcp_config.json), reachable from Customize > MCPs",
    signInSnippet:
      "devin mcp add -s user holyarrs {url}\ndevin mcp login holyarrs",
    keySnippet:
      '{\n  "mcpServers": {\n    "holyarrs": {\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
    steps:
      'Open the config file from Customize, then the MCPs tab, and add the entry. On https leave out the headers part: Devin Desktop starts the browser sign-in, or press "Authenticate" next to the server under Customize > MCPs, then "Allow" in HolyArrs. The Devin command-line tool shares the file and can add it on https.',
    check: "holyarrs is listed and switched on under Customize > MCPs.",
    secondHand: false,
  },
  {
    family: "code-editors",
    id: "zed",
    name: "Zed",
    cloud: false,
    signIn: true,
    key: true,
    where:
      "Settings > AI > MCP Servers > Add Server > Add Remote Server, or context_servers in the settings file",
    signInSnippet:
      '{\n  "context_servers": {\n    "holyarrs": {\n      "url": "{url}"\n    }\n  }\n}',
    keySnippet:
      '{\n  "context_servers": {\n    "holyarrs": {\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
    steps:
      'Open Settings > AI > MCP Servers > Add Server > Add Remote Server, or run zed: open settings file and add the context_servers part. On https leave out the headers part; Zed starts the browser sign-in by itself, then press "Allow" in HolyArrs.',
    check: 'The server has a green dot with "Server is active".',
    secondHand: false,
  },
  {
    family: "code-editors",
    id: "jetbrains-ai",
    name: "JetBrains AI Assistant",
    cloud: false,
    signIn: false,
    key: true,
    where:
      "Settings | Tools | AI Assistant | Model Context Protocol (MCP) > Add",
    signInSnippet: "",
    keySnippet:
      '{\n  "mcpServers": {\n    "holyarrs": {\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
    steps:
      "In your JetBrains IDE open Settings | Tools | AI Assistant | Model Context Protocol (MCP) and press Add. Pick the HTTP (Streamable HTTP) connection type, paste the entry, pick the server level, then OK and Apply. Whether it sends the headers part or offers browser sign-in is unconfirmed.",
    check:
      "Look at the Status column next to holyarrs; click the icon there to see the HolyArrs tools.",
    secondHand: false,
  },
  {
    family: "code-editors",
    id: "kiro",
    name: "Kiro",
    cloud: false,
    signIn: true,
    key: true,
    where:
      '~/.kiro/settings/mcp.json ("Kiro: Open user MCP config (JSON)"); a workspace\'s own file is .kiro/settings/mcp.json',
    signInSnippet:
      '{\n  "mcpServers": {\n    "holyarrs": {\n      "url": "{url}"\n    }\n  }\n}',
    keySnippet:
      '{\n  "mcpServers": {\n    "holyarrs": {\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer ${HOLYARRS_AI_KEY}"\n      }\n    }\n  }\n}',
    steps:
      'Kiro needs an https address, or http on localhost. Switch on MCP support in Settings, run "Kiro: Open user MCP config (JSON)" from the Command Palette and add the entry; Kiro starts the browser sign-in by itself, then press "Allow". For a key, add the headers part and approve HOLYARRS_AI_KEY under "Mcp Approved Env Vars".',
    check: "",
    secondHand: false,
  },
  {
    family: "vscode-add-ons",
    id: "cline",
    name: "Cline",
    cloud: false,
    signIn: false,
    key: true,
    where:
      'Cline > MCP Servers icon > Configure tab > "Configure MCP Servers" (cline_mcp_settings.json)',
    signInSnippet: "",
    keySnippet:
      '{\n  "mcpServers": {\n    "holyarrs": {\n      "type": "streamableHttp",\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
    steps:
      'Make an AI key named Cline. In Cline click the MCP Servers icon, open the Configure tab, then "Configure MCP Servers", and add the entry. Keep "type": "streamableHttp".',
    check: "holyarrs shows in Cline's list of MCP servers.",
    secondHand: false,
  },
  {
    family: "vscode-add-ons",
    id: "continue",
    name: "Continue",
    cloud: false,
    signIn: false,
    key: true,
    where:
      '~/.continue/config.yaml (Windows: %USERPROFILE%\\.continue\\config.yaml), via the gear next to "Local Config"',
    signInSnippet: "",
    keySnippet:
      "mcpServers:\n  - name: HolyArrs\n    type: streamable-http\n    url: {url}\n    requestOptions:\n      headers:\n        Authorization: Bearer {key}",
    steps:
      "Make an AI key named Continue. Open the local config file and add the entry under mcpServers (a list, so it starts with a dash); Continue reloads it by itself. Switch the chat to agent mode; whether the key header is sent in your version is unconfirmed.",
    check: "holyarrs shows in its list of MCP servers without an error.",
    secondHand: false,
  },
  {
    family: "vscode-add-ons",
    id: "kilo-code",
    name: "Kilo Code",
    cloud: false,
    signIn: true,
    key: true,
    where:
      "~/.config/kilo/kilo.jsonc (or kilo.json); for one project kilo.jsonc in the root or .kilo/kilo.jsonc",
    signInSnippet:
      '{\n  "mcp": {\n    "holyarrs": {\n      "type": "remote",\n      "url": "{url}"\n    }\n  }\n}',
    keySnippet:
      '{\n  "mcp": {\n    "holyarrs": {\n      "type": "remote",\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
    steps:
      'Make an AI key named Kilo Code, or skip it on https. Add the entry under mcp in the settings file. On https leave out the headers part: click the notice that the server needs sign-in, sign in to HolyArrs and press "Allow".',
    check: "holyarrs shows in its list of MCP servers without an error.",
    secondHand: false,
  },
  {
    family: "terminal-tools",
    id: "opencode",
    name: "OpenCode",
    cloud: false,
    signIn: true,
    key: true,
    where: "~/.config/opencode/opencode.json, or opencode.json in a project",
    signInSnippet:
      '{\n  "mcp": {\n    "holyarrs": {\n      "type": "remote",\n      "url": "{url}"\n    }\n  }\n}',
    keySnippet:
      '{\n  "mcp": {\n    "holyarrs": {\n      "type": "remote",\n      "url": "{url}",\n      "oauth": false,\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
    steps:
      'Add the entry to the config file. On https use it without the oauth and headers lines, then run opencode mcp auth holyarrs and press "Allow" in HolyArrs.',
    check: "opencode mcp list shows holyarrs and its sign-in state.",
    secondHand: false,
  },
  {
    family: "terminal-tools",
    id: "warp",
    name: "Warp",
    cloud: false,
    signIn: true,
    key: true,
    where:
      'Settings > Agents > MCP servers (or "Open MCP Servers" in the Command Palette) > + Add > URL',
    signInSnippet: '{\n  "holyarrs": {\n    "url": "{url}"\n  }\n}',
    keySnippet:
      '{\n  "holyarrs": {\n    "url": "{url}",\n    "headers": {\n      "Authorization": "Bearer {key}"\n    }\n  }\n}',
    steps:
      'Open Settings > Agents > MCP servers, press + Add, pick the URL type and paste the entry. On https leave out the headers part; Warp opens the browser sign-in the first time the server starts, then press "Allow" in HolyArrs.',
    check: "",
    secondHand: false,
  },
  {
    family: "terminal-tools",
    id: "amp",
    name: "Amp",
    cloud: false,
    signIn: true,
    key: true,
    where: "~/.config/amp/settings.json, or .amp/settings.json in a project",
    signInSnippet: "amp mcp add holyarrs {url}",
    keySnippet:
      '{\n  "amp.mcpServers": {\n    "holyarrs": {\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
    steps:
      'Add the "amp.mcpServers" part to the settings file. On https leave out the headers part, or run amp mcp add; Amp opens the browser sign-in when you start it, then press "Allow". A server in a project file needs amp mcp approve holyarrs.',
    check: "",
    secondHand: false,
  },
  {
    family: "terminal-tools",
    id: "pi",
    name: "Pi coding agent",
    cloud: false,
    signIn: true,
    key: true,
    where: "~/.pi/agent/mcp.json, or .pi/mcp.json in a trusted project",
    signInSnippet:
      '{\n  "mcpServers": {\n    "holyarrs": {\n      "url": "{url}"\n    }\n  }\n}',
    keySnippet:
      '{\n  "mcpServers": {\n    "holyarrs": {\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
    steps:
      'Needs Pi 0.99.0 or newer. Put the entry in the file, or set HOLYARRS_AI_KEY and run pi mcp add holyarrs with --bearer-token-env-var HOLYARRS_AI_KEY. On https use the entry without headers, then run pi mcp login holyarrs and press "Allow".',
    check: "Type /mcp inside Pi.",
    secondHand: false,
  },
  {
    family: "terminal-tools",
    id: "mistral-vibe",
    name: "Mistral Vibe CLI",
    cloud: false,
    signIn: true,
    key: true,
    where: "~/.vibe/config.toml",
    signInSnippet: "vibe mcp add holyarrs --url {url}",
    keySnippet:
      '[[mcp_servers]]\nname = "holyarrs"\ntransport = "http"\nurl = "{url}"\n\n[mcp_servers.auth]\ntype = "static"\napi_key_env = "HOLYARRS_AI_KEY"\napi_key_header = "Authorization"\napi_key_format = "Bearer {token}"',
    steps:
      'On plain http set HOLYARRS_AI_KEY to your key and add the block to config.toml. On https run vibe mcp add; Vibe should start the browser sign-in, or run /mcp login holyarrs inside Vibe, then press "Allow".',
    check:
      "/mcp lists the servers, and /mcp holyarrs lists the HolyArrs tools.",
    secondHand: false,
  },
  {
    family: "terminal-tools",
    id: "kimi-code",
    name: "Kimi Code CLI",
    cloud: false,
    signIn: true,
    key: true,
    where:
      "~/.kimi-code/mcp.json, or .kimi-code/mcp.json in a trusted project (or /mcp-config inside the tool)",
    signInSnippet:
      '{\n  "mcpServers": {\n    "holyarrs": {\n      "url": "{url}"\n    }\n  }\n}',
    keySnippet:
      '{\n  "mcpServers": {\n    "holyarrs": {\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
    steps:
      'Add the entry to the file. On https leave out the headers part, then run /mcp-config login holyarrs inside the tool and press "Allow" in HolyArrs.',
    check: "/mcp shows each server's connection state.",
    secondHand: false,
  },
  {
    family: "terminal-tools",
    id: "devin",
    name: "Devin (cloud)",
    cloud: true,
    signIn: true,
    key: true,
    where: "Devin > Customize > MCPs > Add MCP > Add custom MCP",
    signInSnippet: "",
    keySnippet: "",
    steps:
      'In Customize > MCPs press Add MCP, then Add custom MCP: Server Name HolyArrs, transport HTTP, Server URL your public https AI address. For OAuth save, press Connect and "Allow" in HolyArrs (pick Personal under Access); for Auth Header keep the name Authorization and set the value to Bearer <your AI key>.',
    check: "Press Save, then Test tools; it lists the HolyArrs tools.",
    secondHand: false,
  },
  {
    family: "self-hosted-chat",
    id: "open-webui",
    name: "Open WebUI",
    cloud: false,
    signIn: true,
    key: true,
    where:
      "Admin Panel > Settings > Integrations > External Tool Servers > + Add Connection",
    signInSnippet: "",
    keySnippet: "",
    steps:
      'As an Open WebUI admin, add a connection with Type "MCP (Streamable HTTP)", the AI address as Server URL, Auth "Bearer" and the AI key (without the word Bearer) as Key. On https you can instead pick the self-registering "OAuth 2.1" Auth and leave Key empty; set a fixed WEBUI_SECRET_KEY. Turn the HolyArrs tools on per chat.',
    check:
      "Open a new chat and its tools menu; the HolyArrs tools should be listed.",
    secondHand: false,
  },
  {
    family: "self-hosted-chat",
    id: "librechat",
    name: "LibreChat",
    cloud: false,
    signIn: true,
    key: true,
    where:
      "librechat.yaml (mcpServers, plus mcpSettings.allowedAddresses for a home address), or MCP Settings > Add MCP in the side panel",
    signInSnippet: "",
    keySnippet:
      'mcpServers:\n  holyarrs:\n    type: streamable-http\n    url: {url}\n    requiresOAuth: false\n    headers:\n      Authorization: "Bearer {{HOLYARRS_AI_KEY}}"\n    customUserVars:\n      HOLYARRS_AI_KEY:\n        title: "HolyArrs AI key"\n        description: "Make one in HolyArrs under Settings > AI connections"',
    steps:
      'On a home address the admin first adds the host and port under mcpSettings > allowedAddresses. Add the entry to librechat.yaml (each person types their own key) and restart, or use "Add MCP" with Transport Streamable HTTP and Authentication "API Key" with "Header Format" "Bearer". On https pick "OAuth" with Client ID and Secret empty, then press "Allow".',
    check:
      'HolyArrs is listed under "MCP Servers" in the "+" menu, and its tools show in the agent "Tool Library".',
    secondHand: false,
  },
  {
    family: "self-hosted-chat",
    id: "lobehub",
    name: "LobeHub",
    cloud: false,
    signIn: false,
    key: true,
    where: "Settings > Connectors > add an MCP server by hand (HTTP)",
    signInSnippet: "",
    keySnippet: "Authorization: Bearer {key}",
    steps:
      "Use the desktop app on a home address. Open Settings > Connectors, add an MCP server by hand, choose HTTP and enter the AI address; put the key in a Bearer API key field or add the header. Save, then turn it on for an agent under Agent Settings > Connectors.",
    check: "Settings > Connectors shows HolyArrs as connected.",
    secondHand: false,
  },
  {
    family: "desktop-apps",
    id: "lm-studio",
    name: "LM Studio",
    cloud: false,
    signIn: false,
    key: true,
    where:
      '%USERPROFILE%\\.lmstudio\\mcp.json (Windows) or ~/.lmstudio/mcp.json, via the "Program" tab > "Install" > "Edit mcp.json"',
    signInSnippet: "",
    keySnippet:
      '{\n  "mcpServers": {\n    "holyarrs": {\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
    steps:
      'Needs LM Studio 0.3.17 or newer. In the right sidebar\'s "Program" tab pick "Install", then "Edit mcp.json", add the entry and save. Use a key on https and localhost too.',
    check: "The HolyArrs tools are listed with the server.",
    secondHand: false,
  },
  {
    family: "desktop-apps",
    id: "jan",
    name: "Jan",
    cloud: false,
    signIn: true,
    key: true,
    where:
      'Settings > "MCP Servers" > "+ Add MCP Server" (newer versions: "Connectors")',
    signInSnippet: "",
    keySnippet:
      "-y\nmcp-remote@latest\n{url}\n--transport\nhttp-only\n--allow-http\n--header\nAuthorization:${AUTH_HEADER}",
    steps:
      'On https or localhost set Transport to "HTTP", paste the AI address and press "Sign in" on the server\'s Auth line, then "Allow". On plain http use the local bridge (needs Node.js): Transport "STDIO", Command npx, the Args one per line, and Env AUTH_HEADER set to Bearer <your AI key>.',
    check: "The HolyArrs tools are listed with the server.",
    secondHand: false,
  },
  {
    family: "desktop-apps",
    id: "anythingllm",
    name: "AnythingLLM",
    cloud: false,
    signIn: false,
    key: true,
    where: "plugins/anythingllm_mcp_servers.json in its storage folder",
    signInSnippet: "",
    keySnippet:
      '{\n  "mcpServers": {\n    "holyarrs": {\n      "type": "streamable",\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
    steps:
      'Needs Desktop v1.8.0 or newer. Open "Agent Skills" once so the file exists, add the entry, then open "Agent Skills" again to load it. In a chat start your message with @agent.',
    check: 'The server\'s status shows under "Agent Skills".',
    secondHand: false,
  },
  {
    family: "desktop-apps",
    id: "cherry-studio",
    name: "Cherry Studio",
    cloud: false,
    signIn: false,
    key: true,
    where: 'Settings > MCP > MCP Servers > Add > "Import from JSON"',
    signInSnippet: "",
    keySnippet:
      '{\n  "mcpServers": {\n    "holyarrs": {\n      "type": "streamableHttp",\n      "url": "{url}",\n      "headers": {\n        "Authorization": "Bearer {key}"\n      }\n    }\n  }\n}',
    steps:
      'Open Settings > MCP > MCP Servers, press Add and choose "Import from JSON". Paste the entry and save. Switch HolyArrs on in the MCP settings of the agent you chat with.',
    check: "The server shows as running.",
    secondHand: true,
  },
  {
    family: "desktop-apps",
    id: "msty-studio",
    name: "Msty Studio",
    cloud: false,
    signIn: false,
    key: true,
    where: 'Toolbox > Tools > "Add New Tool" (type HTTP)',
    signInSnippet: "",
    keySnippet: "",
    steps:
      'Open Toolbox > Tools, press "Add New Tool" and pick the type "HTTP". Paste the AI address in "MCP Server URL" and in "Headers" add Authorization with the value Bearer <your AI key>. Name it HolyArrs and press "Add".',
    check: 'Try it in the "Tool Console".',
    secondHand: false,
  },
  {
    family: "desktop-apps",
    id: "5ire",
    name: "5ire",
    cloud: false,
    signIn: false,
    key: true,
    where:
      "Tools > New (second-hand), with command, arguments and environment fields",
    signInSnippet: "",
    keySnippet:
      '{\n  "mcpServers": {\n    "holyarrs": {\n      "command": "npx",\n      "args": [\n        "-y",\n        "mcp-remote@latest",\n        "{url}",\n        "--transport",\n        "http-only",\n        "--allow-http",\n        "--header",\n        "Authorization:${AUTH_HEADER}"\n      ],\n      "env": {\n        "AUTH_HEADER": "Bearer {key}"\n      }\n    }\n  }\n}',
    steps:
      "Needs v0.11.0 or newer and Node.js. Add a new tool and fill in the command, arguments and environment variables from the local bridge entry. Switch the tool on.",
    check: "",
    secondHand: true,
  },
  {
    family: "local-agents",
    id: "hermes-agent",
    name: "Hermes Agent",
    cloud: false,
    signIn: true,
    key: true,
    where: "~/.hermes/config.yaml (mcp_servers)",
    signInSnippet:
      'mcp_servers:\n  holyarrs:\n    url: "{url}"\n    auth: oauth',
    keySnippet:
      'mcp_servers:\n  holyarrs:\n    url: "{url}"\n    headers:\n      Authorization: "Bearer {key}"',
    steps:
      'Add the entry to ~/.hermes/config.yaml, then type /reload-mcp or start a new session. With sign-in, Hermes opens HolyArrs in your browser on first connect (or run hermes mcp login holyarrs); press "Allow".',
    check: "Run hermes mcp test holyarrs.",
    secondHand: false,
  },
  {
    family: "local-agents",
    id: "goose",
    name: "Goose",
    cloud: false,
    signIn: true,
    key: true,
    where:
      "goose configure > Add Extension > Remote Extension (Streamable HTTP); desktop: Extensions > Add custom extension; file: ~/.config/goose/config.yaml (Windows: %APPDATA%\\Block\\goose\\config\\config.yaml)",
    signInSnippet:
      'extensions:\n  holyarrs:\n    name: HolyArrs\n    type: streamable_http\n    uri: "{url}"\n    enabled: true\n    timeout: 300',
    keySnippet:
      'extensions:\n  holyarrs:\n    name: HolyArrs\n    type: streamable_http\n    uri: "{url}"\n    enabled: true\n    timeout: 300\n    headers:\n      Authorization: "Bearer {key}"',
    steps:
      'Run goose configure, pick "Add Extension", then "Remote Extension (Streamable HTTP)", and give the AI address as URI. For custom headers, on plain http say yes and add Authorization with Bearer <your AI key>; on https or localhost say no and Goose starts the browser sign-in.',
    check: "The extension list in Goose shows HolyArrs.",
    secondHand: false,
  },
  {
    family: "local-agents",
    id: "ollmcp",
    name: "ollmcp",
    cloud: false,
    signIn: false,
    key: true,
    where: "holyarrs.json passed with -j, or saved once with ollmcp mcp add",
    signInSnippet: "",
    keySnippet:
      'ollmcp mcp add --transport http holyarrs {url} --header "Authorization: Bearer {key}"',
    steps:
      "Needs Python 3.11 or newer; install with uv tool install --upgrade ollmcp. Save the server with the command, or save the entry as holyarrs.json and start with ollmcp -j holyarrs.json -m <model>.",
    check: "Type /tools in the chat.",
    secondHand: false,
  },
  {
    family: "local-runners",
    id: "llama-cpp",
    name: "llama.cpp web UI",
    cloud: false,
    signIn: false,
    key: true,
    where:
      "mcp.json in the folder you start llama-server from, loaded with --mcp-servers-config mcp.json",
    signInSnippet: "",
    keySnippet:
      '{\n  "mcpServers": {\n    "holyarrs": {\n      "command": "npx",\n      "args": [\n        "-y",\n        "mcp-remote@latest",\n        "{url}",\n        "--transport",\n        "http-only",\n        "--allow-http",\n        "--header",\n        "Authorization:${AUTH_HEADER}"\n      ],\n      "env": {\n        "AUTH_HEADER": "Bearer {key}"\n      }\n    }\n  }\n}',
    steps:
      "Needs Node.js for the mcp-remote bridge. Save the entry as mcp.json, start llama-server -m <your model>.gguf --mcp-servers-config mcp.json, and open the web UI. This is experimental in llama.cpp.",
    check: "The HolyArrs tools show in the llama.cpp web UI.",
    secondHand: false,
  },
  {
    family: "local-runners",
    id: "localai",
    name: "LocalAI",
    cloud: false,
    signIn: false,
    key: true,
    where:
      'The model\'s YAML config (mcp block), or "Remote MCP Servers" in the model editor',
    signInSnippet: "",
    keySnippet:
      'mcp:\n  remote: |\n    {\n      "mcpServers": {\n        "holyarrs": {\n          "url": "{url}",\n          "token": "{key}"\n        }\n      }\n    }',
    steps:
      'Add the mcp block to the model\'s YAML config, or paste the inner { "mcpServers": ... } part into "Remote MCP Servers" in the model editor. Reload the model. In Docker use HolyArrs\'s network address.',
    check:
      "In LocalAI's chat, the MCP > Servers tab lists the model's servers and any errors.",
    secondHand: false,
  },
  {
    family: "local-runners",
    id: "text-generation-webui",
    name: "text-generation-webui",
    cloud: false,
    signIn: false,
    key: true,
    where: 'Chat sidebar > "MCP servers"',
    signInSnippet: "",
    keySnippet: "{url},Authorization: Bearer {key}",
    steps:
      'Load a model with tool-call support in the Model tab. In the chat sidebar open "MCP servers" and add one line: the address, a comma, then the header. Chat as usual; tools are offered to the model by themselves.',
    check: "",
    secondHand: false,
  },
  {
    family: "local-runners",
    id: "koboldcpp",
    name: "KoboldCpp",
    cloud: false,
    signIn: false,
    key: true,
    where:
      "mcp.json, loaded with koboldcpp --mcpfile mcp.json or in the launcher",
    signInSnippet: "",
    keySnippet:
      '{\n  "mcpServers": {\n    "holyarrs": {\n      "command": "npx",\n      "args": [\n        "-y",\n        "mcp-remote@latest",\n        "{url}",\n        "--transport",\n        "http-only",\n        "--allow-http",\n        "--header",\n        "Authorization:${AUTH_HEADER}"\n      ],\n      "env": {\n        "AUTH_HEADER": "Bearer {key}"\n      }\n    }\n  }\n}',
    steps:
      'Needs version 1.106 or newer and Node.js. Save the entry as mcp.json and start koboldcpp --model <your model>.gguf --mcpfile mcp.json. In KoboldAI Lite turn on tool calling, fetch the tools, and turn on "Manually Approve Tools".',
    check: "Watch KoboldCpp as it starts for MCP errors.",
    secondHand: false,
  },
  {
    family: "local-runners",
    id: "page-assist",
    name: "Page Assist (browser extension)",
    cloud: false,
    signIn: true,
    key: true,
    where: "Page Assist's MCP settings page",
    signInSnippet: "",
    keySnippet: "",
    steps:
      'Page Assist\'s MCP settings take a server address with a Bearer token or OAuth 2.1; follow Page Assist\'s own MCP guide. It calls from inside your browser, so HolyArrs may refuse it with "request from another site refused" (unconfirmed). If it works, turn on "Require approval before running MCP tools".',
    check: "",
    secondHand: false,
  },
];
