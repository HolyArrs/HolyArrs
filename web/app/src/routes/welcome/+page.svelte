<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import Icon, { type IconName } from '#lib/Icon.svelte';
	import TestTag from '#lib/ui/TestTag.svelte';
	import Confirm from '#lib/account/Confirm.svelte';
	import ConnectBridge from '../settings/sources/ConnectBridge.svelte';
	import MoveFlow from '../settings/move/+page.svelte';
	import { page } from '$app/state';
	import { goto, pushState } from '$app/navigation';
	import { onMount, setContext, tick } from 'svelte';
	import { loaderReady, playLoader, SETUP_HOLD_MS } from '#lib/loader.ts';
	import { api, message } from '#lib/api.ts';
	import { to } from '#lib/paths.ts';
	import { session } from '#lib/session.svelte.ts';
	import { partsNow, refreshParts } from '#lib/parts.svelte.ts';
	import { needsProblem, type PartName } from '#lib/parts.ts';
	import { applyParts } from '#lib/controls.ts';
	import { theme } from '#lib/theme.svelte.ts';
	import SigninBackdrop from '#lib/SigninBackdrop.svelte';
	import { emptyForm, formBody, formFrom, kindWords, reachWords, type DownloaderForm, type DownloaderView } from '#lib/downloaders.ts';
	import type { LibraryFolder } from '#lib/types.ts';
	const mark = $derived(theme.value === 'light' ? '/brand/mark-color.svg' : '/brand/mark-color-on-dark.svg');

	// First run, right after the owner is made, forks once: bring the old apps
	// (Import, then a Downloader or Source only if Import brought none), or
	// start fresh (media and folders, a Downloader, a Source). Both end on a
	// short done screen. A later visit to /welcome (no first=1) changes only
	// the parts and returns to Home.
	type Step = 'start' | 'media' | 'downloader' | 'source' | 'import' | 'done';
	const STEPS: Step[] = ['start', 'media', 'downloader', 'source', 'import', 'done'];
	const firstRun = $derived(page.url.searchParams.get('first') === '1');
	function stepFromUrl(q = page.url.searchParams): Step {
		const s = q.get('step');
		// Older addresses: parts and folders are one step now.
		if (s === 'parts' || s === 'folder' || s === 'folders') return 'media';
		return STEPS.includes(s as Step) ? (s as Step) : 'start';
	}
	// The step, and which way the owner went, stay in the address so a reload keeps them.
	let step = $state<Step>(stepFromUrl());
	let fromApps = $state(page.url.searchParams.get('from') === 'apps' || page.url.searchParams.get('step') === 'import');
	// Import path: the steps still needed after the move (no Downloader or Source came over).
	let extra = $state<Step[]>([]);
	let busy = $state(false);
	let error = $state('');

	function go(s: Step) {
		error = '';
		// Done says what is left, so it waits for that list.
		if (s === 'done' && step !== 'done') {
			busy = true;
			void loadTodo().finally(() => {
				busy = false;
				show(s);
			});
		} else show(s);
	}
	// How many steps this visit pushed: the in-app Back walks history while it can.
	let depth = 0;
	function show(s: Step) {
		step = s;
		const q = ['first=1', s !== 'start' ? `step=${s}` : '', fromApps && s !== 'start' && s !== 'import' ? 'from=apps' : ''].filter(Boolean).join('&');
		// One history entry per step, so Back and Forward walk the steps.
		depth += 1;
		pushState(to(`/welcome?${q}`), { extra: [...extra], depth });
		void enter(s);
		// A new step: focus its heading, so keyboard and screen reader start there.
		void tick().then(focusStep);
	}
	// Read out after the step has changed, so it names the new step, not the old one.
	let announce = $state('');
	function focusStep() {
		document.getElementById('welcome-title')?.focus();
		announce = `Step ${stepNo} of ${order.length}: ${head.h}`;
	}
	// Back or Forward in the browser: show the step the address names.
	onMount(() => {
		const onPop = (e: PopStateEvent) => {
			if (!firstRun) return;
			const q = new URLSearchParams(location.search);
			const s = stepFromUrl(q);
			if (s === step) return;
			const st = e.state?.['sveltekit:states'] as { extra?: Step[]; depth?: number } | undefined;
			depth = st?.depth ?? 0;
			const kept = st?.extra;
			error = '';
			fromApps = q.get('from') === 'apps' || s === 'import';
			if (kept) extra = kept;
			step = s;
			void enter(s);
			void tick().then(focusStep);
		};
		window.addEventListener('popstate', onPop);
		return () => window.removeEventListener('popstate', onPop);
	});
	const order = $derived<Step[]>(
		fromApps
			? ['start', 'import', ...(extra.length ? extra : step === 'downloader' || step === 'source' ? [step] : []), 'done']
			: ['start', 'media', 'downloader', 'source', 'done']
	);
	const stepNo = $derived(order.indexOf(step) + 1);
	const prev = $derived(step === 'done' ? undefined : order[order.indexOf(step) - 1]);

	// Setup is done: a short Orbit while Home (or Users) opens.
	// It also ends the setup sign-in's no-password-check window (server side).
	async function home(path = '/') {
		playLoader('', SETUP_HOLD_MS);
		await api.post('/auth/setup/done').catch(() => {});
		await goto(to(path), { replace: true }).finally(loaderReady);
	}

	// ---- The fork ----
	function pickPath(apps: boolean) {
		fromApps = apps;
		extra = [];
		go(apps ? 'import' : 'media');
	}

	// The parts as the server has them now.
	function partsOn(): Record<PartName, boolean> {
		const on = (p: PartName) => partsNow.answer?.parts.find((x) => x.part === p)?.on === true;
		return { movies: on('movies'), shows: on('shows'), music: on('music'), subtitles: on('subtitles'), wishlist: on('wishlist') };
	}

	// ---- Parts: a later visit to /welcome, outside first run ----
	let want = $state<Record<'movies' | 'shows' | 'music' | 'subtitles', boolean>>({ movies: true, shows: true, music: false, subtitles: true });
	let family = $state<'yes' | 'no'>('no');
	const choices: { part: 'movies' | 'shows' | 'music' | 'subtitles'; label: string; icon: IconName }[] = [
		{ part: 'movies', label: 'Movies', icon: 'film' },
		{ part: 'shows', label: 'TV shows', icon: 'tv' },
		{ part: 'music', label: 'Music', icon: 'music' },
		{ part: 'subtitles', label: 'Subtitles', icon: 'subtitle' }
	];
	const full = $derived<Record<PartName, boolean>>({ ...want, wishlist: family === 'yes' });
	const problem = $derived(needsProblem(full));
	async function submitParts(e: SubmitEvent) {
		e.preventDefault();
		if (problem) return void (error = problem);
		error = '';
		busy = true;
		try {
			await applyParts(full);
			await refreshParts();
			await home();
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}

	// ---- Media: a switch and a Library folder per part, on one screen ----
	type MediaPart = 'movies' | 'shows' | 'music';
	type Kind = 'movie' | 'show' | 'music';
	// saved/id: the folder already made; editing reopens it to type a new path.
	type Row = { part: MediaPart; kind: Kind; label: string; icon: IconName; hint: string; path: string; saved: string; id: number | null; editing: boolean; error: string };
	const kindRows: Pick<Row, 'part' | 'kind' | 'label' | 'icon' | 'hint'>[] = [
		{ part: 'movies', kind: 'movie', label: 'Movies', icon: 'film', hint: '/movies' },
		{ part: 'shows', kind: 'show', label: 'TV shows', icon: 'tv', hint: '/tv' },
		{ part: 'music', kind: 'music', label: 'Music', icon: 'music', hint: '/music' }
	];
	let rows = $state<Row[]>([]);
	let media = $state<Record<MediaPart, boolean>>({ movies: true, shows: true, music: false });
	const mediaOff = $derived(!media.movies && !media.shows && !media.music);
	async function loadMedia() {
		let have: LibraryFolder[] = [];
		try {
			have = (await api.get<{ library_folders: LibraryFolder[] }>('/library-folders')).library_folders ?? [];
		} catch {
			// show empty rows
		}
		rows = kindRows.map((k) => {
			const f = have.find((x) => x.kind === k.kind);
			return { ...k, path: '', saved: f?.path ?? '', id: f?.id ?? null, editing: false, error: '' };
		});
	}
	// The server's folder answers, in plain words.
	function folderWords(text: string): string {
		if (/absolute/i.test(text)) return 'Use the full folder path, like /data/movies.';
		if (/already a library folder/i.test(text)) return 'This folder is already used, or sits inside one that is.';
		if (/does not exist|not a folder|no such file/i.test(text)) return "That folder doesn't exist. Check the path.";
		return text ? text[0].toUpperCase() + text.slice(1) : text;
	}
	// skip: keep the parts, add no folders now (Settings has them later).
	async function saveMedia(skip: boolean) {
		if (mediaOff) return;
		error = '';
		// Continue needs a folder for every part that is on (Skip for now adds none).
		if (!skip) {
			const empty = rows.filter((r) => media[r.part] && !(r.saved && !r.editing) && !r.path.trim());
			for (const r of empty) r.error = 'Add a folder or turn this off.';
			if (empty.length) return void tick().then(() => document.getElementById(`folder-${empty[0].kind}`)?.focus());
		}
		busy = true;
		try {
			await refreshParts();
			// Subtitles come on with Movies or TV shows; Requests stays as it is.
			await applyParts({ ...media, subtitles: media.movies || media.shows, wishlist: partsOn().wishlist });
			await refreshParts();
		} catch (err) {
			busy = false;
			return void (error = message(err));
		}
		let failed = false;
		for (const r of rows) {
			if (skip || !media[r.part]) continue;
			if (r.editing && r.path.trim() === r.saved) r.editing = false;
			if ((r.saved && !r.editing) || !r.path.trim()) continue;
			r.error = '';
			// A changed folder is replaced: on first run it holds nothing yet.
			// If the new path is refused, the old one is put back.
			const old = r.editing && r.id != null ? r.saved : '';
			try {
				if (old) {
					await api.del(`/library-folders/${r.id}`);
					r.saved = '';
					r.id = null;
				}
				// Named after its part ("Movies"), not the last bit of the path.
				const f = await api.post<LibraryFolder>('/library-folders', { path: r.path.trim(), kind: r.kind, label: r.label });
				r.saved = f.path;
				r.id = f.id;
				r.editing = false;
			} catch (err) {
				r.error = folderWords(message(err));
				failed = true;
				if (old && r.id == null) {
					try {
						const back = await api.post<LibraryFolder>('/library-folders', { path: old, kind: r.kind, label: r.label });
						r.saved = back.path;
						r.id = back.id;
					} catch {
						// The old folder could not come back; the row stays empty to fill again.
					}
				}
			}
		}
		busy = false;
		if (!failed) go('downloader');
		else {
			await tick();
			document.getElementById(`folder-${rows.find((r) => r.error)?.kind}`)?.focus();
		}
	}

	// ---- Downloader ----
	const dlKinds: DownloaderForm['kind'][] = ['qbittorrent', 'sabnzbd', 'transmission', 'nzbget', 'deluge'];
	const dlPorts: Record<DownloaderForm['kind'], string> = { qbittorrent: '8080', sabnzbd: '8080', transmission: '9091', nzbget: '6789', deluge: '8112' };
	let dl = $state<DownloaderForm>(emptyForm());
	// Every connected downloader, each with its last test answer; the form shows
	// when there is none yet or the owner adds another.
	let dlList = $state<DownloaderView[]>([]);
	let dlTests = $state<Record<number, { ok: boolean; text: string }>>({});
	let dlAdding = $state(false);
	// The downloader being edited (its form is open), or null when adding a new one.
	let dlEditing = $state<number | null>(null);
	// Connect's answer when the downloader didn't answer, shown under Address.
	let dlError = $state('');
	// Shown under User name when an app that signs in turned down a try without one.
	let dlAuth = $state('');
	const dlForm = $derived(dlList.length === 0 || dlAdding);
	const dlUser = $derived(dl.kind === 'qbittorrent' || dl.kind === 'transmission' || dl.kind === 'nzbget');
	function pickDl(k: DownloaderForm['kind']) {
		error = '';
		dlError = '';
		dlAuth = '';
		const url = dl.url;
		dl = { ...emptyForm(k), url: url !== 'http://' ? url : 'http://' };
	}
	async function listDownloaders(): Promise<DownloaderView[]> {
		try {
			return (await api.get<{ downloaders: DownloaderView[] }>('/downloaders')).downloaders ?? [];
		} catch {
			return [];
		}
	}
	async function loadDownloader() {
		dlList = await listDownloaders();
		// Each one is tested again, so the row shows its version or that it is down.
		await Promise.all(dlList.map((d) => testDl(d.id)));
	}
	async function testDl(id: number) {
		try {
			const t = await api.post<{ ok: boolean; text: string }>(`/downloaders/${id}/test`);
			dlTests = { ...dlTests, [id]: { ok: t.ok, text: t.ok ? t.text : reachWords(t.text) } };
		} catch (err) {
			dlTests = { ...dlTests, [id]: { ok: false, text: reachWords(message(err)) } };
		}
	}
	function editDl(d: DownloaderView) {
		error = '';
		dlError = '';
		dl = formFrom(d);
		dlEditing = d.id;
		dlAdding = true;
	}
	async function removeDl(d: DownloaderView) {
		error = '';
		busy = true;
		try {
			await api.del(`/downloaders/${d.id}`);
			dlList = dlList.filter((x) => x.id !== d.id);
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}
	async function saveDownloader(e: SubmitEvent) {
		e.preventDefault();
		if (!dlForm) return afterDownloader();
		error = '';
		busy = true;
		dlError = '';
		dlAuth = '';
		// A bare host:port gets http:// in front.
		const u = dl.url.trim();
		if (u && !/^[a-z][a-z0-9+.-]*:\/\//i.test(u)) dl.url = `http://${u}`;
		try {
			const editing = dlEditing;
			const v = editing != null ? await api.put<DownloaderView>(`/downloaders/${editing}`, formBody(dl)) : await api.post<DownloaderView>('/downloaders', formBody(dl));
			if (editing != null) dlList = dlList.map((x) => (x.id === v.id ? v : x));
			// Only a downloader that answers becomes a row; a new one that doesn't is taken back and the form stays.
			const t = await api.post<{ ok: boolean; text: string }>(`/downloaders/${v.id}/test`).catch((err) => ({ ok: false, text: message(err) }));
			if (!t.ok) {
				if (editing == null) await api.del(`/downloaders/${v.id}`).catch(() => {});
				else dlEditing = v.id;
				const words = reachWords(t.text);
				if (!askSignIn(words)) dlError = words;
				return;
			}
			dlTests = { ...dlTests, [v.id]: { ok: t.ok, text: t.text } };
			if (editing == null) dlList = [...dlList, v];
			dlEditing = null;
			dlAdding = false;
			dl = emptyForm();
		} catch (err) {
			if (!askSignIn(reachWords(message(err)))) error = message(err);
		} finally {
			busy = false;
			// A failed Connect: focus the field the answer is about (the key, or else the address).
			if (dlError || (error && dlForm)) void tick().then(() => document.getElementById(!dlError && /key|password/i.test(error) ? 'dl-secret' : 'dl-url')?.focus());
		}
	}
	// An app that signs in, turned down with no user name typed: ask for both,
	// under User name (the server's answer names an API key field the wizard doesn't have).
	function askSignIn(words: string): boolean {
		if (!dlUser || dl.username.trim() || words.startsWith("Can't reach")) return false;
		dlAuth = 'Add the user name and password.';
		void tick().then(() => document.getElementById('dl-user')?.focus());
		return true;
	}
	// Fresh setup always asks for a Source next; after an import only when none came over.
	async function afterDownloader() {
		if (fromApps && (await sourceCount()) > 0) return go('done');
		go('source');
	}

	// ---- Source ----
	let sources = $state(0);
	// Sources plus linked Jackett/Prowlarr (a fresh link may not have synced its Sources yet).
	async function sourceCount(): Promise<number> {
		const n = async (path: string, key: string) => {
			try {
				return ((await api.get<Record<string, unknown[]>>(path))[key] ?? []).length;
			} catch {
				return 0;
			}
		};
		const [s, b] = await Promise.all([n('/sources', 'sources'), n('/bridges', 'bridges')]);
		return s + b;
	}
	async function loadSources() {
		sources = await sourceCount();
	}

	// ---- Import: the Settings move flow, right here in the panel ----
	const IMPORT_QUERY = 'first=1&step=import';
	// Import offers every app, except a downloader or Prowlarr that is already
	// connected here. The list is filled before the Read sheet first opens.
	const hideApps: string[] = [];
	setContext('move-hide-apps', hideApps);
	setContext('move-hide-count', true);
	let moveStep = $state('read');
	// After the move: ask only for what Import didn't bring.
	async function afterImport() {
		busy = true;
		const [dls, srcs] = await Promise.all([listDownloaders(), sourceCount()]);
		busy = false;
		extra = [...(dls.length ? [] : (['downloader'] as Step[])), ...(srcs ? [] : (['source'] as Step[]))];
		go(extra[0] ?? 'done');
	}
	async function loadHidden() {
		const [dls, bridges] = await Promise.all([
			listDownloaders(),
			api.get<{ bridges: { kind: string }[] }>('/bridges').then((r) => r.bridges ?? [], () => [])
		]);
		hideApps.length = 0;
		for (const k of ['qbittorrent', 'sabnzbd'] as const) if (dls.some((d) => d.kind === k)) hideApps.push(k);
		if (bridges.some((b) => b.kind === 'prowlarr')) hideApps.push('prowlarr');
	}

	// ---- Done ----
	// What a skipped step left to do, each a Settings page.
	type Todo = { label: string; path: string };
	let todo = $state<Todo[]>([]);
	// What is set: a short line each ("Movies · /data/movies", "qBittorrent", "Prowlarr").
	let haveSet = $state<string[]>([]);
	async function loadTodo() {
		const list = <T,>(path: string, key: string) => api.get<Record<string, T[]>>(path).then((r) => r[key] ?? [], () => null);
		const [fl, dl, br, srcs] = await Promise.all([
			list<LibraryFolder>('/library-folders', 'library_folders'),
			list<DownloaderView>('/downloaders', 'downloaders'),
			list<{ kind: string }>('/bridges', 'bridges'),
			sourceCount()
		]);
		const kindName: Record<string, string> = { movie: 'Movies', show: 'TV shows', music: 'Music' };
		const bridgeName: Record<string, string> = { prowlarr: 'Prowlarr', jackett: 'Jackett' };
		haveSet = [
			...(fl ?? []).map((f) => `${kindName[f.kind] ?? f.label} · ${f.path}`),
			...(dl ?? []).map((d) => kindWords[d.kind] ?? d.name),
			...(br ?? []).map((b) => bridgeName[b.kind] ?? b.kind)
		];
		// Unknown (the list failed to load) counts as set, so nothing is asked twice.
		const folders = fl ? fl.length : 1;
		const dls = dl ? dl.length : 1;
		todo = [
			...(folders ? [] : [{ label: 'Folders', path: '/settings/library-folders' }]),
			...(dls ? [] : [{ label: 'Downloader', path: '/settings/downloaders' }]),
			...(srcs ? [] : [{ label: 'Source', path: '/settings/sources' }])
		];
	}
	// Invite family: turns on Requests (what the old "family" question did), then opens Users.
	async function invite() {
		error = '';
		busy = true;
		try {
			await refreshParts();
			const now = partsOn();
			if (!now.wishlist) await applyParts({ ...now, wishlist: true });
			await home('/settings/users');
		} catch (err) {
			error = message(err);
			busy = false;
		}
	}

	function back() {
		if (!prev) return;
		// Step back through history, so the browser's Back and Forward stay in order.
		if (depth > 0) history.back();
		else go(prev);
	}
	async function enter(s: Step) {
		if (s === 'media') await loadMedia();
		if (s === 'downloader') await loadDownloader();
		if (s === 'source') await loadSources();
		if (s === 'import') await loadHidden();
		if (s === 'done' && !todo.length) await loadTodo();
	}
	onMount(() => {
		if (firstRun) void enter(step);
	});
	// Moving asks for a recent password check (past the setup window): the
	// move flow only signals it, so the wizard shows the confirm box itself
	// (an owner with no password gets the move flow's set-a-password card).
	let importCheck = $state(false);
	onMount(() => {
		const onReauth = () => {
			if (step === 'import') importCheck = true;
		};
		window.addEventListener('holyarrs:reauth', onReauth);
		return () => window.removeEventListener('holyarrs:reauth', onReauth);
	});
	$effect(() => {
		if (importCheck && session.s.localRecent) importCheck = false;
	});

	const titles: Record<Step, { h: string; p: string }> = {
		start: { h: 'Coming from other apps?', p: 'We can bring over your shows, movies, folders and settings. Media files stay where they are.' },
		media: { h: 'Where’s your media?', p: 'Turn on what you collect and add its folder.' },
		downloader: { h: 'Which app downloads?', p: 'Connect the one you use.' },
		source: { h: 'Where should we search?', p: 'Link Prowlarr or Jackett and your sites come with it.' },
		import: { h: 'Bring your apps', p: 'We only read them. Nothing moves until you say so.' },
		done: { h: 'You’re set', p: 'Find something on Home and we’ll get it.' }
	};
	const head = $derived(
		!firstRun
			? { h: 'What do you use?', p: 'You can change this later.' }
			: step === 'done' && todo.length
				? { h: 'HolyArrs is ready', p: 'A few things are left. Finish them anytime in Settings.' }
				: step === 'done' && fromApps
					? { h: 'Welcome home', p: 'Your library is here. Your old apps keep running.' }
					: titles[step]
	);
</script>

<svelte:head><title>{head.h} · HolyArrs</title></svelte:head>

{#if firstRun}<SigninBackdrop />{/if}
<div class="welcome" class:over-art={firstRun}>
	<!-- First run keeps the sign-in look: the same moving backdrop behind one glass panel. -->
	<div class="sheet" class:glass={firstRun} class:light={theme.value === 'light'} data-signin-panel={firstRun ? '' : undefined}>
	<div class="brand"><img src={to(mark)} alt="" width="32" height="32" /><span>HolyArrs</span></div>
	{#if firstRun && prev && session.can('users') && (step !== 'import' || moveStep === 'read')}
		<span class="corner"><IconButton icon="chevronLeft" variant="tinted" label="Back" onclick={back} disabled={busy} /></span>
	{/if}
	{#if firstRun}
		<ol class="dots" aria-label="Step {stepNo} of {order.length}">
			{#each order as s, i (s)}<li class:done={i + 1 < stepNo} class:now={i + 1 === stepNo}></li>{/each}
		</ol>
	{/if}
	<header class="head">
		<h1 id="welcome-title" tabindex="-1">{head.h}</h1>
		<p>{head.p}</p>
	</header>
	{#if firstRun}<p class="sr-only" aria-live="polite">{announce}</p>{/if}

	{#if !session.can('users') || (firstRun && step !== 'start' && !session.can('settings'))}
		<p class="notice warn">Only the owner or an admin with Settings access can finish setup.</p>
	{:else if !firstRun}
		<form class="card" onsubmit={submitParts} data-testid="welcome">
			<!-- Big tiles, each a real checkbox (it covers the tile, so a tap anywhere toggles it). -->
			<div class="tiles" role="group" aria-label="Parts">
				{#each choices as c (c.part)}
					<label class="tile" class:on={want[c.part]}>
						<input type="checkbox" bind:checked={want[c.part]} aria-label={c.label} />
						<span class="tick" aria-hidden="true"><Icon name="check" size={14} stroke={2.5} /></span>
						<Icon name={c.icon} size={28} />
						<span class="tile-label">{c.label}</span>
					</label>
				{/each}
			</div>

			<fieldset class="family">
				<legend>Will family or friends ask for titles?</legend>
				<div class="seg">
					<label class:on={family === 'no'}><input type="radio" name="family" value="no" bind:group={family} />No, just me</label>
					<label class:on={family === 'yes'}><input type="radio" name="family" value="yes" bind:group={family} />Yes</label>
				</div>
				<p class="hint">Yes adds a Requests page.</p>
			</fieldset>

			{#if error || problem}<p class="form-error" role="alert">{error || problem}</p>{/if}
			<Button variant="primary" size="lg" full type="submit" disabled={busy || !!problem} {busy}>Continue</Button>
		</form>
	{:else if step === 'start'}
		<div class="card" data-testid="welcome-start">
			<div class="tiles two-up">
				<button class="tile big" type="button" onclick={() => pickPath(true)}>
					<Icon name="download" size={26} />
					<span class="tile-label">Bring my apps</span>
					<span class="tile-sub">Sonarr, Radarr, Lidarr, Overseerr…</span>
				</button>
				<button class="tile big" type="button" onclick={() => pickPath(false)}>
					<Icon name="plus" size={26} />
					<span class="tile-label">Start fresh</span>
					<span class="tile-sub">Pick folders and a downloader.</span>
				</button>
			</div>
		</div>
	{:else if step === 'media'}
		<form class="card" onsubmit={(e) => { e.preventDefault(); void saveMedia(false); }} data-testid="welcome-media">
			<ul class="rows">
				{#each rows as r (r.kind)}
					<li class="frow" class:off={!media[r.part]}>
						<span class="ficon" aria-hidden="true"><Icon name={r.icon} size={22} /></span>
						<div class="fbody">
							<!-- The whole top line labels the switch, so the tap target is the row. -->
							<label class="ftop">
								<span class="fname">{r.label}</span>
								<span class="switch"><input type="checkbox" bind:checked={media[r.part]} onchange={() => (r.error = '')} disabled={busy} /><span class="sw" aria-hidden="true"></span></span>
							</label>
							{#if media[r.part]}
								{#if r.saved && !r.editing}
									<div class="fsaved"><span class="mono">{r.saved}</span><button type="button" class="quiet" disabled={busy} onclick={async () => { r.path = r.saved; r.editing = true; await tick(); document.getElementById(`folder-${r.kind}`)?.focus(); }} aria-label="Change {r.label} folder">Change</button></div>
								{:else}
									<input id="folder-{r.kind}" class="input mono" bind:value={r.path} oninput={() => (r.error = '')} aria-label="{r.label} folder" placeholder={r.hint} autocomplete="off" spellcheck="false" aria-invalid={r.error ? 'true' : undefined} aria-describedby={r.error ? `folder-${r.kind}-error` : undefined} />
									{#if r.error}<p class="form-error" id="folder-{r.kind}-error" role="alert">{r.error}</p>{/if}
									{#if r.editing && r.saved}<button type="button" class="quiet keep" disabled={busy} onclick={() => { r.path = r.saved; r.editing = false; r.error = ''; }}>Keep {r.saved}</button>{/if}
								{/if}
							{/if}
						</div>
					</li>
				{/each}
			</ul>
			<p class="hint">On Docker, use the path inside the container.</p>
			{#if error || mediaOff}<p class="form-error" role="alert">{error || 'Turn on at least one.'}</p>{/if}
			<Button variant="primary" size="lg" full type="submit" disabled={busy || mediaOff} {busy}>Continue</Button>
			<button class="alt" type="button" disabled={busy || mediaOff} onclick={() => saveMedia(true)}>Skip for now</button>
		</form>
	{:else if step === 'downloader'}
		<form class="card" onsubmit={saveDownloader} data-testid="welcome-downloader">
			{#if dlList.length}
				<ul class="conn" aria-label="Connected downloaders">
					{#each dlList as d (d.id)}
						{@const test = dlTests[d.id]}
						<li>
							<div class="conn-row">
							<div class="conn-text"><b>{d.name}</b><span class="addr mono">{d.url}</span></div>
							{#if test}<TestTag ok={test.ok} text={test.text} app={kindWords[d.kind]} />{/if}
							</div>
							{#if test && !test.ok}<p class="form-error" role="alert">{test.text}</p>{/if}
							{#if !dlForm}
								<div class="row-acts">
									<button type="button" class="quiet" disabled={busy} onclick={() => editDl(d)} aria-label="Edit {d.name}">Edit</button>
									<button type="button" class="quiet danger" disabled={busy} onclick={() => removeDl(d)} aria-label="Remove {d.name}">Remove</button>
								</div>
							{/if}
						</li>
					{/each}
				</ul>
				{#if !dlForm}
					<button class="add-row" type="button" disabled={busy} onclick={() => { error = ''; dlAdding = true; }}><Icon name="plus" size={16} />Add another</button>
				{/if}
			{/if}
			{#if dlForm}
				<div class="tiles five" role="radiogroup" aria-label="Downloader app">
					{#each dlKinds as k (k)}
						<label class="tile small" class:on={dl.kind === k}>
							<input type="radio" name="dlkind" value={k} checked={dl.kind === k} onchange={() => pickDl(k)} aria-label={kindWords[k]} />
							<span class="tile-label">{kindWords[k]}</span>
						</label>
					{/each}
				</div>
				<div class="field">
					<label for="dl-url">Address</label>
					<input id="dl-url" class="input mono" type="text" inputmode="url" bind:value={dl.url} oninput={() => (dlError = '')} placeholder="192.168.1.10:{dlPorts[dl.kind]}" autocomplete="off" spellcheck="false" aria-invalid={dlError ? 'true' : undefined} aria-describedby={dlError ? 'dl-url-error' : undefined} />
					{#if dlError}<p class="form-error" id="dl-url-error" role="alert">{dlError}</p>{/if}
				</div>
				<div class="two">
					{#if dlUser}
						<div class="field">
							<label for="dl-user">User name</label>
							<input id="dl-user" class="input" bind:value={dl.username} oninput={() => (dlAuth = '')} autocomplete="off" autocapitalize="none" spellcheck="false" aria-invalid={dlAuth ? 'true' : undefined} aria-describedby={dlAuth ? 'dl-user-error' : undefined} />
								{#if dlAuth}<p class="form-error" id="dl-user-error" role="alert">{dlAuth}</p>{/if}
						</div>
					{/if}
					<div class="field">
						<label for="dl-secret">{dl.kind === 'sabnzbd' ? 'API key' : 'Password'}</label>
						<input id="dl-secret" class="input" type="password" bind:value={dl.secret} autocomplete="new-password" />
					</div>
				</div>
			{/if}
			{#if error}<p class="form-error" role="alert">{error}</p>{/if}
			<Button variant="primary" size="lg" full type="submit" disabled={busy} {busy}>{dlEditing != null ? 'Save' : dlForm ? 'Connect' : 'Continue'}</Button>
			{#if dlForm && dlList.length}
				<button class="alt" type="button" disabled={busy} onclick={() => { error = ''; dlError = ''; dlAdding = false; dlEditing = null; dl = emptyForm(); }}>Cancel</button>
			{:else}
				<button class="alt" type="button" disabled={busy} onclick={afterDownloader}>Skip for now</button>
			{/if}
		</form>
	{:else if step === 'source'}
		<div class="card source" data-testid="welcome-source">
			<ConnectBridge onchange={loadSources} compact />
			<div class="foot">
				{#if sources > 0}
					<Button variant="primary" size="lg" full type="button" disabled={busy} onclick={() => go('done')}>Continue</Button>
				{:else}
					<button class="alt" type="button" disabled={busy} onclick={() => go('done')}>Skip for now</button>
				{/if}
			</div>
		</div>
	{:else if step === 'import'}
		<div class="card move" data-testid="welcome-move">
			{#if importCheck && session.s.hasPassword}
				<Confirm action="password">{#snippet children()}{/snippet}</Confirm>
			{/if}
			<MoveFlow embedded={{ query: IMPORT_QUERY, ondone: afterImport, onstep: (s) => (moveStep = s) }} />
			{#if moveStep === 'read'}
				<button class="alt" type="button" disabled={busy} onclick={() => pickPath(false)}>Start fresh instead</button>
			{/if}
		</div>
	{:else}
		<div class="card" data-testid="welcome-done">
			{#if haveSet.length}
				<ul class="todo" aria-label="Set up">
					{#each haveSet as h (h)}
						<li><span class="todo-row"><span class="set-name"><span class="set-tick" aria-hidden="true"><Icon name="check" size={14} stroke={2.5} /></span><span>{h}</span></span></span></li>
					{/each}
				</ul>
			{/if}
			{#if todo.length}
				<ul class="todo" aria-label="Still to do">
					{#each todo as t (t.label)}
						<li><a href={to(t.path)} onclick={(e) => { e.preventDefault(); void home(t.path); }}><span>{t.label}</span><span class="todo-end"><span class="todo-state">Not set</span><Icon name="chevronRight" size={16} /></span></a></li>
					{/each}
				</ul>
			{/if}
			{#if error}<p class="form-error" role="alert">{error}</p>{/if}
			<Button variant="primary" size="lg" full type="button" disabled={busy} {busy} onclick={() => home()}>Open Home</Button>
			<button class="alt" type="button" disabled={busy} onclick={invite}>Invite family</button>
		</div>
	{/if}
	</div>
</div>

<style>
	.welcome {
		position: relative;
		/* One width for every step: what Import's rows need. */
		width: min(680px, calc(100% - 32px));
		margin: 0 auto;
		padding: clamp(28px, 7vh, 72px) 0 48px;
		box-sizing: border-box;
	}
	.brand { display: flex; align-items: center; justify-content: center; gap: 10px; font: 600 17px/1 var(--font-ui); color: var(--text); }
	/* The round back button sits top-left, level with the logo. */
	.corner { position: absolute; top: -2px; left: 0; }
	.sheet { position: relative; }
	/* First run: the sign-in page's glass over its backdrop (same tint and blur). */
	.over-art { z-index: 1; }
	.sheet.glass {
		--pad: clamp(22px, 4vw, 34px);
		padding: var(--pad);
		border-radius: 28px;
		background: rgb(14 14 18 / 0.46);
		-webkit-backdrop-filter: blur(40px) saturate(1.7) brightness(0.72);
		backdrop-filter: blur(40px) saturate(1.7) brightness(0.72);
		border: 1px solid rgb(255 255 255 / 0.07);
		box-shadow: 0 0 0 1px rgb(0 0 0 / 0.35), 0 30px 90px rgb(0 0 0 / 0.55), 0 6px 18px rgb(0 0 0 / 0.35);
	}
	.sheet.glass.light {
		background: rgb(255 253 250 / 0.66);
		-webkit-backdrop-filter: blur(64px) saturate(1.5) brightness(1.5);
		backdrop-filter: blur(64px) saturate(1.5) brightness(1.5);
		border-color: rgb(255 255 255 / 0.55);
		box-shadow: 0 0 0 1px rgb(255 255 255 / 0.25), 0 24px 70px rgb(0 0 0 / 0.2), 0 4px 14px rgb(0 0 0 / 0.08);
		--focus-ring: 0 0 0 2px rgb(255 255 255 / 0.9), 0 0 0 4px #1d1d1f;
	}
	@supports not ((backdrop-filter: blur(1px)) or (-webkit-backdrop-filter: blur(1px))) {
		.sheet.glass { background: rgb(18 18 22 / 0.92); }
		.sheet.glass.light { background: rgb(248 246 242 / 0.94); }
	}
	.glass .corner { top: calc(var(--pad) - 2px); left: var(--pad); }
	.glass .head { margin-bottom: 22px; }
	/* Inside the glass the step's card is just its content. */
	.sheet.glass > :global(.card) { background: transparent; box-shadow: none; padding: 0; border-radius: 0; }
	/* The Jackett/Prowlarr card on the Source step joins the glass too: no second box, edges lined up. */
	.sheet.glass :global(.ui-settings) { background: transparent; box-shadow: none; border-radius: 0; overflow: visible; }
	.sheet.glass :global(.ui-settings > header), .sheet.glass :global(.ui-settings > .body) { padding-left: 0; padding-right: 0; }
	.sheet.glass :global(.ui-settings > header) { padding-top: 0; }
	.dots { display: flex; justify-content: center; gap: 6px; margin: 22px 0 0; }
	.dots li { width: 6px; height: 6px; border-radius: 3px; background: var(--line-strong); transition: width var(--dur) var(--ease-out), background var(--dur) var(--ease-out); }
	.dots li.done { background: var(--text-muted); }
	.dots li.now { width: 22px; background: var(--accent-fill); }
	.head { margin: 18px 0 28px; text-align: center; }
	.head h1 { font: var(--t-large-title); letter-spacing: var(--t-large-title-track); text-wrap: balance; }
	.head p { margin: 8px auto 0; max-width: 44ch; color: var(--text-muted); font: var(--t-body); text-wrap: pretty; }
	.sheet > :global(.card) {
		position: relative;
		border: 0;
		border-radius: 24px;
		background: var(--surface-1);
		box-shadow: var(--shadow-card), 0 0 0 1px var(--line);
		padding: clamp(18px, 4vw, 28px);
	}
	fieldset { border: 0; padding: 0; margin: 0; min-width: 0; }
	legend { margin-bottom: 12px; font: var(--t-headline); color: var(--text); }

	/* Tiles: an icon and a word; picked tiles take the accent, with a tick. */
	.tiles { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 10px; }
	.tiles.five { grid-template-columns: repeat(3, minmax(0, 1fr)); margin-bottom: 18px; }
	.tiles.two-up { grid-template-columns: repeat(2, minmax(0, 1fr)); }
	.tile {
		position: relative;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 10px;
		min-height: 108px;
		padding: 14px 8px;
		border-radius: 16px;
		border: 1px solid var(--line);
		background: var(--surface-2);
		color: var(--text-muted);
		font: inherit;
		text-decoration: none;
		cursor: pointer;
		transition: border-color var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out), transform var(--dur-fast) var(--ease-out);
	}
	.tile.small { min-height: 56px; padding: 10px 6px; }
	.tile.big { min-height: 150px; padding: 18px 14px; }
	.tile.big:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	/* The real control covers the whole tile, invisible, so taps and clicks land on it. */
	.tile input { position: absolute; inset: 0; width: 100%; height: 100%; margin: 0; opacity: 0; cursor: pointer; }
	.tile-label { font: 600 14px/1.2 var(--font-ui); color: var(--text); text-align: center; }
	.tile-sub { font: var(--t-footnote); color: var(--text-muted); text-align: center; }
	@media (hover: hover) { .tile:hover { border-color: var(--line-strong); } }
	.tile:active { transform: scale(0.98); }
	.tile.on { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 7%, var(--surface-1)); color: var(--accent-text); }
	.tile:has(input:focus-visible) { box-shadow: var(--focus-ring); }
	.tick {
		position: absolute;
		top: 9px;
		right: 9px;
		width: 20px;
		height: 20px;
		border-radius: 50%;
		display: grid;
		place-items: center;
		border: 1.5px solid var(--line-strong);
		color: transparent;
		transition: background-color var(--dur-fast) var(--ease-out), border-color var(--dur-fast) var(--ease-out);
	}
	.tile.on .tick { background: var(--accent); border-color: var(--accent); color: #fff; }

	/* Family: a two-way switch, the picked side lifted. */
	.family { margin-top: 26px; }
	.seg { display: grid; grid-template-columns: 1fr 1fr; gap: 4px; padding: 4px; border-radius: 12px; background: var(--surface-2); border: 1px solid var(--line); }
	.seg label {
		position: relative;
		display: grid;
		place-items: center;
		min-height: 40px;
		border-radius: 9px;
		color: var(--text-muted);
		font: 500 15px/1 var(--font-ui);
		cursor: pointer;
		transition: background-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out);
	}
	.seg input { position: absolute; inset: 0; width: 100%; height: 100%; margin: 0; opacity: 0; cursor: pointer; }
	.seg label.on { background: var(--surface-1); color: var(--text); box-shadow: 0 1px 3px rgb(0 0 0 / 0.14), 0 0 0 0.5px var(--line); }
	.seg label:has(input:focus-visible) { box-shadow: var(--focus-ring); }
	.hint { display: block; margin-top: 10px; color: var(--text-muted); font: var(--t-footnote); }

	.welcome :global(.card) > :global(.ui-btn) { margin-top: 24px; }
	/* Done: the one button sits right under the heading. */
	[data-testid='welcome-done'] > :global(.ui-btn) { margin-top: 0; }
	.alt {
		display: block;
		margin: 10px auto 0;
		min-height: 36px;
		padding: 0 12px;
		border: 0;
		border-radius: var(--r-sm);
		background: none;
		color: var(--text-muted);
		font: 500 14px/1 var(--font-ui);
		cursor: pointer;
	}
	@media (hover: hover) { .alt:hover { color: var(--text); background: color-mix(in srgb, var(--text) 6%, transparent); } }
	.alt:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.alt:disabled { opacity: 0.5; cursor: default; }

	/* Media: one row per part, its icon, its name and switch, then its folder. */
	.rows { display: flex; flex-direction: column; gap: 10px; }
	.frow { display: flex; align-items: flex-start; gap: 14px; padding: 12px; border-radius: 16px; border: 1px solid var(--line); background: var(--surface-2); transition: opacity var(--dur-fast) var(--ease-out); }
	.frow.off .ficon { opacity: 0.55; }
	.frow.off .fname { color: var(--text-muted); }
	.ficon { flex: none; width: 40px; height: 40px; border-radius: 12px; display: grid; place-items: center; background: var(--surface-1); color: var(--text-muted); box-shadow: 0 0 0 1px var(--line); }
	.fbody { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 8px; }
	.ftop { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 44px; cursor: pointer; }
	.ftop .switch { margin: 0; flex: none; }
	.fname { font: 600 15px/1.2 var(--font-ui); color: var(--text); }
	.fsaved { display: flex; align-items: center; justify-content: space-between; gap: 10px; color: var(--text-muted); font-size: 13px; overflow-wrap: anywhere; }
	.frow .form-error { margin-top: 0; }

	/* A connected downloader or source: a plain row, its version as a small tag. */
	.conn { list-style: none; margin: 0 0 12px; padding: 0; display: grid; gap: 10px; }
	.conn-row { display: flex; align-items: center; gap: 12px; padding: 14px 16px; border-radius: 16px; background: var(--surface-2); border: 1px solid var(--line); }
	.conn-text { flex: 1; min-width: 0; }
	.conn-text b { display: block; font: 600 15px/1.3 var(--font-ui); color: var(--text); }
	.conn-text .addr { display: block; margin-top: 2px; font-size: 12.5px; color: var(--text-muted); overflow-wrap: anywhere; }
	.quiet.keep { align-self: flex-start; padding-left: 0; overflow-wrap: anywhere; }
	.quiet { flex: none; align-self: center; display: inline-flex; align-items: center; min-height: 32px; border: 0; padding: 0 8px; background: none; color: var(--text-muted); font: 500 13px/1.2 var(--font-ui); cursor: pointer; }
	.quiet:hover:not(:disabled) { color: var(--text); }
	.quiet.danger:hover:not(:disabled) { color: var(--danger-text, #ff6961); }
	.row-acts { display: flex; gap: 16px; padding: 6px 4px 0; }
	.quiet:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 6px; }
	.add-row { width: 100%; margin-top: 0; display: flex; align-items: center; gap: 10px; min-height: 48px; padding: 0 16px; border: 1px dashed color-mix(in srgb, var(--text) 18%, transparent); border-radius: 16px; background: none; color: var(--text-muted); font: 500 15px/1 var(--font-ui); cursor: pointer; transition: color 0.15s, border-color 0.15s; }
	.add-row:hover:not(:disabled) { color: var(--text); border-color: color-mix(in srgb, var(--text) 32%, transparent); }
	.add-row:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.two { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 12px; margin-top: 12px; align-items: start; }
	.two > :global(.field) { margin: 0; }
	.source :global(.settings-card), .source :global(section) { box-shadow: none; }
	/* The wizard heading says it all: no second title in the card. */
	.sheet.glass .source :global(.ui-settings > header) { display: none; }
	.source .foot { margin-top: 8px; }
	.source .foot .alt { margin-top: 4px; }
	.head h1:focus { outline: none; }
	/* Done with steps left: each one a row that opens its Settings page. */
	.todo { list-style: none; margin: 0 0 20px; padding: 0; display: grid; gap: 8px; }
	.todo a, .todo-row { display: flex; align-items: center; justify-content: space-between; min-height: 48px; padding: 0 16px; border-radius: 14px; background: var(--surface-2); border: 1px solid var(--line); color: var(--text); font: 600 15px/1.2 var(--font-ui); text-decoration: none; }
	@media (hover: hover) { .todo a:hover { border-color: var(--line-strong); } }
	.todo a:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.todo :global(svg) { color: var(--text-muted); }
	.todo-end { display: inline-flex; align-items: center; gap: 6px; }
	.set-name { display: inline-flex; align-items: center; gap: 10px; min-width: 0; overflow-wrap: anywhere; padding: 12px 0; }
	.set-tick { flex: none; width: 20px; height: 20px; border-radius: 50%; display: grid; place-items: center; background: var(--accent); color: #fff; }
	.todo .set-tick :global(svg) { color: #fff; }
	.todo-state { color: var(--text-muted); font: 500 14px/1.2 var(--font-ui); }
	/* Import: the move flow's cards stack inside the glass with plain gaps. */
	.move { display: flex; flex-direction: column; gap: 18px; }
	.move .alt { margin-top: 0; }
	/* No tab bar here, so the phone footer sits on the bottom edge. */
	.move :global(.step-foot) { --tabbar: 0px; }

	@media (max-width: 520px) {
		.tiles { grid-template-columns: repeat(2, minmax(0, 1fr)); }
		.tiles.five { grid-template-columns: repeat(2, minmax(0, 1fr)); }
		.tiles.two-up { grid-template-columns: 1fr; }
		.tile { min-height: 96px; }
		.tile.small { min-height: 52px; }
		.tile.big { min-height: 110px; }
		.frow { gap: 12px; padding: 10px; }
		/* A phone fits the short steps without scrolling. */
		.welcome { padding: 12px 0 0; }
		.rows { gap: 8px; }
		.welcome :global(.card) > :global(.ui-btn) { margin-top: 16px; }
		.alt { margin-top: 4px; }
		.head { margin: 12px 0 16px; }
		.glass .head { margin-bottom: 16px; }
		.hint { margin-top: 8px; }
		.dots { margin-top: 16px; }
		.ficon { width: 36px; height: 36px; }
	}
	@media (pointer: coarse), (max-width: 520px) {
		.alt { min-height: 44px; }
		.source :global(.acts .ui-btn) { min-height: 44px; }
		.seg label { min-height: 44px; }
	}
</style>
