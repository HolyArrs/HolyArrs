import { t } from '#lib/core/i18n/i18n.svelte.ts';
import { mergeDetail } from './detail-merge.ts';
import { protocolOf } from './size.ts';
import { rowPath } from './downloaders.ts';
import { splitTrackers } from './manage.ts';
import { calmErr, type Det, type Detail, type Peer, type TFile, type Tracker } from './pane-facts.ts';
import type { UsenetView } from './usenet.ts';
import { untrack } from 'svelte';
import { api, message, ApiError } from '../core/api.ts';
import { toasts } from '../core/toasts.svelte.ts';
import type { DownloadRow } from '../core/types.ts';

/** What the open download's pane needs from the page that owns it. */
export type PaneContext = {
	openId: () => number;
	openTab: () => string;
	paneOpen: () => boolean;
	rows: () => DownloadRow[];
	watch: () => boolean;
};

// The client is read live while it may hold the torrent: downloading, finished or seeding.
export const live = (r: DownloadRow) => !!r.client_item || (r.phase !== 'removed' && r.phase !== 'vanished');

const trPath = (r: DownloadRow) => `/downloaders/${r.downloader_id}/items/${r.hash}/trackers`;

/** The open download's real detail, read from the server (and through it from the torrent client) while its pane is open. Nothing is made up: what is not reported shows as a dash or an empty table. Also holds the state of the Files and Trackers tabs. */
export class PaneDetail {
	det = $state.raw<Det>({ id: 0 });
	// Selection and folder state for the Files, Peers and Trackers tables.
	sel = $state<Record<string, string>>({});
	closed = $state<Record<string, boolean>>({});
	// Files tab: a filter box (Ctrl+F while the tab shows, Esc clears) and a selection of files and folders (Ctrl or Cmd click, Shift range) that gets one priority at once.
	fq = $state('');
	fsel = $state<Set<string>>(new Set());
	fqEl = $state<HTMLInputElement>();
	filesBusy = $state(false);
	// A priority change is a control route: an old sign-in is asked for the password inline, then the change is sent again.
	prioWait = $state<{ r: DownloadRow; indexes: number[]; priority: number } | null>(null);
	// Tracker editing: a picked tracker can get a new address or tier, or go; new ones are added one per line.
	trUrl = $state('');
	trTier = $state(0);
	trAdd = $state('');
	trBusy = $state(false);

	#fAnchor = '';
	#ctx: PaneContext;

	constructor(ctx: PaneContext) {
		this.#ctx = ctx;
		$effect(() => {
			// Another download opened: its files start unfiltered and unpicked.
			void ctx.openId();
			this.fq = '';
			this.fsel = new Set();
		});
		$effect(() => {
			// Typed tracker addresses belong to the download they were typed for.
			void ctx.openId();
			this.trUrl = '';
			this.trAdd = '';
		});
		// A password ask belongs to the row and tab it was made in.
		$effect(() => {
			void ctx.openId();
			void ctx.openTab();
			untrack(() => (this.prioWait = null));
		});
		// Reads the open download's detail now and every 3 seconds while it shows. The effect depends only on the row id, the tab and whether the pane shows, not on the row object (replaced on every list refresh) nor on det.
		$effect(() => {
			const id = ctx.openId();
			const tab = ctx.openTab();
			if (!id || !tab || !ctx.paneOpen()) return;
			const read = () =>
				untrack(() => {
					const r = ctx.rows().find((x) => x.id === id);
					// A decision with no download yet (negative id) has nothing to read.
					if (r && !ctx.watch() && !(r.id < 0 && !r.client_item)) void this.readDetail(r, tab);
				});
			read();
			const t = setInterval(() => {
				if (document.visibilityState === 'visible') read();
			}, 3000);
			const back = () => {
				if (document.visibilityState === 'visible') read();
			};
			document.addEventListener('visibilitychange', back);
			return () => {
				clearInterval(t);
				document.removeEventListener('visibilitychange', back);
			};
		});
	}

