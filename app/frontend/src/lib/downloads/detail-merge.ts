// Merges a fresh read of the open download's detail into the last one. Fields whose content did not change keep their old reference, and when nothing changed the old object comes back, so the caller can skip the state write and the panel does not re-render on every poll.
export function mergeDetail<T extends { id: number }>(prev: T, patch: Partial<T> & { id: number }): T {
	if (prev.id !== patch.id) return { ...patch } as T;
	let next: T | null = null;
	for (const k of Object.keys(patch) as (keyof T)[]) {
		const v = patch[k] as T[keyof T];
		if (same(prev[k], v)) continue;
		next ??= { ...prev };
		next[k] = v;
	}
	return next ?? prev;
}

function same(a: unknown, b: unknown): boolean {
	if (a === b) return true;
	if (a == null || b == null || typeof a !== 'object' || typeof b !== 'object') return false;
	return JSON.stringify(a) === JSON.stringify(b);
}
