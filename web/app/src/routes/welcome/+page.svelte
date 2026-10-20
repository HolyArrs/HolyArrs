<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { loaderReady, playLoader, SETUP_HOLD_MS } from '#lib/loader.ts';
	import { api, message } from '#lib/api.ts';
	import { to } from '#lib/paths.ts';
	import { session } from '#lib/session.svelte.ts';
	import { refreshParts } from '#lib/parts.svelte.ts';
	import { needsProblem, partStops, type PartName } from '#lib/parts.ts';
	import { applyParts } from '#lib/controls.ts';
	import { theme } from '#lib/theme.svelte.ts';
	const mark = $derived(theme.value === 'light' ? '/brand/mark-color.svg' : '/brand/mark-color-on-dark.svg');

	type Step = 'parts' | 'folder';

	// First run, right after the owner is made: which parts to use, then a
	// Library folder; metadata needs no setup. A later visit to
	// /welcome changes only the parts and returns to Home.
	const firstRun = $derived(page.url.searchParams.get('first') === '1');
	let step = $state<Step>('parts');
	let want = $state<Record<'movies' | 'shows' | 'music' | 'subtitles', boolean>>({ movies: true, shows: true, music: false, subtitles: true });
	let family = $state<'yes' | 'no'>('no');
	let folderPath = $state('');
	let folderName = $state('');
	let folderKind = $state<'movie' | 'show' | 'music'>('movie');
	let busy = $state(false);
	let error = $state('');

	const choices: { part: 'movies' | 'shows' | 'music' | 'subtitles'; label: string }[] = [
		{ part: 'movies', label: 'Movies' },
		{ part: 'shows', label: 'Shows' },
		{ part: 'music', label: 'Music' },
		{ part: 'subtitles', label: 'Subtitles' }
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
		if (!folderPath.trim()) return void (error = 'Type the folder path, as HolyArrs sees it (inside the container if you use Docker).');
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
	const stepNo = $derived(step === 'parts' ? 1 : 2);
	const stepCount = 2;
</script>

<svelte:head><title>{step === 'parts' ? 'What do you use?' : 'Library folder'} · HolyArrs</title></svelte:head>

<div class="welcome">
	<div class="brand"><img src={to(mark)} alt="" width="40" height="40" /><span>HolyArrs</span></div>
	{#if firstRun || step !== 'parts'}
		<ol class="dots" aria-label="Step {stepNo} of {stepCount}">
			{#each Array.from({ length: stepCount }, (_, n) => n + 1) as n (n)}<li class:done={n < stepNo} class:now={n === stepNo}></li>{/each}
		</ol>
	{/if}
	<header class="head">
		{#if step === 'parts'}
			<h1>What do you use?</h1>
			<p>Pick the parts of HolyArrs you want. Parts you leave out stay off and use nothing; change this any time in Settings &gt; Parts and controls.</p>
		{:else}
			<p class="step-word">Step {stepCount} of {stepCount}</p>
			<h1>Library folder</h1>
			<p>Add one folder where finished movies, shows or music should live. You can add more later in Settings.</p>
		{/if}
	</header>

	{#if !session.can('users')}
		<p class="notice warn">Only the owner and admins finish first-run setup.</p>
	{:else if step === 'parts'}
		<form class="card" onsubmit={submitParts} data-testid="welcome">
			{#if firstRun}<p class="step-word">Step 1 of {stepCount}</p>{/if}
			<fieldset>
				<legend>Parts</legend>
				<ul class="choices">
					{#each choices as c (c.part)}
						<li>
							<label class="checkline">
								<input type="checkbox" bind:checked={want[c.part]} aria-label={c.label} aria-describedby="stop-{c.part}" />
								<span><b>{c.label}</b><br /><span id="stop-{c.part}" class="hint">Off: {partStops[c.part]}</span></span>
							</label>
						</li>
					{/each}
				</ul>
			</fieldset>

			<fieldset>
				<legend>Will family or friends ask for titles?</legend>
				<p class="hint" id="family-hint">Yes turns on the Wishlist: people you invite ask, and you approve. Off: {partStops.wishlist}</p>
				<div class="row" role="radiogroup" aria-describedby="family-hint">
					<label class="checkline"><input type="radio" name="family" value="no" bind:group={family} /> No, just me</label>
					<label class="checkline"><input type="radio" name="family" value="yes" bind:group={family} /> Yes</label>
				</div>
			</fieldset>

			{#if error || problem}<p class="form-error" role="alert">{error || problem}</p>{/if}
			<div class="row acts">
				<Button variant="primary" size="lg" type="submit" disabled={busy || !!problem} aria-busy={busy}>Continue</Button>
				<Button type="button" disabled={busy} onclick={() => saveParts({ movies: true, shows: false, music: false, subtitles: false, wishlist: false })}>Just movies</Button>
			</div>
			<p class="xs muted">Change it later in Settings &gt; Parts and controls. Turning a part off keeps its data.</p>
			<p class="xs muted" data-testid="welcome-update-privacy">
				Update check (on; turn it off in Settings &gt; This app): once a day HolyArrs asks api.github.com for the newest release, sending only the header User-Agent:
				HolyArrs/(its version). Nothing else is sent.
			</p>
		</form>
	{:else}
		<form class="card" onsubmit={saveFolder}>
			<div class="field">
				<label for="lf-kind">Holds</label>
				<select id="lf-kind" class="input" bind:value={folderKind}>
					<option value="movie">Movies</option>
					<option value="show">Shows</option>
					<option value="music">Music</option>
				</select>
			</div>
			<div class="field">
				<label for="lf-path">Folder path</label>
				<input id="lf-path" class="input mono" bind:value={folderPath} placeholder={folderKind === 'show' ? '/shows' : folderKind === 'music' ? '/music' : '/movies'} autocomplete="off" spellcheck="false" />
			</div>
			<div class="field">
				<label for="lf-label">Name <span class="muted">(optional)</span></label>
				<input id="lf-label" class="input" bind:value={folderName} placeholder={folderKind === 'show' ? 'Shows' : folderKind === 'music' ? 'Music' : 'Movies'} maxlength="64" />
			</div>
			{#if error}<p class="form-error" role="alert">{error}</p>{/if}
			<div class="go"><Button variant="primary" size="lg" full type="submit" disabled={busy} aria-busy={busy}>Add folder and open Home</Button></div>
		</form>
	{/if}
</div>

<style>
	.welcome {
		width: min(640px, calc(100% - 32px));
		min-height: 100dvh;
		margin: 0 auto;
		padding: clamp(24px, 6vw, 64px) 0;
	}
	.brand { display: flex; align-items: center; justify-content: center; gap: 10px; font: var(--t-headline); color: var(--text-muted); }
	.dots { display: flex; justify-content: center; gap: 8px; margin: 20px 0 0; }
	.dots li { width: 8px; height: 8px; border-radius: 50%; background: var(--line-strong); transition: width var(--dur) var(--ease-out), background var(--dur); }
	.dots li.done { background: var(--ok); }
	.dots li.now { width: 24px; border-radius: 4px; background: var(--accent-fill); }
	.head { margin: 24px 0; text-align: center; }
	.head h1 { font: var(--t-large-title); letter-spacing: var(--t-large-title-track); text-wrap: balance; }
	.head p:not(.step-word) { margin: 8px auto 0; max-width: 52ch; color: var(--text-muted); font: var(--t-body); }
	.step-word { color: var(--text-muted); font: var(--t-footnote); font-weight: 600; margin-bottom: 4px; }
	.welcome > :global(.card) { border: 0; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: var(--shadow-card), 0 0 0 1px var(--line); padding: clamp(18px, 4vw, 28px); }
	fieldset { border: 0; padding: 0; margin: 0 0 24px; }
	legend { margin-bottom: 10px; font: var(--t-title3); }
	.choices { display: flex; flex-direction: column; gap: 4px; }
	.acts { flex-wrap: wrap; gap: 8px; margin-bottom: 12px; }
	.go { margin-top: 20px; }
	@media (max-width: 480px) {
		.acts > :global(*) { flex: 1 1 100%; }
	}
</style>
