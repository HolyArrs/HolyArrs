<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import Icon, { type IconName } from '#lib/Icon.svelte';
	import { page } from '$app/state';
	import { goto, replaceState } from '$app/navigation';
	import { loaderReady, playLoader, SETUP_HOLD_MS } from '#lib/loader.ts';
	import { api, message } from '#lib/api.ts';
	import { to } from '#lib/paths.ts';
	import { session } from '#lib/session.svelte.ts';
	import { refreshParts } from '#lib/parts.svelte.ts';
	import { needsProblem, type PartName } from '#lib/parts.ts';
	import { applyParts } from '#lib/controls.ts';
	import { theme } from '#lib/theme.svelte.ts';
	const mark = $derived(theme.value === 'light' ? '/brand/mark-color.svg' : '/brand/mark-color-on-dark.svg');

	type Step = 'parts' | 'folder';

	// First run, right after the owner is made: which parts to use, then a
	// Library folder; metadata needs no setup. A later visit to
	// /welcome changes only the parts and returns to Home.
	const firstRun = $derived(page.url.searchParams.get('first') === '1');
	// The folder step is kept in the address so a reload stays on it.
	let step = $state<Step>(page.url.searchParams.get('step') === 'folder' ? 'folder' : 'parts');
	let want = $state<Record<'movies' | 'shows' | 'music' | 'subtitles', boolean>>({ movies: true, shows: true, music: false, subtitles: true });
	let family = $state<'yes' | 'no'>('no');
	let folderPath = $state('');
	let folderName = $state('');
	let folderKind = $state<'movie' | 'show' | 'music'>('movie');
	let busy = $state(false);
	let error = $state('');

	// The labels say it; Settings > Parts has the on/off detail.
	const choices: { part: 'movies' | 'shows' | 'music' | 'subtitles'; label: string; icon: IconName }[] = [
		{ part: 'movies', label: 'Movies', icon: 'film' },
		{ part: 'shows', label: 'TV shows', icon: 'tv' },
		{ part: 'music', label: 'Music', icon: 'music' },
		{ part: 'subtitles', label: 'Subtitles', icon: 'subtitle' }
	];
	const kinds: { value: 'movie' | 'show' | 'music'; label: string; icon: IconName }[] = [
		{ value: 'movie', label: 'Movies', icon: 'film' },
		{ value: 'show', label: 'TV shows', icon: 'tv' },
		{ value: 'music', label: 'Music', icon: 'music' }
	];
	const full = $derived<Record<PartName, boolean>>({ ...want, wishlist: family === 'yes' });
	const problem = $derived(needsProblem(full));
	function firstFolderKind(parts: Record<PartName, boolean>): 'movie' | 'show' | 'music' {
		if (parts.movies) return 'movie';
		if (parts.shows) return 'show';
		return 'music';
	}

	async function saveParts(parts: Record<PartName, boolean>) {
		error = '';
		busy = true;
		try {
			await applyParts(parts);
			await refreshParts();
			if (firstRun) {
				folderKind = firstFolderKind(parts);
				step = 'folder';
				replaceState(to('/welcome?first=1&step=folder'), page.state);
			}
			else await home();
		} catch (e) {
			error = message(e);
		} finally {
			busy = false;
		}
	}

	// Setup is done: a short Orbit while Home opens.
	async function home() {
		playLoader('', SETUP_HOLD_MS);
		await goto(to('/'), { replace: true }).finally(loaderReady);
	}

	function submitParts(e: SubmitEvent) {
		e.preventDefault();
		if (problem) return void (error = problem);
		void saveParts(full);
	}

	async function saveFolder(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		if (!folderPath.trim()) return void (error = "Enter the folder path as HolyArrs sees it. On Docker, that's the path inside the container.");
		busy = true;
		try {
			await api.post('/library-folders', { path: folderPath.trim(), kind: folderKind, label: folderName.trim() });
			await home();
		} catch (e) {
			error = message(e);
		} finally {
			busy = false;
		}
	}
	// Back to step 1 (the parts are already saved; Continue there saves again).
	function back() {
		error = '';
		step = 'parts';
		replaceState(to('/welcome?first=1'), page.state);
	}
	const stepNo = $derived(step === 'parts' ? 1 : 2);
	const stepCount = 2;
</script>

<svelte:head><title>{step === 'parts' ? 'What do you use?' : 'Library folder'} · HolyArrs</title></svelte:head>

