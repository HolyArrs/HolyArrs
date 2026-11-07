
// Follow labels, hints and the request that toggles a follow.

type Client = {
	put: <T>(p: string, body?: unknown) => Promise<T>;
	del: <T>(p: string) => Promise<T>;
};

export type FollowTarget = { titleId: number } | { tmdb: string; kind?: 'show' | 'movie' };

export function followLabel(on: boolean): string {
	return on ? 'Following' : 'Follow';
}

// Wording differs by kind, and by whether a movie is still waiting for the Library.
export function followHint(kind: string, on: boolean, waiting = false): string {
	if (kind === 'movie') {
		const what = waiting ? 'when it is in the Library' : 'its dates';
		return on ? `On your calendar; you hear ${waiting ? what : 'about it'}. Tap to stop.` : `Follow to see ${waiting ? 'it' : what} on your calendar${waiting ? ` and hear ${what}` : ''}. Nothing is asked for.`;
	}
	const what = kind === 'artist' ? 'new albums' : waiting ? 'it when it lands and about new seasons' : 'new episodes and seasons';
	return on ? `On your calendar; you hear about ${what}. Tap to stop.` : `Follow to see it on your calendar and hear about ${what}. Nothing is asked for.`;
}

// A known title uses its id; an unknown one is followed by its TMDB id.
export function followPath(t: FollowTarget): string {
	return 'titleId' in t ? `/titles/${t.titleId}/follow` : `/metadata/${t.kind ?? 'show'}/${encodeURIComponent(t.tmdb)}/follow`;
}

export function movieFollowHint(name: string, on: boolean): string {
	return on ? `You hear when ${name} is in the Library. Tap to stop.` : `Follow ${name} to hear when it is out and in the Library. Nothing is asked for.`;
}

// Returns the server's answer so the button shows the real state.
export async function setFollow(client: Client, target: number | FollowTarget, on: boolean): Promise<boolean> {
	const path = followPath(typeof target === 'number' ? { titleId: target } : target);
	const r = on ? await client.put<{ following: boolean }>(path) : await client.del<{ following: boolean }>(path);
	return r.following;
}
