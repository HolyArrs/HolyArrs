// The move from the old apps: read them, see the plan, try it, move.
// Plain helpers for Settings > Move from other apps.

export const apps = [
	{ kind: 'radarr', name: 'Radarr', what: 'movies' },
	{ kind: 'sonarr', name: 'Sonarr', what: 'shows' },
	{ kind: 'lidarr', name: 'Lidarr', what: 'music' },
	{ kind: 'prowlarr', name: 'Prowlarr', what: 'Sources' },
	{ kind: 'bazarr', name: 'Bazarr', what: 'subtitles' },
	{ kind: 'seerr', name: 'Seerr', what: 'people and asks' },
	{ kind: 'overseerr', name: 'Overseerr', what: 'people and asks' },
	{ kind: 'qbittorrent', name: 'qBittorrent', what: 'a Downloader' },
	{ kind: 'sabnzbd', name: 'SABnzbd', what: 'a Downloader' }
] as const;

export type AppKind = (typeof apps)[number]['kind'];

export function appName(kind: string): string {
	return apps.find((a) => a.kind === kind)?.name ?? kind;
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

export interface Line {
	group: string;
	key: string;
	name: string;
	result: string;
	detail?: string;
	needs?: string;
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
	state: 'queued' | 'running' | 'done' | 'failed' | 'undone';
	job_id?: number;
	report?: Report;
	finished_at?: string;
	undone_at?: string;
	undo_state?: 'queued' | 'running' | 'failed';
	undo_job_id?: number;
}

/** running says a move or its undo is still going. */
export function running(m: Move): boolean {
	return m.state === 'queued' || m.state === 'running' || m.undo_state === 'queued' || m.undo_state === 'running';
}

/** sameName says a dry-run line left a person out because someone here has their name. */
export function sameName(l: Line): boolean {
	return l.group === 'people' && l.result === 'skipped' && (l.detail ?? '').includes('same name');
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

/** resultWords say each result in plain words. */
export const resultWords: Record<string, string> = {
	create: 'Will be added',
	created: 'Added',
	adopt: 'Will be adopted in place',
	adopted: 'Adopted in place',
	change: 'Will change',
	changed: 'Changed',
	matched: 'Matched',
	already: 'Already here',
	skipped: 'Left out',
	not_found: 'Not found here',
	failed: 'Failed',
	removed: 'Undone',
	kept: 'Kept'
};

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

/** linesByGroup keeps the report's lines in plan order, per group. */
export function linesByGroup(r: Report): { group: string; lines: Line[]; counts: Record<string, number> }[] {
	const out: { group: string; lines: Line[]; counts: Record<string, number> }[] = [];
	for (const l of r.lines) {
		let g = out.find((x) => x.group === l.group);
		if (!g) {
			g = { group: l.group, lines: [], counts: {} };
			out.push(g);
		}
		g.lines.push(l);
		g.counts[l.result] = (g.counts[l.result] ?? 0) + 1;
	}
	return out;
}

/** summary says a report's counts in one line ("3 Added, 1 Already here"). */
export function summary(counts: Record<string, number>): string {
	const order = Object.keys(resultWords);
	return Object.entries(counts)
		.filter(([, n]) => n > 0)
		.sort(([a], [b]) => order.indexOf(a) - order.indexOf(b))
		.map(([k, n]) => `${n} ${(resultWords[k] ?? k).toLowerCase()}`)
		.join(', ');
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
	if (/(^|[\\/])\.\.([\\/]|$)/.test(r.from + '/' + r.to)) return 'Paths may not hold "..".';
	return '';
}

/** addSuggestions adds suggested rules whose old path has no rule yet. */
export function addSuggestions(rules: PathRule[], suggested: PathRule[]): PathRule[] {
	const out = cleanRules(rules);
	for (const s of suggested) {
		if (!out.some((r) => r.from === s.from)) out.push({ ...s });
	}
	return out;
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