<div class="welcome">
	<div class="brand"><img src={to(mark)} alt="" width="32" height="32" /><span>HolyArrs</span></div>
	{#if firstRun && step === 'folder' && session.can('users')}
		<span class="corner"><IconButton icon="chevronLeft" variant="tinted" label="Back" onclick={back} disabled={busy} /></span>
	{/if}
	{#if firstRun || step !== 'parts'}
		<ol class="dots" aria-label="Step {stepNo} of {stepCount}">
			{#each Array.from({ length: stepCount }, (_, n) => n + 1) as n (n)}<li class:done={n < stepNo} class:now={n === stepNo}></li>{/each}
		</ol>
	{/if}
	<header class="head">
		{#if step === 'parts'}
			<h1>What do you use?</h1>
			<p>You can change this later.</p>
		{:else}
			<h1>Library folder</h1>
			<p>Where should finished downloads go? Add one now, more later.</p>
		{/if}
	</header>

	{#if !session.can('users')}
		<p class="notice warn">Only the owner or an admin can finish setup.</p>
	{:else if step === 'parts'}
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
				<p class="hint">Yes adds a Requests page. They ask, you approve.</p>
			</fieldset>

			{#if error || problem}<p class="form-error" role="alert">{error || problem}</p>{/if}
			<Button variant="primary" size="lg" full type="submit" disabled={busy || !!problem} {busy}>Continue</Button>
			<button class="alt" type="button" disabled={busy} onclick={() => saveParts({ movies: true, shows: false, music: false, subtitles: false, wishlist: false })}>Just movies</button>
		</form>
	{:else}
		<form class="card folder" onsubmit={saveFolder}>
			<fieldset>
				<legend>This folder holds</legend>
				<div class="tiles three">
					{#each kinds as k (k.value)}
						<label class="tile small" class:on={folderKind === k.value}>
							<input type="radio" name="kind" value={k.value} bind:group={folderKind} aria-label={k.label} />
							<Icon name={k.icon} size={24} />
							<span class="tile-label">{k.label}</span>
						</label>
					{/each}
				</div>
			</fieldset>
			<div class="field">
				<label for="lf-path">Folder path</label>
				<input id="lf-path" class="input mono" bind:value={folderPath} placeholder={folderKind === 'show' ? '/shows' : folderKind === 'music' ? '/music' : '/movies'} autocomplete="off" spellcheck="false" />
			</div>
			<div class="field">
				<label for="lf-label">Name <span class="muted">(optional)</span></label>
				<input id="lf-label" class="input" bind:value={folderName} placeholder={folderKind === 'show' ? 'TV shows' : folderKind === 'music' ? 'Music' : 'Movies'} maxlength="64" />
			</div>
			{#if error}<p class="form-error" role="alert">{error}</p>{/if}
			<div class="go"><Button variant="primary" size="lg" full type="submit" disabled={busy} {busy}>Add folder and open Home</Button></div>
		</form>
	{/if}
</div>

<style>
	.welcome {
		width: min(560px, calc(100% - 32px));
		min-height: 100dvh;
		margin: 0 auto;
		padding: clamp(28px, 7vh, 72px) 0 48px;
	}
	.brand { display: flex; align-items: center; justify-content: center; gap: 10px; font: 600 17px/1 var(--font-ui); color: var(--text); }
	.dots { display: flex; justify-content: center; gap: 6px; margin: 22px 0 0; }
	.dots li { width: 6px; height: 6px; border-radius: 3px; background: var(--line-strong); transition: width var(--dur) var(--ease-out), background var(--dur) var(--ease-out); }
	.dots li.done { background: var(--text-muted); }
	.dots li.now { width: 22px; background: var(--accent-fill); }
	.head { margin: 18px 0 28px; text-align: center; }
	.head h1 { font: var(--t-large-title); letter-spacing: var(--t-large-title-track); text-wrap: balance; }
	.head p { margin: 8px auto 0; max-width: 44ch; color: var(--text-muted); font: var(--t-body); text-wrap: pretty; }
	.welcome > :global(.card) {
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
	.tiles.three { grid-template-columns: repeat(3, minmax(0, 1fr)); }
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
		cursor: pointer;
		transition: border-color var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out), transform var(--dur-fast) var(--ease-out);
	}
	.tile.small { min-height: 84px; gap: 8px; }
	/* The real control covers the whole tile, invisible, so taps and clicks land on it. */
	.tile input { position: absolute; inset: 0; width: 100%; height: 100%; margin: 0; opacity: 0; cursor: pointer; }
	.tile-label { font: 600 14px/1.2 var(--font-ui); color: var(--text); text-align: center; }
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
	.hint { margin-top: 10px; color: var(--text-muted); font: var(--t-footnote); }

	.welcome :global(.card) > :global(.ui-btn) { margin-top: 26px; }
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

	/* Folder step: the round back button sits top-left, level with the logo. */
	.welcome { position: relative; }
	.corner { position: absolute; top: calc(clamp(28px, 7vh, 72px) - 2px); left: 0; }
	.folder .field { margin-top: 18px; }
	.go { margin-top: 24px; }

	@media (max-width: 520px) {
		.tiles { grid-template-columns: repeat(2, minmax(0, 1fr)); }
		.tiles.three { grid-template-columns: repeat(3, minmax(0, 1fr)); }
		.tile { min-height: 96px; }
		.tile.small { min-height: 76px; }
	}
	@media (pointer: coarse) {
		.alt { min-height: 44px; }
		.seg label { min-height: 44px; }
	}
</style>
