import { describe, expect, it, vi } from 'vitest';
import { ApiError } from '../core/api.ts';
import type { Ask } from '../core/types.ts';
import { liveCard, liveCards, type ExploreCard } from '../ui/title-cards.ts';
import { countOf, freshPath, loadChoices, joins, limitReason, lineNews, lineWords, planAskAll, runAskAll, toSend, type AskAllPost } from './ask-all.ts';
import en from '../core/i18n/messages/en/requests.json' with { type: 'json' };

const card = (n: number, extra: Partial<ExploreCard> = {}): ExploreCard => ({ key: `c-${n}`, kind: 'movie', name: `Film ${n}`, year: 2000 + n, ref: `tmdb:${n}`, badges: [], ...extra });
const ask = (id: number, state = 'asked'): Ask => ({ id, state }) as unknown as Ask;
const ok = (id: number, extra: Record<string, unknown> = {}) => ({ ask: ask(id), joined: false, already: false, ...extra });

describe('Request all plan', () => {
	it('skips movies in the Library or already asked for, and repeats', () => {
		const lines = planAskAll([card(1), card(2, { titleId: 9, status: 'in_library' }), card(3, { mine: true }), card(1), card(5, { kind: 'show' })]);
		expect(lines.map((l) => [l.card.name, l.state, l.why])).toEqual([
			['Film 1', 'todo', undefined],
			['Film 2', 'skipped', 'library'],
			['Film 3', 'skipped', 'mine']
		]);
		expect(toSend(lines)).toBe(1);
	});

	it('asks again for a movie this person asked for that was turned down, as a new request', () => {
		const lines = planAskAll([card(4, { mine: true, declined: true })]);
		expect(lines.map((l) => [l.state, l.why])).toEqual([['todo', undefined]]);
		expect(lineWords(lines[0], false, false)).toBe('requests.askAll.willRequest');
	});
});

describe('Request all run', () => {
	it('sends one normal request per movie, in order, with the same body a single request sends', async () => {
		const post = vi.fn<AskAllPost>(async (b) => ok(Number(String(b.id).slice(5))));
		const out = await runAskAll(planAskAll([card(1), card(2), card(3)]), post);
		expect(post.mock.calls.map((c) => c[0])).toEqual([
			{ id: 'tmdb:1', follow: true },
			{ id: 'tmdb:2', follow: true },
			{ id: 'tmdb:3', follow: true }
		]);
		expect(countOf(out, 'requested')).toBe(3);
	});

	it('keeps going after one movie is refused, and says why', async () => {
		const post = vi.fn<AskAllPost>(async (b) => {
			if (b.id === 'tmdb:2') throw new ApiError('This movie is rated above your age limit.', 403);
			if (b.id === 'tmdb:3') throw new ApiError('already in the Library', 409, { title_id: 77 });
			return ok(1);
		});
		const out = await runAskAll(planAskAll([card(1), card(2), card(3), card(4)]), post);
		expect(post).toHaveBeenCalledTimes(4);
		expect(out.map((l) => [l.state, l.why, l.reason])).toEqual([
			['requested', undefined, undefined],
			['skipped', 'server', 'This movie is rated above your age limit.'],
			['skipped', 'library', undefined],
			['requested', undefined, undefined]
		]);
		expect(out[2].card.titleId).toBe(77);
	});

	it('stops sending at the weekly limit and skips the rest with the same reason', async () => {
		const post = vi.fn<AskAllPost>(async (b) => {
			if (b.id === 'tmdb:2') throw new ApiError('you have asked for 5 movies in the last 7 days', 429);
			return ok(1);
		});
		const out = await runAskAll(planAskAll([card(1), card(2), card(3), card(4, { titleId: 4, status: 'in_library' })]), post);
		expect(post).toHaveBeenCalledTimes(2);
		expect(out.map((l) => [l.state, l.why])).toEqual([['requested', undefined], ['skipped', 'limit'], ['skipped', 'limit'], ['skipped', 'library']]);
		expect(out[2].reason).toBe(out[1].reason);
		expect(limitReason(out)).toBe('you have asked for 5 movies in the last 7 days');
		expect(countOf(out, 'requested')).toBe(1);
		expect(countOf(out, 'skipped')).toBe(3);
	});

	it('counts an ask the person already made (another tab) as skipped, and joining someone else’s ask as requested', async () => {
		const post = vi.fn<AskAllPost>(async (b) => (b.id === 'tmdb:1' ? ok(1, { already: true }) : ok(2, { joined: true })));
		const out = await runAskAll(planAskAll([card(1), card(2)]), post);
		expect(out.map((l) => [l.state, l.why])).toEqual([['skipped', 'mine'], ['requested', undefined]]);
	});

	it('reports every finished line as it goes', async () => {
		const seen: number[] = [];
		await runAskAll(planAskAll([card(1), card(2, { mine: true }), card(3)]), async () => ok(1), (_l, i) => seen.push(i));
		expect(seen).toEqual([0, 2]);
	});
});

