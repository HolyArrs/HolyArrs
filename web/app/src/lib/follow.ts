// Follow: a person follows a show or an artist to hear about new episodes,
// seasons and albums. Following asks for nothing.

type Client = {
	put: <T>(p: string, body?: unknown) => Promise<T>;
	del: <T>(p: string) => Promise<T>;
};

/** followLabel is the bell's word. */
export function followLabel(on: boolean): string {
	return on ? 'Following' : 'Follow';
}

/** followHint says what following a title tells you. */
export function followHint(kind: string, on: boolean, waiting = false): string {
	const what = kind === 'artist' ? 'new albums' : waiting ? 'it when it lands and about new seasons' : 'new episodes and seasons';
	return on ? `You hear about ${what}. Tap to stop.` : `Follow to hear about ${what}. Nothing is asked for.`;
}

/** What is followed: a Library title, or a show (or a movie not out yet) by TMDB id. */
export type FollowTarget = { titleId: number } | { tmdb: string; kind?: 'show' | 'movie' };

/** followPath is the follow address of a Library title or of a show or movie by TMDB id. */
export function followPath(t: FollowTarget): string {
	return 'titleId' in t ? `/titles/${t.titleId}/follow` : `/metadata/${t.kind ?? 'show'}/${encodeURIComponent(t.tmdb)}/follow`;
}

/** movieFollowHint says what following a movie that is not in the Library yet tells you. */
export function movieFollowHint(name: string, on: boolean): string {
	return on ? `You hear when ${name} is in the Library. Tap to stop.` : `Follow ${name} to hear when it is out and in the Library. Nothing is asked for.`;
}

/** setFollow follows (on) or unfollows a title; it returns the new state. */
export async function setFollow(client: Client, target: number | FollowTarget, on: boolean): Promise<boolean> {
	const path = followPath(typeof target === 'number' ? { titleId: target } : target);
	const r = on ? await client.put<{ following: boolean }>(path) : await client.del<{ following: boolean }>(path);
	return r.following;
}
