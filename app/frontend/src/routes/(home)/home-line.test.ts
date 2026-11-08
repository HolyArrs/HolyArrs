import { describe, expect, it } from 'vitest';
import { capitalise, greetingName, hash, homeFacts, homeGreeting, homeLine, homeNews, homeLineTemplates, localDay, nearlyDone, noFacts, timeBucket, type HomeFacts } from './home-line.ts';

// The fixed test day is a Tuesday: no day-of-week lines, only the time of day.
const at = (month: number, day: number, hour = 10, minute = 0) => new Date(2026, month - 1, day, hour, minute);
const TUE = (hour = 10, minute = 0) => at(10, 27, hour, minute);
const facts = (f: Partial<HomeFacts>): HomeFacts => ({ ...noFacts, ...f });
const users = Array.from({ length: 60 }, (_, i) => `person${i}`);

describe('the name', () => {
	it('uses the display name when the account has one, else the account name', () => {
		expect(greetingName('Luis Rivera', 'luis')).toBe('Luis');
		expect(greetingName('  Maya  ', 'maya99')).toBe('Maya');
		expect(greetingName('', 'home')).toBe('Home');
		expect(greetingName(undefined, 'luis')).toBe('Luis');
		expect(greetingName(null, null)).toBe('');
	});
	it('capitalises a login name sensibly and leaves typed capitals alone', () => {
		expect(capitalise('home')).toBe('Home');
		expect(capitalise('agent-check')).toBe('Agent Check');
		expect(capitalise('john.smith')).toBe('John Smith');
		expect(capitalise('mary_jane')).toBe('Mary Jane');
		expect(capitalise('luis@example.com')).toBe('Luis');
		expect(capitalise('McKay')).toBe('McKay');
		expect(capitalise('Ana María')).toBe('Ana María');
		expect(capitalise('émile')).toBe('Émile');
	});
	it('drops the name when it only looks like an id', () => {
		expect(greetingName('', 'preview-e34f0722f7')).toBe('');
		expect(greetingName('', 'e2e-owner')).toBe('');
		expect(greetingName('', '1234567')).toBe('');
		expect(homeLine({ name: 'preview-e34f0722f7', now: TUE(10) })).toMatch(/^(Good morning|Morning)(\.|\. )/);
		expect(homeLine({ name: 'preview-e34f0722f7', now: TUE(10) })).not.toContain(',');
	});
	it('greets by the capitalised account name', () => {
		const line = homeLine({ name: 'home', now: TUE(10) });
		expect(line).toContain(', Home');
		expect(homeLine({ name: 'home', displayName: 'Luis Rivera', now: TUE(10) })).toContain(', Luis');
	});
});

