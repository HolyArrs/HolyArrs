import { describe, expect, it } from 'vitest';
import { addSuggestions, appName, cleanRules, linesByGroup, notMovedByGroup, ruleProblem, running, sameName, setChoice, summary, tone, type Report } from './move.ts';

describe('move helpers', () => {
	it('names apps', () => {
		expect(appName('qbittorrent')).toBe('qBittorrent');
		expect(appName('other')).toBe('other');
	});

	it('checks path rules', () => {
		expect(ruleProblem({ from: '/movies', to: '/data/movies' })).toBe('');
		expect(ruleProblem({ from: 'D:\\TV', to: '/tv' })).toBe('');
		expect(ruleProblem({ from: '/movies', to: '' })).toMatch(/both/);
		expect(ruleProblem({ from: 'movies', to: '/x' })).toMatch(/full paths/);
		expect(ruleProblem({ from: '/a/../b', to: '/x' })).toMatch(/\.\./);
	});

	it('keeps rules tidy and adds suggestions once', () => {
		expect(cleanRules([{ from: ' /a ', to: '/b' }, { from: '', to: '' }])).toEqual([{ from: '/a', to: '/b' }]);
		const r = addSuggestions([{ from: '/a', to: '/b' }], [{ from: '/a', to: '/z' }, { from: '/c', to: '/d' }]);
		expect(r).toEqual([
			{ from: '/a', to: '/b' },
			{ from: '/c', to: '/d' }
		]);
	});

	it('groups report lines and sums them', () => {
		const rep: Report = {
			dry: true,
			apps: ['radarr'],
			at: '',
			counts: { create: 2, already: 1, failed: 0 },
			not_moved: [],
			needs_input: [],
			next: [],
			lines: [
				{ group: 'titles', key: 'a', name: 'A', result: 'adopt' },
				{ group: 'sources', key: 'b', name: 'B', result: 'create' },
				{ group: 'titles', key: 'c', name: 'C', result: 'already' }
			]
		};
		const g = linesByGroup(rep);
		expect(g.map((x) => x.group)).toEqual(['titles', 'sources']);
		expect(g[0].counts).toEqual({ adopt: 1, already: 1 });
		expect(summary(rep.counts)).toBe('2 will be added, 1 already here');
		expect(tone('failed')).toBe('bad');
		expect(tone('not_found')).toBe('warn');
		expect(tone('adopted')).toBe('ok');
	});

	it('groups the not-moved list by label', () => {
		const g = notMovedByGroup(
			[
				{ group: 'titles', from: 'x', what: 'a', why: 'b' },
				{ group: 'nope', from: 'y', what: 'c', why: 'd' }
			],
			{ titles: 'Titles' }
		);
		expect(g.map((x) => x.label)).toEqual(['Titles', 'Other settings']);
	});

	it('knows a running undo, a same-name person and per-person choices', () => {
		const m = { id: 1, created_at: '', snapshots: [], state: 'done' as const };
		expect(running(m)).toBe(false);
		expect(running({ ...m, undo_state: 'running' })).toBe(true);
		expect(running({ ...m, undo_state: 'failed' })).toBe(false);
		expect(running({ ...m, state: 'queued' })).toBe(true);
		expect(sameName({ group: 'people', key: 'p', name: 'Ana', result: 'skipped', detail: 'a person with the same name exists here' })).toBe(true);
		expect(sameName({ group: 'people', key: 'p', name: 'Ana', result: 'skipped', detail: 'you left them out' })).toBe(false);
		const c = setChoice({}, 'person:seerr:2', 'merge');
		expect(c).toEqual({ 'person:seerr:2': 'merge' });
		expect(setChoice(c, 'person:seerr:2', null)).toEqual({});
	});
});
