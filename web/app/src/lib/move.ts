// The move from the old apps: read them, choose what to move, move.
// Plain helpers for Settings > Move from other apps.

export const apps = [
	{ kind: 'radarr', name: 'Radarr', what: 'movies', port: 7878 },
	{ kind: 'sonarr', name: 'Sonarr', what: 'shows', port: 8989 },
	{ kind: 'lidarr', name: 'Lidarr', what: 'music', port: 8686 },
	{ kind: 'prowlarr', name: 'Prowlarr', what: 'Sources', port: 9696 },
	{ kind: 'bazarr', name: 'Bazarr', what: 'subtitles', port: 6767 },
	{ kind: 'seerr', name: 'Seerr', what: 'people and requests', port: 5055 },
	{ kind: 'overseerr', name: 'Overseerr', what: 'people and requests', port: 5055 },
	{ kind: 'qbittorrent', name: 'qBittorrent', what: 'Downloader', port: 8080 },
	{ kind: 'sabnzbd', name: 'SABnzbd', what: 'Downloader', port: 8080 }
] as const;

export type AppKind = (typeof apps)[number]['kind'];

export function appName(kind: string): string {
	return apps.find((a) => a.kind === kind)?.name ?? kind;
}

/** appLabel names an app with what it brings over ("Radarr · movies"). */
export function appLabel(kind: string): string {
	const a = apps.find((x) => x.kind === kind);
	return a ? `${a.name} · ${a.what}` : kind;
}

/** placeholderFor is the address hint for an app, on its usual port. */
export function placeholderFor(kind: string): string {
	return `http://10.0.0.5:${apps.find((a) => a.kind === kind)?.port ?? 8080}`;
}

export interface Snapshot {
	id: number;
	created_at: string;
	kind: string;
	via: 'api' | 'backup';
	app_version: string;
	counts: Record<string, number>;
	size: number;
}

export interface PathRule {
	from: string;
	to: string;
}

export interface PersonPlan {
	app?: string;
	role: 'member' | 'admin' | 'owner';
	was_admin?: boolean;
	was_owner?: boolean;
	choice?: 'merge' | 'skip';
	email?: string;
}

export interface PlanItem {
	key: string;
	name: string;
	from: string;
	detail?: string;
	why?: string;
	needs?: string;
	person?: PersonPlan;
	folder?: { old_path: string; path: string; kind: string; label: string };
}

/** The owner's choice for one person: owner, admin, merge or skip. */
export type PersonChoice = 'owner' | 'admin' | 'merge' | 'skip';

export interface PlanGroup {
	name: string;
	label: string;
	skipped: boolean;
	items: PlanItem[];
}

export interface NotMoved {
	group: string;
	from: string;
	what: string;
	why: string;
}

export interface Plan {
	snapshots: number[];
	apps: string[];
	groups: PlanGroup[];
	not_moved: NotMoved[];
	needs_input: string[];
	notes?: string[];
	old_paths: string[];
}

/** Why a person needs a Same person or Leave out choice (set by the server). */
export type ChooseWhy = 'same_name' | 'numeric_id' | 'merge_gone';

export interface Line {
	group: string;
	key: string;
	name: string;
	result: string;
	detail?: string;
	needs?: string;
	choose?: ChooseWhy;
}

export interface Report {
	move?: number;
	dry: boolean;
	undo?: boolean;
	apps: string[];
	at: string;
	lines: Line[];
	counts: Record<string, number>;
	not_moved: NotMoved[];
	needs_input: string[];
	notes?: string[];
	next: string[];
}

export interface Move {
	id: number;
	created_at: string;
	snapshots: number[];
	choices?: { people?: Record<string, string>; skip?: string[]; path_rules?: PathRule[] };
	state: 'queued' | 'running' | 'done' | 'failed' | 'undone';
	job_id?: number;
	report?: Report;
	finished_at?: string;
	undone_at?: string;
	undo_state?: '' | 'queued' | 'running' | 'failed';
	undo_job_id?: number;
}

/** running says a move or its undo is still going. */
export function running(m: Move): boolean {
	return m.state === 'queued' || m.state === 'running' || m.undo_state === 'queued' || m.undo_state === 'running';
}

/** chooseHint says, in plain words, why a person needs a choice. */
export function chooseHint(why: ChooseWhy | string | undefined): string {
	switch (why) {
		case 'numeric_id':
			return "HolyArrs can't tell if this is the same person.";
		case 'merge_gone':
			return 'The person you picked is no longer here.';
	}
	return 'Someone here has the same name. Until you choose, they stay behind and their requests become yours.';
}

