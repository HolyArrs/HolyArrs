import type { UpdateInfo } from '../core/types.ts';

export type BackupRow = { name: string; kind: string; created_at: string; detail?: string; from_version?: string; to_version?: string };

export const PROJECT_REPO = 'HolyArrs/holyarrs';
export const RELEASE_PAGES = `https://github.com/${PROJECT_REPO}/releases/`;
// The backend update check still names the old repo until internal/updates and
// internal/api move together (see plan/github/after-first-push.md); drop this then.
const OLD_RELEASE_PAGES = 'https://github.com/CoderLuii/holyarrs/releases/';
// The Docker Hub name users install from (release.yml pushes it).
export const IMAGE = 'holyarrs/holyarrs';

export function updateWords(u: UpdateInfo): string {
	if (!u.on) return 'The update check is off: HolyArrs never asks for a newer version.';
	if (u.newer) return `HolyArrs ${plainVersion(u.latest ?? '')} is out, newer than this one.`;
	if (!u.checked_at) return 'Not checked yet; HolyArrs checks once a day.';
	if (u.problem) return `The last check did not work: ${u.problem}.`;
	if (u.no_release) return 'No HolyArrs release is published yet.';
	return 'This is the newest version.';
}

// Returns '' when the link is not a plain project release page, so the version shows as text.
export function releaseLink(link: string | undefined): string {
	if (!link || !(link.startsWith(RELEASE_PAGES) || link.startsWith(OLD_RELEASE_PAGES))) return '';
	try {
		const u = new URL(link);
		if (u.href !== link || u.origin !== 'https://github.com' || u.username || u.password || u.search || u.hash || u.pathname.includes('..')) return '';
	} catch {
		return '';
	}
	return link;
}

export function updatePrivacy(version: string): string {
	return `On each check HolyArrs asks api.github.com for the newest release, sending only the header User-Agent: HolyArrs/${version}. No account, no list of your media, no other data.`;
}

// The list is newest first, so the first match is the latest one.
export function goBackBackup(list: BackupRow[], version: string): BackupRow | null {
	const now = bare(version);
	return list.find((b) => b.kind === 'pre-update' && b.from_version && b.to_version && bare(b.to_version) === now && bare(b.from_version) !== now) ?? null;
}

export function imageTag(version: string): string {
	return bare(version);
}

/** Versions show as plain numbers (1.0.0), never v1.0.0. */
export function plainVersion(version: string): string {
	return bare(version);
}

// Image tags have no leading v.
function bare(v: string): string {
	return v.trim().replace(/^v/i, '');
}

export type ReleaseNote = { version: string; date?: string; lines: string[]; link?: string };
type NotesInfo = { version: string; latest?: string; newer: boolean; link?: string; notes?: string };

/** What's New: releases newest first, each with its note lines, from the update info already loaded. */
export function whatsNew(u: NotesInfo | null): ReleaseNote[] {
	if (!u) return [];
	const out: ReleaseNote[] = [];
	// The update info has no publish date, so the newer release gets none.
	if (u.newer && u.latest) out.push({ version: plainVersion(u.latest), lines: noteLines(u.notes), link: u.link });
	out.push({ version: plainVersion(u.version), lines: u.newer ? [] : noteLines(u.notes), link: u.newer ? undefined : u.link });
	return out;
}

export function noteLines(notes: string | undefined): string[] {
	return (notes ?? '').split('\n').map((l) => l.trim()).filter(Boolean);
}
