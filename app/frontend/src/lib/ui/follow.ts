
// Follow labels, hints and the request that toggles a follow.

import { t } from '#lib/core/i18n/i18n.svelte.ts';
type Client = {
	put: <T>(p: string, body?: unknown) => Promise<T>;
	del: <T>(p: string) => Promise<T>;
};

export type FollowTarget = { titleId: number } | { tmdb: string; kind?: 'show' | 'movie' };

export function followLabel(on: boolean): string {
	return on ? t('ui.follow.following') : t('ui.follow.follow');
}

// Wording differs by kind, and by whether a movie is still waiting for the Library.
export function followHint(kind: string, on: boolean, waiting = false): string {
	if (kind === 'movie') {
		if (waiting) return on ? t('ui.follow.movieWaitingOn') : t('ui.follow.movieWaitingOff');
		return on ? t('ui.follow.movieOn') : t('ui.follow.movieOff');
	}
	if (kind === 'artist') return on ? t('ui.follow.artistOn') : t('ui.follow.artistOff');
	if (waiting) return on ? t('ui.follow.showWaitingOn') : t('ui.follow.showWaitingOff');
	return on ? t('ui.follow.showOn') : t('ui.follow.showOff');
}

// A known title uses its id; an unknown one is followed by its TMDB id.
export function followPath(t: FollowTarget): string {
	return 'titleId' in t ? `/titles/${t.titleId}/follow` : `/metadata/${t.kind ?? 'show'}/${encodeURIComponent(t.tmdb)}/follow`;
}

export function movieFollowHint(name: string, on: boolean): string {
	return on ? t('ui.follow.movieNamedOn', { name }) : t('ui.follow.movieNamedOff', { name });
}

// Returns the server's answer so the button shows the real state.
export async function setFollow(client: Client, target: number | FollowTarget, on: boolean): Promise<boolean> {
	const path = followPath(typeof target === 'number' ? { titleId: target } : target);
	const r = on ? await client.put<{ following: boolean }>(path) : await client.del<{ following: boolean }>(path);
	return r.following;
}
