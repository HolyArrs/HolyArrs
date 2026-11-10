import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';
import { size as bytes } from './size.ts';
import { STEP_WORD } from './usenet.ts';
import { duration, percent } from '../core/format.ts';

/** What the health line needs from a row and the client's detail. */
export type HealthIn = {
	phase?: string;
	state?: string;
	progress?: number;
	speed?: number;
	eta?: number;
	ratio?: number;
	client_hint?: string;
	done_at?: string;
	seeding_time?: number;
};

export type LogLine = { at: string; kind: string; summary: string };

/** One short line under the panel title: what the torrent is doing now. */
export function healthLine(h: HealthIn, now = Date.now()): string {
	const pc = `${Math.floor(percent(h.progress))}%`;
	if (h.client_hint === 'errored' || h.phase === 'failed') return t('downloads.health.failed', { pc });
	if (h.client_hint === 'missing') return t('downloads.health.missing', { pc });
	if (h.client_hint === 'checking') return t('downloads.health.checking', { pc });
	if (h.client_hint === 'moving') return t('downloads.health.moving', { pc });
	// A Usenet job past its download: verifying, repairing, unpacking.
	if (h.client_hint && STEP_WORD[h.client_hint]) return STEP_WORD[h.client_hint];
	const ratio = h.ratio != null ? t('downloads.health.ratio', { ratio: h.ratio.toFixed(2) }) : '';
	if (h.phase === 'done') {
		let since = h.seeding_time ? h.seeding_time : 0;
		if (!since && h.done_at) since = Math.max(0, (now - new Date(h.done_at).getTime()) / 1000);
		const age = since ? daysWord(since) : '';
		const word = h.state === 'paused' ? t('downloads.health.doneStill') : t('downloads.health.seeding');
		return [word, ratio, age].filter(Boolean).join(' · ');
	}
	if (h.phase === 'paused') return t('downloads.health.paused', { pc });
	if (h.phase === 'stalled') return t('downloads.health.stalled', { pc });
	if (h.phase === 'waiting') return t('downloads.health.waiting', { pc });
	const parts = [t('downloads.health.downloading'), pc];
	if (h.speed) parts.push(`${bytes(h.speed)}/s`);
	if (h.eta) parts.push(t('downloads.list.left', { time: duration(h.eta) }));
	return parts.join(' · ');
}

/** A span in plain days, or hours under two days. */
export function daysWord(sec: number): string {
	const d = Math.floor(sec / 86400);
	if (d >= 2) return t('downloads.span.days', { d });
	const h = Math.floor(sec / 3600);
	if (h >= 1) return t('downloads.span.hours', { h });
	return t('downloads.span.min', { m: Math.max(1, Math.floor(sec / 60)) });
}

/** The quiet line shown instead of an empty peers table while seeding. */
export function quietPeers(lastSeen: string | undefined, now = Date.now()): string {
	if (!lastSeen) return t('downloads.quiet.none');
	const sec = Math.max(0, (now - new Date(lastSeen).getTime()) / 1000);
	return t('downloads.quiet.last', { when: sec < 60 ? t('downloads.quiet.justNow') : t('downloads.quiet.ago', { span: daysWord(sec) }) });
}

/** Lines newest first, grouped under a day heading. */
export function logDays(log: LogLine[], now = new Date()): { day: string; lines: LogLine[] }[] {
	const out: { day: string; lines: LogLine[] }[] = [];
	const key = (d: Date) => `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`;
	const today = key(now);
	const y = new Date(now);
	y.setDate(y.getDate() - 1);
	const yday = key(y);
	const sorted = [...log].sort((a, b) => new Date(b.at).getTime() - new Date(a.at).getTime());
	for (const l of sorted) {
		const d = new Date(l.at);
		if (Number.isNaN(d.getTime())) continue;
		const k = key(d);
		const day = k === today ? t('downloads.day.today') : k === yday ? t('downloads.day.yesterday') : d.toLocaleDateString(appLocale(), { day: 'numeric', month: 'short', year: d.getFullYear() === now.getFullYear() ? undefined : 'numeric' });
		const last = out[out.length - 1];
		if (last && last.day === day) last.lines.push(l);
		else out.push({ day, lines: [l] });
	}
	return out;
}

/** Bad lines get the warning dot. */
export const badKind = (kind: string) => /fail|error|stall|vanish|missing|errored/i.test(kind);

/** Short letters and a tone for a Downloader kind. */
export function clientMark(kind: string | undefined): { mark: string; tone: string } {
	switch (kind) {
		case 'qbittorrent': return { mark: 'qB', tone: 'qb' };
		case 'transmission': return { mark: 'Tr', tone: 'tr' };
		case 'deluge': return { mark: 'De', tone: 'de' };
		case 'sabnzbd': return { mark: 'SAB', tone: 'sab' };
		case 'nzbget': return { mark: 'NZB', tone: 'nzb' };
	}
	return { mark: '', tone: '' };
}

/** The whole-swarm count worth showing beside the connected one; none when it is 0 or below what is connected. */
export function swarmTotal(connected: number | undefined, total: number | undefined): number {
	return total && total >= (connected ?? 0) ? total : 0;
}

/** "3 of 40", or just "3" when the swarm count is missing or smaller. */
export function swarmLine(connected: number, total: number): string {
	const all = swarmTotal(connected, total);
	return all ? t('downloads.swarm.of', { connected, total: all }) : `${connected}`;
}
