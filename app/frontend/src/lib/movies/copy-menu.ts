// Which actions a movie copy's menu offers.

export type CopyAction = 'edition' | 'protect' | 'unprotect' | 'keepAdd' | 'remove';

export function copyActions(version: { main: boolean }, file?: { Protected?: boolean } | null): CopyAction[] {
	const out: CopyAction[] = file ? ['edition', file.Protected ? 'unprotect' : 'protect', 'keepAdd'] : [];
	if (!version.main) out.push('remove');
	return out;
}

// The badges on a copy's card, each shown once.
export function copyBadges(quality: string[], edition: string, protectedWord: string): string[] {
	return [...new Set([...quality, ...(edition ? [edition] : []), ...(protectedWord ? [protectedWord] : [])])];
}

// True when the server would refuse this edition: more than 40 bytes, path, wildcard, brace or bracket characters, a trailing dot, or invisible direction marks.
export function editionProblem(edition: string): boolean {
	const e = edition.trim();
	return new TextEncoder().encode(e).length > 40 || e.endsWith('.') || /[/\\:*?"<>|[\]{}]/.test(e) || /[\p{Cc}\p{Cf}\p{Zl}\p{Zp}]/u.test(e);
}

// True when the server refused a save because of the edition, not for another reason.
export function editionRefused(status: number, text: string): boolean {
	return status === 400 && /edition/i.test(text);
}

// The device name inside an edition when it is one Windows keeps (CON, NUL, COM1, LPT1, ...), without any extension, or nothing. The server refuses these too.
export function editionDevice(edition: string): string {
	const base = edition.trim().split('.')[0];
	return /^(con|prn|aux|nul|com\d|lpt\d)$/i.test(base) ? base : '';
}

// Puts a saved copy's edition and protection on its card straight away, so the card does not wait for a reload.
export function showSaved<F extends { ID?: number; Protected?: boolean; MediaInfo?: Record<string, unknown> | null }>(slots: { file?: F | null }[], saved: F | null | undefined): void {
	const slot = saved?.ID ? slots.find((s) => s.file?.ID === saved.ID) : undefined;
	if (slot?.file && saved) slot.file = { ...slot.file, Protected: saved.Protected, MediaInfo: saved.MediaInfo };
}

// Saves, then reloads. A reload that fails is not a failed save, so it is not reported as one.
export async function saveThenReload(save: () => Promise<unknown>, reload: () => void | Promise<void>): Promise<void> {
	await save();
	try { await reload(); } catch { /* saved; the page catches up on its next reload */ }
}
