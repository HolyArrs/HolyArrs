// Tags (#108): plain lowercase labels on titles, Downloaders and Sources.
// cleanTag matches the server's tagset.Clean for one label.
import { api } from './api.ts';

export const MAX_TAG = 40;
export const MAX_TAGS = 20;

/** cleanTag lowercases, joins inner spaces with "-", trims "-" and "," and cuts to 40 characters. */
export function cleanTag(raw: string): string {
	const t = raw.toLowerCase().trim().split(/\s+/).join('-').replace(/^[-,]+|[-,]+$/g, '');
	return [...t].slice(0, MAX_TAG).join('');
}

/** addTags adds the labels in raw (split on commas) to list, cleaned, without repeats, at most 20. */
export function addTags(list: string[], raw: string): string[] {
	const out = [...list];
	for (const part of raw.split(',')) {
		const t = cleanTag(part);
		if (t && !out.includes(t) && out.length < MAX_TAGS) out.push(t);
	}
	return out;
}

export interface TagUse {
	label: string;
	titles: number;
	downloaders: number;
	sources: number;
}

let known: Promise<TagUse[]> | undefined;

/** tagsInUse reads every tag in use once per page load (suggestions); forget() after a save. */
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
