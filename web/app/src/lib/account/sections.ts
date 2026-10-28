// My account: which actions must confirm the local sign-in first. The
// confirmation shows inline when such an action starts (Confirm.svelte),
// never as a permanent card.

export type Sensitive = 'password' | 'link' | 'two-factor';

/**
 * needsConfirm says whether an action must first confirm the local sign-in
 * (the HolyArrs password, plus a code when two-factor is on). Password and
 * linking need a recent local sign-in; two-factor changes need a password
 * session, so only a local-network visit (no password) asks first.
 */
export function needsConfirm(action: Sensitive, s: { localRecent: boolean; via: string }): boolean {
	if (action === 'two-factor') return s.via === 'local_network';
	return !s.localRecent;
}
