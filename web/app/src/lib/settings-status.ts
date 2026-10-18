// What needs attention in Settings, worked out only from data the app already
// loads: health checks, stage and linked-app states, the lists of Library
// folders, Sources and Downloaders, the newest backup and the TMDB key.
import type { GroupId } from './settings.ts';
import type { HealthCheck, StageState } from './types.ts';

export interface StatusInput {
	checks?: HealthCheck[] | null;
	states?: StageState[] | null;
	folders?: number | null;
	sources?: { name: string; state: string }[] | null;
	downloaders?: { name: string; state: string }[] | null;
	/** created_at of the newest backup; '' when there is none; null when not known. */
	newestBackup?: string | null;
	/** true when a TMDB key is saved; null when not known (the page needs a recent password). */
	tmdbKey?: boolean | null;
	updateNewer?: boolean | null;
	now?: number;
}

export interface Attention {
	text: string;
	href: string;
	group: GroupId;
}

export type GroupState = 'ok' | 'attention' | 'unset' | 'unknown';

export interface StatusAnswer {
	items: Attention[];
	unset: Partial<Record<GroupId, string[]>>;
	/** Pages whose status is known: true when they are fine. */
	known: Set<string>;
}

const DAY = 86_400_000;

const checkHref: Record<HealthCheck['kind'], { href: string; group: GroupId }> = {
	library_folder: { href: '/settings/library-folders', group: 'library' },
	disk_space: { href: '/settings/library-folders', group: 'library' },
	source: { href: '/settings/sources', group: 'copies' },
	downloader: { href: '/settings/downloaders', group: 'copies' },
	backups: { href: '/settings/backups', group: 'maintenance' },
	update: { href: '/settings/this-app', group: 'general' }
};

const stateHref: Partial<Record<StageState['kind'], { href: string; group: GroupId; what: string }>> = {
	media_server: { href: '/settings/media-servers', group: 'linked', what: 'media server' },
	subtitle_site: { href: '/settings/subtitles', group: 'library', what: 'subtitle site' },
	stage: { href: '/settings/parts', group: 'general', what: 'stage' }
};

const badSource = new Set(['unreachable', 'auth_failed']);
const badDownloader = new Set(['not_answering', 'sign_in_refused']);

/** settingsStatus lists what needs a fix and which groups are not set up yet. */
export function settingsStatus(i: StatusInput): StatusAnswer {
	const items: Attention[] = [];
	const unset: StatusAnswer['unset'] = {};
	const known = new Set<string>();
	const seen = new Set<string>();
	const add = (a: Attention, key: string) => {
		if (seen.has(key)) return;
		seen.add(key);
		items.push(a);
	};
	const now = i.now ?? Date.now();

	for (const c of i.checks ?? []) {
		const to = checkHref[c.kind];
		if (!to) continue;
		known.add(to.href);
		if (c.state === 'ok') continue;
		const subject = c.subject ? `${c.subject}: ` : '';
		add({ text: `${subject}${c.message || 'needs a look'}`, href: to.href, group: to.group }, `${c.kind}:${c.subject}`);
	}
	for (const s of i.states ?? []) {
		const to = stateHref[s.kind];
		if (!to) continue;
		known.add(to.href);
		if (s.state !== 'failed' && !s.stuck) continue;
		add({ text: `The ${to.what} ${s.name} ${s.stuck ? 'is stuck' : 'failed'}`, href: to.href, group: to.group }, `state:${s.key}`);
	}
	if (i.sources) {
		known.add('/settings/sources');
		for (const s of i.sources) if (badSource.has(s.state)) add({ text: `The Source ${s.name} is not answering`, href: '/settings/sources', group: 'copies' }, `source:${s.name}`);
	}
	if (i.downloaders) {
		known.add('/settings/downloaders');
		for (const d of i.downloaders) if (badDownloader.has(d.state)) add({ text: `The Downloader ${d.name} is not answering`, href: '/settings/downloaders', group: 'copies' }, `downloader:${d.name}`);
	}
	if (i.tmdbKey === false) add({ text: 'No TMDB key yet, so names and posters cannot be found', href: '/settings/tmdb', group: 'library' }, 'tmdb');
	if (i.tmdbKey !== null && i.tmdbKey !== undefined) known.add('/settings/tmdb');
	if (i.newestBackup !== null && i.newestBackup !== undefined) {
		known.add('/settings/backups');
		const at = i.newestBackup ? Date.parse(i.newestBackup) : NaN;
		if (!i.newestBackup) add({ text: 'No backup yet', href: '/settings/backups', group: 'maintenance' }, 'backup-age');
		else if (!Number.isNaN(at) && now - at > 7 * DAY) add({ text: 'No backup in the last 7 days', href: '/settings/backups', group: 'maintenance' }, 'backup-age');
	}
	if (i.updateNewer) {
		known.add('/settings/this-app');
		add({ text: 'A newer HolyArrs is out', href: '/settings/this-app', group: 'general' }, 'update:');
	}

	if (i.folders === 0) unset.library = ['/settings/library-folders'];
	if (i.folders != null) known.add('/settings/library-folders');
	const copies: string[] = [];
	if (i.sources && i.sources.length === 0) copies.push('/settings/sources');
	if (i.downloaders && i.downloaders.length === 0) copies.push('/settings/downloaders');
	if (copies.length) unset.copies = copies;
	return { items, unset, known };
}

/** groupState sums up one group: something to fix, not set up, all set, or not known. */
export function groupState(a: StatusAnswer, group: GroupId, hrefs: string[]): { state: GroupState; count: number } {
	const count = a.items.filter((i) => i.group === group).length;
	if (count) return { state: 'attention', count };
	if (a.unset[group]?.length) return { state: 'unset', count: 0 };
	if (hrefs.some((h) => a.known.has(h))) return { state: 'ok', count: 0 };
	return { state: 'unknown', count: 0 };
}

/** pageState sums up one page the same way. */
export function pageState(a: StatusAnswer, href: string): { state: GroupState; count: number } {
	const count = a.items.filter((i) => i.href === href).length;
	if (count) return { state: 'attention', count };
	if (Object.values(a.unset).some((hs) => hs?.includes(href))) return { state: 'unset', count: 0 };
	if (a.known.has(href)) return { state: 'ok', count: 0 };
	return { state: 'unknown', count: 0 };
}

/** stateWords says a status in plain words. */
export function stateWords(s: { state: GroupState; count: number }): string {
	if (s.state === 'attention') return s.count === 1 ? '1 needs attention' : `${s.count} need attention`;
	if (s.state === 'unset') return 'Not set up';
	if (s.state === 'ok') return 'All set';
	return '';
}
