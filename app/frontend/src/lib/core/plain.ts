// Turns raw names and system messages into plain wording for people.

import type { Ask, HealthCheck } from './types.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import { serverWords } from '#lib/core/i18n/server-words.ts';

// A deleted person's row keeps their real name with an invisible separator and
// their id after it. Shown names carry the person between two invisible marks,
// so sentences that hold them can still draw the deleted mark (ui/PersonText).
const storedGone = /^([\s\S]*)\u2063\d+$/;
const shownGone = /\u2063([^\u2063\u2064]*)\u2064/g;

export function shownPerson<T extends string | null | undefined>(name: T): T {
	const m = typeof name === 'string' ? storedGone.exec(name) : null;
	return (m ? `\u2063${m[1].trim()}\u2064` : name) as T;
}

// Whether a stored or shown name belongs to a deleted person.
export function isGone(name: string | null | undefined): boolean {
	return !!name && (storedGone.test(name) || name.includes('\u2064'));
}

// The shown text cut into plain runs and deleted people's names.
export function personParts(text: string | null | undefined): { text: string; gone: boolean }[] {
	const s = shownPerson(text ?? '');
	const out: { text: string; gone: boolean }[] = [];
	let at = 0;
	for (const m of s.matchAll(shownGone)) {
		if (m.index > at) out.push({ text: s.slice(at, m.index), gone: false });
		out.push({ text: m[1], gone: true });
		at = m.index + m[0].length;
	}
	if (at < s.length) out.push({ text: s.slice(at), gone: false });
	return out;
}

// The shown text as plain words, for labels, tooltips and messages: "maria (deleted)".
export function personText(text: string | null | undefined): string {
	return shownPerson(text ?? '').replace(shownGone, (_, name: string) => t('core.plain.deletedName', { name }));
}

// The shown text with the marks taken out, for sorting and initials.
export function bareNames(text: string | null | undefined): string {
	return shownPerson(text ?? '').replace(shownGone, '$1');
}

// A name an Event saved as plain words, marked deleted when it is the deleted actor's own name.
export function asActor(name: string, actorName: string | null | undefined): string {
	return isGone(actorName) && name.trim() === bareNames(actorName) ? shownPerson(actorName ?? '') : name;
}

// Text that names askers, with each one whose saved id now belongs to a deleted person marked.
// The names come stored ("maria" plus the deleted mark) from the server; older Events have none and stay plain.
export function markAsked(text: string, stored: string[] | null | undefined): string {
	let out = text;
	for (const s of stored ?? []) {
		const m = storedGone.exec(s);
		const bare = m?.[1].trim();
		if (!bare) continue;
		const re = new RegExp(`(^|[\\s,(])${bare.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}(?=$|[\\s,.;:)])`);
		out = out.replace(re, (_, pre: string) => pre + shownPerson(s));
	}
	return out;
}

// Plain wording per check kind: first for errors, second for warnings.
const sentences = (): Record<HealthCheck['kind'], [error: string, warning: string]> => ({
	library_folder: [t('core.plain.libraryFolderError'), t('core.plain.libraryFolderWarning')],
	disk_space: [t('core.plain.diskSpaceError'), t('core.plain.diskSpaceWarning')],
	source: [t('core.plain.sourceError'), t('core.plain.sourceWarning')],
	downloader: [t('core.plain.downloaderError'), t('core.plain.downloaderWarning')],
	backups: [t('core.plain.backupsError'), t('core.plain.backupsWarning')],
	update: [t('core.plain.updateError'), t('core.plain.updateWarning')],
	config_disk: [t('core.plain.configDiskError'), t('core.plain.configDiskWarning')],
	config_folder: [t('core.plain.configFolder'), t('core.plain.configFolder')],
	waiting_downloads: [t('core.plain.waitingDownloads'), t('core.plain.waitingDownloads')],
	clear_waiting: [t('core.plain.clearWaiting'), t('core.plain.clearWaiting')],
	save_folder: [t('core.plain.saveFolder'), t('core.plain.saveFolder')],
	no_downloader: [t('core.plain.noDownloader'), t('core.plain.noDownloader')],
	same_downloader: [t('core.plain.sameDownloader'), t('core.plain.sameDownloader')],
	people_check: [t('core.plain.peopleCheck'), t('core.plain.peopleCheck')]
});

