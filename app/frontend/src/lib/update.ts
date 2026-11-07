// The update notice on This app, in plain words.
import type { UpdateInfo } from './updates/index.ts';

export function updateWords(u: UpdateInfo): string {
	if (!u.on) return 'The update check is off: HolyArrs never asks for a newer version.';
	if (u.newer) return `HolyArrs ${u.latest} is out, newer than this one.`;
	if (!u.checked_at) return 'Not checked yet; HolyArrs checks once a day.';
	if (u.problem) return `The last check did not work: ${u.problem}.`;
	if (u.no_release) return 'No HolyArrs release is published yet.';
	return 'This is the newest version.';
}

// The project's repository; a "what is new" link is shown only under its
// release pages (the server checks the same).
export const PROJECT_REPO = 'CoderLuii/holyarrs';
export const RELEASE_PAGES = `https://github.com/${PROJECT_REPO}/releases/`;

// releaseLink is the link to show, or '' to show the version as text only.
export function releaseLink(link: string | undefined): string {
	if (!link || !link.startsWith(RELEASE_PAGES)) return '';
	try {
		const u = new URL(link);
		if (u.href !== link || u.origin !== 'https://github.com' || u.username || u.password || u.search || u.hash || u.pathname.includes('..')) return '';
	} catch {
		return '';
	}
	return link;
}

// What the update check sends, said once where it is turned on.
export function updatePrivacy(version: string): string {
	return `On each check HolyArrs asks api.github.com for the newest release, sending only the header User-Agent: HolyArrs/${version}. No account, no list of your media, no other data.`;
}

// The published image; a release tag is the version without a leading v.
export const IMAGE = 'ghcr.io/coderluii/holyarrs';

// A backup as GET /backups lists it (pre-update ones say which update).
export type BackupRow = { name: string; kind: string; created_at: string; detail?: string; from_version?: string; to_version?: string };

const bare = (v: string) => v.trim().replace(/^v/i, '');

// goBackBackup is the pre-update backup taken for the update to this
// version (newest first in the list), or null: then there is nothing to
// go back to from here.
export function goBackBackup(list: BackupRow[], version: string): BackupRow | null {
	const now = bare(version);
	return list.find((b) => b.kind === 'pre-update' && b.from_version && b.to_version && bare(b.to_version) === now && bare(b.from_version) !== now) ?? null;
}

// imageTag is the tag to change the image back to.
export function imageTag(version: string): string {
	return bare(version);
}
