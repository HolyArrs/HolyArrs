<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api, message } from '#lib/api.ts';
	import { to } from '#lib/paths.ts';
	import { session } from '#lib/session.svelte.ts';
	import { refreshParts } from '#lib/parts.svelte.ts';
	import { needsProblem, partStops, type PartName } from '#lib/parts.ts';
	import { applyParts } from '#lib/controls.ts';

	type Step = 'parts' | 'tmdb' | 'folder';

	// First run, right after the owner is made: which parts to use, then the
	// two things needed before Home can do useful work. A later visit to
	// /welcome changes only the parts and returns to Home.
	const firstRun = $derived(page.url.searchParams.get('first') === '1');
	let step = $state<Step>('parts');
	let want = $state<Record<'movies' | 'shows' | 'music' | 'subtitles', boolean>>({ movies: true, shows: true, music: false, subtitles: true });
	let family = $state<'yes' | 'no'>('no');
	let tmdbKey = $state('');
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
				// TMDB is needed only when Movies or Shows is on. A music-only
				// setup goes straight to its Music Library folder.
				step = parts.movies || parts.shows ? 'tmdb' : 'folder';
			}
			else await goto(to('/'), { replace: true });
		} catch (e) {
			error = message(e);
		} finally {
			busy = false;
		}
	}

	function submitParts(e: SubmitEvent) {
		e.preventDefault();
		if (problem) return void (error = problem);
		void saveParts(full);
	}

	async function saveKey(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		if (!tmdbKey.trim()) return void (error = 'Paste the TMDB read access token.');
		busy = true;
		try {
			await api.put('/settings/tmdb-token', { token: tmdbKey.trim() });
			tmdbKey = '';
			step = 'folder';
		} catch (e) {
			error = message(e);
		} finally {
			busy = false;
		}
	}

	async function saveFolder(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		if (!folderPath.trim()) return void (error = 'Type the folder path, as HolyArrs sees it (inside the container if you use Docker).');
		busy = true;
		try {
			await api.post('/library-folders', { path: folderPath.trim(), kind: folderKind, label: folderName.trim() });
			await goto(to('/'), { replace: true });
		} catch (e) {
			error = message(e);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>{step === 'parts' ? 'What do you use?' : step === 'tmdb' ? 'TMDB key' : 'Library folder'} · HolyArrs</title></svelte:head>

<div class="welcome">
	<header class="page-head">
		<div>
			{#if step === 'parts'}
				<h1>What do you use?</h1>
				<p>Pick the parts of HolyArrs you want. Parts you leave out stay off and use nothing; change this any time in Settings &gt; Parts and controls.</p>
			{:else if step === 'tmdb'}
				<p class="mono xs muted">Step 2 of 3</p>
				<h1>TMDB key</h1>
				<p>HolyArrs uses your TMDB read access token to look up movie and show names, dates and pictures. It is stored encrypted and never shown again in full.</p>
			{:else}
				<p class="mono xs muted">{want.movies || want.shows ? 'Step 3 of 3' : 'Step 2 of 2'}</p>
				<h1>Library folder</h1>
				<p>Add one folder where finished movies, shows or music should live. You can add more later in Settings.</p>
			{/if}
		</div>
	</header>

	{#if !session.can('users')}
		<p class="notice warn">Only the owner and admins finish first-run setup.</p>
	{:else if step === 'parts'}
		<form class="card" onsubmit={submitParts} data-testid="welcome">
			{#if firstRun}<p class="mono xs muted step">Step 1 of {want.movies || want.shows ? '3' : '2'}</p>{/if}
			<fieldset>
				<legend class="h2">Parts</legend>
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
				<legend class="h2">Will family or friends ask for titles?</legend>
				<p class="hint" id="family-hint">Yes turns on the Wishlist: people you invite ask, and you approve. Off: {partStops.wishlist}</p>
				<div class="row" role="radiogroup" aria-describedby="family-hint">
					<label class="checkline"><input type="radio" name="family" value="no" bind:group={family} /> No, just me</label>
					<label class="checkline"><input type="radio" name="family" value="yes" bind:group={family} /> Yes</label>
				</div>
			</fieldset>

			{#if error || problem}<p class="form-error" role="alert">{error || problem}</p>{/if}
			<div class="row acts">
				<button class="btn btn-red" type="submit" disabled={busy || !!problem} aria-busy={busy}>Continue</button>
				<button
					class="btn btn-ghost"
					type="button"
					disabled={busy}
					onclick={() => saveParts({ movies: true, shows: false, music: false, subtitles: false, wishlist: false })}>Just movies</button
				>
			</div>
			<p class="xs muted">Change it later in Settings &gt; Parts and controls. Turning a part off keeps its data.</p>
			<p class="xs muted" data-testid="welcome-update-privacy">
				Update check (on; turn it off in Settings &gt; This app): once a day HolyArrs asks api.github.com for the newest release, sending only the header User-Agent:
				HolyArrs/(its version). Nothing else is sent.
			</p>
		</form>
	{:else if step === 'tmdb'}
		<form class="card" onsubmit={saveKey}>
			<div class="field">
				<label for="tmdb">Paste the key</label>
				<input id="tmdb" class="input mono" type="password" bind:value={tmdbKey} autocomplete="off" spellcheck="false" placeholder="eyJhbGciOi…" />
				<span class="hint">Find the long API Read Access Token on themoviedb.org under Settings, API.</span>
			</div>
			{#if error}<p class="form-error" role="alert">{error}</p>{/if}
			<button class="btn btn-red" type="submit" disabled={busy} aria-busy={busy}>Save and continue</button>
			<p class="xs muted notice-line">This product uses the TMDB API but is not endorsed or certified by TMDB.</p>
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
			<button class="btn btn-red" type="submit" disabled={busy} aria-busy={busy}>Add folder and open Home</button>
		</form>
	{/if}
</div>

<style>
	.welcome {
		display: grid;
		grid-template-columns: minmax(220px, 280px) minmax(0, 680px);
		align-items: start;
		gap: var(--s-8);
		width: min(1040px, calc(100% - 32px));
		min-height: 100dvh;
		margin: 0 auto;
		padding: var(--s-10) 0;
	}
	.welcome > :global(.page-head) {
		position: sticky;
		top: var(--s-8);
		padding-top: var(--s-4);
		border-top: 1px solid var(--line);
	}
	.welcome > :global(.card),
	.welcome > :global(.notice) {
		grid-column: 2;
	}
	fieldset {
		border: 0;
		padding: 0;
		margin: 0 0 var(--s-5);
	}
	legend,
	.step {
		margin-bottom: var(--s-2);
	}
	.choices {
		display: flex;
		flex-direction: column;
		gap: var(--s-3);
	}
	.acts {
		flex-wrap: wrap;
		gap: var(--s-3);
	}
	.notice-line {
		margin-top: var(--s-5);
	}
	@media (max-width: 800px) {
		.welcome {
			display: block;
			width: min(680px, calc(100% - 32px));
			padding: var(--s-6) 0;
		}
		.welcome > :global(.page-head) {
			position: static;
			padding-top: var(--s-3);
		}
	}
	@media (max-width: 480px) {
		.acts > :global(.btn) {
			width: 100%;
			min-height: 44px;
		}
	}
</style>
