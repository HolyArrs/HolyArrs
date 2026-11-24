import { describe, expect, it } from 'vitest';
import { askKinds, showJoinedWelcome } from './joined-welcome.ts';

describe('welcome for a person who joined', () => {
	const base = { role: 'member', setupOpen: false, seen: undefined as unknown };
	it.each([
		['a new member', base, true],
		['an admin who joined', { ...base, role: 'admin' }, true],
		['the owner (owner setup has its own steps)', { ...base, role: 'owner' }, false],
		['setup still open', { ...base, setupOpen: true }, false],
		['already said got it', { ...base, seen: '1' }, false],
		['a saved value from elsewhere is not "seen"', { ...base, seen: 'x' }, true],
		['no role yet (signed out)', { ...base, role: '' }, false]
	])('%s', (_n, input, want) => {
		expect(showJoinedWelcome(input)).toBe(want);
	});

	it.each([
		['all parts on', { canAsk: true, off: [] as string[] }, ['movies', 'shows', 'music']],
		['music part off', { canAsk: true, off: ['music'] }, ['movies', 'shows']],
		['only shows on', { canAsk: true, off: ['movies', 'music'] }, ['shows']],
		['may not ask', { canAsk: false, off: [] }, []]
	])('asks: %s', (_n, i, want) => {
		expect(askKinds(i.canAsk, (p) => i.off.includes(p))).toEqual(want);
	});
});