/** setChoice sets or clears (null) one person's choice. */
export function setChoice(people: Record<string, PersonChoice>, key: string, c: PersonChoice | null): Record<string, PersonChoice> {
	const out = { ...people };
	if (c === null) delete out[key];
	else out[key] = c;
	return out;
}

/** groupLabels name the plan's groups (as the plan does). */
export const groupLabels: Record<string, string> = {
	folders: 'Library folders',
	pick_rules: 'Pick rules',
	titles: 'Titles',
	never_add: 'Never-add list',
	outside_lists: 'Outside lists',
	sources: 'Sources',
	downloaders: 'Downloaders',
	naming: 'Naming',
	subtitles: 'Subtitles',
	people: 'People',
	wishlist: 'Requests',
	alerts: 'Alerts',
	other: 'Other settings'
};

/** The order groups are shown in (the server's GroupOrder). */
export const groupOrder = ['folders', 'pick_rules', 'titles', 'never_add', 'outside_lists', 'sources', 'downloaders', 'naming', 'subtitles', 'people', 'wishlist', 'alerts'];

// Result words: [before the move, after it]. The server says create in a
// test run and created in a real move; both read the same here.
const words: Record<string, [string, string]> = {
	create: ['Will be added', 'Added'],
	adopt: ['Will use the one here', 'Using the one here'],
	change: ['Will be updated', 'Updated'],
	matched: ['Matches one here', 'Matched'],
	already: ['Already here', 'Already here'],
	skipped: ['Left out', 'Left out'],
	not_found: ['No folder here', 'No folder here'],
	failed: ["Won't work", 'Failed']
};
const pastToBase: Record<string, string> = { created: 'create', adopted: 'adopt', changed: 'change' };

/** resultWords says one result in plain words: before the move (the check), after it, or after an undo. */
export function resultWords(result: string, after = false, undo = false): string {
	if (undo) {
		if (result === 'removed') return 'Removed';
		if (result === 'kept') return 'Kept';
		if (result === 'failed') return 'Not removed';
	}
	if (result === 'removed') return 'Removed';
	if (result === 'kept') return 'Kept';
	const w = words[pastToBase[result] ?? result];
	if (!w) return result.replaceAll('_', ' ');
	return pastToBase[result] ? w[1] : w[after ? 1 : 0];
}

// The order results are said in, problems last.
const resultOrder = ['create', 'created', 'adopt', 'adopted', 'change', 'changed', 'matched', 'already', 'removed', 'kept', 'skipped', 'not_found', 'failed'];

/** tone colours a result: good, warn or bad. */
export function tone(result: string): 'ok' | 'warn' | 'bad' | 'quiet' {
	switch (result) {
		case 'create':
		case 'created':
		case 'adopt':
		case 'adopted':
		case 'change':
		case 'changed':
		case 'matched':
		case 'removed':
			return 'ok';
		case 'not_found':
		case 'kept':
			return 'warn';
		case 'failed':
			return 'bad';
	}
	return 'quiet';
}

/** needsLook says a line wants the owner's eye (failed, not found, left out). */
export function needsLook(result: string): boolean {
	return result === 'failed' || result === 'not_found' || result === 'skipped' || result === 'kept';
}

/** summary says counts in one line ("3 will be added, 1 already here"), zeros hidden. */
export function summary(counts: Record<string, number>, after = false, undo = false): string {
	return Object.entries(counts)
		.filter(([, n]) => n > 0)
		.sort(([a], [b]) => order(a) - order(b))
		.map(([k, n]) => `${n} ${resultWords(k, after, undo).toLowerCase()}`)
		.join(' · ');
}

function order(result: string): number {
	const i = resultOrder.indexOf(result);
	return i < 0 ? resultOrder.length : i;
}

/** Bucket is one group of a report: its lines and their counts per result. */
export interface Bucket {
	group: string;
	label: string;
	lines: Line[];
	counts: Record<string, number>;
}

