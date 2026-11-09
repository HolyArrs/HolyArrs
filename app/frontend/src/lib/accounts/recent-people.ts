export type RecentWay = 'password' | 'plex' | 'jellyfin' | 'emby' | 'oidc';

// `login` is the name typed for a Jellyfin or Emby account, when it differs; `pic` a small JPEG of their profile picture, as a data: address, and `picFrom` the picture address it was copied from (it changes with the picture).
export type RecentPerson = { name: string; way: RecentWay; login?: string; pic?: string; picFrom?: string };

// People who signed in on this device, newest first, with the way each one used, so a shared screen (a TV, a family tablet) can switch between them. Names, ways and a small copy of each picture; each switch is still a full sign-in.
const KEY = 'holyarrs.recent-people';

const MAX = 6;

const WAYS: readonly string[] = ['password', 'plex', 'jellyfin', 'emby', 'oidc'];

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

export function rememberPerson(name: string, way: RecentWay = 'password', login?: string) {
	if (!name) return;
	const all = recentPeople();
	const p: RecentPerson = { name, way };
	if (login && login !== name) p.login = login;
	const old = all.find((o) => o.name === name);
	if (old?.pic) {
		p.pic = old.pic;
		if (old.picFrom) p.picFrom = old.picFrom;
	}
	save([p, ...all.filter((o) => o.name !== name)]);
}

export function forgetPerson(name: string) {
	save(recentPeople().filter((o) => o.name !== name));
}

/** Keeps (or, with null, drops) the small copy of a remembered person's picture. */
export function setPersonPic(name: string, pic: string | null, from?: string) {
	save(
		recentPeople().map((o) => {
			if (o.name !== name) return o;
			const { pic: _p, picFrom: _f, ...rest } = o;
			return pic ? { ...rest, pic, ...(from ? { picFrom: from } : {}) } : rest;
		})
	);
}

/** Copies a signed-in person's picture (src, their picture address) into the device list as a 96 px JPEG, so the sign-in page can show it. It does nothing for someone not remembered here, or when the copy is already current. */
export async function keepPersonPic(name: string, src: string | null) {
	const p = recentPeople().find((o) => o.name === name);
	if (!p) return;
	if (!src) {
		if (p.pic) setPersonPic(name, null);
		return;
	}
	if (p.pic && p.picFrom === src) return;
	try {
		const img = new Image();
		img.src = src;
		await img.decode();
		const c = document.createElement('canvas');
		c.width = c.height = 96;
		const g = c.getContext('2d');
		if (!g) return;
		g.drawImage(img, 0, 0, 96, 96);
		setPersonPic(name, c.toDataURL('image/jpeg', 0.82), src);
	} catch {
		// No copy: the row keeps the letter.
	}
}

function read(v: unknown): RecentPerson | null {
	// Older lists hold plain names: those people used the password.
	if (typeof v === 'string') return v ? { name: v, way: 'password' } : null;
	if (!v || typeof v !== 'object') return null;
	const o = v as Record<string, unknown>;
	if (typeof o.name !== 'string' || !o.name) return null;
	const way = typeof o.way === 'string' && WAYS.includes(o.way) ? (o.way as RecentWay) : 'password';
	const p: RecentPerson = { name: o.name, way };
	if (typeof o.login === 'string' && o.login && o.login !== o.name) p.login = o.login;
	if (typeof o.pic === 'string' && o.pic.startsWith('data:image/jpeg;base64,') && o.pic.length < 40_000) {
		p.pic = o.pic;
		if (typeof o.picFrom === 'string') p.picFrom = o.picFrom;
	}
	return p;
}

function save(people: RecentPerson[]) {
	try {
		localStorage.setItem(KEY, JSON.stringify(people.slice(0, MAX)));
	} catch {
		// Private windows may refuse storage; the list just stays empty.
	}
}
