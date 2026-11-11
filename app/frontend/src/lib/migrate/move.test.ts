import { describe, expect, it } from 'vitest';
import { appName, hostOf, instanceKey, latestPerInstance, asStep, buckets, chooseHint, cleanPath, cleanRules, countWord, countWords, extraProblems, folderProblem, moveCounts, moveStateWord, moveTotal, needsGroups, OTHER, placeholderFor, resultWords, ruleProblem, rulesFrom, running, setChoice, sharedTargets, stepGate, summary, tone, type Report } from './move.ts';

describe('move helpers', () => {
	it('names apps, with what they bring and their usual port', () => {
		expect(appName('qbittorrent')).toBe('qBittorrent');
		expect(appName('other')).toBe('other');
		expect(placeholderFor('radarr')).toMatch(/:7878$/);
		expect(placeholderFor('sonarr')).toMatch(/:8989$/);
		expect(placeholderFor('lidarr')).toMatch(/:8686$/);
		expect(placeholderFor('prowlarr')).toMatch(/:9696$/);
		expect(placeholderFor('bazarr')).toMatch(/:6767$/);
		expect(placeholderFor('overseerr')).toMatch(/:5055$/);
		expect(placeholderFor('qbittorrent')).toMatch(/:8080$/);
	});

	it('checks path rules', () => {
		expect(ruleProblem({ from: '/movies', to: '/data/movies' })).toBe('');
		expect(ruleProblem({ from: 'D:\\TV', to: '/tv' })).toBe('');
		expect(ruleProblem({ from: '/movies', to: '' })).toMatch(/both/);
		expect(ruleProblem({ from: 'movies', to: '/x' })).toMatch(/full paths/);
		expect(ruleProblem({ from: '/a/../b', to: '/x' })).toMatch(/\.\./);
		expect(cleanRules([{ from: ' /a ', to: '/b' }, { from: '', to: '' }])).toEqual([{ from: '/a', to: '/b' }]);
	});

	it('keys folder rows by old path and checks free rules row by row', () => {
		expect(extraProblems([{ from: '', to: '' }, { from: '/a', to: '' }])).toEqual(['', 'Fill in both paths.']);
		expect(folderProblem('/old/movies', '', '')).toBe('');
		expect(folderProblem('/old/movies', '/data/movies', '')).toBe('');
		expect(folderProblem('/old/movies', OTHER, '')).toMatch(/Type the path/);
		expect(folderProblem('/old/movies', OTHER, 'movies')).toMatch(/full paths/);
		const rules = rulesFrom({ '/old/movies': '/data/movies', '/old/tv': '', '/old/x': OTHER }, { '/old/x': ' /mnt/x ' }, [
			{ from: '/old/movies', to: '/ignored' },
			{ from: ' /c ', to: '/d' },
			{ from: '', to: '' }
		]);
		expect(rules).toEqual([
			{ from: '/old/movies', to: '/data/movies' },
			{ from: '/old/x', to: '/mnt/x' },
			{ from: '/c', to: '/d' }
		]);
	});

	it('says results in plain words, before and after the move', () => {
		expect(resultWords('create')).toBe('Will be added');
		expect(resultWords('create', true)).toBe('Added');
		expect(resultWords('created')).toBe('Added');
		expect(resultWords('adopt')).toBe('Will use the one here');
		expect(resultWords('adopted', true)).toBe('Using the one here');
		expect(resultWords('change')).toBe('Will be updated');
		expect(resultWords('matched', true)).toBe('Matched');
		expect(resultWords('not_found')).toBe('No folder here');
		expect(resultWords('failed')).toBe("Won't work");
		expect(resultWords('failed', true)).toBe('Failed');
		expect(resultWords('failed', true, true)).toBe('Not removed');
		expect(resultWords('removed', true, true)).toBe('Removed');
		expect(resultWords('kept', true, true)).toBe('Kept');
		expect(resultWords('odd_new')).toBe('odd new');
		expect(tone('failed')).toBe('bad');
		expect(tone('not_found')).toBe('warn');
		expect(tone('adopted')).toBe('ok');
		expect(tone('removed')).toBe('ok');
		expect(tone('kept')).toBe('warn');
		expect(tone('skipped')).toBe('quiet');
		expect(summary({ create: 2, already: 1, failed: 0 })).toBe('2 will be added · 1 already here');
		expect(summary({ created: 840, not_found: 4, failed: 2 }, true)).toBe('840 added · 4 no folder here · 2 failed');
		expect(moveTotal({ create: 3, adopt: 1, already: 5, skipped: 2 })).toBe(4);
	});

	it('sorts a report into buckets in one pass, with the people who need a choice', () => {
		const rep: Report = {
			dry: true,
			apps: ['radarr'],
			at: '',
			counts: {},
			not_moved: [],
			needs_input: [],
			next: [],
			lines: [
				{ group: 'titles', key: 'a', name: 'A', result: 'adopt' },
				{ group: 'people', key: 'p', name: 'Pia', result: 'skipped', choose: 'numeric_id' },
				{ group: 'titles', key: 'c', name: 'C', result: 'already' }
			]
		};
		const b = buckets(rep);
		expect(b.groups.map((x) => x.group)).toEqual(['titles', 'people']);
		expect(b.groups[0].label).toBe('Titles');
		expect(b.groups[0].counts).toEqual({ adopt: 1, already: 1 });
		expect(b.groups[0].lines.map((l) => l.key)).toEqual(['a', 'c']);
		expect(b.choose.map((l) => l.key)).toEqual(['p']);
		expect(b.total).toBe(3);
		expect(buckets(null).groups).toEqual([]);
		expect(chooseHint('numeric_id')).toMatch(/can't tell/);
		expect(chooseHint('merge_gone')).toMatch(/no longer here/);
		expect(chooseHint('same_name')).toMatch(/same name/);
	});

	it('groups keys to type again by what comes before the first quote or colon', () => {
		const g = needsGroups(['radarr: indexer "Old": api key', 'Sources "Old": your key', 'radarr: notification "Disc": url', 'just words']);
		expect(g.map((x) => x.label)).toEqual(['Radarr', 'Sources', 'Other']);
		expect(g[0].items).toEqual(['indexer "Old": api key', 'notification "Disc": url']);
		expect(g[1].items).toEqual(['"Old": your key']);
		expect(g[2].items).toEqual(['just words']);
	});

	it('says read counts in our words: no raw keys, no zeros, right plurals', () => {
		expect(countWord('download_clients', 1)).toBe('1 Downloader');
		expect(countWord('indexers', 4)).toBe('4 Sources');
		expect(countWord('quality_profiles', 2)).toBe('2 sets of Pick rules');
		expect(countWord('needs_reentry', 3)).toBe('3 keys to type again');
		expect(countWord('tags', 0)).toBe('');
		expect(countWord('mystery_thing', 5)).toBe('');
		const w = countWords('radarr', { titles: 812, indexers: 4, download_clients: 1, tags: 0, custom_formats: 0, mystery: 9 });
		expect(w).toBe('812 titles, 4 Sources, 1 Downloader');
		expect(w).not.toMatch(/_|0 |mystery/);
		expect(countWords('seerr', { users: 2, requests: 40, tags: 3, notifications: 1, app_links: 2 })).toBe('2 people, 40 requests, 3 tags and 3 more');
		expect(countWords('radarr', {})).toBe('');
	});

	it('says move states in plain words', () => {
		const m = { state: 'done' as const };
		expect(moveStateWord({ state: 'queued' })).toEqual({ word: 'Waiting', tone: 'wait' });
		expect(moveStateWord({ state: 'running' })).toEqual({ word: 'Importing', tone: 'info' });
		expect(moveStateWord(m).word).toBe('Imported');
		expect(moveStateWord({ state: 'failed' }).word).toBe('Stopped with errors');
		expect(moveStateWord({ state: 'undone' }).word).toBe('Undone');
		expect(moveStateWord({ state: 'done', undo_state: 'running' }).word).toBe('Undoing');
		expect(moveStateWord({ state: 'done', undo_state: 'failed' }).word).toBe('Undo failed');
		expect(moveCounts({ state: 'undone', report: { dry: false, apps: [], at: '', lines: [], counts: { removed: 836, kept: 4 }, not_moved: [], needs_input: [], next: [], undo: true } })).toBe(
			'836 removed · 4 kept'
		);
	});

	it('gates each step', () => {
		expect(stepGate({ step: 'read', picked: 0 })).toEqual({ ok: false, reason: 'Turn on at least one read.' });
		expect(stepGate({ step: 'read', picked: 1 }).ok).toBe(true);
		expect(stepGate({ step: 'choose', picked: 1, planning: true, planned: true })).toEqual({ ok: false, reason: 'Updating…' });
		expect(stepGate({ step: 'choose', picked: 1, planned: true, folderErrors: 1, groupsOn: 2, groupsWithItems: 2 }).reason).toBe('Fix the folder marked above.');
		expect(stepGate({ step: 'choose', picked: 1, planned: true, ruleErrors: 1, groupsOn: 2, groupsWithItems: 2 }).reason).toBe('Fix the path rule marked above.');
		expect(stepGate({ step: 'choose', picked: 1, planned: true, groupsOn: 0, groupsWithItems: 2 }).reason).toBe('Turn on at least one group.');
		expect(stepGate({ step: 'choose', picked: 1, planned: true, groupsOn: 0, groupsWithItems: 0 }).ok).toBe(false);
		expect(stepGate({ step: 'choose', picked: 1, planned: true, groupsOn: 1, groupsWithItems: 1 }).ok).toBe(true);
		expect(stepGate({ step: 'move', picked: 1, tried: true, stale: true }).ok).toBe(false);
		expect(stepGate({ step: 'move', picked: 1, tried: true, stale: true }).ok).toBe(false);
		expect(stepGate({ step: 'move', picked: 1, tried: false }).ok).toBe(false);
		expect(stepGate({ step: 'move', picked: 1, tried: true, trying: true }).reason).toBe('Updating…');
		expect(stepGate({ step: 'move', picked: 1, tried: true, openChoices: 3 })).toEqual({ ok: true, reason: '3 still need a choice. They stay behind if you import now.' });
		expect(stepGate({ step: 'move', picked: 1, tried: true, openChoices: 1 }).reason).toBe('1 still needs a choice. It stays behind if you import now.');
		expect(stepGate({ step: 'move', picked: 1, tried: false, failed: true })).toEqual({ ok: false, reason: 'The test run failed.' });
		expect(stepGate({ step: 'move', picked: 1, tried: true, failed: true }).ok).toBe(false);
		expect(stepGate({ step: 'move', picked: 1, tried: true, safeMode: true }).ok).toBe(false);
		expect(stepGate({ step: 'move', picked: 1, tried: true, moving: true }).ok).toBe(false);
		expect(stepGate({ step: 'move', picked: 1, tried: true }).ok).toBe(true);
		expect(asStep('try')).toBe('choose');
		expect(asStep('move')).toBe('move');
		expect(asStep('nope')).toBeNull();
	});

	it('knows a running undo and per-person choices', () => {
		const m = { id: 1, created_at: '', snapshots: [], state: 'done' as const };
		expect(running(m)).toBe(false);
		expect(running({ ...m, undo_state: 'running' })).toBe(true);
		expect(running({ ...m, undo_state: 'failed' })).toBe(false);
		expect(running({ ...m, state: 'queued' })).toBe(true);
		const c = setChoice({}, 'person:seerr:2', 'merge');
		expect(c).toEqual({ 'person:seerr:2': 'merge' });
		expect(setChoice(c, 'person:seerr:2', 'skip')).toEqual({ 'person:seerr:2': 'skip' });
		expect(setChoice(c, 'person:seerr:2', null)).toEqual({});
	});
});

describe('reads per copy of an app', () => {
	it('keeps one row per kind and address, newest first', () => {
		const reads = [
			{ id: 4, kind: 'sonarr', address: 'http://sonarr-hd:8989' },
			{ id: 3, kind: 'sonarr', address: 'http://sonarr-4k:8989' },
			{ id: 2, kind: 'sonarr', address: 'http://sonarr-hd:8989' },
			{ id: 1, kind: 'radarr', address: 'http://sonarr-hd:8989' }
		];
		expect(latestPerInstance(reads).map((s) => s.id)).toEqual([4, 3, 1]);
		expect(instanceKey({ kind: 'sonarr' })).toBe(instanceKey({ kind: 'sonarr', address: '' }));
		expect(hostOf('http://sonarr-4k:8989')).toBe('sonarr-4k:8989');
		expect(hostOf('not an address')).toBe('not an address');
	});

	it('lists the other old folders that point at the same new folder', () => {
		const picks = { '/mnt/78/Movies': OTHER, '/mnt/78/4K Movies': OTHER, '/tv': '/data/tv', '/off': '' };
		const others = { '/mnt/78/Movies': '/data/movies ', '/mnt/78/4K Movies': '/data/movies' };
		expect(sharedTargets(picks, others)).toEqual({ '/mnt/78/Movies': ['/mnt/78/4K Movies'], '/mnt/78/4K Movies': ['/mnt/78/Movies'] });
		expect(sharedTargets({ '/a': '/x', '/b': '/y' }, {})).toEqual({});
		expect(sharedTargets({ '/a': OTHER, '/b': OTHER }, { '/a': '/data/movies/', '/b': '/data//movies' })).toEqual({ '/a': ['/b'], '/b': ['/a'] });
	});

	it('cleans a path the way the server does before comparing', () => {
		expect(cleanPath('/data/movies/')).toBe('/data/movies');
		expect(cleanPath('D:\\Media\\Movies')).toBe('D:/Media/Movies');
		expect(cleanPath('/data//movies')).toBe('/data/movies');
		expect(cleanPath('/')).toBe('/');
		expect(cleanPath('')).toBe('');
	});
});