describe('Request all on live state', () => {
	const here = (c: ExploreCard): ExploreCard => (c.key === 'c-2' ? { ...c, mine: true } : c);
	it('leaves out a movie asked for since the page loaded, and the own movie of the page', () => {
		const lines = planAskAll(liveCards([card(1), card(2), card(3)], here, { tmdb: '3', ask: 'asked' }));
		expect(lines.map((l) => [l.card.name, l.state, l.why])).toEqual([
			['Film 1', 'todo', undefined],
			['Film 2', 'skipped', 'mine'],
			['Film 3', 'skipped', 'mine']
		]);
		expect(toSend(lines)).toBe(1);
	});
});

describe('Request all read again', () => {
	it('reads the series past every cache', () => {
		expect(freshPath('/metadata/movie/1?region=US', 5)).toBe('/metadata/movie/1?region=US&fresh=5');
		expect(freshPath('/metadata/movie/1', 5)).toBe('/metadata/movie/1?fresh=5');
	});
	it('lets a request someone else made since the page loaded count, so the movie joins it', () => {
		const fresh = card(3, { ask: ask(8) as unknown as ExploreCard['ask'], status: 'asked' as ExploreCard['status'] });
		const lines = planAskAll(liveCards([fresh], (c) => c));
		expect(joins(lines[0])).toBe(true);
		expect(lineWords(lines[0], false, true)).toBe('requests.askAll.willRequest');
	});
	it('keeps a movie the fresh read finds in the Library there, over an ask made here', () => {
		const lines = planAskAll(liveCards([card(3, { titleId: 4, status: 'in_library' })], (c) => liveCard(c, { ask: 'asked' })));
		expect(lines[0].why).toBe('library');
	});
});

describe('Add all (people who manage the Library)', () => {
	it('adds a movie the person had requested, and still skips what is in the Library', () => {
		const lines = planAskAll([card(1), card(2, { mine: true }), card(3, { titleId: 3, status: 'in_library' })], true);
		expect(lines.map((l) => [l.state, l.why])).toEqual([['todo', undefined], ['todo', undefined], ['skipped', 'library']]);
	});

	it('keeps the Library title of each movie added and goes on after one fails', async () => {
		const post = vi.fn<AskAllPost>(async (b) => {
			if (b.id === 'tmdb:2') throw new ApiError('No folder for movies', 400);
			return { title: { id: 40 + Number(String(b.id).slice(5)) } as never };
		});
		const out = await runAskAll(planAskAll([card(1), card(2), card(3)], true), post);
		expect(out.map((l) => [l.state, l.why, l.title?.id])).toEqual([['requested', undefined, 41], ['skipped', 'server', undefined], ['requested', undefined, 43]]);
	});
});

describe('What each line of the sheet says', () => {
	it('says the weekly limit is reached, not that a movie will be requested, when no asks are left', () => {
		const lines = planAskAll([card(1), card(2, { titleId: 2, status: 'in_library' })]);
		expect(lines.map((l) => lineWords(l, false, false))).toEqual(['requests.askAll.willRequest', 'requests.askAll.inLibrary']);
		expect(lines.map((l) => lineWords(l, false, true))).toEqual(['requests.askAll.limit', 'requests.askAll.inLibrary']);
	});

	it('uses the Add all words for people who manage the Library, and the server reason has no key', () => {
		expect(lineWords({ card: card(1), state: 'todo' }, true, false)).toBe('requests.addAll.willAdd');
		expect(lineWords({ card: card(1), state: 'requested' }, true, false)).toBe('requests.addAll.added');
		expect(lineWords({ card: card(1), state: 'requested' }, false, false)).toBe('requests.askAll.requested');
		expect(lineWords({ card: card(1), state: 'skipped', why: 'server', reason: 'x' }, false, false)).toBeNull();
	});

	it('the intro says skipped movies are listed, since the sheet lists them', () => {
		expect(en['requests.askAll.intro']).toContain('already requested by you are listed but not requested again');
		expect(en['requests.askAll.intro']).not.toContain('left out');
	});
});

