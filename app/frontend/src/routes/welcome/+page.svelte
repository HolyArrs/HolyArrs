<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { STEPS, type Step } from '#lib/setup/index.ts';
	import { createWelcomeParts } from '#lib/setup/index.ts';
	import { createWelcomeMedia } from '#lib/setup/index.ts';
	import { createWelcomeDownloader } from '#lib/setup/index.ts';
	import { createWelcomeSources } from '#lib/setup/index.ts';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import TestTag from '#lib/ui/TestTag.svelte';
	import Confirm from '#lib/ui/Confirm.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import { ConnectBridge } from '#lib/sources/index.ts';
	import MoveFlow from '../settings/move/+page.svelte';
	import { refreshParts } from '#lib/mediafiles/index.ts';
	import { applyParts } from '#lib/setup/index.ts';
	import { theme, SigninBackdrop } from '#lib/accounts/index.ts';
	import { emptyForm, kindWords } from '#lib/downloads/index.ts';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount, tick, untrack } from 'svelte';
	import { loaderReady, playLoader, SETUP_HOLD_MS } from '#lib/core/loader.ts';
	import { api, message } from '#lib/core/api.ts';
	import { to } from '#lib/core/paths.ts';
	import { session } from '#lib/core/session.svelte.ts';
	const mark = $derived(theme.value === 'light' ? '/brand/mark-color.svg' : '/brand/mark-color-on-dark.svg');

	// First run, right after the owner is made, forks once: bring the old apps (Import, then a Downloader or Source only if Import brought none), or start fresh (media and folders, a Downloader, a Source). Both end on a short done screen. A later visit to /welcome (no first=1) changes only the parts and returns to Home.
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
			void sourcesStep.loadTodo().finally(() => {
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
		void goto(to(`/welcome?${q}`), { shallow: true, state: { extra: [...extra], depth } });
		void enter(s);
		// A new step: focus its heading, so keyboard and screen reader start there.
		void tick().then(focusStep);
	}
	// Read out after the step has changed, so it names the new step, not the old one.
	let announce = $state('');
	function focusStep() {
		document.getElementById('welcome-title')?.focus();
		announce = t('welcome.stepOf', { n: stepNo, total: order.length, title: head.h });
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
			const s = stepFromUrl(q);
			if (leaving || s === step) return;
			error = '';
			fromApps = q.get('from') === 'apps' || s === 'import';
			step = s;
			void enter(s);
			void tick().then(focusStep);
		};
		window.addEventListener('popstate', onPop);
		// A step address that names no step starts over, in the address too.
		if (firstRun && page.url.searchParams.get('step') && step === 'start') void goto(to('/welcome?first=1'), { shallow: true, replace: true, state: {} });
		return () => window.removeEventListener('popstate', onPop);
	});
	// Each step entry's record; SvelteKit restores page.state on Back and Forward.
	$effect(() => {
		const st = page.state as { extra?: Step[]; depth?: number };
		untrack(() => {
			depth = st.depth ?? 0;
			if (st.extra) extra = st.extra;
		});
	});
	const order = $derived<Step[]>(
		fromApps
			? ['start', 'import', ...(extra.length ? extra : step === 'downloader' || step === 'source' ? [step] : []), 'done']
			: ['start', 'media', 'downloader', 'source', 'done']
	);
	const stepNo = $derived(order.indexOf(step) + 1);
	const prev = $derived(step === 'done' ? undefined : order[order.indexOf(step) - 1]);

	// Setup is done: a short Orbit while Home (or Users) opens. It also ends the setup sign-in's no-password-check window (server side). The step entries fold back into the first one, which Home then replaces, so the browser's Back after setup leaves the wizard.
	let leaving = false;
	async function home(path = '/') {
		playLoader('', SETUP_HOLD_MS);
		// Finished: the rest of the app opens. A failed save keeps the owner on Done, before the step entries fold back.
		error = '';
		try {
			await api.post('/auth/setup/done');
			session.s.setupOpen = false;
		} catch {
			leaving = false;
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

	// The fork.
	function pickPath(apps: boolean) {
		fromApps = apps;
		extra = [];
		go(apps ? 'import' : 'media');
	}

	// The parts as the server has them now.
	const partsStepCtx = {
		get error() { return error; },
		set error(v) { error = v; },
		get busy() { return busy; },
		set busy(v) { busy = v; },
		home,
	};
	const partsStep = createWelcomeParts(partsStepCtx);
	const mediaStepCtx = {
		partsOn: partsStep.partsOn,
		get busy() { return busy; },
		set busy(v) { busy = v; },
		get error() { return error; },
		set error(v) { error = v; },
		go,
	};
	const mediaStep = createWelcomeMedia(mediaStepCtx);
	const downloaderStepCtx = {
		get error() { return error; },
		set error(v) { error = v; },
		get busy() { return busy; },
		set busy(v) { busy = v; },
		get fromApps() { return fromApps; },
		get sourcesStep() { return sourcesStep; },
		go,
	};
	const downloaderStep = createWelcomeDownloader(downloaderStepCtx);
	const sourcesStepCtx = {
		get busy() { return busy; },
		set busy(v) { busy = v; },
		listDownloaders: downloaderStep.listDownloaders,
		get extra() { return extra; },
		set extra(v) { extra = v; },
		go,
	};
	const sourcesStep = createWelcomeSources(sourcesStepCtx);
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

	function back() {
		if (!prev) return;
		// Step back through history, so the browser's Back and Forward stay in order.
		if (depth > 0) history.back();
		else go(prev);
	}
	async function enter(s: Step) {
		if (s === 'media') await mediaStep.loadMedia();
		if (s === 'downloader') await downloaderStep.loadDownloader();
		if (s === 'source') await sourcesStep.loadSources();
		if (s === 'import') await sourcesStep.loadHidden();
		if (s === 'done' && !sourcesStep.todo.length) await sourcesStep.loadTodo();
	}
	onMount(() => {
		if (firstRun && !wizardDone()) void enter(step);
	});
	// Moving asks for a recent password check (past the setup window): the move flow only signals it, so the wizard shows the confirm box itself (an owner with no password gets the move flow's set-a-password card).
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

	const titles = $derived<Record<Step, { h: string; p: string }>>({
		start: { h: t('welcome.comingFromOtherApps'), p: t('welcome.weCanBringOver') },
		media: { h: t('welcome.whereSYourMedia'), p: t('welcome.turnOnWhatYou') },
		downloader: { h: t('welcome.whichAppDownloads'), p: t('welcome.connectTheOneYou') },
		source: { h: t('welcome.whereShouldWeSearch'), p: t('welcome.linkProwlarrOrJackett') },
		import: { h: t('welcome.bringYourApps'), p: t('welcome.weOnlyReadThem') },
		done: { h: t('welcome.youReSet'), p: t('welcome.findSomethingOnHome') }
	});
	const head = $derived(
		!firstRun
			? { h: t('welcome.whatDoYouUse'), p: t('welcome.youCanChangeThis') }
			: step === 'done' && sourcesStep.todo.length
				? { h: t('welcome.holyarrsIsReady'), p: t('welcome.aFewThingsAre') }
				: step === 'done' && fromApps
					? { h: t('welcome.welcomeHome'), p: t('welcome.yourLibraryIsHere') }
					: titles[step]
	);
</script>

<svelte:head><title>{head.h} · HolyArrs</title></svelte:head>

{#if firstRun}<SigninBackdrop /><div class="dim" aria-hidden="true"></div>{/if}
<!-- First run sits on a dark picture with white text, so its cards and sheets use the dark colours in any theme. -->
<div class="welcome" class:over-art={firstRun} data-theme={firstRun ? 'dark' : undefined}>
	<!-- First run keeps the sign-in look: the same moving backdrop behind one glass panel. -->
	<div class="sheet" class:glass={firstRun} class:light={theme.value === 'light'} data-signin-panel={firstRun ? '' : undefined}>
	<div class="brand"><img src={to(mark)} alt={firstRun ? 'HolyArrs' : ''} width={firstRun ? 28 : 32} height={firstRun ? 28 : 32} />{#if !firstRun}<span>HolyArrs</span>{/if}</div>
	{#if firstRun && prev && session.can('users') && (step !== 'import' || sourcesStep.moveStep === 'read')}
		<span class="corner"><IconButton icon="chevronLeft" variant="tinted" label={t('welcome.back')} onclick={back} disabled={busy} /></span>
	{/if}
	{#if firstRun}
		<ol class="dots" aria-label={t('welcome.stepOf2', { n: stepNo, total: order.length })}>
			{#each order as s, i (s)}<li class:done={i + 1 < stepNo} class:now={i + 1 === stepNo}></li>{/each}
		</ol>
	{/if}
	<header class="head">
		<h1 id="welcome-title" tabindex="-1">{head.h}</h1>
		<p>{head.p}</p>
	</header>
	{#if firstRun}<p class="sr-only" aria-live="polite">{announce}</p>{/if}

	{#if !session.can('users') || (firstRun && step !== 'start' && !session.can('settings'))}
		<p class="notice warn">{t('welcome.onlyTheOwnerOr')}</p>
	{:else if !firstRun}
		<form class="card" onsubmit={partsStep.submitParts} data-testid="welcome">
			<!-- Big tiles, each a real checkbox (it covers the tile, so a tap anywhere toggles it). -->
			<div class="tiles" role="group" aria-label={t('welcome.parts')}>
				{#each partsStep.choices as c (c.part)}
					<label class="tile" class:on={partsStep.want[c.part]}>
						<input type="checkbox" bind:checked={partsStep.want[c.part]} aria-label={c.label} />
						<span class="tick" aria-hidden="true"><Icon name="check" size={14} stroke={2.5} /></span>
						<Icon name={c.icon} size={28} />
						<span class="tile-label">{c.label}</span>
					</label>
				{/each}
			</div>

			<fieldset class="family">
				<legend>{t('welcome.willFamilyOrFriends')}</legend>
				<div class="seg">
					<label class:on={partsStep.family === 'no'}><input type="radio" name="family" value="no" bind:group={partsStep.family} />{t('welcome.noJustMe')}</label>
					<label class:on={partsStep.family === 'yes'}><input type="radio" name="family" value="yes" bind:group={partsStep.family} />{t('welcome.yes')}</label>
				</div>
				<p class="hint">{t('welcome.yesAddsARequests')}</p>
			</fieldset>

			{#if error || partsStep.problem}<p class="form-error" role="alert">{error || partsStep.problem}</p>{/if}
			<Button variant="primary" size="lg" full type="submit" disabled={busy || !!partsStep.problem} {busy}>{t('welcome.continue')}</Button>
		</form>
	{:else if step === 'start'}
		<div class="card" data-testid="welcome-start">
			<div class="tiles two-up">
				<button class="tile big" type="button" onclick={() => pickPath(true)}>
					<Icon name="download" size={26} />
					<span class="tile-label">{t('welcome.bringMyApps')}</span>
					<span class="tile-sub">{t('welcome.sonarrRadarrLidarrOverseerr')}</span>
				</button>
				<button class="tile big" type="button" onclick={() => pickPath(false)}>
					<Icon name="plus" size={26} />
					<span class="tile-label">{t('welcome.startFresh')}</span>
					<span class="tile-sub">{t('welcome.pickFoldersAndA')}</span>
				</button>
			</div>
		</div>
	{:else if step === 'media'}
		<form class="card" onsubmit={(e) => { e.preventDefault(); void mediaStep.saveMedia(false); }} data-testid="welcome-media">
			<PageLoader loading={!mediaStep.rows.length} label={t('welcome.loadingFolders')}>
			<ul class="rows media-list">
				{#each mediaStep.rows as r (r.kind)}
					<li class="frow" class:off={!mediaStep.media[r.part]}>
						<span class="ficon" aria-hidden="true"><Icon name={r.icon} size={20} /></span>
						<div class="fbody">
							<!-- The whole top line labels the switch, so the tap target is the row. -->
							<label class="ftop">
								<span class="fname">{r.label}</span>
								<span class="switch"><input type="checkbox" bind:checked={mediaStep.media[r.part]} onchange={() => (r.error = '')} disabled={busy} /><span class="sw" aria-hidden="true"></span></span>
							</label>
							{#if mediaStep.media[r.part]}
								{#if r.saved && !r.editing}
									<div class="fsaved"><span class="path">{r.saved}</span><button type="button" class="quiet" disabled={busy} onclick={async () => { r.path = r.saved; r.editing = true; await tick(); document.getElementById(`folder-${r.kind}`)?.focus(); }} aria-label={t('welcome.changeFolder', { name: r.label })}>{t('welcome.change')}</button></div>
								{:else}
									<input id="folder-{r.kind}" class="input" bind:value={r.path} oninput={() => (r.error = '')} aria-label={t('welcome.folder', { name: r.label })} placeholder={r.hint} autocomplete="off" spellcheck="false" aria-invalid={r.error ? 'true' : undefined} aria-describedby={r.error ? `folder-${r.kind}-error` : undefined} />
									{#if r.error}<p class="form-error" id="folder-{r.kind}-error" role="alert">{r.error}</p>{/if}
									{#if r.editing && r.saved}<button type="button" class="quiet keep" disabled={busy} onclick={() => { r.path = r.saved; r.editing = false; r.error = ''; }}>{t('welcome.keep', { path: r.saved })}</button>{/if}
								{/if}
							{/if}
						</div>
					</li>
				{/each}
			</ul>
			</PageLoader>
			<p class="hint">{t('welcome.onDockerUseThe')}</p>
			{#if error || mediaStep.mediaOff}<p class="form-error" role="alert">{error || t('welcome.turnOnAtLeast')}</p>{/if}
			<Button variant="primary" size="lg" full type="submit" disabled={busy || mediaStep.mediaOff} {busy}>{t('welcome.continue')}</Button>
			<button class="alt" type="button" disabled={busy || mediaStep.mediaOff} onclick={() => mediaStep.saveMedia(true)}>{t('welcome.skipForNow')}</button>
		</form>
	{:else if step === 'downloader'}
		<form class="card" onsubmit={downloaderStep.saveDownloader} data-testid="welcome-downloader">
			{#if downloaderStep.dlLoadError}
				<LoadError hasData={false} message={downloaderStep.dlLoadError} onretry={downloaderStep.loadDownloader} />
			{:else if downloaderStep.dlList.length}
				<ul class="conn" aria-label={t('welcome.connectedDownloaders')}>
					{#each downloaderStep.dlList as d (d.id)}
						{@const test = downloaderStep.dlTests[d.id]}
						<li>
							<div class="conn-row">
							<div class="conn-text"><b>{d.name}</b><span class="addr">{d.url.replace(/^https?:\/\//, '')}</span></div>
							{#if test}<TestTag ok={test.ok} text={test.text} app={kindWords[d.kind]} />{/if}
							</div>
							{#if test && !test.ok}<p class="form-error" role="alert">{test.text}</p>{/if}
							{#if !downloaderStep.dlForm}
								<div class="row-acts">
									<button type="button" class="quiet" disabled={busy} onclick={() => downloaderStep.editDl(d)} aria-label={t('welcome.edit', { name: d.name })}>{t('welcome.edit2')}</button>
									<button type="button" class="quiet danger" disabled={busy} onclick={() => downloaderStep.removeDl(d)} aria-label={t('welcome.remove', { name: d.name })}>{t('welcome.remove2')}</button>
								</div>
							{/if}
						</li>
					{/each}
				</ul>
				{#if !downloaderStep.dlForm}
					<button class="add-row" type="button" disabled={busy} onclick={() => { error = ''; downloaderStep.dlAdding = true; }}><Icon name="plus" size={16} />{t('welcome.addAnother')}</button>
				{/if}
			{/if}
			{#if downloaderStep.dlForm}
				<div class="tiles five" role="radiogroup" aria-label={t('welcome.downloaderApp')}>
					{#each downloaderStep.dlKinds as k (k)}
						<label class="tile small" class:on={downloaderStep.dl.kind === k}>
							<input type="radio" name="dlkind" value={k} checked={downloaderStep.dl.kind === k} onchange={() => downloaderStep.pickDl(k)} aria-label={kindWords[k]} />
							<span class="tile-label">{kindWords[k]}</span>
						</label>
					{/each}
				</div>
				<div class="field">
					<label for="dl-url">{t('welcome.address')}</label>
					<input id="dl-url" class="input" type="text" inputmode="url" bind:value={downloaderStep.dl.url} oninput={() => (downloaderStep.dlError = '')} placeholder="192.168.1.10:{downloaderStep.dlPorts[downloaderStep.dl.kind]}" autocomplete="off" spellcheck="false" aria-invalid={downloaderStep.dlError ? 'true' : undefined} aria-describedby={downloaderStep.dlError ? 'dl-url-error' : undefined} />
					{#if downloaderStep.dlError}<p class="form-error" id="dl-url-error" role="alert">{downloaderStep.dlError}</p>{/if}
				</div>
				<div class="two">
					{#if downloaderStep.dlUser}
						<div class="field">
							<label for="dl-user">{t('welcome.userName')}</label>
							<input id="dl-user" class="input" bind:value={downloaderStep.dl.username} oninput={() => (downloaderStep.dlAuth = '')} autocomplete="off" autocapitalize="none" spellcheck="false" aria-invalid={downloaderStep.dlAuth ? 'true' : undefined} aria-describedby={downloaderStep.dlAuth ? 'dl-user-error' : undefined} />
								{#if downloaderStep.dlAuth}<p class="form-error" id="dl-user-error" role="alert">{downloaderStep.dlAuth}</p>{/if}
						</div>
					{/if}
					<div class="field">
						<label for="dl-secret">{downloaderStep.dl.kind === 'sabnzbd' ? t('welcome.apiKeyLabel') : t('welcome.passwordLabel')}</label>
						<input id="dl-secret" class="input" type="password" bind:value={downloaderStep.dl.secret} autocomplete="new-password" />
					</div>
				</div>
			{/if}
			{#if error}<p class="form-error" role="alert">{error}</p>{/if}
			<Button variant="primary" size="lg" full type="submit" disabled={busy} {busy}>{downloaderStep.dlEditing != null ? t('welcome.save') : downloaderStep.dlForm ? t('welcome.connect', { name: kindWords[downloaderStep.dl.kind] }) : t('welcome.continue')}</Button>
			{#if downloaderStep.dlForm && downloaderStep.dlList.length}
				<button class="alt" type="button" disabled={busy} onclick={() => { error = ''; downloaderStep.dlError = ''; downloaderStep.dlAdding = false; downloaderStep.dlEditing = null; downloaderStep.dl = emptyForm(); }}>{t('welcome.cancel')}</button>
			{:else}
				<button class="alt" type="button" disabled={busy} onclick={downloaderStep.afterDownloader}>{t('welcome.skipForNow')}</button>
			{/if}
		</form>
	{:else if step === 'source'}
		<div class="card source" data-testid="welcome-source">
			<ConnectBridge onchange={sourcesStep.loadSources} compact />
			<div class="foot">
				{#if sourcesStep.sources > 0}
					<Button variant="primary" size="lg" full type="button" disabled={busy} onclick={() => go('done')}>{t('welcome.continue')}</Button>
				{:else}
					<button class="alt" type="button" disabled={busy} onclick={() => go('done')}>{t('welcome.skipForNow')}</button>
				{/if}
			</div>
		</div>
	{:else if step === 'import'}
		<div class="card move" data-testid="welcome-move">
			{#if importCheck && session.s.hasPassword}
				<Confirm action="password">{#snippet children()}{/snippet}</Confirm>
			{/if}
			<MoveFlow embedded={{ query: sourcesStep.IMPORT_QUERY, ondone: sourcesStep.afterImport, onstep: (s) => (sourcesStep.moveStep = s) }} />
			{#if sourcesStep.moveStep === 'read'}
				<button class="alt" type="button" disabled={busy} onclick={() => pickPath(false)}>{t('welcome.startFreshInstead')}</button>
			{/if}
		</div>
	{:else}
		<div class="card" data-testid="welcome-done">
			{#if sourcesStep.haveSet.length}
				<ul class="todo" aria-label={t('welcome.setUp')}>
					{#each sourcesStep.haveSet as h (h)}
						<li><span class="todo-row"><span class="set-name"><span class="set-tick" aria-hidden="true"><Icon name="check" size={14} stroke={2.5} /></span><span>{h}</span></span></span></li>
					{/each}
				</ul>
			{/if}
			{#if sourcesStep.todo.length}
				<ul class="todo" aria-label={t('welcome.stillToDo')}>
					{#each sourcesStep.todo as item (item.label)}
						<li><a href={to(item.path)} onclick={(e) => { e.preventDefault(); void home(item.path); }}><span>{item.label}</span><span class="todo-end"><span class="todo-state">{t('welcome.notSet')}</span><Icon name="chevronRight" size={16} /></span></a></li>
					{/each}
				</ul>
			{/if}
			{#if error}<p class="form-error" role="alert">{error}</p>{/if}
			<Button variant="primary" size="lg" full type="button" disabled={busy} {busy} onclick={() => home()}>{t('welcome.openHome')}</Button>
			<button class="alt" type="button" disabled={busy} onclick={invite}>{t('welcome.inviteFamily')}</button>
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
	/* The Jackett or Prowlarr card on the Source step joins the glass too: no second box, edges lined up. */
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
	/* Media: one quiet group, rows split by a hairline (like Settings lists). */
	.media-list { gap: 0; border-radius: 16px; border: 1px solid var(--line); background: var(--surface-2); overflow: hidden; }
	.media-list .frow { border: 0; border-radius: 0; background: none; padding: 6px 16px; gap: 12px; }
	.media-list .frow + .frow { border-top: 1px solid var(--line); }
	.media-list .frow:has(input.input) { padding-bottom: 14px; }
	.media-list .ficon { width: 20px; height: 44px; background: none; box-shadow: none; border-radius: 0; }
	.path { color: var(--text); font: 400 14px/1.3 var(--font-ui); overflow-wrap: anywhere; }
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
	.source :global(section) { box-shadow: none; }
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
		.media-list .ficon { width: 20px; height: 44px; }
		.media-list .frow { padding: 6px 14px; }
	}
	@media (pointer: coarse), (max-width: 520px) {
		.alt { min-height: 44px; }
		.source :global(.acts .ui-btn) { min-height: 44px; }
		.seg label { min-height: 44px; }
	}

	/* One layer inside the glass card: lists are flat rows split by hairlines, choices are outlined, and only text fields keep a filled box. */
	.tile { background: transparent; }
	@media (hover: hover) { .tile:hover { background: color-mix(in srgb, var(--text) 4%, transparent); } }
	.media-list { background: none; border: 0; border-radius: 0; border-top: 1px solid var(--line); border-bottom: 1px solid var(--line); overflow: visible; }
	.media-list .frow { padding-left: 2px; padding-right: 2px; }
	.conn { gap: 0; border-top: 1px solid var(--line); }
	.conn > li { border-bottom: 1px solid var(--line); }
	.conn-row { background: none; border: 0; border-radius: 0; padding: 14px 2px 4px; }
	.conn .row-acts { padding: 0 0 12px; }
	.todo { gap: 0; border-top: 1px solid var(--line); }
	.todo > li { border-bottom: 1px solid var(--line); }
	.todo a, .todo-row { background: none; border: 0; border-radius: 0; padding: 0 2px; min-height: 52px; }
	/* Done and still-to-do read as one list. */
	.todo:has(+ .todo) { margin-bottom: 0; }
	.todo + .todo { border-top: 0; }
	@media (hover: hover) { .todo a:hover { color: var(--text); background: color-mix(in srgb, var(--text) 4%, transparent); } }
	/* First run, "no card": content sits straight on the dimmed, blurred picture. */
	.dim { position: fixed; inset: 0; z-index: 0; pointer-events: none; background: radial-gradient(55% 60% at 50% 38%, rgb(0 0 0 / 0.38), transparent 80%), rgb(10 10 12 / 0.28); -webkit-backdrop-filter: blur(14px) saturate(1.25); backdrop-filter: blur(14px) saturate(1.25); }
	.welcome.over-art { width: min(440px, calc(100% - 32px)); color: #fff; --focus-ring: 0 0 0 2px rgb(10 10 12 / 0.9), 0 0 0 4px #fff; }
	.welcome.over-art:has([data-testid='welcome-move']) { width: min(600px, calc(100% - 32px)); }
	.sheet.glass, .sheet.glass.light { padding: 0; border: 0; border-radius: 0; background: none; box-shadow: none; -webkit-backdrop-filter: none; backdrop-filter: none; }
	.glass .corner { top: -8px; left: 0; }
	.glass .corner :global(.ui-ibtn) { background: rgb(255 255 255 / 0.1); color: #fff; }
	.glass .brand img { width: 28px; height: 28px; }
	.glass .brand { min-height: 28px; }
	.glass .dots { margin-top: 18px; }
	.glass .dots li, .glass .dots li.done { background: rgb(255 255 255 / 0.3); }
	.glass .dots li.now { width: 18px; background: #fff; }
	.glass .head { margin: 22px 0 28px; }
	.glass .head h1 { font: 700 30px/1.15 var(--font-ui); letter-spacing: -0.02em; color: #fff; }
	.glass .head p { color: rgb(255 255 255 / 0.72); font: 400 15px/1.45 var(--font-ui); }
	.glass :global(.ui-btn.primary) { height: 48px; min-height: 48px; border-radius: 999px; background: #fff; color: #111; border: 0; box-shadow: none; }
	@media (hover: hover) { .glass :global(.ui-btn.primary:hover:not(:disabled)) { background: rgb(255 255 255 / 0.88); } }
	.glass .alt { color: rgb(255 255 255 / 0.7); font-size: 15px; min-height: 44px; }
	@media (hover: hover) { .glass .alt:hover:not(:disabled) { color: #fff; background: rgb(255 255 255 / 0.08); } }
	.glass :global(.input) { min-height: 46px; height: 46px; border-radius: 12px; background: rgb(255 255 255 / 0.08); border: 1px solid rgb(255 255 255 / 0.1); color: #fff; }
	.glass :global(.input::placeholder) { color: rgb(255 255 255 / 0.45); }
	.glass :global(.input:hover) { border-color: rgb(255 255 255 / 0.2); }
	.glass :global(.input:focus-visible) { border-color: rgb(255 255 255 / 0.6); box-shadow: 0 0 0 3px rgb(255 255 255 / 0.18); background-color: rgb(255 255 255 / 0.1); }
	.glass :global(.input[aria-invalid='true']) { border-color: var(--fail); }
	.glass :global(.field label), .glass .field > label { font: 600 13px/1.3 var(--font-ui); color: rgb(255 255 255 / 0.85); }
	.glass :global(.hint), .glass :global(.muted), .glass .hint { color: rgb(255 255 255 / 0.55); }
	.glass :global(.cb-addr) { color: #fff; font-weight: 600; }
	.glass :global(.link), .glass :global(.quiet) { color: rgb(255 255 255 / 0.7); }
	.glass :global(.ui-seg) { display: grid; width: 100%; padding: 3px; border-radius: 12px; background: rgb(255 255 255 / 0.08); }
	.glass :global(.ui-seg .thumb) { top: 3px; bottom: 3px; left: 3px; width: calc((100% - 6px) / var(--n)); border-radius: 9px; background: rgb(255 255 255 / 0.92); }
	.glass :global(.ui-seg button) { min-height: 40px; color: rgb(255 255 255 / 0.7); font-size: 15px; }
	.glass :global(.ui-seg button[aria-checked='true']) { color: #111; }
	.glass :global(.acts.one-row > .ui-btn) { flex: 1 1 100%; }
	/* Rows: flat, white hairlines. */
	.glass .media-list, .glass .conn, .glass .todo { border-color: rgb(255 255 255 / 0.1); }
	.glass .media-list .frow + .frow, .glass .conn > li, .glass .todo > li { border-color: rgb(255 255 255 / 0.1); }
	.glass .fname, .glass .conn-text b, .glass .path, .glass .todo a, .glass .todo-row { color: #fff; }
	.glass .ficon, .glass .conn-text .addr, .glass .fsaved, .glass .todo-state, .glass .todo :global(svg) { color: rgb(255 255 255 / 0.6); }
	.glass .frow.off .fname { color: rgb(255 255 255 / 0.55); }
	.glass .set-tick { background: none; width: 18px; }
	.glass .todo .set-tick :global(svg) { color: #fff; }
	.glass :global(.bridges > li) { background: none; border: 0; border-radius: 0; border-top: 1px solid rgb(255 255 255 / 0.1); border-bottom: 1px solid rgb(255 255 255 / 0.1); padding: 14px 2px; }
	.glass .add-row, .glass :global(.add-row) { border-color: rgb(255 255 255 / 0.18); color: rgb(255 255 255 / 0.7); }
	/* Choice tiles: outlined, see-through, a faint fill on hover. */
	.glass .tile { border-color: rgb(255 255 255 / 0.14); background: transparent; color: rgb(255 255 255 / 0.75); }
	@media (hover: hover) { .glass .tile:hover { border-color: rgb(255 255 255 / 0.24); background: rgb(255 255 255 / 0.05); } }
	.glass .tile-label { color: #fff; }
	.glass .tile-sub { color: rgb(255 255 255 / 0.6); }
	.glass .tile.on { border-color: rgb(255 255 255 / 0.9); background: rgb(255 255 255 / 0.1); color: #fff; }
	@media (max-width: 520px) {
		.glass .head h1 { font-size: 26px; }
		.glass .head { margin: 18px 0 22px; }
	}
</style>
