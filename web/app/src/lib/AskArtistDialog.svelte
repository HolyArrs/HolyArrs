<script lang="ts">
	// A member asks for an artist: the whole artist, or the albums they pick.
	import { api, message, ApiError } from './api.ts';
	import { albumKind, albumWords, albumYear } from './music.ts';
	import type { Ask, AskableAlbum, LookupArtist } from './types.ts';

	let {
		onasked,
		onrefused
	}: {
		onasked: (artist: LookupArtist, r: { ask: Ask; joined: boolean; already: boolean }) => void;
		onrefused?: (artist: LookupArtist, body: Record<string, unknown>) => void;
	} = $props();

	let dialog: HTMLDialogElement | undefined = $state();
	let picked = $state<LookupArtist | null>(null);
	let albums = $state<AskableAlbum[] | null>(null);
	let whole = $state(true);
	let chosen = $state<string[]>([]);
	let busy = $state(false);
	let error = $state('');

	/** open shows the dialog for one artist and reads its albums. */
	export async function open(a: LookupArtist) {
		picked = a;
		albums = null;
		whole = true;
		chosen = [];
		error = '';
		dialog?.showModal();
		try {
			const r = await api.get<{ artists: LookupArtist[] }>(`/artists/lookup?term=${encodeURIComponent(`mbid:${a.mbid}`)}&albums=true`);
			albums = r.artists[0]?.ask_albums ?? [];
			if (r.artists[0]) picked = { ...a, ...r.artists[0] };
		} catch (err) {
			albums = [];
			error = message(err);
		}
	}

	function pick(mbid: string, on: boolean) {
		chosen = on ? [...chosen, mbid] : chosen.filter((x) => x !== mbid);
	}

	const chosenNames = $derived((albums ?? []).filter((a) => chosen.includes(a.mbid)).map((a) => ({ mbid: a.mbid, name: a.name })));

	async function ask(e: SubmitEvent) {
		e.preventDefault();
		if (!picked) return;
		busy = true;
		error = '';
		try {
			const r = await api.post<{ ask: Ask; joined: boolean; already: boolean }>('/asks', { kind: 'artist', id: `mbid:${picked.mbid}`, albums: whole ? [] : chosen });
			dialog?.close();
			onasked(picked, r);
		} catch (err) {
			error = message(err);
			if (err instanceof ApiError && err.status === 409) onrefused?.(picked, err.body);
		} finally {
			busy = false;
		}
	}
</script>

<dialog bind:this={dialog} aria-labelledby="ask-artist-title" class="modal">
	{#if picked}
		<form onsubmit={ask}>
			<h2 id="ask-artist-title" class="h2">Ask for {picked.name}</h2>
			{#if picked.ask}
				<p class="small muted" style="margin-top: var(--s-2)">Someone asked for {albumWords(picked.ask.albums)} already ({picked.ask.state_label}). Asking for the same joins their ask.</p>
			{/if}
			<fieldset class="pick">
				<legend class="small">What would you like?</legend>
				<label class="checkline">
					<input type="radio" name="what" checked={whole} onchange={() => (whole = true)} />
					<span><b>Every album</b><br /><span class="hint">Every album of the usual kinds, and albums that come out later.</span></span>
				</label>
				<label class="checkline">
					<input type="radio" name="what" checked={!whole} onchange={() => (whole = false)} disabled={!albums?.length} />
					<span><b>Only some albums</b><br /><span class="hint">Pick them below.</span></span>
				</label>
			</fieldset>
			{#if !whole}
				<fieldset class="albums">
					<legend class="sr-only">Albums</legend>
					{#each albums ?? [] as al (al.mbid)}
						<label class="album" class:off={al.kept || al.left_out} data-testid="ask-album">
							<input type="checkbox" checked={chosen.includes(al.mbid)} disabled={al.kept || al.left_out} onchange={(e) => pick(al.mbid, e.currentTarget.checked)} />
							<span class="grow">
								<b>{al.name}</b>
								<span class="xs muted block">{[albumYear(al.release_date), albumKind(al)].filter(Boolean).join(' · ')}{al.kept ? ' · In the Library' : al.left_out ? ' · Not included; ask the owner' : ''}</span>
							</span>
						</label>
					{/each}
				</fieldset>
			{:else if albums === null}
				<div class="sk" style="height: 60px; margin-top: var(--s-3)"></div>
			{/if}
			{#if error}<p class="form-error" role="alert">{error}</p>{/if}
			<div class="row modal-actions">
				<button class="btn btn-quiet" type="button" onclick={() => dialog?.close()}>Cancel</button>
				<button class="btn btn-red" type="submit" disabled={busy || (!whole && chosen.length === 0)} aria-busy={busy}>
					{whole ? 'Ask for every album' : chosen.length ? `Ask for ${albumWords(chosenNames)}` : 'Pick an album'}
				</button>
			</div>
		</form>
	{/if}
</dialog>

<style>
	.modal {
		width: min(520px, calc(100vw - 32px));
		max-height: calc(100dvh - 32px);
		overflow: auto;
		padding: var(--s-6);
		border: 0;
		border-radius: var(--r-modal);
		background: var(--surface-1);
		color: var(--text);
		box-shadow: var(--shadow-modal);
	}
	.modal::backdrop {
		background: var(--scrim);
	}
	.modal-actions {
		justify-content: flex-end;
		margin-top: var(--s-6);
		flex-wrap: wrap;
	}
	.pick,
	.albums {
		border: 0;
		padding: 0;
		margin: var(--s-4) 0 0;
	}
	.albums {
		display: flex;
		flex-direction: column;
		gap: var(--s-2);
	}
	.album {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		min-height: 48px;
		padding: var(--s-2) var(--s-3);
		border: 1px solid var(--line-strong);
		border-radius: var(--r-btn);
		cursor: pointer;
	}
	.album:has(input:checked) {
		border-color: var(--ha-red);
		background: var(--ha-red-soft);
	}
	/* Muted grey on the tint, or faded further, falls below the contrast floor. */
	.album:has(input:checked) .muted {
		color: var(--text);
	}
	.album.off {
		cursor: default;
		color: var(--text-muted);
	}
	.block {
		display: block;
	}
	.grow {
		min-width: 0;
		overflow-wrap: anywhere;
	}
</style>
