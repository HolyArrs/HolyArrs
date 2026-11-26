// The name check answer (POST /releases/name-check) and the extra tags it shows
// beyond the quality badges every release list uses.
import type { ReleaseView } from '#lib/core/types.ts';

interface Conf<T> {
	value: T;
	conf: string;
}

export interface NameCheckCopy {
	version?: string;
	pick_rules: string;
	episodes?: string;
	album?: string;
	outcome: string;
	passed: boolean;
	points: number;
	step?: string;
	good_enough: boolean;
	has_file: boolean;
	reasons: { code: string; effect: string; text: string }[];
}

export interface NameCheckResult {
	name: string;
	parsed: {
		title?: Conf<string>;
		year?: Conf<number>;
		group?: Conf<string>;
		edition?: string[];
		flags?: string[];
		show?: { seasons?: number[]; episodes?: number[]; air_date?: string; absolute?: number[]; special?: boolean };
	};
	read?: ReleaseView['read'];
	title: { id: number; name: string; kind: string; year?: number; sure: boolean; detail: string } | null;
	copies: NameCheckCopy[];
	note?: string;
}

const two = (n: number): string => String(n).padStart(2, '0');

// Episode numbers the way release names write them: S01E02, S01E02-E04, S01, S01-S03.
export function showNumbers(s: NameCheckResult['parsed']['show']): string {
	if (!s) return '';
	if (s.air_date) return s.air_date;
	const seasons = s.seasons ?? [];
	const eps = s.episodes ?? [];
	let out = '';
	if (seasons.length > 1) out = `S${two(seasons[0])}-S${two(seasons[seasons.length - 1])}`;
	else if (seasons.length === 1) out = `S${two(seasons[0])}`;
	if (eps.length === 1) out += `E${two(eps[0])}`;
	else if (eps.length > 1) out += `E${two(eps[0])}-E${two(eps[eps.length - 1])}`;
	if (!out && s.absolute?.length) out = s.absolute.map((n) => `#${n}`).join(', ');
	return out;
}

// Tags the quality badges leave out: episode numbers, edition, other marks and the group.
export function nameCheckTags(r: NameCheckResult): string[] {
	const p = r.parsed;
	const out: string[] = [];
	const nums = showNumbers(p.show);
	if (nums) out.push(nums);
	for (const e of p.edition ?? []) out.push(e.replaceAll('_', ' '));
	for (const f of p.flags ?? []) if (!/^(proper|repack)$/.test(f)) out.push(f.replaceAll('_', ' '));
	if (p.group?.value) out.push(`-${p.group.value}`);
	return out;
}
