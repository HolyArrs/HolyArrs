// The line under "Home": a short, warm greeting that follows the person's name,
// the time of day, the day of the week and what is happening in the household.
// Pure functions only: the page passes in what it already loaded, and the same
// person gets the same line all day (seeded by date and name, never random).
import { looksLikeId } from '#lib/core/plain.ts';
import type { HomeAnswer } from './home.ts';
import type { Ask } from '#lib/core/types.ts';

export type TimeBucket = 'early' | 'morning' | 'afternoon' | 'evening' | 'late';

/** timeBucket parts the day: early morning 5-8, morning 9-11, afternoon 12-16, evening 17-21, late night 22-4. */
export function timeBucket(hour: number): TimeBucket {
	if (hour >= 5 && hour < 9) return 'early';
	if (hour >= 9 && hour < 12) return 'morning';
	if (hour >= 12 && hour < 17) return 'afternoon';
	if (hour >= 17 && hour < 22) return 'evening';
	return 'late';
}

// ---- Name ----

/** capitalise makes a login name read like a name: "agent-check" becomes "Agent Check". A name typed with capitals stays as typed. */
export function capitalise(name: string): string {
	let n = name.trim();
	if (n.includes('@')) n = n.slice(0, n.indexOf('@'));
	if (/\p{Lu}/u.test(n)) return n;
	return n
		.split(/[._\s-]+/)
		.filter(Boolean)
		.map((w) => w.charAt(0).toLocaleUpperCase() + w.slice(1))
		.join(' ');
}

/**
 * greetingName is the name to greet someone by: the display name when the
 * account has one (its first word), else the account name made to read like
 * a name. It is '' when what is left looks like an id and not a person.
 */
export function greetingName(displayName: string | null | undefined, accountName: string | null | undefined): string {
	const display = (displayName ?? '').trim();
	if (display && !looksLikeId(display)) return display.split(/\s+/)[0];
	return looksLikeId(accountName) ? '' : capitalise(accountName ?? '');
}

// ---- Facts: what is happening in the household ----

export interface HomeFacts {
	/** Names of the titles this person asked for that arrived in the last three days. */
	askedArrived: string[];
	/** Asks waiting for an answer. Only counted for people who approve. */
	waiting: number;
	/** What has something new today: how many titles, and the first of them. */
	newToday: { count: number; title?: string };
	/** Downloads about to finish: of this person's own asks, and all of them (managers only). */
	finishing: { mine: number; all: number };
}

export const noFacts: HomeFacts = { askedArrived: [], waiting: 0, newToday: { count: 0 }, finishing: { mine: 0, all: 0 } };

const DAY = 86400000;

