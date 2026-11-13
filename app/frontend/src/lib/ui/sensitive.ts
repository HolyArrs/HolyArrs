// Decides when an account action must ask the user to confirm first.

export type Sensitive = 'password' | 'link' | 'two-factor';

// Two-factor changes are stricter; a password with none set has nothing to confirm.
export function needsConfirm(action: Sensitive, s: { localRecent: boolean; via: string; hasPassword?: boolean; mustTwoFactor?: boolean }): boolean {
	if (action === 'two-factor') return s.via === 'local_network' || (s.via === 'session' && !s.localRecent && !s.mustTwoFactor);
	if (action === 'password' && s.hasPassword === false) return false;
	return !s.localRecent;
}

// A visit let in by the local-network skip is no sign-in, so a password box here can never confirm it. Gives the sign-in page link that comes back to `back` (an app path), or '' when this visit can confirm in place.
export function signInFirst(s: { via: string }, back: string): string {
	return s.via === 'local_network' ? `/login?next=${encodeURIComponent(back)}` : '';
}
