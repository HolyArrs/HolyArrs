// The Settings home's own pictures: posters from this Library and the people on this install. The first paint uses what this device kept from the last visit (or nothing: the cards then show line art), and the shared Library read the rest of the app already makes (libraryBrief, kept 60 s) fills it in after. No new server work: people come from /users only for those who may see Users, and only after the page is up.

import { pickPosters, type Person } from './pick.ts';
import { libraryBrief } from '#lib/mediafiles/index.ts';
import { api } from '#lib/core/api.ts';
import { proxied } from '#lib/core/tmdb-image.ts';
import { to } from '#lib/core/paths.ts';

interface Kept {
	posters: string[];
	people: Person[];
}

const KEY = 'holyarrs-settings-art';

/** The 250 px copy of a Library picture (the wall is blurred, the cards are small). */
export function small(url: string): string {
	const via = proxied(url);
	if (via !== url) return via;
	if (/^\/api\/v1\/artwork\/[0-9a-f]+\.(?:jpg|jpeg|png|webp)$/.test(url)) return to(`${url}?size=500`);
	return url.startsWith('/') ? to(url) : url;
}

export const homeArt = $state<Kept & { member: number | undefined }>({ posters: [], people: [], member: undefined });

let started = 0;

/** Fills homeArt: kept copy first, then the shared reads. */
export function startArt(member: number | undefined, mayUsers: boolean, self: Person) {
	if (homeArt.member !== member) Object.assign(homeArt, load(member), { member });
	const mine = ++started;
	const withSelf = (list: Person[]) => (list.some((p) => p.name === self.name) ? list : [self, ...list]).slice(0, 12);
	if (!homeArt.people.length && self.name) homeArt.people = [self];
	void libraryBrief()
		.then((titles) => {
			if (mine !== started) return;
			const posters = pickPosters(titles).map(small);
			if (posters.length) return posters;
			// An empty Library still gets a wall: this week's trending movies.
			return api
				.get<{ items?: { poster?: string }[] }>('/discover?row=trending&kind=movie')
				.then((r) => [...new Set((r.items ?? []).map((i) => i.poster ?? '').filter(Boolean))].slice(0, 16).map(small))
				.catch(() => [] as string[]);
		})
		.then((posters) => {
			if (mine !== started || !posters) return;
			if (posters.join() !== homeArt.posters.join()) homeArt.posters = posters;
			save(member, { posters: homeArt.posters, people: homeArt.people });
		})
		.catch(() => {});
	if (!mayUsers) return;
	void api
		.get<{ users: { name: string; avatar?: string | null; state?: string }[] }>('/users')
		.then((r) => {
			if (mine !== started) return;
			const people = withSelf((r.users ?? []).filter((u) => u.state !== 'disabled').map((u) => ({ name: u.name, avatar: u.avatar ?? null })));
			if (JSON.stringify(people) !== JSON.stringify(homeArt.people)) homeArt.people = people;
			save(member, { posters: homeArt.posters, people: homeArt.people });
		})
		.catch(() => {});
}

function load(member: number | undefined): Kept {
	try {
		const raw = localStorage.getItem(`${KEY}:${member ?? 0}`);
		const v = raw ? (JSON.parse(raw) as Partial<Kept>) : null;
		return { posters: Array.isArray(v?.posters) ? v.posters.filter((s) => typeof s === 'string') : [], people: Array.isArray(v?.people) ? v.people : [] };
	} catch {
		return { posters: [], people: [] };
	}
}

function save(member: number | undefined, k: Kept) {
	try {
		localStorage.setItem(`${KEY}:${member ?? 0}`, JSON.stringify(k));
	} catch {
		/* Private window or full: the next visit just starts with line art. */
	}
}
