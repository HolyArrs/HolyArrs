// People who signed in on this device, newest first, with the way each one
// used, so a shared screen (a TV, a family tablet) can switch between them.
// Names, ways and a small copy of each picture: each switch is still a full
// sign-in (the server's picture address needs one, so the sign-in page cannot load it).
const KEY = 'holyarrs.recent-people';
const MAX = 6;

export type RecentWay = 'password' | 'plex' | 'jellyfin' | 'emby' | 'oidc';
// `login` is the name typed for a Jellyfin or Emby account, when it differs;
// `pic` a small JPEG of their profile picture, as a data: address.
export type RecentPerson = { name: string; way: RecentWay; login?: string; pic?: string };

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
	if (typeof o.pic === 'string' && o.pic.startsWith('data:image/jpeg;base64,') && o.pic.length < 40_000) p.pic = o.pic;
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
	const all = recentPeople();
	const p: RecentPerson = { name, way };
	if (login && login !== name) p.login = login;
	const pic = all.find((o) => o.name === name)?.pic;
	if (pic) p.pic = pic;
	save([p, ...all.filter((o) => o.name !== name)]);
}

export function forgetPerson(name: string) {
	save(recentPeople().filter((o) => o.name !== name));
}

/** setPersonPic keeps (or, with null, drops) the small copy of a remembered person's picture. */
export function setPersonPic(name: string, pic: string | null) {
	save(recentPeople().map((o) => (o.name !== name ? o : pic ? { ...o, pic } : { name: o.name, way: o.way, ...(o.login ? { login: o.login } : {}) })));
}