/** buckets sorts a report's lines once, in one pass: per group (server order), counts per result, and the people who need a choice. */
export function buckets(r: Pick<Report, 'lines'> | null | undefined): { groups: Bucket[]; choose: Line[]; total: number } {
	const by = new Map<string, Bucket>();
	const choose: Line[] = [];
	let total = 0;
	for (const l of r?.lines ?? []) {
		let g = by.get(l.group);
		if (!g) {
			g = { group: l.group, label: groupLabels[l.group] ?? l.group, lines: [], counts: {} };
			by.set(l.group, g);
		}
		g.lines.push(l);
		g.counts[l.result] = (g.counts[l.result] ?? 0) + 1;
		if (l.choose) choose.push(l);
		total++;
	}
	return { groups: [...by.values()], choose, total };
}

/** moving counts the lines a move would add, use or update (what "Move N items" says). */
export function moveTotal(counts: Record<string, number>): number {
	let n = 0;
	for (const k of ['create', 'created', 'adopt', 'adopted', 'change', 'changed', 'matched']) n += counts[k] ?? 0;
	return n;
}

/** staying counts the lines that stay behind (left out, not found, failed). */
export function stayTotal(counts: Record<string, number>): number {
	return (counts.skipped ?? 0) + (counts.not_found ?? 0) + (counts.failed ?? 0);
}

/** notMovedByGroup counts the not-moved list per group label. */
export function notMovedByGroup(list: NotMoved[], labels: Record<string, string>): { label: string; items: NotMoved[] }[] {
	const out: { label: string; items: NotMoved[] }[] = [];
	for (const n of list) {
		const label = labels[n.group] ?? 'Other settings';
		let g = out.find((x) => x.label === label);
		if (!g) {
			g = { label, items: [] };
			out.push(g);
		}
		g.items.push(n);
	}
	return out;
}

/** needsGroups groups the keys to type again by the words before the first quote or colon ("radarr", "Sources"); a line with neither is shown as it is. */
export function needsGroups(list: string[]): { label: string; items: string[] }[] {
	const out: { label: string; items: string[] }[] = [];
	for (const line of list) {
		const cut = [line.indexOf('"'), line.indexOf(':')].filter((i) => i > 0);
		const at = cut.length ? Math.min(...cut) : -1;
		const head = at > 0 ? line.slice(0, at).trim() : '';
		const label = head ? (apps.some((a) => a.kind === head) ? appName(head) : head) : 'Other';
		const rest = at > 0 ? line.slice(at).replace(/^:\s*/, '').trim() : line;
		let g = out.find((x) => x.label === label);
		if (!g) {
			g = { label, items: [] };
			out.push(g);
		}
		g.items.push(rest || line);
	}
	return out;
}

// The words for a read's counts: never a raw key. Unknown keys are dropped.
const countNames: Record<string, [string, string]> = {
	titles: ['title', 'titles'],
	folders: ['Library folder', 'Library folders'],
	quality_profiles: ['set of Pick rules', 'sets of Pick rules'],
	custom_formats: ['trait', 'traits'],
	delay_profiles: ['wait rule', 'wait rules'],
	indexers: ['Source', 'Sources'],
	download_clients: ['Downloader', 'Downloaders'],
	import_lists: ['Outside list', 'Outside lists'],
	notifications: ['Alert', 'Alerts'],
	app_links: ['linked app', 'linked apps'],
	users: ['person', 'people'],
	requests: ['request', 'requests'],
	tags: ['tag', 'tags'],
	subtitle_languages: ['subtitle language', 'subtitle languages'],
	subtitle_profiles: ['subtitle language set', 'subtitle language sets'],
	subtitle_sites: ['Subtitle site', 'Subtitle sites'],
	needs_reentry: ['key to type again', 'keys to type again']
};
// What each app mostly brings over: said first.
const mainCount: Record<string, string> = {
	radarr: 'titles',
	sonarr: 'titles',
	lidarr: 'titles',
	prowlarr: 'indexers',
	bazarr: 'subtitle_sites',
	seerr: 'users',
	overseerr: 'users',
	qbittorrent: 'download_clients',
	sabnzbd: 'download_clients'
};

/** countWord says one count ("1 Downloader", "812 titles"), or '' for zero or an unknown key. */
export function countWord(key: string, n: number): string {
	const w = countNames[key];
	if (!w || !(n > 0)) return '';
	return `${n} ${n === 1 ? w[0] : w[1]}`;
}

