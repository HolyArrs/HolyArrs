// Display helpers that turn raw numbers, dates and states into short text.

export type Tone = 'ok' | 'wait' | 'fail' | 'idle';

// Which tone each backend state shows as; unlisted states read as idle.
const okStates = ['in_library', 'finished', 'done', 'chosen', 'placed', 'enabled', 'ok'];
const failStates = ['failed', 'stalled', 'vanished', 'refused', 'rejected', 'needs_a_decision', 'partly_done', 'missing',
	'read_only', 'unreachable', 'auth_failed', 'disabled'];
const waitStates = ['searching', 'downloading', 'importing', 'queued', 'sent', 'held', 'placing', 'running', 'paused',
	'no_hardlink', 'wanted', 'not_out_yet', 'better_wanted', 'unknown'];

export function bytes(n: number | null | undefined): string {
	if (!n || n < 0) return '0 B';
	const u = ['B', 'KB', 'MB', 'GB', 'TB'];
	let i = 0;
	let v = n;
	while (v >= 1024 && i < u.length - 1) {
		v /= 1024;
		i++;
	}
	return (i === 0 ? v.toFixed(0) : v.toFixed(v < 10 ? 1 : 0)) + ' ' + u[i];
}

export function duration(seconds: number | null | undefined): string {
	if (seconds == null || seconds < 0) return '';
	if (seconds < 60) return `${Math.round(seconds)}s`;
	const m = Math.floor(seconds / 60);
	if (m < 60) return `${m}m`;
	const h = Math.floor(m / 60);
	if (h < 48) return `${h}h ${m % 60}m`;
	return `${Math.floor(h / 24)}d ${h % 24}h`;
}

export function percent(p: number | null | undefined): number {
	if (!p || p < 0) return 0;
	return Math.min(100, Math.round(p * 1000) / 10);
}

// Date-only values are pinned to UTC so they do not slip a day in other zones.
export function date(s: string | null | undefined): string {
	if (!s) return '';
	const d = new Date(s);
	if (Number.isNaN(d.getTime())) return '';
	const day = /^\d{4}-\d{2}-\d{2}(T00:00:00(\.0+)?Z)?$/.test(s);
	return d.toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric', ...(day ? { timeZone: 'UTC' } : {}) });
}

export function dateTime(s: string | null | undefined): string {
	if (!s) return '';
	const d = new Date(s);
	if (Number.isNaN(d.getTime())) return '';
	return d.toLocaleString(undefined, { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' });
}

// Under 45 seconds reads as now so counts do not flicker.
export function ago(s: string | null | undefined, now = Date.now()): string {
	if (!s) return '';
	const t = new Date(s).getTime();
	if (Number.isNaN(t)) return '';
	const sec = Math.max(0, Math.round((now - t) / 1000));
	if (sec < 45) return 'just now';
	if (sec < 3600) return `${Math.round(sec / 60)} min ago`;
	if (sec < 86400) return `${Math.round(sec / 3600)} h ago`;
	return `${Math.round(sec / 86400)} d ago`;
}

// Spelled out for screen readers and wide layouts.
export function agoLong(s: string | null | undefined, now = Date.now()): string {
	return ago(s, now).replace(/^(\d+) (d|h|min) ago$/, (_, n: string, u: string) => `${n} ${{ d: 'day', h: 'hour', min: 'minute' }[u]}${n === '1' ? '' : 's'} ago`);
}

// Maps a state to ok, wait, fail or idle for colour.
export function tone(state: string | null | undefined): Tone {
	if (!state) return 'idle';
	if (okStates.includes(state)) return 'ok';
	if (failStates.includes(state)) return 'fail';
	if (waitStates.includes(state)) return 'wait';
	return 'idle';
}

export function words(state: string | null | undefined): string {
	if (!state) return '';
	const s = state.replace(/_/g, ' ');
	return s.charAt(0).toUpperCase() + s.slice(1);
}

export function greeting(h = new Date().getHours()): string {
	if (h < 5) return 'Good night';
	if (h < 12) return 'Good morning';
	if (h < 18) return 'Good afternoon';
	return 'Good evening';
}

export function folderHealth(health: string): string {
	switch (health) {
		case 'ok':
			return 'Healthy';
		case 'missing':
			return 'Folder not found';
		case 'read_only':
			return 'Cannot write here';
		case 'no_hardlink':
			return 'Works, but files are copied (no hardlinks)';
		default:
			return 'Not checked yet';
	}
}
