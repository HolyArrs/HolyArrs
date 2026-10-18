<script lang="ts">
	import Field from '#lib/ui/Field.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { onMount } from 'svelte';
	import { api, message, ApiError } from '#lib/api.ts';
	import { to } from '#lib/paths.ts';
	import { session } from '#lib/session.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import EmptyState from '#lib/EmptyState.svelte';

	const NOTICE = 'This product uses the TMDB API but is not endorsed or certified by TMDB.';

	let masked = $state('');
	let from = $state<'stored' | 'file' | ''>('');
	let loaded = $state(false);
	let missingRoute = $state(false);
	let needsProof = $state(false);
	let error = $state('');
	let newKey = $state('');
	let saving = $state(false);
	let formError = $state('');

	onMount(async () => {
		// Admin-scoped routes require a recent local-password proof for browser
		// sessions. Avoid a request the server must reject while keeping API-key
		// clients compatible with their own scope checks.
		if (session.s.via !== 'api_key' && !session.s.localRecent) {
			needsProof = true;
			loaded = true;
			return;
		}
		try {
			const r = await api.get<{ token: string; from: 'stored' | 'file' }>('/settings/tmdb-token');
			masked = r.token;
			from = r.from;
		} catch (e) {
			if (e instanceof ApiError && e.status === 404) missingRoute = true;
			else if (e instanceof ApiError && e.status === 403) needsProof = true;
			else error = message(e);
		} finally {
			loaded = true;
		}
	});

	async function save(e: SubmitEvent) {
		e.preventDefault();
		formError = '';
		if (!newKey.trim()) {
			formError = 'Paste the TMDB read access token.';
			return;
		}
		saving = true;
		try {
			const r = await api.put<{ token: string; from: 'stored' }>('/settings/tmdb-token', { token: newKey.trim() });
			masked = r.token;
			from = r.from;
			newKey = '';
			toasts.push('TMDB key saved. Search is ready.', 'ok');
		} catch (err) {
			if (err instanceof ApiError && err.status === 403) {
				needsProof = true;
				formError = '';
			} else formError = message(err);
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head><title>TMDB key · HolyArrs</title></svelte:head>

<PageHeader title="Metadata" subtitle="HolyArrs looks up movie names, dates and posters on TMDB, with your own read access token. It is stored encrypted and never shown again in full." />

{#if !loaded}
	<div class="sk" style="height: 160px"></div>
{:else if error}
	<p class="notice bad" role="alert">{error}</p>
{:else if needsProof}
	<section class="card proof-state" aria-labelledby="h-confirm-key">
		<h2 id="h-confirm-key" class="h2">Confirm your password to manage this key</h2>
		<p class="small muted">TMDB credentials are protected settings. Confirm your HolyArrs password in My account, then return here to view the saved state or replace the token.</p>
		<Button href={to('/settings/account')}>Open My account</Button>
	</section>
{:else if missingRoute || from === 'file'}
	<EmptyState icon="key" title="The key comes from a file">
		<p>This HolyArrs reads its TMDB token from the file named by <code>HOLYARRS_TMDB_TOKEN_FILE</code>. Change the file to change the key.</p>
	</EmptyState>
{:else}
	<section class="card" id="tmdb-card" tabindex="-1" aria-labelledby="h-key">
		<div class="card-head">
			<h2 id="h-key" class="h2">Read access token</h2>
			<span class="chip {masked ? 'ok' : ''}">{masked ? 'Saved' : 'Not set'}</span>
		</div>
		{#if masked}<p class="small muted">Current key: <span class="mono">{masked}</span></p>{/if}
		<form onsubmit={save} style="margin-top: var(--s-4)">
			<Field id="tmdb" label={masked ? 'Replace the key' : 'Paste the key'}>
				<input id="tmdb" class="input mono" type="password" bind:value={newKey} autocomplete="off" spellcheck="false" placeholder="eyJhbGciOi…" />
				<span class="hint">Find it on themoviedb.org under Settings, API: the long "API Read Access Token", not the short API key.</span>
			</Field>
			{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
			<Button variant="primary" type="submit" disabled={saving} aria-busy={saving} style="margin-top: var(--s-4)">Save the key</Button>
		</form>
	</section>
{/if}

<p class="xs muted notice-line">
	Movie data and pictures from <a href="https://www.themoviedb.org/" rel="noopener noreferrer" target="_blank">TMDB</a>. {NOTICE}
</p>

<style>
	.notice-line {
		margin-top: var(--s-6);
	}
	.notice-line a {
		text-decoration: underline;
	}
	.proof-state {
		max-width: 720px;
	}
	.proof-state p {
		margin: var(--s-2) 0 var(--s-4);
	}
	code {
		font-family: var(--font-mono);
		color: var(--text);
	}
</style>
