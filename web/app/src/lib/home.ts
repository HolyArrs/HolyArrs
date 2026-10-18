// Home: small pure helpers so the page stays a layout.
import { duration } from './format.ts';
import { healthSentence, looksLikeId, plainReason } from './plain.ts';
import type { DownloadRow } from './downloaders.ts';
import type { HealthCheck } from './types.ts';

/**
 * greetName is the name to greet someone by, or '' when the name looks like
 * an id (preview-…, e2e-…, a long hex or number string) and not a person.
 */
export function greetName(name: string | null | undefined): string {
	return looksLikeId(name) ? '' : (name ?? '').trim();
}

/** downloadLine is the meta line under a download card: percent and time left. */
export function downloadLine(d: Pick<DownloadRow, 'progress' | 'eta' | 'state' | 'label'>): string {
	if (d.state === 'queued') return 'Waiting to start';
	if (d.state === 'stalled') return 'Stalled';
	const pct = Math.round(Math.max(0, Math.min(1, d.progress)) * 100);
	return d.eta ? `${pct}% · ${duration(d.eta)} left` : `${pct}%`;
}

/** downloadSubtitle names what a download covers: an episode range, an album or the version. */
export function downloadSubtitle(d: Pick<DownloadRow, 'covers' | 'album' | 'year' | 'version'>): string {
	if (d.covers) return d.covers;
	if (d.album) return d.album;
	return [d.year, d.version && d.version !== 'Main' ? d.version : ''].filter(Boolean).join(' · ');
}

/** isActive says whether a download is still in progress. */
export function isActive(d: Pick<DownloadRow, 'active' | 'state'>): boolean {
	return d.active || d.state === 'downloading' || d.state === 'queued' || d.state === 'stalled';
}

export interface Attention {
	key: string;
	tone: 'fail' | 'wait';
	word: string;
	text: string;
	/** Raw text for admins, shown inside a Details disclosure. */
	detail?: string;
	href: string;
	action: string;
}

/** healthFix is where a health problem is fixed. */
export function healthFix(kind: HealthCheck['kind']): string {
	switch (kind) {
		case 'library_folder':
		case 'disk_space':
			return '/settings/library-folders';
		case 'source':
			return '/settings/sources';
		case 'downloader':
			return '/settings/downloaders';
		case 'backups':
			return '/settings/backups';
		default:
			return '/settings/this-app';
	}
}

/** attention puts failed downloads, decisions and health problems in one list, worst first. */
export function attention(input: {
	failed: Pick<DownloadRow, 'id' | 'title' | 'title_id' | 'text'>[];
	decisions: { slot_id: number; title: string; reason?: string; episode?: { label: string } | null }[];
	health: HealthCheck[];
}): Attention[] {
	const out: Attention[] = [];
	for (const c of input.health.filter((h) => h.state === 'error'))
		out.push({ key: `h${c.key}`, tone: 'fail', word: 'Problem', text: healthSentence(c), detail: [c.subject, c.message].filter(Boolean).join(': ') || undefined, href: healthFix(c.kind), action: 'Fix' });
	for (const d of input.failed) {
		const r = plainReason(d.text || 'The download failed.');
		out.push({ key: `f${d.id}`, tone: 'fail', word: 'Failed', text: `${d.title}: ${r.text}`, detail: r.detail, href: '/downloads', action: 'Open Downloads' });
	}
	for (const d of input.decisions) {
		const r = plainReason(d.reason || 'HolyArrs needs you to choose.');
		out.push({ key: `d${d.slot_id}`, tone: 'wait', word: 'Your call', text: `${d.title}${d.episode ? ` ${d.episode.label}` : ''}: ${r.text}`, detail: r.detail, href: '/downloads/decide', action: 'Decide' });
	}
	for (const c of input.health.filter((h) => h.state === 'warning'))
		out.push({ key: `h${c.key}`, tone: 'wait', word: 'Warning', text: healthSentence(c), detail: [c.subject, c.message].filter(Boolean).join(': ') || undefined, href: healthFix(c.kind), action: 'Look' });
	return out;
}
