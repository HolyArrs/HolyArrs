<script lang="ts" module>
	import type { AskBrief } from './types.ts';

	export interface AskMovie {
		tmdb: string;
		name: string;
		year?: number | string;
		ask?: AskBrief;
	}
</script>

<script lang="ts">
	// A member asks for a movie: one confirm step, so a card or result button
	// never asks in a single click.
	import Sheet from './ui/Sheet.svelte';
	import Button from './ui/Button.svelte';
	import { api, message, ApiError } from './api.ts';
	import type { Ask } from './types.ts';

	let {
		onasked,
		onrefused
	}: {
		onasked: (movie: AskMovie, r: { ask: Ask; joined: boolean; already: boolean }) => void;
		onrefused?: (movie: AskMovie, body: Record<string, unknown>) => void;
	} = $props();

	let sheetOpen = $state(false);
	let picked = $state<AskMovie | null>(null);
	let busy = $state(false);
	let error = $state('');

	/** open shows the dialog for one movie. */
	export function open(m: AskMovie) {
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
			const r = await api.post<{ ask: Ask; joined: boolean; already: boolean }>('/asks', { id: `tmdb:${picked.tmdb}` });
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

<Sheet bind:open={sheetOpen} size="sm" title={picked ? `Ask for ${picked.name}${picked.year ? ` (${picked.year})` : ''}` : 'Ask for a movie'}>
	{#if picked}
		<form id="ask-movie-title" class="sheet-form" onsubmit={ask}>
			<p class="small muted">
				{picked.ask
					? `Someone asked for it already (${picked.ask.state_label}). Asking joins their ask, and you hear when it is ready.`
					: 'It goes on the Wishlist. You hear when it is approved and when it is ready to watch.'}
			</p>
			{#if error}<p class="form-error" role="alert">{error}</p>{/if}
		</form>
	{/if}
	{#snippet footer()}
		<Button variant="plain" onclick={() => (sheetOpen = false)}>Cancel</Button>
		<Button variant="primary" type="submit" form="ask-movie-title" {busy}>{picked ? `Ask for ${picked.name}` : 'Ask'}</Button>
	{/snippet}
</Sheet>

<style>
	.sheet-form { display: flex; flex-direction: column; gap: 4px; }
	.form-error { margin: 8px 0 0; color: var(--fail); font: var(--t-callout); }
</style>
