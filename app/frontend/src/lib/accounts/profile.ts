// My profile: the display name rules, shared with the server.

export const DISPLAY_NAME_MAX = 64;

/** Trims what was typed; the server trims the same way. */
export function cleanDisplayName(typed: string): string {
	return typed.trim();
}

/** Why a typed name cannot be saved, or '' when it can. */
export function displayNameError(typed: string): string {
	const s = cleanDisplayName(typed);
	if ([...s].length > DISPLAY_NAME_MAX) return `Use at most ${DISPLAY_NAME_MAX} characters.`;
	// eslint-disable-next-line no-control-regex
	if (/[\u0000-\u001f\u007f-\u009f]/.test(s)) return 'Use plain text only.';
	return '';
}

/** The display name when set, else the username. */
export function shownName(displayName: string, username: string): string {
	return cleanDisplayName(displayName) || username;
}

export const ROLE_WORD: Record<string, string> = { owner: 'Owner', admin: 'Admin', member: 'Member' };
export const roleWord = (role: string): string => ROLE_WORD[role] ?? role;
