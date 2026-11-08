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
	// A member asks for a movie in one confirm step, so a card or result button never asks in a single click.
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

<Sheet bind:open={sheetOpen} size="sm" title={picked ? `Ask for ${picked.name}${picked.year ? ` (${picked.year})` : ''}` : 'Ask for a movie'}>
	{#if picked}
		<form id="ask-movie-title" class="sheet-form" onsubmit={ask}>
			<p class="small muted">
				{fourK && picked.ask4k
					? `Someone asked for a 4K copy already (${picked.ask4k.state_label}). Asking joins their ask, and you hear when it is ready.`
					: fourK
					? 'A 4K copy is its own ask: it goes on the Requests page and is approved apart from the regular copy.'
					: picked.ask
					? `Someone asked for it already (${picked.ask.state_label}). Asking joins their ask, and you hear when it is ready.`
					: 'You’ll hear when it’s approved and when it’s ready.'}
			</p>
			<label class="checkline" data-testid="follow-box">
				<input type="checkbox" bind:checked={follow} />
				<span><b>Follow</b><br /><span class="hint">Show it on my calendar.</span></span>
			</label>
			{#if !picked.fourKOnly}
				<label class="checkline" data-testid="four-k-box">
					<input type="checkbox" bind:checked={fourK} />
					<span><b>4K copy</b><br /><span class="hint">Ask for the 4K version.</span></span>
				</label>
			{/if}
			{#if error}<p class="form-error" role="alert">{error}</p>{/if}
		</form>
	{/if}
	{#snippet footer()}
		<Button variant="plain" onclick={() => (sheetOpen = false)}>Cancel</Button>
		<Button variant="primary" type="submit" form="ask-movie-title" {busy}>{picked ? `Ask for ${picked.name}${fourK ? ' in 4K' : ''}` : 'Ask'}</Button>
	{/snippet}
</Sheet>

<style>
	.sheet-form { display: flex; flex-direction: column; gap: 4px; }
	.form-error { margin: 8px 0 0; color: var(--fail); font: var(--t-callout); }
</style>
