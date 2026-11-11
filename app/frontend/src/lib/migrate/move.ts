import { t } from '#lib/core/i18n/i18n.svelte.ts';
import { serverWords } from '#lib/core/i18n/server-words.ts';
export type AppKind = (typeof apps)[number]['kind'];

export interface Snapshot {
	id: number;
	created_at: string;
	kind: string;
	via: 'api' | 'backup';
	/** The app's address for a read over its API; '' for a backup. */
	address?: string;
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

/** The name of a group of settings, in the person's language; undefined for a group HolyArrs has no name for. */
export function groupLabel(group: string): string | undefined {
	const labels: Record<string, string> = {
		folders: t('migrate.libraryFolders'),
		pick_rules: t('migrate.pickRules'),
		titles: t('migrate.titles'),
		never_add: t('migrate.neverAddList'),
		outside_lists: t('migrate.outsideLists'),
		sources: t('migrate.sources'),
		downloaders: t('migrate.downloaders'),
		naming: t('migrate.naming'),
		subtitles: t('migrate.subtitles'),
		people: t('migrate.people'),
		wishlist: t('migrate.requests'),
		alerts: t('migrate.alerts'),
		other: t('migrate.otherSettings')
	};
	return labels[group];
}

export const groupOrder = ['folders', 'pick_rules', 'titles', 'never_add', 'outside_lists', 'sources', 'downloaders', 'naming', 'subtitles', 'people', 'wishlist', 'alerts'];

const words = (): Record<string, [string, string]> => ({
	create: [t('migrate.willBeAdded'), t('migrate.added')],
	adopt: [t('migrate.willUseTheOne'), t('migrate.usingTheOneHere')],
	change: [t('migrate.willBeUpdated'), t('migrate.updated')],
	matched: [t('migrate.matchesOneHere'), t('migrate.matched')],
	already: [t('migrate.alreadyHere'), t('migrate.alreadyHere')],
	skipped: [t('migrate.leftOut'), t('migrate.leftOut')],
	not_found: [t('migrate.noFolderHere'), t('migrate.noFolderHere')],
	failed: [t('migrate.wonTWork'), t('migrate.failed')]
});

const pastToBase: Record<string, string> = { created: 'create', adopted: 'adopt', changed: 'change' };

const resultOrder = ['create', 'created', 'adopt', 'adopted', 'change', 'changed', 'matched', 'already', 'removed', 'kept', 'skipped', 'not_found', 'failed'];

// Each entry is the message keys of the one form and the many form; both carry {n}.
const countNames: Record<string, [string, string]> = {
	titles: ['migrate.title', 'migrate.titles2'],
	folders: ['migrate.libraryFolder', 'migrate.libraryFolders2'],
	quality_profiles: ['migrate.setOfPickRules', 'migrate.setsOfPickRules'],
	custom_formats: ['migrate.trait', 'migrate.traits'],
	delay_profiles: ['migrate.waitRule', 'migrate.waitRules'],
	indexers: ['migrate.source', 'migrate.sources2'],
	download_clients: ['migrate.downloader', 'migrate.downloaders2'],
	import_lists: ['migrate.outsideList', 'migrate.outsideLists2'],
	notifications: ['migrate.alert', 'migrate.alerts2'],
	app_links: ['migrate.linkedApp', 'migrate.linkedApps'],
	users: ['migrate.person', 'migrate.people2'],
	requests: ['migrate.request', 'migrate.requests2'],
	tags: ['migrate.tag', 'migrate.tags'],
	subtitle_languages: ['migrate.subtitleLanguage', 'migrate.subtitleLanguages'],
	subtitle_profiles: ['migrate.subtitleLanguageSet', 'migrate.subtitleLanguageSets'],
	subtitle_sites: ['migrate.subtitleSite', 'migrate.subtitleSites'],
	needs_reentry: ['migrate.keyToTypeAgain', 'migrate.keysToTypeAgain']
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

const titleWord: Record<string, [string, string]> = { radarr: ['migrate.movie', 'migrate.movies'], sonarr: ['migrate.show', 'migrate.shows'], lidarr: ['migrate.artist', 'migrate.artists'] };

export const OTHER = '\u0000other';

const stepIds: Step[] = ['read', 'choose', 'move', 'done'];

export const steps = (): { id: Step; word: string; title: string }[] => [
	{ id: 'read', word: t('migrate.read'), title: t('migrate.readYourApps') },
	{ id: 'choose', word: t('migrate.choose'), title: t('migrate.whatToMove') },
	{ id: 'move', word: t('migrate.move'), title: t('migrate.move') },
	{ id: 'done', word: t('migrate.done'), title: t('migrate.done') }
];

export function appName(kind: string): string {
	return apps.find((a) => a.kind === kind)?.name ?? kind;
}

/** One copy of an old app: its kind and the address it was read from. */
export function instanceKey(s: Pick<Snapshot, 'kind' | 'address'>): string {
	return `${s.kind} ${s.address ?? ''}`;
}

/** The newest read of each copy (reads come newest first). */
export function latestPerInstance<T extends Pick<Snapshot, 'kind' | 'address'>>(snaps: T[]): T[] {
	const seen = new Set<string>();
	return snaps.filter((s) => {
		const k = instanceKey(s);
		if (seen.has(k)) return false;
		seen.add(k);
		return true;
	});
}

/** Host and port of an address, to tell two copies of one app apart. */
export function hostOf(address: string): string {
	try {
		return new URL(address).host || address;
	} catch {
		return address;
	}
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
			return t('migrate.holyarrsCanTTell');
		case 'merge_gone':
			return t('migrate.thePersonYouPicked');
	}
	return t('migrate.someoneHereHasThe');
}

export function setChoice(people: Record<string, PersonChoice>, key: string, c: PersonChoice | null): Record<string, PersonChoice> {
	const out = { ...people };
	if (c === null) delete out[key];
	else out[key] = c;
	return out;
}

export function resultWords(result: string, after = false, undo = false): string {
	if (undo) {
		if (result === 'removed') return t('migrate.removed');
		if (result === 'kept') return t('migrate.kept');
		if (result === 'failed') return t('migrate.notRemoved');
	}
	if (result === 'removed') return t('migrate.removed');
	if (result === 'kept') return t('migrate.kept');
	const w = words()[pastToBase[result] ?? result];
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

/** The server's "3 of 10 imported" line as "3 of 10", in the picked language. */
export function progressWords(note: string): string {
	const m = /^(\d+) of (\d+)\s+(?:imported|undone)\.?$/.exec(note);
	return m ? t('migrate.doneOfTotal', { done: m[1], total: m[2] }) : note;
}

/** A note from a read app ("sonarr: the app has no tag list; skipped") in the picked language. */
export function noteWords(note: string): string {
	const whole = serverWords(note);
	if (whole !== note) return whole;
	const m = /^([a-z]+): ([\s\S]+)$/.exec(note);
	return m ? `${m[1]}: ${serverWords(m[2])}` : note;
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
			g = { group: l.group, label: groupLabel(l.group) ?? l.group, lines: [], counts: {} };
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
		const label = head ? (apps.some((a) => a.kind === head) ? appName(head) : head) : t('migrate.other');
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
	return t(n === 1 ? w[0] : w[1], { n });
}

export function bestStats(kind: string, counts: Record<string, number>): string {
	return (bestKeys[kind] ?? [])
		.map((k) => {
			const n = counts[k] ?? 0;
			if (!(n > 0)) return '';
			const w = k === 'titles' && titleWord[kind] ? titleWord[kind] : countNames[k];
			return w ? t(n === 1 ? w[0] : w[1], { n }) : '';
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
	if (rest > 0) return t('migrate.andMore', { list: shown.join(', '), n: rest });
	return shown.join(', ');
}

export function moveStateWord(m: Pick<Move, 'state' | 'undo_state'>): { word: string; tone: 'ok' | 'info' | 'wait' | 'fail' | 'idle' } {
	if (m.undo_state === 'queued' || m.undo_state === 'running') return { word: t('migrate.undoing'), tone: 'info' };
	if (m.undo_state === 'failed' && m.state !== 'undone') return { word: t('migrate.undoFailed'), tone: 'fail' };
	switch (m.state) {
		case 'queued':
			return { word: t('migrate.waiting'), tone: 'wait' };
		case 'running':
			return { word: t('migrate.moving'), tone: 'info' };
		case 'done':
			return { word: t('migrate.moved'), tone: 'ok' };
		case 'failed':
			return { word: t('migrate.stoppedWithErrors'), tone: 'fail' };
		case 'undone':
			return { word: t('migrate.undone'), tone: 'idle' };
	}
	return { word: t('migrate.waiting'), tone: 'wait' };
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
	if (!r.from || !r.to) return t('migrate.fillInBothPaths');
	if (!full(r.from) || !full(r.to)) return t('migrate.bothMustBeFull');
	if (/(^|[\\/])\.\.([\\/]|$)/.test(r.from + '/' + r.to)) return t('migrate.pathsCanTContain');
	return '';
}

export function folderTo(pick: string | undefined, other: string | undefined): string {
	if (!pick) return '';
	return pick === OTHER ? (other ?? '').trim() : pick;
}

// A path written the way the server keeps it: forward slashes, no doubled
// or trailing slash, so two spellings of one folder compare equal.
export function cleanPath(p: string): string {
	const c = p.trim().replace(/\\/g, '/').replace(/\/{2,}/g, '/');
	return c.length > 1 ? c.replace(/\/$/, '') : c;
}

export function folderProblem(from: string, pick: string | undefined, other: string | undefined): string {
	if (!pick) return '';
	if (pick === OTHER && !(other ?? '').trim()) return t('migrate.typeThePathHere');
	return ruleProblem({ from, to: folderTo(pick, other) });
}

// For each old folder, the other old folders that point at the same new folder.
export function sharedTargets(picks: Record<string, string>, others: Record<string, string>): Record<string, string[]> {
	const by = new Map<string, string[]>();
	for (const [from, pick] of Object.entries(picks)) {
		const to = cleanPath(folderTo(pick, others[from]));
		if (to) by.set(to, [...(by.get(to) ?? []), from]);
	}
	const out: Record<string, string[]> = {};
	for (const froms of by.values()) if (froms.length > 1) for (const f of froms) out[f] = froms.filter((x) => x !== f);
	return out;
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
	return stepIds.indexOf(s);
}

export function asStep(v: string | null): Step | null {
	if (v === 'try') return 'choose';
	return stepIds.includes(v as Step) ? (v as Step) : null;
}

export function stepGate(s: GateState): { ok: boolean; reason: string } {
	switch (s.step) {
		case 'read':
			return s.picked > 0 ? { ok: true, reason: '' } : { ok: false, reason: t('migrate.turnOnAtLeast') };
		case 'choose':
			if (s.picked === 0) return { ok: false, reason: t('migrate.turnOnAtLeast') };
			if (s.planning || !s.planned) return { ok: false, reason: t('migrate.updating') };
			if ((s.folderErrors ?? 0) > 0) return { ok: false, reason: t('migrate.fixTheFolderMarked') };
			if ((s.ruleErrors ?? 0) > 0) return { ok: false, reason: t('migrate.fixThePathRule') };
			if ((s.groupsWithItems ?? 0) === 0) return { ok: false, reason: t('migrate.nothingNewToMove') };
			if ((s.groupsOn ?? 0) === 0) return { ok: false, reason: t('migrate.turnOnAtLeast2') };
			return { ok: true, reason: '' };
		case 'move': {
			if (s.moving) return { ok: false, reason: '' };
			if (s.trying) return { ok: false, reason: s.tried ? t('migrate.updating') : t('migrate.tryingTheMove') };
			if (s.failed) return { ok: false, reason: t('migrate.theTestRunFailed') };
			if (s.stale || !s.tried) return { ok: false, reason: t('migrate.changedSinceTheTest') };
			if (s.step === 'move' && s.safeMode) return { ok: false, reason: t('migrate.holyarrsStartedInSafe') };
			const open = s.openChoices ?? 0;
			if (open > 0) return { ok: true, reason: open === 1 ? t('migrate.n1StillNeedsA') : t('migrate.stillNeedAChoice', { n: open }) };
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
