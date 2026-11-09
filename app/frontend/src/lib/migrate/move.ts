export type AppKind = (typeof apps)[number]['kind'];

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

export interface Bucket {
	group: string;
	label: string;
	lines: Line[];
	counts: Record<string, number>;
}

export type Step = 'read' | 'choose' | 'move' | 'done';

export interface GateState {
	step: Step;
	picked: number;
	planning?: boolean;
	planned?: boolean;
	folderErrors?: number;
	ruleErrors?: number;
	groupsOn?: number;
	groupsWithItems?: number;
	trying?: boolean;
	tried?: boolean;
	failed?: boolean;
	stale?: boolean;
	openChoices?: number;
	moving?: boolean;
	safeMode?: boolean;
}

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

export const groupOrder = ['folders', 'pick_rules', 'titles', 'never_add', 'outside_lists', 'sources', 'downloaders', 'naming', 'subtitles', 'people', 'wishlist', 'alerts'];

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

const resultOrder = ['create', 'created', 'adopt', 'adopted', 'change', 'changed', 'matched', 'already', 'removed', 'kept', 'skipped', 'not_found', 'failed'];

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

export const OTHER = '\u0000other';

export const steps: { id: Step; word: string; title: string }[] = [
	{ id: 'read', word: 'Read', title: 'Read your apps' },
	{ id: 'choose', word: 'Choose', title: 'What to move' },
	{ id: 'move', word: 'Move', title: 'Move' },
	{ id: 'done', word: 'Done', title: 'Done' }
];

export function appName(kind: string): string {
	return apps.find((a) => a.kind === kind)?.name ?? kind;
}

export function placeholderFor(kind: string): string {
	return `http://192.168.1.10:${apps.find((a) => a.kind === kind)?.port ?? 8080}`;
}

export function running(m: Move): boolean {
	return m.state === 'queued' || m.state === 'running' || m.undo_state === 'queued' || m.undo_state === 'running';
}

export function chooseHint(why: ChooseWhy | string | undefined): string {
	switch (why) {
		case 'numeric_id':
			return "HolyArrs can't tell if this is the same person.";
		case 'merge_gone':
			return 'The person you picked is no longer here.';
	}
	return 'Someone here has the same name. Until you choose, they stay behind and their requests become yours.';
}

export function setChoice(people: Record<string, PersonChoice>, key: string, c: PersonChoice | null): Record<string, PersonChoice> {
	const out = { ...people };
	if (c === null) delete out[key];
	else out[key] = c;
	return out;
}

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

export function needsLook(result: string): boolean {
	return result === 'failed' || result === 'not_found' || result === 'skipped' || result === 'kept';
}

export function summary(counts: Record<string, number>, after = false, undo = false): string {
	return Object.entries(counts)
		.filter(([, n]) => n > 0)
		.sort(([a], [b]) => order(a) - order(b))
		.map(([k, n]) => `${n} ${resultWords(k, after, undo).toLowerCase()}`)
		.join(' · ');
}

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

export function moveTotal(counts: Record<string, number>): number {
	let n = 0;
	for (const k of ['create', 'created', 'adopt', 'adopted', 'change', 'changed', 'matched']) n += counts[k] ?? 0;
	return n;
}

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

export function countWord(key: string, n: number): string {
	const w = countNames[key];
	if (!w || !(n > 0)) return '';
	return `${n} ${n === 1 ? w[0] : w[1]}`;
}

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

export function countWords(kind: string, counts: Record<string, number>, max = 3): string {
	const known = Object.entries(counts).filter(([k, n]) => countNames[k] && n > 0);
	const main = mainCount[kind];
	known.sort(([a, x], [b, y]) => (a === main ? -1 : b === main ? 1 : y - x || a.localeCompare(b)));
	const shown = known.slice(0, max).map(([k, n]) => countWord(k, n));
	const rest = known.slice(max).reduce((s, [, n]) => s + n, 0);
	if (rest > 0) return `${shown.join(', ')} and ${rest} more`;
	return shown.join(', ');
}

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

export function moveCounts(m: Pick<Move, 'state' | 'report'>): string {
	const c = m.report?.counts ?? {};
	return summary(c, true, m.state === 'undone' || !!m.report?.undo);
}

export function cleanRules(rules: PathRule[]): PathRule[] {
	return rules.map((r) => ({ from: r.from.trim(), to: r.to.trim() })).filter((r) => r.from !== '' || r.to !== '');
}

export function ruleProblem(r: PathRule): string {
	const full = (p: string) => p.startsWith('/') || /^[A-Za-z]:[\\/]/.test(p);
	if (!r.from || !r.to) return 'Fill in both paths.';
	if (!full(r.from) || !full(r.to)) return 'Both must be full paths, such as /movies.';
	if (/(^|[\\/])\.\.([\\/]|$)/.test(r.from + '/' + r.to)) return `Paths can't contain "..".`;
	return '';
}

export function folderTo(pick: string | undefined, other: string | undefined): string {
	if (!pick) return '';
	return pick === OTHER ? (other ?? '').trim() : pick;
}

export function folderProblem(from: string, pick: string | undefined, other: string | undefined): string {
	if (!pick) return '';
	if (pick === OTHER && !(other ?? '').trim()) return 'Type the path here, or pick a Library folder.';
	return ruleProblem({ from, to: folderTo(pick, other) });
}

export function extraProblems(rules: PathRule[]): string[] {
	return rules.map((r) => {
		const c = { from: r.from.trim(), to: r.to.trim() };
		return c.from === '' && c.to === '' ? '' : ruleProblem(c);
	});
}

export function rulesFrom(picks: Record<string, string>, others: Record<string, string>, extra: PathRule[]): PathRule[] {
	const out: PathRule[] = [];
	for (const [from, pick] of Object.entries(picks)) {
		const to = folderTo(pick, others[from]);
		if (to) out.push({ from, to });
	}
	for (const r of cleanRules(extra)) if (!out.some((x) => x.from === r.from)) out.push(r);
	return out;
}

export function stepIndex(s: Step): number {
	return steps.findIndex((x) => x.id === s);
}

export function asStep(v: string | null): Step | null {
	if (v === 'try') return 'choose';
	return steps.some((s) => s.id === v) ? (v as Step) : null;
}

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

export function plural(n: number, one: string, many = one + 's'): string {
	return `${n} ${n === 1 ? one : many}`;
}

function order(result: string): number {
	const i = resultOrder.indexOf(result);
	return i < 0 ? resultOrder.length : i;
}
