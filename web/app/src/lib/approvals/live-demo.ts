// Preview only (?demo in the URL): fills empty approved asks with each live stage so the
// Approvals page can be judged on every status. Never touches the server or Downloads.
import type { Ask } from '../types.ts';
import type { TitleLive } from './live.ts';

const STAGES: TitleLive[] = [
	{ searching: true },
	{ download: { phase: 'downloading', progress: 0.48 } },
	{ download: { phase: 'importing', progress: 1 } },
	{ download: { phase: 'failed', progress: 0.3 } }
];

export const demoOn = () => typeof location !== 'undefined' && new URLSearchParams(location.search).has('demo');

/** withDemo gives approved asks that have nothing live a stage, each stage once. */
export function withDemo(real: Map<number, TitleLive>, asks: Ask[]): Map<number, TitleLive> {
	const out = new Map(real);
	let i = 0;
	for (const a of asks) {
		if (a.state !== 'approved' || !a.title_id || out.has(a.title_id)) continue;
		out.set(a.title_id, STAGES[i++ % STAGES.length]);
		if (i >= STAGES.length) break; // each stage once
	}
	return out;
}
