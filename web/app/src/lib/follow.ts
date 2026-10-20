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

/** What is followed: a Library title, or a show by TMDB id (not in the Library yet). */
export type FollowTarget = { titleId: number } | { tmdb: string };

/** followPath is the follow address of a Library title or of a show by TMDB id. */
export function followPath(t: FollowTarget): string {
	return 'titleId' in t ? `/titles/${t.titleId}/follow` : `/metadata/show/${encodeURIComponent(t.tmdb)}/follow`;
}

/** setFollow follows (on) or unfollows a title; it returns the new state. */
export async function setFollow(client: Client, target: number | FollowTarget, on: boolean): Promise<boolean> {
	const path = followPath(typeof target === 'number' ? { titleId: target } : target);
	const r = on ? await client.put<{ following: boolean }>(path) : await client.del<{ following: boolean }>(path);
	return r.following;
}
