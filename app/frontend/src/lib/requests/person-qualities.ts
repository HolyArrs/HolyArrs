// The qualities one person may pick, as the person editor ticks them: null is every line (also the ones the owner adds later).

/** Line id is ticked for a person with this choice. */
export function lineTicked(chosen: string[] | null | undefined, id: string): boolean {
	return !chosen || chosen.includes(id);
}

/** Line id is the only one ticked, so it cannot be unticked. */
export function onlyTicked(chosen: string[] | null | undefined, id: string): boolean {
	return !!chosen && chosen.length === 1 && chosen[0] === id;
}

/** The choice after ticking or unticking one line: null when every line is ticked; the last ticked line stays. */
export function tickLine(lines: { id: string }[], chosen: string[] | null | undefined, id: string, on: boolean): string[] | null {
	if (!on && onlyTicked(chosen, id)) return chosen ?? null;
	const next = lines.map((l) => l.id).filter((x) => (x === id ? on : lineTicked(chosen, x)));
	return next.length === lines.length ? null : next;
}

/** The lines the person editor shows for one kind: the shown ones plus any hidden one the person is limited to. Empty (kind left out) when
 * there is nothing to choose and no choice to clear. */
export function editorLines<L extends { id: string; hidden: boolean }>(lines: L[], chosen: string[] | null | undefined): L[] {
	const out = lines.filter((l) => !l.hidden || (!!chosen && chosen.includes(l.id)));
	return out.length > 1 || (chosen && out.length) ? out : [];
}
