// My account: which actions must confirm the local sign-in first. The
// confirmation shows inline when such an action starts (Confirm.svelte),
// never as a permanent card.

export type Sensitive = 'password' | 'link' | 'two-factor';

/**
 * needsConfirm says whether an action must first confirm the local sign-in
 * (the HolyArrs password, plus a code when two-factor is on). Password and
 * linking need a recent local sign-in; two-factor changes need one too,
 * except while an administrator must still enroll (the server lets setup
 * through then), and a local-network visit (no password) always asks.
 */
export function needsConfirm(action: Sensitive, s: { localRecent: boolean; via: string; hasPassword?: boolean; mustTwoFactor?: boolean }): boolean {
	if (action === 'two-factor') return s.via === 'local_network' || (s.via === 'session' && !s.localRecent && !s.mustTwoFactor);
	// No password yet means nothing to confirm with: a fresh media server
	// sign-in is the proof for a first password (My account says so).
	if (action === 'password' && s.hasPassword === false) return false;
	return !s.localRecent;
}