// Signs of raw system text that people should not see as the main message.
const technical = /([A-Za-z]:\\|(^|\s)\/[\w.-]+|GetFile\w+|CreateFile|errno|syscall|dial tcp|EACCES|ENOENT|ENOSPC|0x[0-9a-f]{4,}|\bno such file\b|\bpermission denied\b|\bi\/o timeout\b|\bconnection refused\b|\bx509\b)/i;

// Names that are empty, test accounts or raw ids are not worth showing.
export function looksLikeId(name: string | null | undefined): boolean {
	const n = (name ?? '').trim();
	if (!n) return true;
	if (/^(preview|e2e|test|user|guest)[-_]/i.test(n)) return true;
	if (!/\p{L}/u.test(n)) return true;
	return /^[0-9a-f-]{12,}$/i.test(n);
}

// Shows the name, or a friendly stand-in when it is only an id.
export function personName(name: string | null | undefined, fallback = t('core.plain.someone')): string {
	return looksLikeId(name) ? fallback : shownPerson((name ?? '').trim());
}

// One calm sentence for a health check, never raw system text.
export function healthSentence(c: Pick<HealthCheck, 'kind' | 'state'> & { key?: string; message?: string }): string {
	if (c.key === 'backups:key') {
		return /^the owner/i.test(c.message ?? '') ? t('core.plain.keyKitOwner') : t('core.plain.keyKitNone');
	}
	const pair = sentences()[c.kind];
	if (!pair) return c.state === 'error' ? t('core.plain.somethingNotWorking') : t('core.plain.somethingNeedsLook');
	return c.state === 'error' ? pair[0] : pair[1];
}

// Joins names into a list and folds hidden ids into a count.
export function peopleNames(list: (string | null | undefined)[]): string {
	const real = list.filter((n) => !looksLikeId(n)).map((n) => shownPerson((n ?? '').trim()));
	const hidden = list.length - real.length;
	if (hidden && !real.length) return hidden === 1 ? t('core.plain.someone') : t('core.plain.peopleInHousehold', { count: hidden });
	if (hidden) real.push(hidden === 1 ? t('core.plain.oneOther') : t('core.plain.othersInHousehold', { count: hidden }));
	if (real.length <= 1) return real[0] ?? '';
	return t('core.plain.nameList', { names: real.slice(0, -1).join(', '), last: real[real.length - 1] });
}

// Splits a failure into a readable lead and the raw text kept as detail.
export function plainReason(text: string): { text: string; detail?: string } {
	const raw = text.trim();
	if (!technical.test(raw)) return { text: raw };
	let cut = raw.indexOf(': ');
	while (cut > 0 && !technical.test(raw.slice(cut + 2))) cut = raw.indexOf(': ', cut + 2);
	const head = cut > 0 ? raw.slice(0, cut).trim() : '';
	if (!head || technical.test(head)) return { text: t('core.plain.filesWentWrong'), detail: raw };
	return { text: /[.!?]$/.test(head) ? head : `${head}.`, detail: raw };
}

// The name of who settled a request, for the viewer; empty while still open.
export function decidedWho(a: Pick<Ask, 'state' | 'decided_by'>, me?: { id: number; displayName: string }): string {
	if (!a.decided_by || a.state === 'asked') return '';
	const mine = me && me.id === a.decided_by.id ? me.displayName.trim() : '';
	return mine || a.decided_by.display_name?.trim() || shownPerson(a.decided_by.name) || t('core.plain.anAdmin');
}

// Who settled a request, worded for the viewer; empty while still open.
export function decided(a: Pick<Ask, 'state' | 'decided_by' | 'auto_approved'>, me?: { id: number; displayName: string }): string {
	const who = decidedWho(a, me);
	if (!who) return '';
	if (a.state === 'turned_down') return t('core.plain.declinedBy', { who });
	if (a.auto_approved) return t('core.plain.autoApproved');
	return t('core.plain.approvedBy', { who });
}