// The few counts that matter most per app, in order; titles get the app's own word.
const bestKeys: Record<string, string[]> = {
	radarr: ['titles', 'indexers', 'download_clients'],
	sonarr: ['titles', 'indexers', 'download_clients'],
	lidarr: ['titles', 'indexers', 'download_clients'],
	bazarr: ['subtitle_languages', 'subtitle_sites'],
	seerr: ['users', 'requests'],
	overseerr: ['users', 'requests'],
	prowlarr: ['indexers'],
	qbittorrent: ['download_clients'],
	sabnzbd: ['download_clients']
};
const titleWord: Record<string, [string, string]> = { radarr: ['movie', 'movies'], sonarr: ['show', 'shows'], lidarr: ['artist', 'artists'] };

/** bestStats says a read's most useful counts ("252 movies · 11 Sources · 2 Downloaders"). */
export function bestStats(kind: string, counts: Record<string, number>): string {
	return (bestKeys[kind] ?? [])
		.map((k) => {
			const n = counts[k] ?? 0;
			if (!(n > 0)) return '';
			const w = k === 'titles' && titleWord[kind] ? titleWord[kind] : countNames[k];
			return w ? `${n} ${n === 1 ? w[0] : w[1]}` : '';
		})
		.filter(Boolean)
		.join(' · ');
}

/** countWords says a read's counts: the app's main count first, then the largest, at most max of them, then "and N more". */
export function countWords(kind: string, counts: Record<string, number>, max = 3): string {
	const known = Object.entries(counts).filter(([k, n]) => countNames[k] && n > 0);
	const main = mainCount[kind];
	known.sort(([a, x], [b, y]) => (a === main ? -1 : b === main ? 1 : y - x || a.localeCompare(b)));
	const shown = known.slice(0, max).map(([k, n]) => countWord(k, n));
	const rest = known.slice(max).reduce((s, [, n]) => s + n, 0);
	if (rest > 0) return `${shown.join(', ')} and ${rest} more`;
	return shown.join(', ');
}

/** moveStateWord says where a move is, in plain words, with the pill's tone. */
export function moveStateWord(m: Pick<Move, 'state' | 'undo_state'>): { word: string; tone: 'ok' | 'info' | 'wait' | 'fail' | 'idle' } {
	if (m.undo_state === 'queued' || m.undo_state === 'running') return { word: 'Undoing', tone: 'info' };
	if (m.undo_state === 'failed' && m.state !== 'undone') return { word: 'Undo failed', tone: 'fail' };
	switch (m.state) {
		case 'queued':
			return { word: 'Waiting', tone: 'wait' };
		case 'running':
			return { word: 'Moving', tone: 'info' };
		case 'done':
			return { word: 'Moved', tone: 'ok' };
		case 'failed':
			return { word: 'Stopped with errors', tone: 'fail' };
		case 'undone':
			return { word: 'Undone', tone: 'idle' };
	}
	return { word: 'Waiting', tone: 'wait' };
}

/** moveCounts says a finished move's counts the short way ("840 added · 4 not found · 2 failed"). */
export function moveCounts(m: Pick<Move, 'state' | 'report'>): string {
	const c = m.report?.counts ?? {};
	return summary(c, true, m.state === 'undone' || !!m.report?.undo);
}

/** cleanRules drops empty rows and trims; a half-filled row stays so it can be fixed. */
export function cleanRules(rules: PathRule[]): PathRule[] {
	return rules.map((r) => ({ from: r.from.trim(), to: r.to.trim() })).filter((r) => r.from !== '' || r.to !== '');
}

/** ruleProblem says what is wrong with one rule, or ''. */
export function ruleProblem(r: PathRule): string {
	const full = (p: string) => p.startsWith('/') || /^[A-Za-z]:[\\/]/.test(p);
	if (!r.from || !r.to) return 'Fill in both paths.';
	if (!full(r.from) || !full(r.to)) return 'Both must be full paths, such as /movies.';
	if (/(^|[\\/])\.\.([\\/]|$)/.test(r.from + '/' + r.to)) return `Paths can't contain "..".`;
	return '';
}

/** OTHER is the folder pick that means "type another path". */
export const OTHER = '\u0000other';

/** folderTo is where an old folder goes: the picked Library folder, the typed path, or '' (unset). */
export function folderTo(pick: string | undefined, other: string | undefined): string {
	if (!pick) return '';
	return pick === OTHER ? (other ?? '').trim() : pick;
}

/** folderProblem says what is wrong with one folder row, keyed by its old path; '' when fine or unset. */
export function folderProblem(from: string, pick: string | undefined, other: string | undefined): string {
	if (!pick) return '';
	if (pick === OTHER && !(other ?? '').trim()) return 'Type the path here, or pick a Library folder.';
	return ruleProblem({ from, to: folderTo(pick, other) });
}

