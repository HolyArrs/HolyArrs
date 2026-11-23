<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SetupBar, firstOpen, stepsFor, phaseOf, phasesFor, stepFromName, resumeStep, nextStep, prevStep, type Plan, type Step } from '#lib/setup/index.ts';
	import { LinkServers, type Library, type ServerKind } from '#lib/setup/index.ts';
	import { trailButtons } from '#lib/setup/index.ts';
	import { createWelcomeParts } from '#lib/setup/index.ts';
	import { createWelcomeMedia } from '#lib/setup/index.ts';
	import { createWelcomeDownloader } from '#lib/setup/index.ts';
	import { createWelcomeSources } from '#lib/setup/index.ts';
	import { createWelcomeBring, createWelcomeFinish, createWelcomeTools, BringApps, PartTiles, DownloadsForm, FolderParts, welcomeHead } from '#lib/setup/index.ts';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Confirm from '#lib/ui/Confirm.svelte';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import { ConnectBridge } from '#lib/sources/index.ts';
	import MoveFlow from '../settings/move/+page.svelte';
	import { refreshParts } from '#lib/mediafiles/index.ts';
	import { applyParts } from '#lib/setup/index.ts';
	import { theme, SigninBackdrop, SigninPortals, SigninTheme } from '#lib/accounts/index.ts';
	import { LanguageWants } from '#lib/subtitles/index.ts';
	import { appName, bestStats, countWords, running as moveRunning, type AppKind, type Move } from '#lib/migrate/index.ts';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount, tick, untrack } from 'svelte';
	import { loaderReady, playLoader, SETUP_HOLD_MS } from '#lib/core/loader.ts';
	import { api, message } from '#lib/core/api.ts';
	import { to } from '#lib/core/paths.ts';
	import { session } from '#lib/core/session.svelte.ts';
	const mark = $derived(theme.value === 'light' ? '/brand/mark-color.svg' : '/brand/mark-color-on-dark.svg');

	// First run, right after the owner is made (agreed order): Choose (import or pick parts), Media server (Plex, Jellyfin or Emby owner), Folders, Sources, Downloads, Subtitles (when on, not in Hands-off), Access, Done. Media server and Folders (or Hands-off) must be finished; a reload never lands past the first one that is not. A later visit to /welcome (no first=1) changes only the parts and returns to Home.
	let sheetEl = $state<HTMLElement>();
	const firstRun = $derived(page.url.searchParams.get('first') === '1');
	// Every owner gets Link your media servers (each server optional, so it never blocks a reload).
	const withServer = true;
	// Hands-off mode: HolyArrs never touches files; no folders needed and no Subtitles step. Off when this build has no such setting yet.
	let handsOff = $state(false);
	async function loadHandsOff(): Promise<boolean> {
		return api.get<{ on: boolean }>('/settings/hands-off').then((r) => !!r.on, () => false);
	}
	// The step stays in the address so a reload keeps it; the real one is settled once the server says what is done.
	let step = $state<Step>(resumeStep(stepFromName(page.url.searchParams.get('step')), { withServer, serverDone: true, foldersDone: true }));
	let ready = $state(false);
	async function progress() {
		const [, lf, off] = await Promise.all([
			api.get<{ media_servers: { kind: string }[] }>('/media-servers').then((r) => r.media_servers ?? [], () => []),
			api.get<{ library_folders: unknown[] }>('/library-folders').then((r) => r.library_folders ?? [], () => []),
			loadHandsOff(),
			partsStep.loadParts()
		]);
		handsOff = off;
		return { ...plan, serverDone: true, foldersDone: lf.length > 0 || mediaStep.foldersSkipped() };
	}
	let busy = $state(false);
	let error = $state('');

	function go(s: Step) {
		error = '';
		// Done says what is left, so it waits for that list.
		if (s === 'done' && step !== 'done') {
			busy = true;
			void sourcesStep.loadTodo().finally(() => {
				busy = false;
				show(s);
			});
		} else show(s);
	}
	// How many steps this visit pushed: the in-app Back walks history while it can.
	let depth = 0;
	function show(s: Step) {
		// Apps already read: Parts opens on them at once, never a flash of the Import or Start fresh choice.
		if (s === 'parts' && !chooseSub && hasApps()) chooseSub = 'apps-a';
		step = s;
		noteReach(s);
		const q = `first=1&step=${s}`;
		// One history entry per step, so Back and Forward walk the steps.
		depth += 1;
		void goto(to(`/welcome?${q}`), { shallow: true, state: { depth } });
		void enter(s);
		// A new step: focus its heading, so keyboard and screen reader start there.
		void tick().then(focusStep);
	}
	// Read out after the step has changed, so it names the new step, not the old one.
	let announce = $state('');
	function focusStep() {
		document.getElementById('welcome-title')?.focus();
		// Title only: the step bar above already gives the number, counted over the whole setup.
		announce = head.h;
	}
	// Back or Forward in the browser: show the step the address names. Set once the wizard is finished in this tab: a later Back, Forward or reload onto a step goes to Home instead (the step history may predate a reload).
	const DONE_KEY = 'holyarrs.welcome.done';
	function wizardDone(): boolean {
		try {
			return !session.s.setupOpen && sessionStorage.getItem(DONE_KEY) === '1';
		} catch {
			return false;
		}
	}
	onMount(() => {
		if (firstRun && wizardDone()) {
			leaving = true;
			void goto(to('/'), { replace: true });
			return;
		}
		const onPop = () => {
			if (!firstRun) return;
			if (!leaving && wizardDone()) {
				leaving = true;
				void goto(to('/'), { replace: true });
				return;
			}
			const q = new URLSearchParams(location.search);
			const s = stepFromName(q.get('step')) ?? step;
			if (!leaving && s === 'server' && step === 'server') linkSub = subOf(q.get('sub'));
			if (!leaving && s === 'parts' && step === 'parts' && chooseSub !== (chooseSub = chooseOf(q.get('sub')) ?? (hasApps() ? 'apps-a' : null))) (bring.importing = false), (bring.managing = undefined), void tick().then(focusStep);
			if (leaving || s === step) return;
			error = '';
			step = s;
			if (s === 'server') linkSub = subOf(q.get('sub'));
			if (s === 'parts') chooseSub = chooseOf(q.get('sub')) ?? (hasApps() ? 'apps-a' : null);
			void enter(s);
			void tick().then(focusStep);
		};
		window.addEventListener('popstate', onPop);
		return () => window.removeEventListener('popstate', onPop);
	});
	// Each step entry's record; SvelteKit restores page.state on Back and Forward.
	$effect(() => {
		const st = page.state as { depth?: number };
		untrack(() => {
			depth = st.depth ?? 0;
		});
	});

	// Setup is done: a short Orbit while Home (or Users) opens. It also ends the setup sign-in's no-password-check window (server side). The step entries fold back into the first one, which Home then replaces, so the browser's Back after setup leaves the wizard.
	let leaving = false;
	// Skip setup for now (after the account step): setup ends like Done, the app opens, and a Finish setup card brings the wizard back.
	async function skipRest() {
		if (busy) return;
		busy = true;
		try {
			await api.post('/auth/setup/done?skipped=1');
			session.s.setupOpen = false;
			session.s.setupSkipped = true;
			await goto(to('/'), { replace: true });
		} catch {
			error = t('welcome.couldnTFinishSetup');
		} finally {
			busy = false;
		}
	}
	let homing = false; // One finish at a time (a double click sends one save); a failed save frees the buttons.
	async function home(path = '/') {
		if (homing) return;
		homing = busy = true;
		playLoader('', SETUP_HOLD_MS);
		// Finished: the rest of the app opens. A failed save keeps the owner on Done, before the step entries fold back.
		error = '';
		try {
			await api.post('/auth/setup/done');
			session.s.setupOpen = false;
		} catch {
			leaving = false;
			homing = busy = false;
			error = t('welcome.couldnTFinishSetup');
			loaderReady();
			return;
		}
		try {
			sessionStorage.setItem(DONE_KEY, '1');
		} catch {
			// No storage: the depth fold below still covers this visit.
		}
		if (depth > 0) {
			leaving = true;
			const n = depth;
			await new Promise<void>((done) => {
				const t = setTimeout(done, 500);
				window.addEventListener('popstate', () => (clearTimeout(t), done()), { once: true });
				history.go(-n);
			});
		}
		await goto(to(path), { replace: true }).finally(loaderReady);
	}

	// Media server is done: on to Folders, with the library paths kept to fill them.
	function connected(libs: Library[]) {
		mediaStep.serverLibs = libs;
		go(nextStep('server', plan));
	}
	// The parts as the server has them now.
	const partsStepCtx = {
		get error() { return error; },
		set error(v) { error = v; },
		get busy() { return busy; },
		set busy(v) { busy = v; },
		home,
		// First run: the choice screen moves on to the next step instead of Home.
		get after() { return firstRun ? () => go(nextStep('parts', plan)) : undefined; },
	};
	const partsStep = createWelcomeParts(partsStepCtx);
	// The steps this person gets: the Subtitles tile as picked, Hands-off as set.
	const plan = $derived<Plan>({ withServer, subtitles: partsStep.want.subtitles, handsOff });
	const prev = $derived(step === 'done' ? undefined : prevStep(step, plan));
	const mediaStepCtx = {
		partsOn: partsStep.partsOn,
		get busy() { return busy; },
		set busy(v) { busy = v; },
		get error() { return error; },
		set error(v) { error = v; },
		go,
		next: () => go(nextStep('folders', plan)),
		withServer,
	};
	const mediaStep = createWelcomeMedia(mediaStepCtx);
	const downloaderStepCtx = {
		get error() { return error; },
		set error(v) { error = v; },
		get busy() { return busy; },
		set busy(v) { busy = v; },
		get sourcesStep() { return sourcesStep; },
		go,
	};
	const downloaderStep = createWelcomeDownloader(downloaderStepCtx);
	const sourcesStepCtx = {
		get busy() { return busy; },
		set busy(v) { busy = v; },
		listDownloaders: downloaderStep.listDownloaders,
		reload: async () => {
			await Promise.all([downloaderStep.loadDownloader(), sourcesStep.loadSources()]);
		},
	};
	const sourcesStep = createWelcomeSources(sourcesStepCtx);
	const bring = createWelcomeBring({
		get error() { return error; },
		set error(v) { error = v; },
		get step() { return step; },
		get chooseSub() { return chooseSub; },
		set chooseSub(v) { chooseSub = v; },
		get plan() { return plan; },
		countLine: (k: string, c: Record<string, number>): string => bestStats(k, c).split(' · ')[0] || countWords(k, c, 1),
		sourcesStep,
		partsStep,
		downloaderStep,
		mediaStep,
		go,
		next: () => go(nextStep('folders', plan)),
		appName,
		moveRunning: (m: Move) => moveRunning(m),
	});
	const finish = createWelcomeFinish({
		get error() { return error; },
		set error(v) { error = v; },
		get busy() { return busy; },
		set busy(v) { busy = v; },
		get handsOff() { return handsOff; },
		set handsOff(v) { handsOff = v; },
		get plan() { return plan; },
		sourcesStep,
		downloaderStep,
		bring,
		go,
	});
	const tools = createWelcomeTools({ finish, bring, downloaderStep, sourcesStep, appName, get error() { return error; }, set error(v) { error = v; } });
	async function invite() {
		if (busy) return;
		error = '';
		busy = true;
		try {
			await refreshParts();
			const now = partsStep.partsOn();
			if (!now.wishlist) await applyParts({ ...now, wishlist: true });
			await home('/settings/users');
		} catch (err) {
			error = message(err);
			busy = false;
		}
	}

	// Choose: two big cards, Bring my apps and Start fresh, each its own sub-screen (in the address, so a reload keeps it and Back returns to the cards).
	type ChooseSub = 'apps-a' | 'apps-b' | 'fresh';
	const chooseOf = (v: string | null): ChooseSub | null => (v === 'apps-a' || v === 'apps-b' ? 'apps-a' : v === 'fresh' ? v : null);
	// The furthest setup step reached (this tab): the step bar lets the owner click back up to it.
	let reached = $state<Step>(untrack(() => { try { return (sessionStorage.getItem('holyarrs.welcome.reached') as Step) || 'server'; } catch { return 'server'; } }));
	function noteReach(s: Step) {
		const order = stepsFor(plan);
		if (order.indexOf(s) <= order.indexOf(reached)) return;
		reached = s;
		try { sessionStorage.setItem('holyarrs.welcome.reached', s); } catch { /* this visit only */ }
	}
	const hasApps = () => bring.doneApps.length > 0 || Object.keys(bring.reads).length > 0;
	let chooseSub = $state<ChooseSub | null>(untrack(() => (page.url.searchParams.get('step') === 'parts' ? chooseOf(page.url.searchParams.get('sub')) : null)));
	$effect(() => {
		const k = chooseSub;
		untrack(() => {
			if (!firstRun || step !== 'parts') return;
			const cur = chooseOf(new URLSearchParams(location.search).get('sub'));
			if (cur === k) return;
			if (k) depth += 1;
			void goto(to(`/welcome?first=1&step=parts${k ? `&sub=${k}` : ''}`), { shallow: true, replace: !k, state: { depth } }).then(() => tick()).then(focusStep);
		});
	});
	async function back() {
		// Inside Import: back closes it, on the same card.
		if (step === 'parts' && bring.importing) return void (bring.importing = false);
		if (step === 'parts' && bring.managing) return void (bring.managing = undefined);
		// A sub-screen (Bring my apps, Start fresh, a server) pushed in this visit: back walks history, so the browser's Back stays in order. Else back to the cards.
		// Apps already read: the Import or Start fresh choice is gone, so Back leaves Parts for the step before.
		if (((step === 'parts' && chooseSub && !hasApps()) || (step === 'server' && linkSub)) && depth > 0) return void history.back();
		if (step === 'sources' && tools.srcOpen) return void (tools.srcOpen = null);
		if (step === 'downloads' && tools.dlOpen) return void (tools.dlOpen = false);
		if (step === 'parts' && chooseSub && !hasApps()) return void (chooseSub = null);
		if (step === 'server' && linkSub) return void (linkSub = null);
		if (step === 'server' && linkOpen) return void (linkOpen = null);
		// The first step: back to the account step, still signed in, filled with the saved account to change it (no sign-out).
		if (!prev && step !== 'done') {
			await goto(to('/login?setup=account&edit=1'));
			return;
		}
		if (!prev) return;
		// Step back through history, so the browser's Back and Forward stay in order.
		if (depth > 0) history.back();
		else go(prev);
	}
	async function enter(s: Step) {
		if (bring.importBusy) void bring.checkImport();
		if (s === 'parts') await Promise.all([partsStep.loadParts(), sourcesStep.loadHidden(), bring.syncReads().then(bring.restoreFromMoves)]);
		// Apps already read or imported: Parts opens on them, not on the Import or Start fresh choice (shown again once every app is removed).
		if (s === 'parts' && !chooseSub && hasApps()) chooseSub = 'apps-a';
		if (s === 'folders') {
			await mediaStep.loadMedia();
			await mediaStep.prefillFolders();
		}
		if (s === 'sources') await finish.loadBridges();
		if (s === 'downloads') await downloaderStep.loadDownloader();
		if (s === 'subtitles') await finish.loadSubs();
		if (s === 'done') await Promise.all([sourcesStep.loadTodo(), finish.loadQuality()]);
	}
	// Opening or reloading: never past the first required step that is not finished.
	onMount(async () => {
		if (!firstRun || wizardDone()) return void (ready = true);
		const prog = await progress();
		const want = resumeStep(stepFromName(page.url.searchParams.get('step')), prog);
		// Every step up to the first unfinished required one can be opened, so the step bar lets the owner click to it.
		noteReach(firstOpen(prog));
		if (want !== step || page.url.searchParams.get('step') !== want) {
			step = want;
			void goto(to(`/welcome?first=1&step=${want}`), { shallow: true, replace: true, state: { depth } });
		}
		ready = true;
		void bring.checkImport();
		void enter(step);
	});
	// An import still running: check every few seconds; Sources and Downloads show what it brought as it lands (Sources come early) and once it ends.
	$effect(() => {
		if (!bring.importBusy) return;
		const id = setInterval(async () => {
			await bring.checkImport();
			if (step === 'sources' || !bring.importBusy) await finish.loadBridges();
			if (step === 'downloads' || !bring.importBusy) await downloaderStep.loadDownloader();
		}, 3000);
		return () => clearInterval(id);
	});
	// Link your media servers: a linked server being connected is its own sub-screen.
	const subOf = (v: string | null): ServerKind | null => (v === 'plex' || v === 'jellyfin' || v === 'emby' ? v : null);
	// The server sub-screen is in the address too, so a reload stays on it and the browser's Back returns to the cards.
	let linkSub = $state<ServerKind | null>(untrack(() => subOf(page.url.searchParams.get('sub'))));
	$effect(() => {
		const k = linkSub;
		untrack(() => {
			if (!firstRun || step !== 'server') return;
			const cur = subOf(new URLSearchParams(location.search).get('sub'));
			if (cur === k) return;
			const q = `first=1&step=server${k ? `&sub=${k}` : ''}`;
			if (k) depth += 1;
			void goto(to(`/welcome?${q}`), { shallow: true, replace: !k, state: { depth } }).then(() => tick()).then(focusStep);
		});
	});
	let linkOpen = $state<ServerKind | null>(null); // Hands-off needs no folders, so that to-do is neither counted nor shown.
	const todoLeft = $derived(sourcesStep.todo.filter((i) => !(handsOff && i.path === '/settings/library-folders')));
	const head = $derived(welcomeHead({ firstRun, step, linkOpen, linkSub, chooseSub, importing: bring.importing, importName: bring.headKind ? appName(bring.headKind as AppKind) : '', readOpen: sourcesStep.moveStep === 'read', todo: todoLeft.length, bringFolders: !!bring.pending }));
