// The owner and admin controls: part switches, stages, linked apps and the
// whole app. Turning a part off, restarting a part and the whole-app
// controls ask first: the server answers a confirm id, and the same call
// again with that id does it.

import { api as defaultClient, type Client } from './core/api.ts';
import { switchPlan, type PartName, type PartsAnswer } from './mediafiles/index.ts';

/** Confirm is the server's first answer to a control that asks first. */
export interface Confirm {
	confirm: string;
	text: string;
	expires_in_s?: number;
}

export function isConfirm(r: unknown): r is Confirm {
	return !!r && typeof r === 'object' && typeof (r as Confirm).confirm === 'string';
}

/** linkKinds are the linked apps a reconnect reaches, with their button word. */
export const linkAction: Record<string, string> = {
	downloader: 'Reconnect',
	source: 'Test now',
	media_server: 'Test now',
	subtitle_site: 'Test now'
};

/** linkTarget turns a state key (link:<kind>:<id>) into the reconnect path, or null. */
export function linkTarget(key: string): { kind: string; id: string } | null {
	const m = /^link:([a-z_]+):(.+)$/.exec(key);
	if (!m || !(m[1] in linkAction)) return null;
	return { kind: m[1], id: m[2] };
}

/** onOff reads which parts are on from GET /parts. */
export function onOff(a: PartsAnswer): Record<PartName, boolean> {
	const out = { movies: true, shows: true, music: true, subtitles: true, wishlist: true };
	for (const p of a.parts) out[p.part] = p.on;
	return out;
}

/**
 * applyParts switches the parts to want in an order the server accepts.
 * The person already said yes (the first-run step), so a turn-off is
 * confirmed straight away.
 */
export async function applyParts(want: Record<PartName, boolean>, client: Client = defaultClient): Promise<PartsAnswer> {
	let now = await client.get<PartsAnswer>('/parts');
	for (const s of switchPlan(onOff(now), want)) {
		const path = `/parts/${s.part}/${s.on ? 'on' : 'off'}`;
		const r = await client.post<PartsAnswer | Confirm>(path, {});
		now = (isConfirm(r) ? await client.post<PartsAnswer>(path, { confirm: r.confirm }) : r) as PartsAnswer;
	}
	return now;
}
