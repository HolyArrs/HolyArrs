// The update notice on This app, in plain words.
import type { UpdateInfo } from './types.ts';

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
	return `Once a day HolyArrs asks api.github.com for the newest release, sending only the header User-Agent: HolyArrs/${version}. No account, no list of your media, no other data.`;
}
