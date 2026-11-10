<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import { hiWords, langName, type HI, type Language, type Want } from './api.ts';

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
				<label for="{id}-lang-{i}">{t('subtitles.wants.language')}</label>
				<select id="{id}-lang-{i}" class="input" bind:value={w.language}>
					<option value="" disabled>{t('subtitles.wants.pick')}</option>
					{#each langs as l (l.code)}<option value={l.code}>{l.name}</option>{/each}
				</select>
			</div>
			<div class="field">
				<label for="{id}-hi-{i}">{t('subtitles.wants.hi')}</label>
				<select id="{id}-hi-{i}" class="input" bind:value={w.hearing_impaired}>
					{#each Object.entries(hiWords) as [k, v] (k)}<option value={k as HI}>{v}</option>{/each}
				</select>
			</div>
			<label class="checkline forced">
				<input type="checkbox" bind:checked={w.forced} />
				<span>{t('subtitles.wants.forced')}</span>
			</label>
			<IconButton icon="x" variant="tinted" tooltip={false} onclick={() => wants.splice(i, 1)}  label={t('subtitles.wants.takeOff', { name: langName(w.language, langs) || t('subtitles.wants.thisLanguage') })} />
		</li>
	{:else}
		<li class="small muted">{t('subtitles.wants.empty')}</li>
	{/each}
</ul>
{#if wants.length < 10}
	<div class="add"><Button icon="plus" onclick={add}>{t('subtitles.wants.add')}</Button></div>
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
		padding: 12px;
		border: 0;
		border-radius: var(--r-card);
		background: var(--surface-2);
	}
	.wants { padding: 0; list-style: none; }
	.wants > li.small { color: var(--text-muted); font: var(--t-callout); }
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
		.want :global(.input) {
			min-height: 44px;
		}
		.add :global(.ui-btn) {
			width: 100%;
		}
	}
</style>
