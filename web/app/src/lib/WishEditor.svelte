<script lang="ts">
	// A subtitle wish: languages, each with a hearing-impaired choice and a
	// forced-only switch. Used by Settings > Subtitles (the default) and the
	// subtitle panel of movies, shows and episodes.
	import Icon from './Icon.svelte';
	import { hiWords, langName, type HI, type Language, type Want } from './subtitles.ts';

	let { wants = $bindable(), langs, id }: { wants: Want[]; langs: Language[]; id: string } = $props();

	function add() {
		const used = new Set(wants.map((w) => w.language));
		const next = ['en', 'es', 'fr', 'de'].find((c) => !used.has(c)) ?? '';
		wants.push({ language: next, hearing_impaired: 'normal', forced: false });
	}
</script>

<ul class="wants" data-testid="wish-editor">
	{#each wants as w, i (i)}
		<li class="want" data-testid="wish-row">
			<div class="field">
				<label for="{id}-lang-{i}">Language</label>
				<select id="{id}-lang-{i}" class="input" bind:value={w.language}>
					<option value="" disabled>Pick a language</option>
					{#each langs as l (l.code)}<option value={l.code}>{l.name}</option>{/each}
				</select>
			</div>
			<div class="field">
				<label for="{id}-hi-{i}">Hearing-impaired</label>
				<select id="{id}-hi-{i}" class="input" bind:value={w.hearing_impaired}>
					{#each Object.entries(hiWords) as [k, v] (k)}<option value={k as HI}>{v}</option>{/each}
				</select>
			</div>
			<label class="checkline forced">
				<input type="checkbox" bind:checked={w.forced} />
				<span>Forced only</span>
			</label>
			<button class="btn btn-quiet btn-sm" type="button" onclick={() => wants.splice(i, 1)} aria-label="Take {langName(w.language, langs) || 'this language'} off">
				<Icon name="x" size={16} />
			</button>
		</li>
	{:else}
		<li class="small muted">No languages: no subtitles are looked for.</li>
	{/each}
</ul>
{#if wants.length < 10}
	<button class="btn btn-quiet btn-sm add" type="button" onclick={add}><Icon name="plus" size={16} /> Add a language</button>
{/if}

<style>
	.wants {
		display: flex;
		flex-direction: column;
		gap: var(--s-2);
		margin-top: var(--s-3);
	}
	.want {
		display: grid;
		grid-template-columns: minmax(0, 1.2fr) minmax(0, 1.4fr) auto auto;
		gap: var(--s-3);
		align-items: end;
		padding: var(--s-3);
		border: 1px solid var(--line);
		border-radius: var(--r-sm);
		background: var(--surface-1);
	}
	.want .field {
		margin-top: 0;
	}
	.forced {
		min-height: 40px;
		margin: 0;
		padding-bottom: 1px;
		white-space: nowrap;
	}
	.add {
		margin-top: var(--s-3);
	}
	@media (max-width: 600px) {
		.want {
			grid-template-columns: 1fr auto;
			padding: var(--s-3);
		}
		.want .field {
			grid-column: 1 / -1;
		}
		.want :global(.input),
		.want :global(.btn),
		.add {
			min-height: 44px;
		}
		.add {
			width: 100%;
		}
	}
</style>