describe('What the page is told about each movie', () => {
	it('hands back the Library title of a movie that was added, so the page of that movie can show it', () => {
		expect(lineNews({ card: card(7), state: 'requested', title: { id: 70 } as never })).toEqual({ tmdb: '7', titleId: 70 });
	});

	it('hands back the Library title the server named when the movie was there already', () => {
		expect(lineNews({ card: card(7, { titleId: 71 }), state: 'skipped', why: 'library' })).toEqual({ tmdb: '7', titleId: 71 });
	});

	it('hands back the ask of a movie that was requested, and nothing for other skips', () => {
		expect(lineNews({ card: card(7), state: 'requested', ask: ask(5) })).toEqual({ tmdb: '7', ask: ask(5) });
		expect(lineNews({ card: card(7), state: 'skipped', why: 'limit', reason: 'x' })).toBeNull();
	});
});

describe('Movies someone else already requested', () => {
	const theirs = { ask: { id: 9, state: 'asked', state_label: 'Requested', mine: false, people: 1 } } as Partial<ExploreCard>;

	it('still says the movie will be requested when no asks are left, since joining does not count', () => {
		const lines = planAskAll([card(1), card(2, theirs)]);
		expect(lines.map((l) => lineWords(l, false, true))).toEqual(['requests.askAll.limit', 'requests.askAll.willRequest']);
	});

	it('tells a movie that can join from one that cannot, so the button stays usable only when one can', () => {
		expect(planAskAll([card(1), card(2, theirs)]).some(joins)).toBe(true);
		expect(planAskAll([card(1), card(2, { ...theirs, mine: true }), card(3, { ...theirs, titleId: 3 })]).some(joins)).toBe(false);
	});

	it('keeps sending them after the weekly limit is hit, and skips only the new requests', async () => {
		const post = vi.fn<AskAllPost>(async (b) => {
			if (b.id === 'tmdb:1' || b.id === 'tmdb:2') throw new ApiError('you have asked for 1 movie in the last 7 days', 429);
			return ok(9, { joined: true });
		});
		const out = await runAskAll(planAskAll([card(1), card(2), card(3, theirs), card(4)]), post);
		expect(post.mock.calls.map((c) => c[0].id)).toEqual(['tmdb:1', 'tmdb:3']);
		expect(out.map((l) => [l.state, l.why])).toEqual([['skipped', 'limit'], ['skipped', 'limit'], ['requested', undefined], ['skipped', 'limit']]);
	});
	it('with no asks left from the start, sends only the joins and skips the new requests without asking the server', async () => {
		const post = vi.fn<AskAllPost>(async () => ok(9, { joined: true }));
		const out = await runAskAll(planAskAll([card(1), card(2, theirs), card(3)]), post, () => {}, true);
		expect(post.mock.calls.map((c) => c[0].id)).toEqual(['tmdb:2']);
		expect(out.map((l) => [l.state, l.why])).toEqual([['skipped', 'limit'], ['requested', undefined], ['skipped', 'limit']]);
		expect(limitReason(out)).toBe('');
	});
});

describe('folders and Pick rules for Add all', () => {
	it('an older load that fails after a newer one succeeds leaves no error showing', async () => {
		let failFirst!: () => void;
		const first = new Promise<string[]>((_, no) => (failFirst = () => no(new Error('x'))));
		const shown = { failed: false, folders: [] as string[] };
		const load = loadChoices((failed) => (shown.failed = failed));
		load([first.then((f) => (shown.folders = f))]);
		load([Promise.resolve(['a']).then((f) => (shown.folders = f))]);
		failFirst();
		await new Promise((r) => setTimeout(r, 0));
		expect(shown.folders).toEqual(['a']);
		expect(shown.failed).toBe(false);
	});
	it('a failed newest load shows the error', async () => {
		let failed = false;
		loadChoices((f) => (failed = f))([Promise.reject(new Error('x'))]);
		await new Promise((r) => setTimeout(r, 0));
		expect(failed).toBe(true);
	});
});
