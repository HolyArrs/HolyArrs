<script lang="ts" module>
	import type { AskBrief } from '../core/types.ts';

	export interface AskMovie {
		tmdb: string;
		name: string;
		year?: number | string;
		ask?: AskBrief;
		/** The open 4K ask, if someone asked for a 4K copy. */
		ask4k?: AskBrief;
		/** Only a 4K copy can be asked (the movie is in the Library, or I asked for the regular one). */
		fourKOnly?: boolean;
	}
</script>

<script lang="ts">
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	// A member asks for a movie in one confirm step, so a card or result button never asks in a single click.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { api, message, ApiError } from '../core/api.ts';
	import type { Ask } from '../core/types.ts';

	let {
		onasked,
		onrefused
	}: {
		onasked: (movie: AskMovie, r: { ask: Ask; joined: boolean; already: boolean }) => void;
		onrefused?: (movie: AskMovie, body: Record<string, unknown>) => void;
	} = $props();

	let sheetOpen = $state(false);
	let picked = $state<AskMovie | null>(null);
	// Follow puts it on my calendar (ticked by default).
	let follow = $state(true);
	// Ask for a 4K copy instead; it is its own ask, approved on its own.
	let fourK = $state(false);
	let busy = $state(false);
	let error = $state('');

	/** Shows the dialog for one movie. */
	export function open(m: AskMovie) {
		follow = true;
		fourK = !!m.fourKOnly;
		picked = m;
		error = '';
		sheetOpen = true;
	}

	async function ask(e: SubmitEvent) {
		e.preventDefault();
		if (!picked || busy) return;
		busy = true;
		error = '';
		try {
			const r = await api.post<{ ask: Ask; joined: boolean; already: boolean }>('/asks', { id: `tmdb:${picked.tmdb}`, follow, ...(fourK ? { four_k: true } : {}) });
			sheetOpen = false;
			onasked(picked, r);
		} catch (err) {
			error = message(err);
			if (err instanceof ApiError && err.status === 409) onrefused?.(picked, err.body);
		} finally {
			busy = false;
		}
	}
</script>

<Sheet bind:open={sheetOpen} size="sm" title={picked ? t('requests.ask.title', { name: `${picked.name}${picked.year ? ` (${picked.year})` : ''}` }) : t('requests.askMovie.titleBare')}>
	{#if picked}
		<form id="ask-movie-title" class="sheet-form" onsubmit={ask}>
			<p class="small muted">
				{fourK && picked.ask4k
					? t('requests.askMovie.already4k', { state: serverWords(picked.ask4k.state_label) })
					: fourK
					? t('requests.askMovie.own4k')
					: picked.ask
					? t('requests.askMovie.already', { state: serverWords(picked.ask.state_label) })
					: t('requests.askMovie.hear')}
			</p>
			<label class="checkline" data-testid="follow-box">
				<input type="checkbox" bind:checked={follow} />
				<span><b>{t('requests.add.follow')}</b><br /><span class="hint">{t('requests.add.followHint')}</span></span>
			</label>
			{#if !picked.fourKOnly}
				<label class="checkline" data-testid="four-k-box">
					<input type="checkbox" bind:checked={fourK} />
					<span><b>{t('requests.askMovie.fourK')}</b><br /><span class="hint">{t('requests.askMovie.fourKHint')}</span></span>
				</label>
			{/if}
			{#if error}<p class="form-error" role="alert">{error}</p>{/if}
		</form>
	{/if}
	{#snippet footer()}
		<Button variant="plain" onclick={() => (sheetOpen = false)}>{t('requests.add.cancel')}</Button>
		<Button variant="primary" type="submit" form="ask-movie-title" {busy}>{picked ? t(fourK ? 'requests.askMovie.submit4k' : 'requests.ask.title', { name: picked.name }) : t('requests.ask.bare')}</Button>
	{/snippet}
</Sheet>

<style>
	.sheet-form { display: flex; flex-direction: column; gap: 4px; }
	.form-error { margin: 8px 0 0; color: var(--fail); font: var(--t-callout); }
</style>