/** localDay is the calendar day on the person's own clock, as YYYY-MM-DD. */
export function localDay(d: Date): string {
	return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

interface Progressing {
	state: string;
	progress: number;
	eta?: number | null;
}

/** nearlyDone says a download is about to finish: four fifths done, or five minutes left. */
export function nearlyDone(d: Progressing): boolean {
	return d.state === 'downloading' && (d.progress >= 0.8 || (d.eta != null && d.eta > 0 && d.eta <= 300));
}

/**
 * homeFacts reads what the Home page already loaded and says what is going on.
 * Nothing new is fetched: it only counts.
 */
export function homeFacts(i: {
	home: Pick<HomeAnswer, 'arrived' | 'new_seasons' | 'soon' | 'ask_progress'> | null;
	asks: Pick<Ask, 'state' | 'mine' | 'name' | 'available_at'>[];
	downloads: Progressing[];
	approver: boolean;
	manager: boolean;
	kid: boolean;
	now: Date;
}): HomeFacts {
	const now = i.now.getTime();
	const today = localDay(i.now);
	const askedArrived = [
		...new Set(
			i.asks
				.filter((a) => a.mine && a.state === 'available' && a.available_at && now - Date.parse(a.available_at) <= 3 * DAY && Date.parse(a.available_at) <= now + 3600000)
				.map((a) => a.name)
		)
	];
	const waiting = i.approver && !i.kid ? i.asks.filter((a) => a.state === 'asked').length : 0;

	// Something new today: a file that came in today, a season that starts today or an episode that airs today.
	const fresh = new Map<number, string>();
	for (const c of i.home?.arrived ?? []) if (c.at && localDay(new Date(c.at)) === today) fresh.set(c.title_id, c.name);
	for (const c of [...(i.home?.new_seasons ?? []), ...(i.home?.soon ?? [])]) if (c.at?.slice(0, 10) === today) fresh.set(c.title_id, c.name);

	const mine = Object.values(i.home?.ask_progress ?? {}).filter(nearlyDone).length;
	const all = i.manager && !i.kid ? i.downloads.filter(nearlyDone).length : 0;
	return { askedArrived, waiting, newToday: { count: fresh.size, title: [...fresh.values()][0] }, finishing: { mine, all } };
}

// ---- The lines ----
// {,name} becomes ", Maya" or nothing. {n} is a number, {title} a title.

const TIME: Record<TimeBucket, string[]> = {
	early: ["Good morning{,name}. You're up early.", "Early start{,name}. Here's what's new."],
	morning: ['Good morning{,name}.', "Morning{,name}. Here's what's new in the Library."],
	afternoon: ['Good afternoon{,name}.', 'Afternoon{,name}. Hope the day is going well.'],
	evening: ['Good evening{,name}.', 'Evening{,name}. Hope the day went well.'],
	late: ["It's late{,name}. Here's what's new.", "Quiet night{,name}. Here's what's new."]
};

/** A Kid's late night is one gentle line. */
const KID_LATE = ["Hi{,name}. It's getting late."];

/** Lines for days of the week (0 is Sunday). Tuesday to Thursday have none. */
const DAYS: Record<number, string[]> = {
	1: ["A new week{,name}. Here's what's new."],
	5: ['Happy Friday{,name}.', "It's Friday{,name}. The weekend is nearly here."],
	6: ['Happy Saturday{,name}. No rush today.'],
	0: ['Happy Sunday{,name}. Take it easy.']
};

const ASKED_ARRIVED = { any: 'Something you asked for has arrived{,name}.', one: '{title} is in the Library now{,name}.', many: '{n} of your asks are in the Library now{,name}.' };
const WAITING = ['{n} {asks} waiting for you{,name}.', 'Your Requests page has {n} {asks} to look at{,name}.'];
// New today also counts air and release dates, so these lines never say it arrived.
const NEW_TODAY = { any: 'Something new is out today{,name}.', one: 'New today{,name}: {title}.', many: 'New today{,name}: {n} titles.' };
const FINISHING = { mine: 'Something you asked for is almost here{,name}.', one: 'A download is almost done{,name}.', many: '{n} downloads are almost done{,name}.' };

/** Every line the picker can write, for tests and review. */
export const homeLineTemplates: string[] = [
	...Object.values(TIME).flat(),
	...KID_LATE,
	...Object.values(DAYS).flat(),
	...Object.values(ASKED_ARRIVED),
	...WAITING,
	...Object.values(NEW_TODAY),
	...Object.values(FINISHING)
];

/** hash is a small stable string hash (FNV-1a), so a pick never changes between loads. */
export function hash(s: string): number {
	let h = 0x811c9dc5;
	for (let i = 0; i < s.length; i++) {
		h ^= s.charCodeAt(i);
		h = Math.imul(h, 0x01000193) >>> 0;
	}
	return h >>> 0;
}

function fill(template: string, name: string, vars: Record<string, string | number> = {}): string {
	let out = template.replace('{,name}', name ? `, ${name}` : '');
	for (const [k, v] of Object.entries(vars)) out = out.replaceAll(`{${k}}`, String(v));
	return out;
}

export interface HomeLineInput {
	/** The account name. */
	name?: string | null;
	/** The display name, when the account has one. */
	displayName?: string | null;
	now: Date;
	/** A Kid account: no requests to approve or admin downloads, and gentler late-night lines. */
	kid?: boolean;
	facts?: Partial<HomeFacts>;
}

/**
 * homeLine writes the line under "Home". What is happening comes first, most
 * personal first: something you asked for arrived, asks waiting for you,
 * something new today, downloads about to finish. When it is quiet, the line
 * follows the time of day and, on a Monday, Friday or weekend, the day.
 * The pick is seeded by the date and the account, so it holds all day.
 */
export function homeLine(i: HomeLineInput): string {
	const name = greetingName(i.displayName, i.name);
	const f: HomeFacts = { ...noFacts, ...i.facts };
	const kid = !!i.kid;
	const day = localDay(i.now);
	const seed = `${day}|${(i.name ?? '').trim().toLowerCase()}`;
	const pickOf = <T>(list: T[], salt: string): T => list[hash(`${seed}|${salt}`) % list.length];

	if (f.askedArrived.length) {
		const one = f.askedArrived.length === 1;
		const line = pickOf(one ? [ASKED_ARRIVED.any, ASKED_ARRIVED.one] : [ASKED_ARRIVED.any, ASKED_ARRIVED.many], 'asked');
		return fill(line, name, { title: f.askedArrived[0], n: f.askedArrived.length });
	}
	if (f.waiting > 0 && !kid) return fill(pickOf(WAITING, 'waiting'), name, { n: f.waiting, asks: f.waiting === 1 ? 'ask' : 'asks' });
	if (f.newToday.count > 0) {
		const one = f.newToday.count === 1 && f.newToday.title;
		const line = pickOf(one ? [NEW_TODAY.any, NEW_TODAY.one] : [NEW_TODAY.any, NEW_TODAY.many], 'new');
		return fill(line, name, { title: f.newToday.title ?? '', n: f.newToday.count });
	}
	if (f.finishing.mine > 0) return fill(FINISHING.mine, name);
	if (f.finishing.all > 0 && !kid) return fill(f.finishing.all === 1 ? FINISHING.one : FINISHING.many, name, { n: f.finishing.all });

	const bucket = timeBucket(i.now.getHours());
	const pool = [...(kid && bucket === 'late' ? KID_LATE : TIME[bucket]), ...(DAYS[i.now.getDay()] ?? [])];
	return fill(pickOf(pool, 'quiet'), name);
}

/**
 * homeGreeting is the big line at the top of Home: the time of day and your
 * name ("Good morning, Maya"). It never waits for data, so it shows on the first frame.
 */
export function homeGreeting(i: { name?: string | null; displayName?: string | null; now: Date }): string {
	const name = greetingName(i.displayName, i.name);
	const bucket = timeBucket(i.now.getHours());
	const word = bucket === 'early' || bucket === 'morning' ? 'Good morning' : bucket === 'afternoon' ? 'Good afternoon' : 'Good evening';
	return name ? `${word}, ${name}` : word;
}

/**
 * homeNews is the short note beside Home's switch: what is going on, without a
 * name or a time-of-day greeting (the greeting above already says those).
 * Empty when nothing is going on.
 */
export function homeNews(i: { now: Date; kid?: boolean; facts?: Partial<HomeFacts> }): string {
	const f: HomeFacts = { ...noFacts, ...i.facts };
	const kid = !!i.kid;
	const busy = f.askedArrived.length > 0 || (f.waiting > 0 && !kid) || f.newToday.count > 0 || f.finishing.mine > 0 || (f.finishing.all > 0 && !kid);
	return busy ? homeLine({ name: '', now: i.now, kid, facts: f }) : '';
}
