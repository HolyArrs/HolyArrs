// Bring my apps on the first-run choice screen: the app cards, the import that opens on one, and what the imports brought.
import { setContext, untrack } from 'svelte';
import { api } from '#lib/core/api.ts';
import { session } from '#lib/core/session.svelte.ts';
import { to } from '#lib/core/paths.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import { theme } from '#lib/accounts/index.ts';
// The old app kinds (migrate's AppKind), and its helpers come in through ctx.
type AppKind = string;

export function createWelcomeBring(ctx: any) {
	// Bring my apps opens the move flow in the same card; when it is done, Start fresh opens with the parts it found ticked.
	let importing = $state(false);
	// The app card picked; the import opens on its read.
	let importFrom = $state<AppKind | undefined>(undefined);
	// The app the check found (Seerr picked, Overseerr answered) names the heading.
	let detected = $state<AppKind>('');
	setContext('move-detected-kind', (k: AppKind) => (detected = k));
	// Seerr first: its read brings the Radarr and Sonarr it keeps too.
	const mainApps: { kind: AppKind; name: string; logos: string[] }[] = [
		{ kind: 'seerr', name: 'Seerr / Overseerr', logos: ['seerr'] },
		{ kind: 'radarr', name: 'Radarr', logos: ['radarr'] },
		{ kind: 'sonarr', name: 'Sonarr', logos: ['sonarr'] },
		{ kind: 'lidarr', name: 'Lidarr', logos: ['lidarr'] },
		{ kind: 'bazarr', name: 'Bazarr', logos: ['bazarr'] }
	];
	const appLogo = (k: string) => to(`/apps/${k === 'radarr' && theme.value === 'light' ? 'radarr-light' : k}.${k === 'bazarr' || k === 'jackett' ? 'png' : 'svg'}`);
	function openImport(k: AppKind) {
		ctx.error = '';
		importFrom = k;
		via = checks[k];
		moving = false;
		againId = undefined;
		importing = true;
	}
	// Edit on a done card: that copy's read opens again, its address filled and its kept key in use.
	let againId = $state<number | undefined>(undefined);
	function editApp(k: string, id?: number) {
		openImport(k);
		againId = id ?? reads[k]?.at(-1)?.id;
	}
	// Manage on a card with several copies: the list of that app's copies, inside the same card.
	let managing = $state<string | undefined>(undefined);
	// One copy: its count. Several: one line, "Radarr × 2 · 649 movies", so the card keeps its size.
	function wordsFor(k: string, list: Read[]): string {
		if (list.length === 1) return ctx.countLine(k, list[0].counts);
		const sum: Record<string, number> = {};
		for (const x of list) for (const [c, n] of Object.entries(x.counts)) sum[c] = (sum[c] ?? 0) + n;
		return [t('welcome.choose.copies', { name: ctx.appName(k), n: list.length }), ctx.countLine(k, sum)].filter(Boolean).join(' · ');
	}
	// A read done in setup goes straight back to the cards with a tick and its count; the move runs later, once, for every read (Continue).
	let moving = $state(false);
	// Every copy read per app (Radarr and Radarr 4K): the count line adds them up; a new read of the same address replaces the old one.
	type Read = { id: number; address: string; counts: Record<string, number>; from?: string };
	// A Radarr or Sonarr Seerr keeps that HolyArrs could not reach: its row says Needs a look, and opens prefilled (the key stays on the server).
	type Check = { kind: string; url: string; address: string };
	const CHECKS_KEY = 'holyarrs.welcome.checks';
	let checks = $state<Record<string, Check>>({});
	let via = $state<Check | undefined>(undefined);
	function setChecks(c: Record<string, Check>) {
		checks = c;
		keep(CHECKS_KEY, c);
	}
	function onunread(k: string, address: string, from: string, fromUrl: string) {
		if (!(reads[k] ?? []).some((x) => x.address === address)) setChecks({ ...checks, [k]: { kind: from, url: fromUrl, address } });
	}
	function onread(k: string, snap: { id: number; address?: string; counts: Record<string, number> }, from?: string, replaced?: number[]) {
		// The same address, the copy being edited (its address may have changed), or the same app at another address (the server matched its key) is replaced, not added.
		const same = (x: Read) => x.address === (snap.address ?? '') || (k === importFrom && againId !== undefined && x.id === againId) || !!replaced?.includes(x.id);
		const old = (reads[k] ?? []).filter(same).map((x) => x.id);
		const list = [...(reads[k] ?? []).filter((x) => !same(x)), { id: snap.id, address: snap.address ?? '', counts: snap.counts, ...(from ? { from: ctx.appName(from) } : {}) }];
		reads = { ...reads, [k]: list };
		setPendingIds([...pendingIds.filter((i) => !old.includes(i)), snap.id]);
		keep(READS_KEY, reads);
		const w = wordsFor(k, list);
		markImported(k, w);
		// A Radarr or Sonarr that came with Seerr: its row shows a small "From Overseerr" tag.
		const { [k]: _f, ...others } = fromOf;
		fromOf = from && list.length === 1 ? { ...others, [k]: ctx.appName(from) } : others;
		keep(FROM_KEY, fromOf);
		if (checks[k] && (checks[k].address === snap.address || (k === importFrom && via))) {
			const { [k]: _c, ...rest } = checks;
			setChecks(rest);
		}
		// Any read in setup (the picked app, Seerr answering as Overseerr, a linked one) lands back on the app rows; only Continue's move keeps the import open.
		if (!moving) importing = false;
	}
	// The reads this browser kept, checked against the server: a read the server no longer has (its data was reset, or it was
	// removed elsewhere) is dropped, so a card never shows done for nothing and Folders always gets the reads that are really there.
	async function syncReads() {
		const have = await api.get<{ snapshots: { id: number }[] }>('/migrate/snapshots').then((r) => new Set((r.snapshots ?? []).map((x) => x.id)), () => null);
		if (!have) return;
		let changed = false;
		// An import removes the reads it used: a read already imported (not pending) keeps its card; only a pending read the
		// server no longer has goes. With no imports on the server at all, every read it lacks is old (its data was reset).
		const ms = await api.get<{ moves: { state: string; snapshots?: number[] }[] }>('/migrate/moves').then((r) => r.moves ?? [], () => null);
		const moved = !ms || ms.length > 0;
		// A read an import is using right now is gone from the reads but still counts: its card stays.
		const inMove = new Set((ms ?? []).filter((m) => m.state === 'queued' || m.state === 'running' || m.state === 'done').flatMap((m) => m.snapshots ?? []));
		for (const [kind, list] of Object.entries(reads)) {
			const keep2 = list.filter((x) => have.has(x.id) || inMove.has(x.id) || (moved && !pendingIds.includes(x.id)));
			if (keep2.length === list.length) continue;
			changed = true;
			if (keep2.length) reads = { ...reads, [kind]: keep2 };
			else {
				const { [kind]: _r, ...rest } = reads;
				const { [kind]: _w, ...words } = doneWords;
				reads = rest;
				doneWords = words;
				doneApps = doneApps.filter((x) => x !== kind);
			}
		}
		if (pendingIds.some((i) => !have.has(i))) setPendingIds(pendingIds.filter((i) => have.has(i)));
		if (!changed) return;
		keep(READS_KEY, reads);
		keep(WORDS_KEY, doneWords);
		keep(APPS_DONE_KEY, doneApps);
	}
	// Nothing kept in this browser (another browser, or its data cleared): the apps a finished import came from get their
	// ticked card back from the server's own import history, so Parts never looks empty after an import.
	async function restoreFromMoves() {
		if (doneApps.length || Object.keys(reads).length) return;
		const ms = await api.get<{ moves: { state: string; undone_at?: string; report?: { apps?: string[] } }[] }>('/migrate/moves').then((r) => r.moves ?? [], () => []);
		const apps = new Set(ms.filter((m) => m.state === 'done' && !m.undone_at).flatMap((m) => m.report?.apps ?? []));
		for (const k of apps) markImported(k === 'overseerr' ? 'seerr' : k);
		// Reads still waiting on the server (read in another tab, or this browser's data cleared) get their cards back too.
		const snaps = await api.get<{ snapshots: { id: number; kind: string; address?: string; counts: Record<string, number> }[] }>('/migrate/snapshots').then((r) => r.snapshots ?? [], () => []);
		for (const s of snaps) onread(s.kind, s);
	}
	// Remove on a done card: forgets what was read from that app and not moved yet (like Remove in Settings > Import). Seerr and Overseerr share one card, so both go.
	// Copies already moved stay: their items are in HolyArrs, so the card keeps its tick.
	async function removeApp(k: string) {
		for (const kind of k === 'seerr' || k === 'overseerr' ? ['seerr', 'overseerr'] : [k]) {
			const all = reads[kind] ?? [];
			const gone = all.filter((x) => pendingIds.includes(x.id));
			const moved = all.filter((x) => !pendingIds.includes(x.id));
			await Promise.all(gone.map((x) => api.del(`/migrate/snapshots/${x.id}`).catch(() => undefined)));
			setPendingIds(pendingIds.filter((i) => !gone.some((x) => x.id === i)));
			if (moved.length) {
				reads = { ...reads, [kind]: moved };
				doneWords = { ...doneWords, [kind]: wordsFor(kind, moved) };
			} else {
				const { [kind]: _r, ...rest } = reads;
				const { [kind]: _w, ...words } = doneWords;
				reads = rest;
				doneWords = words;
				doneApps = doneApps.filter((x) => x !== kind);
			}
		}
		if (checks[k]) setChecks(Object.fromEntries(Object.entries(checks).filter(([x]) => x !== k)));
		keep(READS_KEY, reads);
		keep(WORDS_KEY, doneWords);
		keep(APPS_DONE_KEY, doneApps);
	}
	// Whether that app (or the shared Seerr card) has a read still waiting to move, so Remove can undo it.
	const canRemove = (k: string, id?: number) =>
		id !== undefined ? pendingIds.includes(id) : (k === 'seerr' || k === 'overseerr' ? ['seerr', 'overseerr'] : [k]).some((x) => (reads[x] ?? []).some((r) => pendingIds.includes(r.id)));
	// Remove in the Manage list: forgets that one copy; the last one goes like Remove on the card.
	async function removeRead(k: string, id: number) {
		const list = (reads[k] ?? []).filter((x) => x.id !== id);
		if (!list.length) {
			managing = undefined;
			return removeApp(k);
		}
		await api.del(`/migrate/snapshots/${id}`).catch(() => undefined);
		setPendingIds(pendingIds.filter((i) => i !== id));
		reads = { ...reads, [k]: list };
		doneWords = { ...doneWords, [k]: wordsFor(k, list) };
		keep(READS_KEY, reads);
		keep(WORDS_KEY, doneWords);
		if (list.length === 1) managing = undefined;
	}
	// Continue under the app rows: no import here; the reads wait for the Folders step, which asks where their folders are and runs the import.
	// Bring my apps never shows the parts screen: the parts follow what was read (changeable later in Settings), then on to Folders.
	async function continueApps() {
		ctx.error = '';
		const has = (...k: string[]) => k.some((x) => (reads[x] ?? []).length > 0);
		if (has('radarr', 'sonarr', 'lidarr', 'bazarr', 'seerr', 'overseerr')) {
			const media = has('radarr', 'sonarr', 'lidarr'); // only Seerr or Bazarr read: the media parts stay as they were
			const w = ctx.partsStep.want;
			ctx.partsStep.want = { movies: media ? has('radarr') : w.movies, shows: media ? has('sonarr') : w.shows, music: media ? has('lidarr') : w.music, subtitles: has('bazarr'), wishlist: has('seerr', 'overseerr') };
		}
		await ctx.partsStep.submitParts(new SubmitEvent('submit'));
	}
	const IMPORTED_KEY = 'holyarrs.welcome.imported';
	function wasImported(): boolean {
		try {
			return sessionStorage.getItem(IMPORTED_KEY) === '1';
		} catch {
			return false;
		}
	}
	let importedNow = $state(false);
	const didImport = $derived(importedNow || wasImported());
	async function imported() {
		await ctx.sourcesStep.afterImport();
		try {
			sessionStorage.setItem(IMPORTED_KEY, '1');
		} catch {
			// No storage: Found lines show for this visit only.
		}
		importedNow = true;
		const kinds = await api.get<{ library_folders: { kind: string }[] }>('/library-folders').then((r) => (r.library_folders ?? []).map((f) => f.kind), () => []);
		await ctx.partsStep.loadParts();
		ctx.partsStep.tickFound(kinds);
		importing = false;
		setPendingIds([]);
		// The move for the reads is done: on to the next screen (Downloaders and indexers, then the parts).
		if (moving) ctx.chooseSub = 'fresh';
		moving = false;
		// Folders step: the import made the library folders; save them and go on (a part still without one stays on Folders).
		if (ctx.step === 'folders') {
			await ctx.mediaStep.loadMedia();
			await ctx.mediaStep.prefillFolders();
			await ctx.mediaStep.saveMedia(false);
		}
	}
	// Bring my apps, step A: the apps imported this visit get a tick (kept for this tab, so a reload still shows them).
	const APPS_DONE_KEY = 'holyarrs.welcome.importedApps';
	let doneApps = $state<string[]>(
		untrack(() => {
			try {
				return JSON.parse(sessionStorage.getItem(APPS_DONE_KEY) ?? '[]') ?? [];
			} catch {
				return [];
			}
		})
	);
	// The count line under each tick, and whether reads are waiting for the move (both kept for this tab too).
	const WORDS_KEY = 'holyarrs.welcome.importedWords';
	const PENDING_KEY = 'holyarrs.welcome.readsPending';
	const load = (key: string, empty: string) => {
		try {
			return JSON.parse(sessionStorage.getItem(key) ?? empty) ?? JSON.parse(empty);
		} catch {
			return JSON.parse(empty);
		}
	};
	const keep = (key: string, v: unknown) => {
		try {
			sessionStorage.setItem(key, JSON.stringify(v));
		} catch {
			// No storage: kept for this visit only.
		}
	};
	let doneWords = $state<Record<string, string>>(untrack(() => load(WORDS_KEY, '{}')));
	const FROM_KEY = 'holyarrs.welcome.importedFrom';
	let fromOf = $state<Record<string, string>>(untrack(() => load(FROM_KEY, '{}')));
	const READS_KEY = 'holyarrs.welcome.reads';
	let reads = $state<Record<string, Read[]>>(untrack(() => load(READS_KEY, '{}')));
	checks = untrack(() => load(CHECKS_KEY, '{}'));

	// The reads not moved yet (an older tab kept only a yes/no: then every read counts).
	const PENDING_IDS_KEY = 'holyarrs.welcome.pendingIds';
	let pendingIds = $state<number[]>(untrack(() => load(PENDING_IDS_KEY, load(PENDING_KEY, 'false') === true ? JSON.stringify(Object.values(reads).flat().map((x) => x.id)) : '[]')));

	const pending = $derived(pendingIds.length > 0);
	function setPendingIds(ids: number[]) {
		pendingIds = ids;
		keep(PENDING_IDS_KEY, ids);
		keep(PENDING_KEY, ids.length > 0);
	}
	function markImported(k: string, words?: string) {
		if (words !== undefined) {
			doneWords = { ...doneWords, [k]: words };
			keep(WORDS_KEY, doneWords);
		}
		if (doneApps.includes(k)) return;
		doneApps = [...doneApps, k];
		keep(APPS_DONE_KEY, doneApps);
	}
	// Continue setup while an import runs: it keeps going on the server, and the later steps say so until it ends.
	let importBusy = $state(false);
	async function checkImport() {
		const ms = await api.get<{ moves: { undo_state?: unknown }[] }>('/migrate/moves').then((r) => r.moves ?? [], () => []);
		importBusy = ms.some((m) => ctx.moveRunning(m) && !m.undo_state);
	}
	// Folders step: once the import has started, setup goes on to the next step; the import keeps going on the server and the later steps say so until it ends.
	function folderAway() {
		importing = false;
		moving = false;
		setPendingIds([]);
		importBusy = true;
		void ctx.partsStep.loadParts();
		ctx.next();
	}
	function importAway() {
		importing = false;
		moving = false;
		setPendingIds([]);
		importBusy = true;
		// Stay on the parts step: Start fresh still asks which parts to turn on, with the running-import note.
		ctx.chooseSub = 'fresh';
	}

	// Moving asks for a recent password check (past the setup window): the move flow only signals it, so the wizard shows the confirm box itself (an owner with no password gets the move flow's set-a-password card).
	let importCheck = $state(false);
	$effect(() => {
		const onReauth = () => {
			// The import runs from Parts (one app) or Folders (after Bring my apps).
			if (ctx.step === 'parts' || ctx.step === 'folders') importCheck = true;
		};
		window.addEventListener('holyarrs:reauth', onReauth);
		return () => window.removeEventListener('holyarrs:reauth', onReauth);
	});
	$effect(() => {
		if (importCheck && session.s.localRecent) importCheck = false;
	});

	return {
		get importCheck() { return importCheck; },
		get importing() { return importing; },
		set importing(v) { importing = v; },
		get importFrom() { return importFrom; },
		get headKind() { return detected || importFrom; },
		get didImport() { return didImport; },
		get doneApps() { return doneApps; },
		get doneWords() { return doneWords; },
		get moving() { return moving; },
		get importBusy() { return importBusy; },
		// Reads still waiting to move: Skip for now must move them too, like Continue.
		get pending() { return pending; },
		get pendingIds() { return pendingIds; },
		mainApps,
		appLogo,
		openImport,
		onread,
		editApp,
		get againId() { return againId; },
		get reads() { return reads; },
		get fromOf() { return fromOf; },
		get checks() { return checks; },
		get via() { return via; },
		onunread,
		get managing() { return managing; },
		set managing(v) { managing = v; },
		removeRead,
		countLine: (k: string, c: Record<string, number>): string => ctx.countLine(k, c),
		removeApp,
		canRemove,
		continueApps,
		imported,
		checkImport,
		importAway,
		syncReads,
		restoreFromMoves,
		folderAway,
	};
}
