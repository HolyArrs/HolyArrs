// AI connections (Settings > AI connections): which keys are AI keys and
// the short setup text for Claude Desktop and Claude Code.
import type { ApiKey } from './types.ts';

/** The scope an AI key holds, and nothing else. */
export const AI_SCOPE = 'mcp';

/** isAIKey: the key reaches only the AI route. */
export function isAIKey(k: ApiKey): boolean {
	return (k.scopes ?? []).includes(AI_SCOPE);
}

/** aiURL is the address an AI app connects to, from the page's own origin and base path. */
export function aiURL(origin: string, base = ''): string {
	return origin.replace(/\/+$/, '') + base.replace(/\/+$/, '') + '/api/v1/mcp';
}

/** claudeCodeCommand adds HolyArrs to Claude Code. */
export function claudeCodeCommand(url: string, key = '<your AI key>'): string {
	return `claude mcp add --transport http holyarrs ${url} --header "Authorization: Bearer ${key}"`;
}

/** claudeDesktopConfig is the claude_desktop_config.json entry (through the mcp-remote helper). */
export function claudeDesktopConfig(url: string, key = '<your AI key>'): string {
	const cfg = {
		mcpServers: {
			holyarrs: { command: 'npx', args: ['-y', 'mcp-remote', url, '--header', `Authorization: Bearer ${key}`] }
		}
	};
	return JSON.stringify(cfg, null, 2);
}
