// Asked by: who in the household asked for a title (title page only;
// cards stay factual). The server already
// trims the askers to the ones this reader may see (owner, admins and
// managers see everyone; members see themselves plus askers who chose to
// show their name when the household allows it; Kids see only themselves).
//
// Every word this feature shows lives in WORDS, so a wording change is one
// edit here.

export type Asker = { id: number; name: string; asked_at: string; me: boolean };
export type AskedByAnswer = { titles: Record<string, { askers: Asker[] }>; sees_all: boolean };

export const WORDS = {
	you: 'You asked',
	one: (a: string) => `Asked by ${a}`,
	two: (a: string, b: string) => `Asked by ${a} and ${b}`,
	many: (a: string, others: number) => `Asked by ${a} and ${others} ${others === 1 ? 'other' : 'others'}`,
	youAnd: (others: number) => `You and ${others} ${others === 1 ? 'other' : 'others'} asked`,
	/** Tooltip: "Ana asked for this on 12 Oct. Only you and the admins can see this." */
	tip: (who: string, date: string) => `${who} asked for this on ${date}. Only you, the owner, admins and managers can see this.`,
	sharedTip: (who: string, date: string) => `${who} asked for this on ${date}. ${who} chose to show their name to the household.`,
	youTip: (date: string) => `You asked for this on ${date}. Only you, the owner, admins and managers can see this.`,
	dateSep: ' · ',
	listTitle: 'Who asked',
	// Settings
	householdLabel: 'Show who asked',
	householdHelp: 'Who can see the names of the people who asked for a title. Kids only ever see their own asks.',
	householdChoices: [
		{ value: 'approvers', label: 'Owner, admins and managers only' },
		{ value: 'everyone', label: 'Everyone in the household' }
	],
	personalLabel: 'Show my name on my asks',
	personalHelp: 'When the household allows it, other members see that you asked. The owner, admins and managers always can.'
} as const;

/** First name only: "Ana Lopez" -> "Ana". */
export function firstName(name: string): string {
	return (name || '').trim().split(/\s+/)[0] || name;
}

/** "12 Oct" in the reader's locale order, English month. */
export function shortDate(iso: string): string {
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return '';
	return d.toLocaleDateString('en-GB', { day: 'numeric', month: 'short' });
}

/** The line: "You asked", "Asked by Ana", "Asked by Ana and Leo", "Asked by Ana and 2 others". */
export function askedByLine(askers: Asker[]): string {
	if (!askers.length) return '';
	const others = askers.filter((a) => !a.me);
	const me = askers.some((a) => a.me);
	if (me && !others.length) return WORDS.you;
	if (me) return WORDS.youAnd(others.length);
	const n = others.map((a) => firstName(a.name));
	if (n.length === 1) return WORDS.one(n[0]);
	if (n.length === 2) return WORDS.two(n[0], n[1]);
	return WORDS.many(n[0], n.length - 1);
}

/** The tooltip for the first asker shown. */
export function askedByTip(askers: Asker[], seesAll: boolean): string {
	if (!askers.length) return '';
	const first = askers[0];
	const date = shortDate(first.asked_at);
	if (first.me && askers.length === 1) return WORDS.youTip(date);
	const other = askers.find((a) => !a.me) ?? first;
	const who = firstName(other.name);
	const when = shortDate(other.asked_at);
	return seesAll ? WORDS.tip(who, when) : WORDS.sharedTip(who, when);
}

/** Title page line: "Asked by Ana · 12 Oct". */
export function askedByDated(askers: Asker[]): string {
	const line = askedByLine(askers);
	if (!line) return '';
	const date = shortDate(askers[0].asked_at);
	return date ? line + WORDS.dateSep + date : line;
}

/** Initial for an avatar. */
export function initial(name: string): string {
	return (firstName(name)[0] || '?').toUpperCase();
}

/** Asks GET /asked-by for many titles at once; {} on no ids. */
export async function loadAskedBy(get: <T>(path: string) => Promise<T>, ids: number[]): Promise<AskedByAnswer> {
	const uniq = [...new Set(ids.filter((n) => n > 0))].slice(0, 200);
	if (!uniq.length) return { titles: {}, sees_all: false };
	return get<AskedByAnswer>(`/asked-by?title_ids=${uniq.join(',')}`);
}
