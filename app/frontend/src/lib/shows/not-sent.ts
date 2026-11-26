// Episodes searched by hand on this page, so a Release found but not sent can say so.
const KEEP_MS = 30 * 60 * 1000;

export interface NotSent {
	name: string;
	reason: string;
}

export function createNotSentWatch(now = () => Date.now()) {
	const watched = new Map<number, { name: string; until: number }>();
	return {
		/** Remembers the Slots one hand search covers. */
		watch(slotIds: number[], name: string) {
			for (const id of slotIds) watched.set(id, { name, until: now() + KEEP_MS });
		},
		/** The episode and reason when a live slot event says a watched Slot's Release was not sent; null otherwise. */
		take(data: unknown): NotSent | null {
			const d = data as { id?: unknown; not_sent?: unknown; reason?: unknown } | null;
			if (!d || d.not_sent !== true || typeof d.id !== 'number') return null;
			const w = watched.get(d.id);
			if (!w) return null;
			watched.delete(d.id);
			if (w.until < now()) return null;
			return { name: w.name, reason: typeof d.reason === 'string' ? d.reason.replace(/^not sent: /, '') : '' };
		}
	};
}
