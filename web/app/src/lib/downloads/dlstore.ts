// Push updates for the Downloads page (setting downloads.sync = on).
// The page reads a snapshot over REST (rows plus each client's rev and the
// server's boot id) and then applies keyed row patches from /live topic
// "dl". Each client's rev is its own sequence: a patch from fromRev to
// toRev applies only on top of exactly fromRev. Older patches are already
// in the snapshot and are dropped; a patch past the rev the page holds
// means messages were missed, so the page reads its snapshot again.
import type { DownloadRow } from '../downloaders.ts';

export type DlPatch = {
	kind: 'patch';
	boot?: string;
	client: number;
	fromRev: number;
	toRev: number;
	upsert?: ({ key: string } & Record<string, unknown>)[];
	remove?: string[];
	totals?: { speed: number; up_speed: number };
};

export type DlHealth = {
	kind: 'health';
	boot?: string;
	client: number;
	health: string;
	name?: string;
	stale?: boolean;
	seen_at?: string;
};

export type DlMessage = DlPatch | DlHealth;

/** Sync is what the page knows of the server's snapshot. */
export type Sync = { boot: string; revs: Record<string, number> };

/** fromAnswer reads the push state from a view answer; null when the server does not push (legacy). */
export function fromAnswer(a: { boot?: string; revs?: Record<string, number> }): Sync | null {
	if (!a.revs) return null;
	return { boot: a.boot ?? '', revs: { ...a.revs } };
}

export type Outcome =
	| { kind: 'applied'; rows: DownloadRow[]; sync: Sync; upTotal?: number }
	| { kind: 'dropped' }
	| { kind: 'reread'; client?: number };

/**
 * applyPatch puts one patch on the rows. A row a patch names but the page
 * does not hold is added only when the patch carries a whole row (a new
 * client torrent); HolyArrs' own rows come and go with page reads.
 * withId gives a new client row its stand-in id.
 */
export function applyPatch(rows: DownloadRow[], sync: Sync, p: DlPatch, withId: (r: DownloadRow) => DownloadRow = (r) => r): Outcome {
	if (p.boot && sync.boot && p.boot !== sync.boot) return { kind: 'reread' };
	const have = sync.revs[String(p.client)] ?? 0;
	if (p.toRev <= have) return { kind: 'dropped' };
	if (p.fromRev !== have) return { kind: 'reread', client: p.client };
	const gone = new Set(p.remove ?? []);
	const ups = new Map((p.upsert ?? []).map((u) => [u.key, u]));
	const out: DownloadRow[] = [];
	for (const r of rows) {
		const k = r.live_key;
		if (k && gone.has(k) && r.client_item) continue;
		const u = k ? ups.get(k) : undefined;
		if (!u) {
			out.push(r);
			continue;
		}
		ups.delete(k!);
		out.push(merge(r, u));
	}
	for (const u of ups.values()) {
		if (u.client_item && typeof u.name === 'string') out.push(withId(merge({} as DownloadRow, u)));
	}
	return { kind: 'applied', rows: out, sync: { boot: sync.boot, revs: { ...sync.revs, [String(p.client)]: p.toRev } }, upTotal: p.totals?.up_speed };
}

/** merge sets each field the patch names; null clears it. */
function merge(r: DownloadRow, u: Record<string, unknown>): DownloadRow {
	const next: Record<string, unknown> = { ...r };
	for (const [f, v] of Object.entries(u)) {
		if (f === 'key') continue;
		if (v === null) delete next[f];
		else next[f] = v;
	}
	if (!next.live_key) next.live_key = u.key;
	return next as unknown as DownloadRow;
}

/** isDl says a live message is a dl patch or health event. */
export function isDl(x: unknown): x is DlMessage {
	const m = x as DlMessage | null;
	return !!m && typeof m === 'object' && (m.kind === 'patch' || m.kind === 'health') && typeof m.client === 'number';
}

/**
 * mergeClient puts one client's page (a view read with ?client=) in place of
 * that client's rows: the other clients' rows and revs stay as they are, so
 * a gap on one client never rereads the whole list.
 */
export function mergeClient(rows: DownloadRow[], sync: Sync, client: number, page: DownloadRow[], revs: Record<string, number> | undefined): { rows: DownloadRow[]; sync: Sync } {
	const kept = rows.filter((r) => r.downloader_id !== client);
	const rev = revs?.[String(client)] ?? sync.revs[String(client)] ?? 0;
	const fresh = page.filter((r) => r.downloader_id === client);
	// HolyArrs' own rows stay newest first, client rows after them, as a full read gives.
	const own = [...kept.filter((r) => !r.client_item), ...fresh.filter((r) => !r.client_item)].sort((a, b) => b.id - a.id);
	const outside = [...kept.filter((r) => r.client_item), ...fresh.filter((r) => r.client_item)];
	return { rows: [...own, ...outside], sync: { boot: sync.boot, revs: { ...sync.revs, [String(client)]: rev } } };
}
