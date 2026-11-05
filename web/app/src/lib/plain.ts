// Plain words for people: no internal ids or raw error text on the page.
import type { HealthCheck } from './types.ts';

/** looksLikeId says whether a name is an internal id (preview-…, e2e-…, hex, digits), not a person. */
export function looksLikeId(name: string | null | undefined): boolean {
	const n = (name ?? '').trim();
	if (!n) return true;
	if (/^(preview|e2e|test|user|guest)[-_]/i.test(n)) return true;
	if (!/\p{L}/u.test(n)) return true;
	return /^[0-9a-f-]{12,}$/i.test(n);
}

/** personName is a name to show for someone, or a household word when it looks like an id. */
export function personName(name: string | null | undefined, fallback = 'Someone in your household'): string {
	return looksLikeId(name) ? fallback : (name ?? '').trim();
}

const sentences: Record<HealthCheck['kind'], [error: string, warning: string]> = {
	library_folder: ['HolyArrs cannot reach a Library folder.', 'A Library folder needs a look.'],
	disk_space: ['A Library folder is out of space.', 'A Library folder is running low on space.'],
	source: ['A Source is not answering.', 'A Source answered with a problem.'],
	downloader: ['A Downloader is not answering.', 'A Downloader answered with a problem.'],
	backups: ['Backups are not being made.', 'The last backup is getting old.'],
	update: ['HolyArrs could not check for an update.', 'An update for HolyArrs is waiting.'],
	config_disk: ['The disk that holds the config folder is almost full.', 'The disk that holds the config folder is running low on space.'],
	config_folder: ['/config is not mapped to a folder, so HolyArrs data is lost when the container is recreated.', '/config is not mapped to a folder, so HolyArrs data is lost when the container is recreated.'],
	waiting_downloads: ["Downloads are waiting because their Downloader can't use the files.", "Downloads are waiting because their Downloader can't use the files."]
};

/** healthSentence says a health problem in a plain sentence; the raw text goes in a details part. */
export function healthSentence(c: Pick<HealthCheck, 'kind' | 'state'> & { key?: string }): string {
	// The key kit check shares the backups kind but is its own problem.
	if (c.key === 'backups:key' && c.state !== 'ok') return 'No key kit is saved: backups do not hold the master key.';
	const pair = sentences[c.kind];
	if (!pair) return c.state === 'error' ? 'Something in HolyArrs is not working.' : 'Something in HolyArrs needs a look.';
	return c.state === 'error' ? pair[0] : pair[1];
}

/** peopleNames lists who asked: real names first, ids folded into "others in your household". */
export function peopleNames(list: (string | null | undefined)[]): string {
	const real = list.filter((n) => !looksLikeId(n)).map((n) => (n ?? '').trim());
	const hidden = list.length - real.length;
	if (hidden && !real.length) return hidden === 1 ? 'Someone in your household' : `${hidden} people in your household`;
	if (hidden) real.push(hidden === 1 ? 'one other in your household' : `${hidden} others in your household`);
	if (real.length <= 1) return real[0] ?? '';
	return `${real.slice(0, -1).join(', ')} and ${real[real.length - 1]}`;
}

// Signs that the rest of a message is for a developer: paths, system calls, network errors.
const technical = /([A-Za-z]:\\|(^|\s)\/[\w.-]+|GetFile\w+|CreateFile|errno|syscall|dial tcp|EACCES|ENOENT|ENOSPC|0x[0-9a-f]{4,}|\bno such file\b|\bpermission denied\b|\bi\/o timeout\b|\bconnection refused\b|\bx509\b)/i;

/**
 * plainReason keeps the human part of a message and moves the technical
 * rest to detail: "The file could not be placed: GetFileAttributesEx C:\…"
 * reads "The file could not be placed." with the raw text as detail.
 */
export function plainReason(text: string): { text: string; detail?: string } {
	const t = text.trim();
	if (!technical.test(t)) return { text: t };
	let cut = t.indexOf(': ');
	while (cut > 0 && !technical.test(t.slice(cut + 2))) cut = t.indexOf(': ', cut + 2);
	const head = cut > 0 ? t.slice(0, cut).trim() : '';
	if (!head || technical.test(head)) return { text: 'Something went wrong with the files.', detail: t };
	return { text: /[.!?]$/.test(head) ? head : `${head}.`, detail: t };
}
