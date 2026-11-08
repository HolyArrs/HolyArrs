import type { UpdateInfo } from '../core/types.ts';

export type BackupRow = { name: string; kind: string; created_at: string; detail?: string; from_version?: string; to_version?: string };

export const PROJECT_REPO = 'CoderLuii/holyarrs';
export const RELEASE_PAGES = `https://github.com/${PROJECT_REPO}/releases/`;
export const IMAGE = 'ghcr.io/coderluii/holyarrs';

export function updateWords(u: UpdateInfo): string {
	if (!u.on) return 'The update check is off: HolyArrs never asks for a newer version.';
	if (u.newer) return `HolyArrs ${u.latest} is out, newer than this one.`;
	if (!u.checked_at) return 'Not checked yet; HolyArrs checks once a day.';
	if (u.problem) return `The last check did not work: ${u.problem}.`;
	if (u.no_release) return 'No HolyArrs release is published yet.';
	return 'This is the newest version.';
}

// Returns '' when the link is not a plain project release page, so the version shows as text.
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

// Image tags have no leading v.
function bare(v: string): string {
	return v.trim().replace(/^v/i, '');
}
