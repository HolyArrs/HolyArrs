import { api as defaultClient, type Client } from '../core/api.ts';
import { switchPlan, type PartName, type PartsAnswer } from '../mediafiles/index.ts';
import { t } from '../core/i18n/i18n.svelte.ts';

// The server answers a risky control with a confirm id first.
export interface Confirm {
	confirm: string;
	text: string;
	expires_in_s?: number;
}

// Linked app kinds that can be reconnected, with their button word.
export const linkAction: Record<string, string> = {
	get downloader() { return t('setup.reconnect'); },
	get source() { return t('setup.testNow'); },
	get media_server() { return t('setup.testNow'); },
	get subtitle_site() { return t('setup.testNow'); }
};

export function isConfirm(r: unknown): r is Confirm {
	return !!r && typeof r === 'object' && typeof (r as Confirm).confirm === 'string';
}

// Reads a state key like link:<kind>:<id>.
export function linkTarget(key: string): { kind: string; id: string } | null {
	const m = /^link:([a-z_]+):(.+)$/.exec(key);
	if (!m || !(m[1] in linkAction)) return null;
	return { kind: m[1], id: m[2] };
}

export function onOff(a: PartsAnswer): Record<PartName, boolean> {
	const out = { movies: true, shows: true, music: true, subtitles: true, wishlist: true };
	for (const p of a.parts) out[p.part] = p.on;
	return out;
}

// The person already agreed in the first-run step, so a turn-off is confirmed at once.
export async function applyParts(want: Record<PartName, boolean>, client: Client = defaultClient): Promise<PartsAnswer> {
	let now = await client.get<PartsAnswer>('/parts');
	for (const s of switchPlan(onOff(now), want)) {
		const path = `/parts/${s.part}/${s.on ? 'on' : 'off'}`;
		const r = await client.post<PartsAnswer | Confirm>(path, {});
		now = (isConfirm(r) ? await client.post<PartsAnswer>(path, { confirm: r.confirm }) : r) as PartsAnswer;
	}
	return now;
}
