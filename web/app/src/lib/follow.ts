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
export function followHint(kind: string, on: boolean): string {
	const what = kind === 'artist' ? 'new albums' : 'new episodes and seasons';
	return on ? `You hear about ${what}. Tap to stop.` : `Follow to hear about ${what}. Nothing is asked for.`;
}

/** setFollow follows (on) or unfollows a Library title; it returns the new state. */
export async function setFollow(client: Client, titleId: number, on: boolean): Promise<boolean> {
	const path = `/titles/${titleId}/follow`;
	const r = on ? await client.put<{ following: boolean }>(path) : await client.del<{ following: boolean }>(path);
	return r.following;
}
