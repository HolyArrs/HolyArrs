// People who signed in on this device, newest first, with the way each one
// used, so a shared screen (a TV, a family tablet) can switch between them.
// Names and ways only: each switch is still a full sign-in.
const KEY = 'holyarrs.recent-people';
const MAX = 6;

export type RecentWay = 'password' | 'plex' | 'jellyfin' | 'emby' | 'oidc';
// `login` is the name typed for a Jellyfin or Emby account, when it differs.
export type RecentPerson = { name: string; way: RecentWay; login?: string };

const WAYS: readonly string[] = ['password', 'plex', 'jellyfin', 'emby', 'oidc'];

function read(v: unknown): RecentPerson | null {
	// Older lists hold plain names: those people used the password.
	if (typeof v === 'string') return v ? { name: v, way: 'password' } : null;
	if (!v || typeof v !== 'object') return null;
	const o = v as Record<string, unknown>;
	if (typeof o.name !== 'string' || !o.name) return null;
	const way = typeof o.way === 'string' && WAYS.includes(o.way) ? (o.way as RecentWay) : 'password';
	const p: RecentPerson = { name: o.name, way };
	if (typeof o.login === 'string' && o.login && o.login !== o.name) p.login = o.login;
	return p;
}

export function recentPeople(): RecentPerson[] {
	try {
		const v = JSON.parse(localStorage.getItem(KEY) ?? '[]');
		if (!Array.isArray(v)) return [];
		const out: RecentPerson[] = [];
		for (const x of v) {
			const p = read(x);
			if (p && !out.some((o) => o.name === p.name)) out.push(p);
		}
		return out.slice(0, MAX);
	} catch {
		return [];
	}
}

function save(people: RecentPerson[]) {
	try {
		localStorage.setItem(KEY, JSON.stringify(people.slice(0, MAX)));
	} catch {
		// Private windows may refuse storage; the list just stays empty.
	}
}

export function rememberPerson(name: string, way: RecentWay = 'password', login?: string) {
	if (!name) return;
	const p: RecentPerson = { name, way };
	if (login && login !== name) p.login = login;
	save([p, ...recentPeople().filter((o) => o.name !== name)]);
}

export function forgetPerson(name: string) {
	save(recentPeople().filter((o) => o.name !== name));
}
