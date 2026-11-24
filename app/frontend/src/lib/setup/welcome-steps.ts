// The steps of the first-run wizard after sign-in, in order.
export type Step = 'parts' | 'server' | 'folders' | 'sources' | 'downloads' | 'subtitles' | 'access' | 'done';
// 'server' is Link your media servers (Plex, Jellyfin, Emby; each optional), right after the account step.
export const STEPS: Step[] = ['server', 'parts', 'folders', 'sources', 'downloads', 'subtitles', 'access', 'done'];

// The bar across the top of first-run setup: Welcome and Sign in on the sign-in page, then the wizard's steps.
export type Phase = 'language' | 'welcome' | 'signin' | Step;
export const PHASES: Phase[] = ['language', 'welcome', 'signin', ...STEPS];

// The kinds of media server whose sign-in leads to a Media server step.
export type ServerKind = 'plex' | 'jellyfin' | 'emby';
export function serverKindOf(authMethod: string): ServerKind | null {
	return authMethod === 'plex' || authMethod === 'jellyfin' || authMethod === 'emby' ? authMethod : null;
}

// What decides which steps this person gets: a media server sign-in, the Subtitles part, and Hands-off mode (no Subtitles step).
export type Plan = { withServer: boolean; subtitles?: boolean; handsOff?: boolean };

// Which stop a wizard step belongs to.
export function phaseOf(s: Step): Phase {
	return s;
}

// A step name from the address; older names land on the step that holds them now.
export function stepFromName(s: string | null): Step | null {
	if (s === 'connect') return 'server';
	if (s === 'media' || s === 'folder') return 'folders';
	if (s === 'apps' || s === 'start' || s === 'import') return 'parts';
	if (s === 'downloader') return 'downloads';
	if (s === 'source') return 'sources';
	return STEPS.includes(s as Step) ? (s as Step) : null;
}

// The steps this owner walks: Media server only after a media server sign-in, Subtitles only when that part is on and HolyArrs may touch files.
export function stepsFor(p: Plan): Step[] {
	return STEPS.filter((s) => (s !== 'server' || p.withServer) && (s !== 'subtitles' || (p.subtitles !== false && !p.handsOff)));
}

// The bar's stops for this person.
export function phasesFor(p: Plan): Phase[] {
	return ['language', 'welcome', 'signin', ...stepsFor(p)];
}

export type Progress = Plan & { serverDone: boolean; foldersDone: boolean };

// The first required step not finished yet ('done' when all are). Folders count as done in Hands-off mode.
export function firstOpen(p: Progress): Step {
	if (p.withServer && !p.serverDone) return 'server';
	if (!p.foldersDone && !p.handsOff) return 'folders';
	return 'done';
}

// Where the wizard opens: the step asked for, but never past the first unfinished required one; with none asked for, the first step.
export function resumeStep(asked: Step | null, p: Progress): Step {
	const order = stepsFor(p);
	const open = firstOpen(p);
	if (!asked || !order.includes(asked)) return order[0];
	return order.indexOf(asked) > order.indexOf(open) ? open : asked;
}

// The step after s for this owner.
export function nextStep(s: Step, p: Plan): Step {
	const order = stepsFor(p);
	const i = order.indexOf(s);
	return i < 0 ? order[order.length - 1] : order[Math.min(i + 1, order.length - 1)];
}

// The step before s, or undefined at the first one.
export function prevStep(s: Step, p: Plan): Step | undefined {
	const order = stepsFor(p);
	const i = order.indexOf(s);
	return i > 0 ? order[i - 1] : undefined;
}

// Picking a Plex address: the address and port it fills in, as a full address.
export function plexAddress(c: { address: string; port: number; secure: boolean }): string {
	return `${c.secure ? 'https' : 'http'}://${c.address}:${c.port}`;
}

// One address of one of the owner's Plex servers, as the server lists it: the
// secure plex.direct address when Plex has one, and plain http only when local.
export type PlexChoice = { id: string; name: string; address: string; port: number; local: boolean; url: string; secure_url?: string };

// How a Plex choice reads in the list: "Home · 10.0.0.5 · local".
export function plexChoiceLabel(c: PlexChoice, words: { local: string; remote: string }): string {
	return `${c.name} · ${c.address} · ${c.local ? words.local : words.remote}`;
}

// The order to try a Plex choice in. A local (home network) server: its plain
// IP address first, like Seerr, then its secure address. A remote server only
// ever uses its secure address.
export function plexTries(c: PlexChoice): string[] {
	if (!c.local) return [c.secure_url || c.url];
	return c.secure_url && c.secure_url !== c.url ? [c.url, c.secure_url] : [c.url];
}

// Whether a failed try may move on to plain http: only when the server could
// not be reached at all, never on a certificate or TLS error.
export function plexMayFallBack(message: string): boolean {
	const m = message.toLowerCase();
	if (/certificate|x509|tls|ssl|handshake|https/.test(m)) return false;
	return /nothing answers|no answer|name not found|connection refused|actively refused|refused on port|timeout|timed out|deadline exceeded|no such host|unreachable|did not answer/.test(m);
}

// Host, port and the SSL box for an address, so the form always shows what is used.
export function plexFields(u: string): { host: string; port: string; ssl: boolean } | null {
	try {
		const p = new URL(u);
		const ssl = p.protocol === 'https:';
		return { host: p.hostname, port: p.port || (ssl ? '443' : '80'), ssl };
	} catch {
		return null;
	}
}

// Which of the owner's servers is picked first: the first local one, else the first.
export function defaultPlexPick(list: PlexChoice[]): string {
	if (!list.length) return 'manual';
	const i = list.findIndex((c) => c.local);
	return String(i < 0 ? 0 : i);
}

// One library on the media server, as Connect server lists it.
export type Library = { key: string; title: string; kind: 'movie' | 'show' | 'music'; paths: string[]; on: boolean; count?: number };

// What the Host, Port and SSL fields show for a listed Plex server: always its
// plain IP, while the address actually used (the secure plex.direct one for a
// remote server) is kept apart and saved as it is.
export function plexShown(c: PlexChoice, used: string): { host: string; port: string; ssl: boolean } {
	const f = plexFields(used);
	return { host: c.address, port: f?.port ?? String(c.port), ssl: f?.ssl ?? false };
}

// The address saved: a listed server's own address, or exactly what was typed.
export function plexFormUrl(f: { pick: string; listed: string; host: string; port: string; ssl: boolean }): string {
	if (f.pick !== 'manual' && f.listed) return f.listed;
	const h = f.host.trim().replace(/^https?:\/\//, '');
	return h ? plexAddress({ address: h, port: Number(f.port) || 32400, secure: f.ssl }) : '';
}