describe('time of day', () => {
	it('parts the day into five buckets', () => {
		const at = (h: number) => timeBucket(h);
		expect([0, 3, 4].map(at)).toEqual(['late', 'late', 'late']);
		expect([5, 8].map(at)).toEqual(['early', 'early']);
		expect([9, 11].map(at)).toEqual(['morning', 'morning']);
		expect([12, 16].map(at)).toEqual(['afternoon', 'afternoon']);
		expect([17, 21].map(at)).toEqual(['evening', 'evening']);
		expect([22, 23].map(at)).toEqual(['late', 'late']);
	});
	it('says the right part of the day when it is quiet', () => {
		const expected: [number, RegExp][] = [
			[6, /^(Good morning|Early start)/],
			[10, /^(Good morning|Morning)/],
			[14, /^(Good afternoon|Afternoon)/],
			[19, /^(Good evening|Evening)/],
			[23, /^(It's late|Quiet night)/],
			[2, /^(It's late|Quiet night)/]
		];
		for (const [hour, re] of expected) for (const u of users) expect(homeLine({ name: u, now: TUE(hour) }), `${hour}h ${u}`).toMatch(re);
	});
	it('gives a Kid one gentle late-night line', () => {
		for (const u of users) expect(homeLine({ name: u, kid: true, now: TUE(23) })).toMatch(/^Hi, .+\. It's getting late\.$/);
	});
});

describe('day of the week', () => {
	const lines = (d: Date) => new Set(users.map((u) => homeLine({ name: u, now: d })));
	const has = (set: Set<string>, re: RegExp) => [...set].some((l) => re.test(l));
	it('has weekend lines on Saturday and Sunday only', () => {
		const sat = lines(at(10, 31)); // Saturday
		const sun = lines(at(11, 1)); // Sunday
		expect(has(sat, /Happy Saturday/)).toBe(true);
		expect(has(sat, /Sunday|Friday/)).toBe(false);
		expect(has(sun, /Happy Sunday/)).toBe(true);
		expect(has(sun, /Saturday|Friday/)).toBe(false);
		expect(has(lines(TUE()), /Saturday|Sunday|Friday|new week/)).toBe(false);
	});
	it('has Friday and Monday lines, mixed with the time of day', () => {
		const fri = lines(at(10, 30));
		expect(has(fri, /Friday/)).toBe(true);
		expect(has(fri, /^(Good morning|Morning)/)).toBe(true);
		expect(has(lines(at(10, 26)), /A new week/)).toBe(true);
		expect(has(lines(at(10, 28)), /Friday|Saturday|Sunday|new week/)).toBe(false);
	});
});

describe('what is happening', () => {
	const all = facts({ askedArrived: ['Gladiator'], waiting: 4, newToday: { count: 2, title: 'Severance' }, finishing: { mine: 1, all: 3 } });
	const line = (f: Partial<HomeFacts>, extra: { kid?: boolean } = {}) => users.map((u) => homeLine({ name: u, now: TUE(10), facts: f, ...extra }));
	const every = (list: string[], re: RegExp) => list.every((l) => re.test(l));

	it('puts something you asked for first, then asks waiting, then news, then downloads', () => {
		expect(every(line(all), /asked for|Gladiator|asks are in the Library/)).toBe(true);
		expect(every(line({ ...all, askedArrived: [] }), /\d+ asks waiting for you|Your Requests page has 4 asks/)).toBe(true);
		expect(every(line({ ...all, askedArrived: [], waiting: 0 }), /New today|Something new is out today/)).toBe(true);
		expect(every(line({ ...all, askedArrived: [], waiting: 0, newToday: { count: 0 } }), /Something you asked for is almost here/)).toBe(true);
		expect(every(line({ ...all, askedArrived: [], waiting: 0, newToday: { count: 0 }, finishing: { mine: 0, all: 3 } }), /^3 downloads are almost done/)).toBe(true);
		expect(every(line({ ...all, askedArrived: [], waiting: 0, newToday: { count: 0 }, finishing: { mine: 0, all: 0 } }), /^(Good morning|Morning)/)).toBe(true);
	});
	it('names the title when there is one, and counts when there are several', () => {
		const one = new Set(line(facts({ askedArrived: ['Gladiator'] })));
		expect([...one].some((l) => l.startsWith('Gladiator is in the Library now'))).toBe(true);
		const many = new Set(line(facts({ askedArrived: ['Gladiator', 'Coco', 'Alien'] })));
		expect([...many].some((l) => l.startsWith('3 of your asks are in the Library now'))).toBe(true);
		const news = new Set(line(facts({ newToday: { count: 1, title: 'Severance' } })));
		expect([...news].some((l) => l.startsWith('New today, ') && l.endsWith(': Severance.'))).toBe(true);
		const lots = new Set(line(facts({ newToday: { count: 5, title: 'Severance' } })));
		expect([...lots].some((l) => l.endsWith(': 5 titles.'))).toBe(true);
		expect(line(facts({ waiting: 1 })).some((l) => l.includes('1 ask waiting'))).toBe(true);
		expect(line(facts({ waiting: 1 })).some((l) => l.includes('1 asks'))).toBe(false);
		expect(every(line(facts({ finishing: { mine: 0, all: 1 } })), /^A download is almost done/)).toBe(true);
	});
	it('never says a date-only title arrived or is in the Library', () => {
		for (const count of [1, 2, 5]) for (const l of line(facts({ newToday: { count, title: 'Severance' } }))) expect(l, l).not.toMatch(/landed|arrived|in the Library/);
	});
	it('keeps quiet lines from claiming anything is new today', () => {
		for (const hour of [6, 10, 14, 19, 23]) for (const l of line(noFacts).concat(users.map((u) => homeLine({ name: u, now: TUE(hour) })))) expect(l, l).not.toMatch(/new today/i);
	});
	it('keeps a Kid away from requests to approve and the owner downloads', () => {
		expect(every(line(facts({ waiting: 9 }), { kid: true }), /^(Good morning|Morning)/)).toBe(true);
		expect(every(line(facts({ finishing: { mine: 0, all: 4 } }), { kid: true }), /^(Good morning|Morning)/)).toBe(true);
		expect(every(line(facts({ finishing: { mine: 1, all: 4 } }), { kid: true }), /Something you asked for is almost here/)).toBe(true);
		expect(every(line(facts({ askedArrived: ['Coco'] }), { kid: true }), /asked for|Coco/)).toBe(true);
	});
});

describe('one line all day', () => {
	it('holds the same line for the same person for the whole part of the day', () => {
		const first = homeLine({ name: 'maya', now: TUE(9, 0) });
		for (const [h, m] of [[9, 1], [10, 30], [11, 59]]) expect(homeLine({ name: 'maya', now: TUE(h, m) })).toBe(first);
		for (let i = 0; i < 20; i++) expect(homeLine({ name: 'maya', now: TUE(9, 0) })).toBe(first);
		expect(homeLine({ name: 'Maya', now: TUE(10) })).toBe(homeLine({ name: 'maya', now: TUE(10) }));
	});
	it('holds an activity line all day too', () => {
		const f = facts({ waiting: 3 });
		const first = homeLine({ name: 'maya', now: TUE(8), facts: f });
		for (const h of [9, 12, 17, 22]) expect(homeLine({ name: 'maya', now: TUE(h), facts: f })).toBe(first);
	});
	it('changes from day to day and from person to person', () => {
		const days = new Set(Array.from({ length: 28 }, (_, i) => homeLine({ name: 'maya', now: at(10, 1 + i, 10) })));
		expect(days.size).toBeGreaterThan(1);
		const people = new Set(users.map((u) => homeLine({ name: u, now: TUE(10) })));
		expect(people.size).toBeGreaterThan(1);
	});
	it('hashes the same text to the same number', () => {
		expect(hash('2026-10-27|maya')).toBe(hash('2026-10-27|maya'));
		expect(hash('a')).not.toBe(hash('b'));
	});
});

describe('the lines themselves', () => {
	const avoid = [/\bMonitored\b/i, /\bActivity\b/, /\bWanted\b/, /\bCutoff\b/, /Quality Profile/, /Root Folder/, /Download Client/, /Interactive Search/, /Manual Import/, /\bGrabbed\b/, /\bConnections\b/, /\bDiscover\b/, /\bCollections\b/, /\bSeries\b/i, /\bgrab/i, /queue item/i, /notification/i, /pipeline/i, /\btracked\b/i, /\bfollowed\b/i, /\bblocklist/i, /\bindexer/i];
	it('are about twenty-five short, plain lines', () => {
		expect(homeLineTemplates.length).toBeGreaterThanOrEqual(22);
		expect(homeLineTemplates.length).toBeLessThanOrEqual(30);
		expect(new Set(homeLineTemplates).size).toBe(homeLineTemplates.length);
		for (const l of homeLineTemplates) {
			expect(l.length, l).toBeLessThanOrEqual(60);
			expect(l, l).not.toContain('!');
			expect(l, l).not.toMatch(/\p{Extended_Pictographic}/u);
			expect(l, l).not.toMatch(/\n/);
			for (const re of avoid) expect(l, `${l} ${re}`).not.toMatch(re);
		}
	});
	it('use the glossary words for what they say', () => {
		const text = homeLineTemplates.join(' ');
		expect(text).toContain('Library');
		expect(text).toContain('Requests');
		expect(text).not.toMatch(/download client|\brequest\b/i);
	});
	it('read as one clean line with and without a name', () => {
		for (const l of homeLineTemplates) {
			const named = l.replace('{,name}', ', Luis').replace('{title}', 'Alien').replace('{n}', '3').replace('{asks}', 'asks');
			const bare = l.replace('{,name}', '').replace('{title}', 'Alien').replace('{n}', '3').replace('{asks}', 'asks');
			expect(named, l).not.toMatch(/[{}]/);
			expect(bare, l).not.toMatch(/\s,|\s\.|[{}]| {2}/);
		}
	});
});

describe('facts from what Home already loaded', () => {
	const now = TUE(10);
	const today = localDay(now);
	const card = (id: number, name: string, when: string) => ({ key: `k${id}`, kind: 'show' as const, title_id: id, name, href: '/', at: when });
	const base = { home: null, asks: [], downloads: [], approver: false, manager: false, kid: false, now };

	it('finds what you asked for that just arrived', () => {
		const asks = [
			{ state: 'available' as const, mine: true, name: 'Gladiator', available_at: new Date(now.getTime() - 3600000).toISOString() },
			{ state: 'available' as const, mine: true, name: 'Old one', available_at: new Date(now.getTime() - 9 * 86400000).toISOString() },
			{ state: 'available' as const, mine: false, name: 'Not mine', available_at: new Date(now.getTime() - 3600000).toISOString() },
			{ state: 'approved' as const, mine: true, name: 'Not yet', available_at: undefined }
		];
		expect(homeFacts({ ...base, asks }).askedArrived).toEqual(['Gladiator']);
	});
	it('counts waiting asks for people who approve, never for a Kid', () => {
		const asks = [{ state: 'asked' as const, mine: false, name: 'A' }, { state: 'asked' as const, mine: false, name: 'B' }, { state: 'approved' as const, mine: false, name: 'C' }];
		expect(homeFacts({ ...base, asks, approver: true }).waiting).toBe(2);
		expect(homeFacts({ ...base, asks, approver: false }).waiting).toBe(0);
		expect(homeFacts({ ...base, asks, approver: true, kid: true }).waiting).toBe(0);
	});
	it('counts what is new today once per title', () => {
		const home = {
			arrived: [card(1, 'Alien', new Date(2026, 9, 27, 3).toISOString()), card(2, 'Old', new Date(2026, 9, 20, 3).toISOString())],
			new_seasons: [card(3, 'Severance', `${today}T00:00:00.000000000Z`), card(1, 'Alien', `${today}T00:00:00.000000000Z`)],
			soon: [card(4, 'South Park', '2026-10-29T00:00:00.000000000Z')],
			ask_progress: {}
		};
		const n = homeFacts({ ...base, home }).newToday;
		expect(n.count).toBe(2);
		expect(n.title).toBe('Alien');
		expect(homeFacts({ ...base, home: { ...home, arrived: [], new_seasons: [] } }).newToday.count).toBe(0);
	});
	it('counts downloads that are nearly done, and only owners see all of them', () => {
		const downloads = [
			{ state: 'downloading', progress: 0.9, eta: 120 },
			{ state: 'downloading', progress: 0.2, eta: 4000 },
			{ state: 'downloading', progress: 0.3, eta: 200 },
			{ state: 'queued', progress: 0, eta: 50 }
		];
		const home = { arrived: [], new_seasons: [], soon: [], ask_progress: { '7': { progress: 0.95, state: 'downloading' as const }, '8': { progress: 0.1, state: 'queued' as const } } };
		expect(homeFacts({ ...base, home, downloads, manager: true }).finishing).toEqual({ mine: 1, all: 2 });
		expect(homeFacts({ ...base, home, downloads, manager: false }).finishing).toEqual({ mine: 1, all: 0 });
		expect(nearlyDone({ state: 'downloading', progress: 0.1 })).toBe(false);
		expect(nearlyDone({ state: 'stalled', progress: 0.99 })).toBe(false);
	});
});

describe('greeting and news beside the switch', () => {
	const at = (h: number) => new Date(2026, 10, 5, h, 0);
	it('greets by the time of day and the name', () => {
		expect(homeGreeting({ name: 'luis', now: at(7) })).toBe('Good morning, Luis');
		expect(homeGreeting({ name: 'luis', now: at(14) })).toBe('Good afternoon, Luis');
		expect(homeGreeting({ name: 'luis', now: at(23) })).toBe('Good evening, Luis');
		expect(homeGreeting({ name: '', now: at(10) })).toBe('Good morning');
	});
	it('says what is going on without a name, and nothing when it is quiet', () => {
		expect(homeNews({ now: at(10), facts: { waiting: 2 } })).toMatch(/2 asks/);
		expect(homeNews({ now: at(10), facts: { waiting: 2 } })).not.toMatch(/,\s*\./);
		expect(homeNews({ now: at(10), facts: noFacts })).toBe('');
		expect(homeNews({ now: at(10), kid: true, facts: { waiting: 3 } })).toBe('');
	});
});
