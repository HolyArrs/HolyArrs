// Turns raw names and system messages into plain wording for people.

import type { Ask, HealthCheck } from './types.ts';

// Plain wording per check kind: first for errors, second for warnings.
const sentences: Record<HealthCheck['kind'], [error: string, warning: string]> = {
	library_folder: ['HolyArrs cannot reach a Library folder.', 'A Library folder needs a look.'],
	disk_space: ['A Library folder is out of space.', 'A Library folder is running low on space.'],
	source: ['A Source is not answering.', 'A Source has a problem.'],
	downloader: ['A Downloader is not answering.', 'A Downloader has a problem.'],
	backups: ['Backups are not being made.', 'The last backup is getting old.'],
	update: ['HolyArrs could not check for an update.', 'An update for HolyArrs is waiting.'],
	config_disk: ['The disk that holds the config folder is almost full.', 'The disk that holds the config folder is running low on space.'],
	config_folder: ['/config is not mapped to a folder, so HolyArrs data is lost when the container is recreated.', '/config is not mapped to a folder, so HolyArrs data is lost when the container is recreated.'],
	waiting_downloads: ["Downloads are waiting because their Downloader can't use the files.", "Downloads are waiting because their Downloader can't use the files."]
};

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
export function personName(name: string | null | undefined, fallback = 'Someone in your household'): string {
	return looksLikeId(name) ? fallback : (name ?? '').trim();
}

// One calm sentence for a health check, never raw system text.
export function healthSentence(c: Pick<HealthCheck, 'kind' | 'state'> & { key?: string; message?: string }): string {
	if (c.key === 'backups:key') {
		return /^the owner/i.test(c.message ?? '') ? 'The owner should save the key kit: backups do not hold the master key.' : 'No key kit is saved: backups do not hold the master key.';
	}
	const pair = sentences[c.kind];
	if (!pair) return c.state === 'error' ? 'Something in HolyArrs is not working.' : 'Something in HolyArrs needs a look.';
	return c.state === 'error' ? pair[0] : pair[1];
}

// Joins names into a list and folds hidden ids into a count.
export function peopleNames(list: (string | null | undefined)[]): string {
	const real = list.filter((n) => !looksLikeId(n)).map((n) => (n ?? '').trim());
	const hidden = list.length - real.length;
	if (hidden && !real.length) return hidden === 1 ? 'Someone in your household' : `${hidden} people in your household`;
	if (hidden) real.push(hidden === 1 ? 'one other in your household' : `${hidden} others in your household`);
	if (real.length <= 1) return real[0] ?? '';
	return `${real.slice(0, -1).join(', ')} and ${real[real.length - 1]}`;
}

// Splits a failure into a readable lead and the raw text kept as detail.
export function plainReason(text: string): { text: string; detail?: string } {
	const t = text.trim();
	if (!technical.test(t)) return { text: t };
	let cut = t.indexOf(': ');
	while (cut > 0 && !technical.test(t.slice(cut + 2))) cut = t.indexOf(': ', cut + 2);
	const head = cut > 0 ? t.slice(0, cut).trim() : '';
	if (!head || technical.test(head)) return { text: 'Something went wrong with the files.', detail: t };
	return { text: /[.!?]$/.test(head) ? head : `${head}.`, detail: t };
}

// Who settled a request, worded for the viewer; empty while still open.
export function decided(a: Pick<Ask, 'state' | 'decided_by' | 'auto_approved'>, me?: { id: number; displayName: string }): string {
	if (!a.decided_by || a.state === 'asked') return '';
	const mine = me && me.id === a.decided_by.id ? me.displayName.trim() : '';
	const who = mine || a.decided_by.display_name?.trim() || a.decided_by.name || 'an admin';
	if (a.state === 'turned_down') return `Declined by ${who}`;
	if (a.auto_approved) return 'Auto-approved';
	return `Approved by ${who}`;
}
