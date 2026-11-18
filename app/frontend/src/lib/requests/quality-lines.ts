// What people can ask for, as Request rules edits it: adding a line of the owner's own, moving a line and removing one.
import { qualityName } from './quality-row.ts';
import type { QualityLine } from './types.ts';

/** The most lines one kind may have. */
export const LINES_MAX = 8;

/** A line the owner added (Standard and 4K are built in). */
export function ownLine(id: string): boolean {
	return id.startsWith('own-');
}

/** The list with a new line of the owner's own at the end, on the given Pick rules. */
export function addLine(lines: QualityLine[], rulesId: number): QualityLine[] {
	if (lines.length >= LINES_MAX) return lines;
	let n = 1;
	while (lines.some((l) => l.id === `own-${n}`)) n++;
	return [...lines, { id: `own-${n}`, name: '', label: '', hidden: false, extra: false, pick_rules_id: rulesId, library_folder_id: 0, ready: true }];
}

/** The list with line i one place up (-1) or down (1); unchanged at either end. */
export function moveLine(lines: QualityLine[], i: number, by: -1 | 1): QualityLine[] {
	const j = i + by;
	if (i < 0 || i >= lines.length || j < 0 || j >= lines.length) return lines;
	const out = [...lines];
	[out[i], out[j]] = [out[j], out[i]];
	return out;
}

/** The list without line i; a built-in line stays. */
export function removeLine(lines: QualityLine[], i: number): QualityLine[] {
	return lines[i] && ownLine(lines[i].id) ? lines.filter((_, j) => j !== i) : lines;
}

/** The name the owner sees for a line: their own label, else the name people see, else the built-in one. */
export function ownerName(l: Pick<QualityLine, 'id' | 'name' | 'label'>): string {
	return l.label?.trim() || qualityName(l.id, l.name.trim());
}
