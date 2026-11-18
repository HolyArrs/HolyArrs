// Tag cleaning and a cached list of tags already in use.
import { api } from '../core/api.ts';

export interface TagUse {
	label: string;
	titles: number;
	downloaders: number;
	sources: number;
	folders?: number;
}

export const MAX_TAG = 40;
export const MAX_TAGS = 20;

// Shared promise so the tag list is fetched once until forgotten.
let known: Promise<TagUse[]> | undefined;

// Tags are lowercase, dash-joined, and capped by characters not bytes.
export function cleanTag(raw: string): string {
	const t = raw.toLowerCase().trim().split(/\s+/).join('-').replace(/^[-,]+|[-,]+$/g, '');
	return [...t].slice(0, MAX_TAG).join('');
}

export function addTags(list: string[], raw: string): string[] {
	const out = [...list];
	for (const part of raw.split(',')) {
		const t = cleanTag(part);
		if (t && !out.includes(t) && out.length < MAX_TAGS) out.push(t);
	}
	return out;
}

// A failed fetch clears the cache so the next call retries.
export function tagsInUse(): Promise<TagUse[]> {
	known ??= api
		.get<{ tags: TagUse[] }>('/tags')
		.then((r) => r.tags ?? [])
		.catch(() => {
			known = undefined;
			return [];
		});
	return known;
}

export function forgetTags() {
	known = undefined;
}

/** Whether taking the last tag away switched "only these tags" off: it was on, there were tags, and now there are none. */
export function switchDropped(was: { tags: string[]; only: boolean }, now: string[]): boolean {
	return was.only && was.tags.length > 0 && now.length === 0;
}
