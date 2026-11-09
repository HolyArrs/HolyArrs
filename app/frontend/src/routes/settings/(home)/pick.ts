// Which Library pictures the Settings home shows. Plain functions, so the choice is tested without a page.

export interface Person {
	name: string;
	avatar: string | null;
}

interface Brief {
	id: number;
	kind?: string;
	artwork?: string[];
}

/** Keeps up to 16 movie and show posters, newest additions first. */
export function pickPosters(titles: Brief[], max = 16): string[] {
	const seen = new Set<string>();
	const out: string[] = [];
	const list = [...titles].filter((t) => (t.kind === 'movie' || t.kind === 'show') && t.artwork?.[0]).sort((a, b) => b.id - a.id);
	for (const t of list) {
		const a = t.artwork![0];
		if (seen.has(a)) continue;
		seen.add(a);
		out.push(a);
		if (out.length >= max) break;
	}
	return out;
}

/** Up to two letters for a person's circle. */
export function initials(name: string): string {
	const parts = name.replace(/[^\p{L}\p{N}\s._-]+/gu, '').trim().split(/[\s._-]+/).filter(Boolean);
	if (!parts.length) return '?';
	if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
	return (parts[0][0] + parts[1][0]).toUpperCase();
}

/** Repeats the posters to fill the blurred background (n tiles), or nothing when there are none. */
export function wall(posters: string[], n = 40): string[] {
	if (!posters.length) return [];
	return Array.from({ length: n }, (_, i) => posters[i % posters.length]);
}
