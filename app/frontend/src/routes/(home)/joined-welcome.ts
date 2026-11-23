// The first-visit welcome for a person who joined: shown on Home until they close it, remembered on their account. The owner gets the setup steps instead.

export type AskKind = 'movies' | 'shows' | 'music';

const kinds: AskKind[] = ['movies', 'shows', 'music'];

/** Whether Home shows the welcome: anyone but the owner, once setup is done, until they close it. */
export function showJoinedWelcome(i: { role: string; setupOpen: boolean; seen: unknown }): boolean {
	return !!i.role && i.role !== 'owner' && !i.setupOpen && i.seen !== '1';
}

/** The kinds this person can ask for: none without asking, and only parts that are on. */
export function askKinds(canAsk: boolean, partOff: (part: AskKind) => boolean): AskKind[] {
	return canAsk ? kinds.filter((k) => !partOff(k)) : [];
}