	fileClick(e: MouseEvent | KeyboardEvent, r: DownloadRow, keys: string[], key: string) {
		this.sel[`${r.id}:f`] = key;
		if (e.ctrlKey || e.metaKey) {
			const n = new Set(this.fsel);
			if (n.has(key)) n.delete(key);
			else n.add(key);
			this.fsel = n;
			this.#fAnchor = key;
		} else if (e.shiftKey && this.#fAnchor && keys.includes(this.#fAnchor)) {
			const [a, b] = [keys.indexOf(this.#fAnchor), keys.indexOf(key)].sort((x, y) => x - y);
			this.fsel = new Set([...this.fsel, ...keys.slice(a, b + 1)]);
		} else {
			this.fsel = new Set([key]);
			this.#fAnchor = key;
		}
	}

	async readDetail(r: DownloadRow, tab: string) {
		const id = r.id;
		const path = rowPath(r);
		const next: Partial<Det> & { id: number } = { id };
		try {
			next.detail = await api.get<Detail>(`${path}/detail`);
		} catch {
			// Keep the last Info on a failed refresh.
		}
		if (protocolOf(r) === 'torrent' && live(r)) {
			if (tab === 'Files') {
				try {
					next.files = (await api.get<{ files: TFile[] }>(`${path}/files`)).files ?? [];
					next.filesErr = undefined;
				} catch {
					// A failed refresh keeps the last list; the calm note shows only when there is none.
					next.filesErr = calmErr();
				}
			}
			if (tab === 'Peers' || tab === 'Trackers') {
				try {
					const sw = await api.get<{ trackers: Tracker[]; peers: Peer[] }>(`${path}/swarm`);
					next.trackers = sw.trackers ?? [];
					next.peers = [...(sw.peers ?? [])].sort((a, b) => a.address.localeCompare(b.address));
					next.swarmErr = undefined;
				} catch {
					next.swarmErr = calmErr();
				}
			}
		}
		if (protocolOf(r) === 'usenet' && live(r) && (tab === 'Servers' || tab === 'Log')) {
			try {
				next.usenet = await api.get<UsenetView>(`${path}/usenet`);
				next.usenetErr = undefined;
			} catch {
				next.usenetErr = calmErr();
			}
		}
		if (this.#ctx.openId() !== id) return;
		// Only a real change is written, so a quiet poll costs no re-render.
		const merged = mergeDetail(this.det, next);
		if (merged !== this.det) this.det = merged;
	}

	pickTracker(r: DownloadRow, x: Tracker) {
		this.sel[`${r.id}:t`] = x.ref || x.url;
		this.trUrl = '';
		this.trTier = x.tier;
	}

	async trackerCall(r: DownloadRow, run: () => Promise<unknown>, done: string): Promise<boolean> {
		this.trBusy = true;
		try {
			await run();
			toasts.push(done, 'ok');
			await this.readDetail(r, 'Trackers');
			return true;
		} catch (e) {
			toasts.push(t('downloads.pd.cantUpdate'), 'fail', { body: message(e) });
			return false;
		} finally {
			this.trBusy = false;
		}
	}

	async addTrackers(r: DownloadRow) {
		const { urls, bad } = splitTrackers(this.trAdd);
		if (bad.length) return toasts.push(t('downloads.pd.notAddress'), 'fail', { body: bad[0] });
		if (!urls.length) return;
		if (await this.trackerCall(r, () => api.post(trPath(r), { urls }), urls.length === 1 ? t('downloads.pd.added.one') : t('downloads.pd.added.other', { count: urls.length }))) this.trAdd = '';
	}

	saveTracker(r: DownloadRow, x: Tracker) {
		const url = this.trUrl.trim();
		const tier = Number(this.trTier);
		if (url && splitTrackers(url).bad.length) return toasts.push(t('downloads.pd.notAddress'), 'fail');
		void this.trackerCall(r, () => api.put(trPath(r), { ref: x.ref, url, ...(tier !== x.tier ? { tier } : {}) }), t('downloads.pd.saved'));
	}

	removeTracker(r: DownloadRow, x: Tracker) {
		void this.trackerCall(r, () => api.post(`${trPath(r)}/remove`, { refs: [x.ref] }), t('downloads.pd.removed'));
	}

	async setPrio(r: DownloadRow, indexes: number[], priority: number) {
		if (!indexes.length) return;
		this.filesBusy = true;
		try {
			await api.put(`${rowPath(r)}/files`, { indexes, priority });
			this.prioWait = null;
			await this.readDetail(r, 'Files');
		} catch (e) {
			if (e instanceof ApiError && e.status === 403 && (e.body.code === 'reauth' || /confirm your local password/i.test(e.message))) {
				this.prioWait = { r, indexes, priority };
				void this.readDetail(r, 'Files');
				return;
			}
			toasts.push(t('downloads.pd.cantPrio'), 'fail', { body: message(e) });
		} finally {
			this.filesBusy = false;
		}
	}
}
