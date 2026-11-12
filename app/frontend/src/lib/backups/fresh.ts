// Downloads of a backup or the key kit need a fresh password sign-in. An owner who signed in another way and never set a password can't confirm one, so they are told to set one first.
export function freshAsk(s: { via: string; localRecent: boolean; hasPassword: boolean }): null | 'set' | 'confirm' {
	if (s.via !== 'session' || s.localRecent) return null;
	return s.hasPassword ? 'confirm' : 'set';
}