/** extraProblems checks the free path rules row by row (same index as the rows); an empty row is fine. */
export function extraProblems(rules: PathRule[]): string[] {
	return rules.map((r) => {
		const c = { from: r.from.trim(), to: r.to.trim() };
		return c.from === '' && c.to === '' ? '' : ruleProblem(c);
	});
}

/** rulesFrom builds the path rules sent to the server: folder rows (keyed by old path) first, then the free rules. */
export function rulesFrom(picks: Record<string, string>, others: Record<string, string>, extra: PathRule[]): PathRule[] {
	const out: PathRule[] = [];
	for (const [from, pick] of Object.entries(picks)) {
		const to = folderTo(pick, others[from]);
		if (to) out.push({ from, to });
	}
	for (const r of cleanRules(extra)) if (!out.some((x) => x.from === r.from)) out.push(r);
	return out;
}

/** Step names, in order. */
export type Step = 'read' | 'choose' | 'move' | 'done';
export const steps: { id: Step; word: string; title: string }[] = [
	{ id: 'read', word: 'Read', title: 'Read your apps' },
	{ id: 'choose', word: 'Choose', title: 'What to move' },
	{ id: 'move', word: 'Move', title: 'Move' },
	{ id: 'done', word: 'Done', title: 'Done' }
];

export function stepIndex(s: Step): number {
	return steps.findIndex((x) => x.id === s);
}

/** asStep reads a step from the address, or null. The old Try it step lands on Choose. */
export function asStep(v: string | null): Step | null {
	if (v === 'try') return 'choose';
	return steps.some((s) => s.id === v) ? (v as Step) : null;
}

export interface GateState {
	step: Step;
	/** Reads switched on. */
	picked: number;
	planning?: boolean;
	planned?: boolean;
	folderErrors?: number;
	/** Path rules with a problem. */
	ruleErrors?: number;
	/** Groups with items that are switched on. */
	groupsOn?: number;
	/** Groups with items at all. */
	groupsWithItems?: number;
	trying?: boolean;
	tried?: boolean;
	/** The last test run failed. */
	failed?: boolean;
	stale?: boolean;
	/** People still waiting for Same person or Leave out. */
	openChoices?: number;
	moving?: boolean;
	safeMode?: boolean;
}

/** stepGate says whether the step's one primary button may go, and the reason shown next to it. */
export function stepGate(s: GateState): { ok: boolean; reason: string } {
	switch (s.step) {
		case 'read':
			return s.picked > 0 ? { ok: true, reason: '' } : { ok: false, reason: 'Turn on at least one read.' };
		case 'choose':
			if (s.picked === 0) return { ok: false, reason: 'Turn on at least one read.' };
			if (s.planning || !s.planned) return { ok: false, reason: 'Updating…' };
			if ((s.folderErrors ?? 0) > 0) return { ok: false, reason: 'Fix the folder marked above.' };
			if ((s.ruleErrors ?? 0) > 0) return { ok: false, reason: 'Fix the path rule marked above.' };
			if ((s.groupsWithItems ?? 0) === 0) return { ok: false, reason: 'Nothing new to move.' };
			if ((s.groupsOn ?? 0) === 0) return { ok: false, reason: 'Turn on at least one group.' };
			return { ok: true, reason: '' };
		case 'move': {
			if (s.moving) return { ok: false, reason: '' };
			if (s.trying) return { ok: false, reason: s.tried ? 'Updating…' : 'Trying the move…' };
			if (s.failed) return { ok: false, reason: 'The test run failed.' };
			if (s.stale || !s.tried) return { ok: false, reason: 'Changed since the test.' };
			if (s.step === 'move' && s.safeMode) return { ok: false, reason: 'HolyArrs started in safe mode.' };
			const open = s.openChoices ?? 0;
			if (open > 0) return { ok: true, reason: open === 1 ? '1 still needs a choice. It stays behind if you move now.' : `${open} still need a choice. They stay behind if you move now.` };
			return { ok: true, reason: '' };
		}
	}
	return { ok: true, reason: '' };
}

/** plural says n and a word, with the plural when n is not 1. */
export function plural(n: number, one: string, many = one + 's'): string {
	return `${n} ${n === 1 ? one : many}`;
}