</script>
<svelte:head><title>{head.h} · HolyArrs</title></svelte:head>

{#if firstRun}<SigninBackdrop />{/if}
<!-- First run sits on a dark picture with white text, so its cards and sheets use the dark colours in any theme. -->
<div class="welcome" class:over-art={firstRun} data-theme={firstRun ? theme.value : undefined}>
	<!-- First run keeps the sign-in look: the same moving backdrop behind one glass panel. -->
	<div class="sheet" class:glass={firstRun} class:light={theme.value === 'light'} data-signin-panel={firstRun ? '' : undefined} bind:this={sheetEl} use:trailButtons>
	<div class="brand"><img src={to(mark)} alt={firstRun ? 'HolyArrs' : ''} width={firstRun ? 28 : 32} height={firstRun ? 28 : 32} />{#if !firstRun}<span>HolyArrs</span>{/if}</div>
	{#if firstRun && step !== 'done' && session.can('users') && (step !== 'parts' || !bring.importing || sourcesStep.moveStep === 'read' || sourcesStep.moveStep === 'choose')}
		<span class="corner"><IconButton icon="chevronLeft" variant="glass" label={t('welcome.back')} onclick={back} disabled={busy} /></span>
	{/if}
	{#if firstRun}
		<SetupBar current={phaseOf(step)} phases={phasesFor(plan)} reach={phaseOf(reached)} onpick={(p) => !busy && go(p as Step)} />
	{/if}
	<header class="head">
		<h1 id="welcome-title" tabindex="-1">{head.h}</h1>
		<p>{head.p}</p>
	</header>
	{#if firstRun}<p class="sr-only" aria-live="polite">{announce}</p>{/if}
	{#if firstRun && session.s.setupOpen && step !== 'done'}<button class="skip-setup" type="button" disabled={busy} onclick={skipRest} data-testid="skip-setup">{t('welcome.skipSetup')}</button>{/if}
	{#if firstRun && bring.importBusy && !(step === 'parts' && bring.importing)}<p class="import-note" role="status" data-testid="import-running">{t('welcome.importRunning')}</p>{/if}

	{#if !session.can('users') || (firstRun && !session.can('settings'))}
		<p class="notice warn">{t('welcome.onlyTheOwnerOr')}</p>
	{:else if !firstRun}
		<form class="card" onsubmit={partsStep.submitParts} data-testid="welcome">
			<PartTiles {partsStep} />
			{#if error || partsStep.problem}<p class="form-error" role="alert">{error || partsStep.problem}</p>{/if}
			<Button variant="primary" size="lg" full type="submit" disabled={busy || !!partsStep.problem} {busy}>{t('welcome.continue')}</Button>
		</form>
	{:else if !ready}
		<PageLoader loading label={t('welcome.loadingFolders')}>{#snippet children()}{/snippet}</PageLoader>
	{:else if step === 'parts'}
		{#if bring.importing}
			<div class="card" data-testid="welcome-parts">
				<section class="sect move" aria-label={t('welcome.parts.import')} data-testid="welcome-move">
					{#if bring.importCheck && session.s.hasPassword}
						<Confirm action="password">{#snippet children()}{/snippet}</Confirm>
					{/if}
					<MoveFlow embedded={{ query: sourcesStep.IMPORT_QUERY, ondone: bring.imported, onstep: (s) => (sourcesStep.moveStep = s), onbrought: downloaderStep.addBrought, onaway: bring.importAway, onback: () => (bring.importing = false), start: bring.importFrom as AppKind | undefined, again: bring.againId, onread: bring.onread, onunread: bring.onunread, via: bring.via, go: bring.moving }} />
				</section>
			</div>
		{:else if chooseSub === 'apps-a'}
			<!-- Bring my apps, step A: the five main apps (Seerr and Overseerr share one card, Seerr first); each opens that app's read in this card. -->
			<div class="card bring" data-testid="welcome-parts">
				<BringApps {appName} view="apps" {bring} {busy} />
				<div class="acts">
					<!-- One button: Continue once an app is read, else Skip for now (both move on). -->
					<Button variant="primary" size="lg" full type="button" disabled={busy} onclick={() => bring.continueApps()} data-testid="apps-continue">{hasApps() ? t('welcome.continue') : t('welcome.skipForNow')}</Button>
				</div>
			</div>
		{:else if chooseSub === 'fresh'}
			<form class="card" onsubmit={partsStep.submitParts} data-testid="welcome-parts">
				<PartTiles {partsStep} />
				{#if error || partsStep.problem}<p class="form-error" role="alert">{error || partsStep.problem}</p>{/if}
				<Button variant="primary" size="lg" full type="submit" disabled={busy || !!partsStep.problem} {busy} data-testid="parts-continue">{t('welcome.continue')}</Button>
			</form>
		{:else}
			<!-- The two ways in: each whole card is the button. -->
			<div class="card choose" data-testid="welcome-choose">
				<BringApps {appName} view="choose" {bring} {busy} onpick={(k) => { error = ''; chooseSub = k; }} />
			</div>
		{/if}
	{:else if step === 'server'}
		<LinkServers bind:busy bind:connecting={linkSub} bind:open={linkOpen} onnext={connected} />
	{:else if step === 'folders' && bring.pending}
		<!-- Apps were read: Folders asks where their old folders are here (found ones ticked), and Continue runs the import. -->
		<div class="card" data-testid="welcome-folders-import"><section class="sect move" aria-label={t('welcome.parts.import')} data-testid="welcome-move">
			{#if bring.importCheck && session.s.hasPassword}<Confirm action="password">{#snippet children()}{/snippet}</Confirm>{/if}
			<MoveFlow embedded={{ query: 'first=1&step=folders', ondone: bring.imported, onaway: bring.folderAway, onstep: (s) => (sourcesStep.moveStep = s), onbrought: downloaderStep.addBrought, go: true, review: true, reads: bring.pendingIds }} />
		</section></div>
	{:else if step === 'folders'}
		<form class="card" onsubmit={(e) => { e.preventDefault(); void mediaStep.saveMedia(false); }} data-testid="welcome-media">
			<PageLoader loading={!mediaStep.rows.length} label={t('welcome.loadingFolders')}>
			<FolderParts parts={mediaStep.rows.filter((r) => mediaStep.media[r.part])} off={mediaStep.rows.filter((r) => !mediaStep.media[r.part])} {busy} onchange={() => void mediaStep.loadMedia()} onturnon={(p) => (mediaStep.media = { ...mediaStep.media, [p]: true })} />
			</PageLoader>
			<p class="hint">{t('welcome.onDockerUseThe')}</p>
			{#if error || mediaStep.mediaOff}<p class="form-error" role="alert">{error || t('welcome.turnOnAtLeast')}</p>{/if}
			<div class="acts">
				<!-- One button: Skip for now until a part has a checked folder, then Continue. -->
				{#if mediaStep.rows.some((r) => r.saved)}<Button variant="primary" size="lg" full type="submit" disabled={busy || mediaStep.mediaOff} {busy} data-testid="folders-continue">{t('welcome.continue')}</Button>
				{:else}<Button variant="primary" size="lg" full type="button" disabled={busy || mediaStep.mediaOff} {busy} onclick={mediaStep.skipFolders} data-testid="folders-skip">{t('welcome.skipForNow')}</Button>{/if}
				<button class="alt" type="button" disabled={busy} onclick={finish.handsOffOn} data-testid="folders-hands-off">{t('welcome.folders.handsOff')}</button>
			</div>
		</form>
	{:else if step === 'sources'}
		<!-- Sources: Prowlarr and Jackett as app rows (each opens its connect), and the indexers an import brought, ticked. -->
		<div class="card" class:bring={!tools.srcOpen} data-testid="welcome-source">
			{#if tools.srcOpen}
				<div class="source"><ConnectBridge onchange={finish.loadBridges} compact kind={tools.srcOpen} imported={tools.srcOpen === 'prowlarr' && sourcesStep.viaProwlarr ? { address: sourcesStep.prowlarrAddr, n: sourcesStep.sources, names: sourcesStep.prowlarrNames, from: (['radarr', 'sonarr', 'lidarr'] as const).filter((k) => (bring.reads[k] ?? []).length).map((k) => appName(k)) } : undefined} /></div>
			{:else}
				<BringApps {appName} view="rows" rows={tools.sourceRows} {bring} {busy} />
			{/if}
			{#if error}<p class="form-error" role="alert">{error}</p>{/if}
			<div class="acts">
				<!-- One action: Continue once a source is in, else Skip for now (both move on). -->
				<Button variant="primary" size="lg" full type="button" disabled={busy} {busy} onclick={() => go(nextStep('sources', plan))} data-testid="sources-continue">{finish.foundSrc.length || finish.foundCount || sourcesStep.sources ? t('welcome.continue') : t('welcome.skipForNow')}</Button>
			</div>
		</div>
	{:else if step === 'downloads'}
		<!-- Downloads: qBittorrent and SABnzbd as app rows; an imported one is ticked with where it came from, Manage and Add another open the form. -->
		<div class="card" class:bring={!tools.dlOpen} data-testid="welcome-downloads">
			{#if tools.dlOpen}
				<DownloadsForm {downloaderStep} {busy} {appName} bind:error />
			{:else}
				<BringApps {appName} view="rows" rows={tools.dlRows} {bring} {busy} />
			{/if}
			{#if error}<p class="form-error" role="alert">{error}</p>{/if}
			<div class="acts">
				<Button variant="primary" size="lg" full type="button" disabled={busy} {busy} onclick={async () => { if (!tools.dlOpen || (await downloaderStep.connectTyped())) go(nextStep('downloads', plan)); }} data-testid="downloads-continue">{downloaderStep.dlList.length || downloaderStep.typed ? t('welcome.continue') : t('welcome.skipForNow')}</Button>
			</div>
		</div>
	{:else if step === 'subtitles'}
		<form class="card" onsubmit={finish.saveSubs} data-testid="welcome-subtitles">
			{#if finish.subs}
				<div class="subs-box"><LanguageWants bind:wants={finish.subs.languages} langs={finish.subLangs} id="welcome-subs" /></div>
			{:else}
				<PageLoader loading={!finish.subsFailed} label={t('welcome.subs.title')}>{#snippet children()}{/snippet}</PageLoader>
				{#if finish.subsFailed}<button class="alt" type="button" disabled={busy} onclick={() => { error = ''; void finish.loadSubs(); }}>{t('welcome.tryAgain')}</button>{/if}
			{/if}
			{#if error}<p class="form-error" role="alert">{error}</p>{/if}
			<div class="acts">
				<!-- One button: Continue once a language is picked, else Skip for now. -->
				{#if finish.subs?.languages.length}<Button variant="primary" size="lg" full type="submit" disabled={busy} {busy} data-testid="subtitles-continue">{t('welcome.continue')}</Button>
				{:else}<Button variant="primary" size="lg" full type="button" disabled={busy} {busy} onclick={() => go(nextStep('subtitles', plan))} data-testid="subtitles-skip">{t('welcome.skipForNow')}</Button>{/if}
			</div>
		</form>
	{:else if step === 'access'}
		<div class="card access" data-testid="welcome-access">
			<div class="field">
				<label for="app-address">{t('welcome.address')}</label>
				<input id="app-address" class="input" dir="ltr" readonly value={finish.appAddress} aria-describedby="app-address-hint" />
				<span class="hint" id="app-address-hint">{t('welcome.access.hint')}</span>
			</div>
			<Button variant="primary" size="lg" full type="button" disabled={busy} {busy} onclick={() => go(nextStep('access', plan))} data-testid="access-continue">{t('welcome.continue')}</Button>
		</div>
	{:else}
		<div class="card" data-testid="welcome-done">
			{#if handsOff}<p class="done-note" data-testid="done-hands-off">{t('welcome.done.handsOff')}</p>{/if}
			<!-- One way in during setup; more ways (Plex, Jellyfin, Emby, a password) are added on the Account page. -->
			<p class="done-note"><a class="done-link" href={to('/account')} onclick={(e) => { e.preventDefault(); void home('/account'); }} data-testid="done-more-ways">{t('welcome.done.moreWays')}</a></p>
			{#if sourcesStep.haveSet.length}
				<ul class="todo" aria-label={t('welcome.setUp')}>
					{#each sourcesStep.haveSet as h (h)}
						<li><span class="todo-row"><span class="set-name"><span class="set-tick" aria-hidden="true"><Icon name="check" size={14} stroke={2.5} /></span><span>{h}</span></span></span></li>
					{/each}
				</ul>
			{/if}
			{#if todoLeft.length}
				<ul class="todo" aria-label={t('welcome.stillToDo')}>
					{#each todoLeft as item (item.label)}
						<li><a href={to(item.path)} onclick={(e) => { e.preventDefault(); void home(item.path); }}><span>{item.label}</span><span class="todo-end">{item.end ?? t('welcome.notSet')}<Icon name="chevronRight" size={16} /></span></a></li>
					{/each}
				</ul>
			{/if}
			{#if finish.quality}
				<ul class="todo" aria-label={t('welcome.done.qualityLabel')}>
					<li><a href={to('/settings/title-defaults')} onclick={(e) => { e.preventDefault(); void home('/settings/title-defaults'); }} data-testid="done-quality"><span>{t('welcome.done.quality', { name: finish.quality })}</span><span class="todo-end">{t('welcome.change')}<Icon name="chevronRight" size={16} /></span></a></li>
				</ul>
			{/if}
			{#if error}<p class="form-error" role="alert">{error}</p>{/if}
			<div class="acts">
				<Button variant="primary" size="lg" full type="button" disabled={busy} {busy} onclick={() => home()}>{t('welcome.openHome')}</Button>
				<button class="alt" type="button" disabled={busy} onclick={invite}>{t('welcome.inviteFamily')}</button>
			</div>
		</div>
	{/if}
	</div>
</div>
{#if firstRun}<SigninPortals symbols={false} /><SigninTheme panelEl={sheetEl} />{/if}

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
	.skip-setup { align-self: center; margin: -6px 0 8px; padding: 6px 10px; border: 0; background: none; color: var(--text-muted); font: var(--t-footnote); text-decoration: underline; text-underline-offset: 3px; cursor: pointer; }
	.skip-setup:hover:not(:disabled) { color: var(--text); }
	.corner { position: absolute; top: -2px; inset-inline-start: 0; } :global([dir='rtl']) .corner :global(svg) { transform: scaleX(-1); }
	.sheet { position: relative; }
	/* First run: the sign-in page's glass over its backdrop (same tint and blur). */
	.over-art { z-index: 1; }
	/* Full-bleed like sign-in: no light scrollbar gutter down the right edge of the backdrop. */
	:global(html:has(.welcome.over-art)) { scrollbar-gutter: auto; background: #050507; color-scheme: dark; }
	/* The body's light-theme white would show below the art on a long step (and fail contrast there); let the dark page colour through. */
	:global(html:has(.welcome.over-art) body) { background: transparent; }
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
	.glass .corner { top: calc(var(--pad) - 2px); inset-inline-start: var(--pad); }
	/* Light glass: the round Back button in white glass with a dark arrow (as on the sign-in page). */
	.glass.light .corner :global(.ui-ibtn.glass) { color: #1d1d1f; background: rgb(255 255 255 / 0.55); border-color: rgb(0 0 0 / 0.12); box-shadow: 0 2px 10px rgb(0 0 0 / 0.12); }
	@media (hover: hover) { .glass.light .corner :global(.ui-ibtn.glass:hover) { background: rgb(255 255 255 / 0.85); } }
	.glass .head { margin-bottom: 22px; }
	/* Inside the glass the step's card is just its content. */
	.sheet.glass > :global(.card) { background: transparent; box-shadow: none; padding: 0; border-radius: 0; }
	/* The Jackett or Prowlarr card on the Source step joins the glass too: no second box, edges lined up. */
	.sheet.glass :global(.ui-settings) { background: transparent; box-shadow: none; border-radius: 0; overflow: visible; } .sheet.glass :global(.ui-settings > header), .sheet.glass :global(.ui-settings > .body) { padding-left: 0; padding-right: 0; }
	.sheet.glass :global(.ui-settings > header) { padding-top: 0; }
	/* Its text follows the card's ink, so light mode reads dark. */
	.sheet.glass :global(.ui-settings) { color: var(--ink); --text: var(--ink); --text-muted: rgb(var(--ink-rgb) / 0.72); --text-faint: rgb(var(--ink-rgb) / 0.55); } .sheet.glass :global(.ui-settings :is(h2, h3, label, legend, .label)) { color: var(--ink); }
	.sect { display: grid; gap: var(--s-3); min-width: 0; }
	/* Desktop: no scrolling inside setup. The long steps widen the card and lay their parts side by side. */
	@media (min-width: 1100px) {
		:global(.card.connect) { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); column-gap: 40px; align-items: start; }
		:global(.card.connect > :not(form):not(.libs)) { grid-column: 1 / -1; }
		:global(.card.connect > .libs) { margin-top: 0; }
	}
	.head { margin: 18px 0 28px; text-align: center; } .head h1 { font: var(--t-large-title); letter-spacing: var(--t-large-title-track); text-wrap: balance; }
	.head p { margin: 8px auto 0; max-width: 44ch; color: var(--text-muted); font: var(--t-body); text-wrap: pretty; }
	.sheet > :global(.card) {
		position: relative;
		border: 0;
		border-radius: 24px;
		background: var(--surface-1);
		box-shadow: var(--shadow-card), 0 0 0 1px var(--line);
		padding: clamp(18px, 4vw, 28px);
	}
	.hint { display: block; margin-top: var(--s-2); color: var(--text-muted); font: var(--t-footnote); }

	/* One spacing scale for every step: --gap before the main button, --s-3 to the quiet link under it. */
	.welcome { --gap: var(--s-6); }
	/* A hint inside a field already gets the field's own gap. */
	.welcome :global(.field > .hint) { margin-top: 0; }
	.welcome :global(.form-error) { margin-top: var(--s-4); } .sheet > :global(.card .pl) { min-height: 0; } /* A step's loader fits its card, so Continue stays in view. */
	.welcome :global(.card) > :global(.ui-btn) { margin-top: var(--gap); }
	/* Done: the one button sits right under the heading. */
	[data-testid='welcome-done'] > :global(.ui-btn) { margin-top: 0; }
	/* A step's main button with its quiet choices under it (one row on desktop). */
	.acts { margin-top: var(--gap); display: flex; flex-direction: column; }
	.alt {
		display: block;
		margin: var(--s-3) auto 0;
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
	.alt:focus-visible { outline: none; box-shadow: var(--focus-ring); } .alt:disabled { opacity: 0.5; cursor: default; }

	/* Media: one row per part, its icon, its name and switch, then its folder. */
	/* Media: one quiet group, rows split by a hairline (like Settings lists). */


	.source :global(section) { box-shadow: none; }
	/* The wizard heading says it all: no second title in the card. */
	.sheet.glass .source :global(.ui-settings > header) { display: none; }
	/* The Connect form's own bottom padding would double the space to Skip. */
	.sheet.glass .source :global(.ui-settings > .body) { padding-bottom: 0; }
	.head h1:focus { outline: none; }
	/* Done with steps left: each one a row that opens its Settings page. */
	.todo { list-style: none; margin: 0; padding: 0; display: grid; gap: 8px; }
	.todo a, .todo-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 48px; padding: 0 16px; border-radius: 14px; background: var(--surface-2); border: 1px solid var(--line); color: var(--text); font: 600 15px/1.2 var(--font-ui); text-decoration: none; }
	@media (hover: hover) { .todo a:hover { border-color: var(--line-strong); } }
	.todo a:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.todo :global(svg) { color: var(--text-muted); }
	.todo-end { flex: none; white-space: nowrap; display: inline-flex; align-items: center; gap: 6px; }
	.todo a > span:first-child { min-width: 0; overflow-wrap: anywhere; }
	.set-name { display: inline-flex; align-items: center; gap: 10px; min-width: 0; overflow-wrap: anywhere; padding: 12px 0; }
	.set-tick { flex: none; width: 20px; height: 20px; border-radius: 50%; display: grid; place-items: center; background: var(--accent); color: #fff; }
	.todo .set-tick :global(svg) { color: #fff; }
	/* Found: what an import brought, one line with Change. */
	.import-note { margin: -12px auto 12px; text-align: center; font: 500 13px/1.3 var(--font-ui); color: rgb(var(--ink-rgb, 255 255 255) / 0.75); }
	.done-link { color: inherit; text-decoration: underline; text-underline-offset: 3px; }
	.done-note { margin: 0 0 var(--s-4); color: rgb(var(--ink-rgb, 0 0 0) / 0.72); font: 400 14px/1.4 var(--font-ui); text-align: center; }
	.access :global(.input[readonly]) { cursor: text; }
	/* Import: the move flow's cards stack inside the glass with plain gaps. */
	.move { display: flex; flex-direction: column; gap: var(--s-4); }
	/* Skip sits as close under the last card as under a main button on the other steps. */
	/* No tab bar here, so the phone footer sits on the bottom edge. */
	.move :global(.step-foot) { --tabbar: 0px; }

	@media (max-width: 520px) {
		/* A phone fits the short steps without scrolling. */
		.welcome { padding: 12px 0 0; }
		.welcome { --gap: var(--s-4); }
		.head { margin: 12px 0 16px; }
		.glass .head { margin-bottom: 16px; }
		.welcome :global(.field) { gap: 4px; }
		/* No app frame under setup on a phone: the glass sheet ends the page. */
		:global(main:has(> .welcome.over-art)) { padding: 0; min-height: 0; }
		.welcome.over-art { padding: 12px 0 12px; }
		.sheet.glass { padding-bottom: 10px; }
	}
	@media (pointer: coarse), (max-width: 520px) {
		.alt, .source :global(:is(.quiet, .link)) { min-height: 44px; }
		.source :global(.acts .ui-btn) { min-height: 44px; }
		.done-link { display: inline-block; padding: 12px 4px; margin: -12px -4px; }
	}

	/* One layer inside the glass card: lists are flat rows split by hairlines, choices are outlined, and only text fields keep a filled box. */
	.todo { gap: 0; border-top: 1px solid var(--line); }
	.todo > li { border-bottom: 1px solid var(--line); }
	.todo a, .todo-row { background: none; border: 0; border-radius: 0; padding: 0 2px; min-height: 52px; }
	/* Done and still-to-do read as one list. */
	.todo:has(+ .todo) { margin-bottom: 0; }
	.todo + .todo { border-top: 0; }
	@media (hover: hover) { .todo a:hover { color: var(--text); background: color-mix(in srgb, var(--text) 4%, transparent); } }
	.welcome.over-art { width: min(672px, calc(100% - 32px)); color: var(--ink); --focus-ring: 0 0 0 2px rgb(10 10 12 / 0.9), 0 0 0 4px var(--ink); }
	.glass .brand img { width: 28px; height: 28px; }
	.glass .brand { min-height: 28px; }
	.glass .head { margin: 22px 0 28px; }
	.glass .head h1 { font: 700 30px/1.15 var(--font-ui); letter-spacing: -0.02em; color: var(--ink); }
	.glass .head p { color: rgb(var(--ink-rgb) / 0.72); font: 400 15px/1.45 var(--font-ui); }
	.glass .alt { color: rgb(var(--ink-rgb) / 0.7); font-size: 15px; min-height: 44px; }
	@media (hover: hover) { .glass .alt:hover:not(:disabled) { color: var(--ink); background: rgb(var(--ink-rgb) / 0.08); } }
	.glass :global(.field label), .glass .field > label { font: 600 13px/1.3 var(--font-ui); color: rgb(var(--ink-rgb) / 0.85); }
	/* Labels and hints centre over their fields on every step; typing stays left. */
	.sheet.glass :global(.field) { text-align: center; }
	.sheet.glass :global(.field .input) { text-align: start; }
	.glass :global(.hint), .glass :global(.muted), .glass .hint { color: rgb(var(--ink-rgb) / 0.55); }
	/* Rows: flat, white hairlines. */
	.glass .todo { border-color: rgb(var(--ink-rgb) / 0.1); }
	.glass .todo > li { border-color: rgb(var(--ink-rgb) / 0.1); }
	.glass .todo a, .glass .todo-row { color: var(--ink); }
	.glass .todo :global(svg) { color: rgb(var(--ink-rgb) / 0.6); }
	.glass .set-tick { background: none; width: 18px; }
	.glass .todo .set-tick :global(svg) { color: var(--ink); }
	.glass :global(.add-row) { border-color: rgb(var(--ink-rgb) / 0.18); color: rgb(var(--ink-rgb) / 0.7); }
	@media (max-width: 520px) {
		.glass .head h1 { font-size: 26px; }
		.glass .head { margin: 18px 0 22px; }
	}
	/* Lists: one glass group, rows split by hairlines. */
	.glass .todo, .glass :global(.liblist) { background: var(--g-bg); border: 1px solid var(--g-line); border-radius: var(--g-r); overflow: hidden; }
	.glass .todo a, .glass .todo-row { padding-left: 14px; padding-right: 14px; }
	.glass .todo > li:last-child { border-bottom: 0; }
	.glass .todo + .todo { border-top: 1px solid var(--g-line); margin-top: 8px; }
	.glass :global(.add-row) { border-radius: var(--g-r); }
	/* Continue is the one primary: a step's inner action (Link Prowlarr) takes the secondary glass look. */
	@media (min-width: 1100px) {
		/* The one setup card (setup-glass.css): the same box as the sign-in page's setup steps, centred inside the frame. The step's own content scrolls inside it if it ever runs long; the page never does. */
		.welcome.over-art { width: 100%; min-height: 100dvh; padding: 0; display: grid; place-items: center; }
		:global(main:has(> .welcome.over-art)) { padding: 0; }
		/* Title at the same spot as the sign-in page's setup steps: 56 px under the step bar's 20 px margin. */
		.glass .head { margin: clamp(24px, 4vh, 56px) 0 18px; }
		.sheet.glass { box-sizing: border-box; width: var(--setup-w); height: var(--setup-h); display: flex; flex-direction: column; overflow: hidden; }
		.sheet.glass > :global(.card) { flex: 1 1 auto; min-height: 0; overflow-y: auto; overscroll-behavior: contain; }
		/* No big gaps: each step's content is one centred group in the card, at a size that fills it (scales with the window height so 900 px still fits). */
		.sheet.glass > :global(.card) { display: flex; flex-direction: column; justify-content: flex-start; }
		/* The content is one group centred in the space above the foot; Continue always sits on the card's foot, so it never moves between steps. */
		.sheet.glass > :global(.card:not(.connect):not([data-testid='welcome-link']):not([data-testid='link-form']):not([data-testid='welcome-folders-import'])) > :global(:first-child) { margin-top: auto; }
		.sheet.glass > :global(.card) > :global(.acts), .sheet.glass > :global(.card) > :global(.ui-btn.primary:last-child) { margin-top: auto; }
		.sheet.glass > :global(.card > *) { flex: none; }
		/* Link your media servers: the cards sit right under the lead (44 px, as on the sign-in steps), not centred in the card. */
		.sheet.glass > :global(.card:is([data-testid='welcome-link'], [data-testid='link-form'])) { justify-content: flex-start; padding-top: 26px; }
		.glass .head h1 { font-size: clamp(32px, 3.6vh, 40px); }
		.glass .head p { font-size: 17px; max-width: 56ch; }
		.glass .todo a, .glass .todo-row { font-size: 17px; }
		.glass .todo a, .glass .todo-row { min-height: clamp(52px, 6.5vh, 96px); }
		.sheet.glass :global(.input) { min-height: 52px; height: 52px; font-size: 17px; }
		.glass .done-note { font-size: 16px; }
		/* The main button and its quiet choices share one row at the foot, held in view while a long step scrolls. */
		/* The quiet choices (Skip for now, ...) sit in one row just above Continue, which keeps the full width on every step. */
		.sheet.glass .acts { flex-direction: row; flex-wrap: wrap; justify-content: center; align-items: center; gap: 0 12px; padding-top: var(--s-3); }
		.acts > :global(.ui-btn) { order: 2; flex: 1 1 100%; margin: 0; }
		.acts > .alt { order: 1; flex: none; margin: 0 0 6px; padding: 0 18px; min-height: 44px; }
		/* Short steps grow their content to fill the card instead of leaving an island. */
		.subs-box { min-height: clamp(120px, 20vh, 240px); max-height: clamp(180px, 42vh, 440px); overflow-y: auto; display: flex; flex-direction: column; justify-content: safe center; font-size: 17px; }
		.access .field { min-height: clamp(160px, 34vh, 400px); display: flex; flex-direction: column; justify-content: center; gap: 12px; }
		/* Folders: the parts side by side, so three folders fit without scrolling. */
		/* Sources: Address and API key side by side, so the step fits without scrolling. */
		.sheet.glass .source :global(.form) { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 16px 24px; align-items: start; }
		.sheet.glass .source :global(.form > *) { grid-column: 1 / -1; }
		.sheet.glass .source :global(.form > .field) { grid-column: auto; }
		/* With the HolyArrs address box too: three across. */
		.sheet.glass .source :global(.form:has(> .field ~ .field ~ .field)) { grid-template-columns: repeat(3, minmax(0, 1fr)); }
	}
	/* Bring my apps, one app's address and key: the form sits under the heading (not pushed to the foot), like Link your media servers. */
	.sect.move:has(:global([data-testid='read-inline'])) { margin-top: 0 !important; flex: 1 1 auto; display: flex; flex-direction: column; min-height: 0; }
	/* Choose: two big cards side by side, words on top, art filling the rest. */
	.sheet > .card.choose { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
	@media (min-width: 1100px) {
		.sheet.glass > .card.choose { display: grid; }
	}
	@media (max-width: 640px) {
		.sheet > .card.choose { grid-template-columns: 1fr; gap: 12px; }
	}
</style>
