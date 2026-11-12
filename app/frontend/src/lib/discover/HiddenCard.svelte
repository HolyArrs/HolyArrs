<!-- Settings > People: every title and genre hidden from browsing, for everyone or one person, with Show again. -->
<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { api, message } from '#lib/core/api.ts';
	import HiddenList from './HiddenList.svelte';
	import { loadHides, type Hide } from './hidden.ts';

	let { people, userId }: { people: ReadonlyMap<number, string>; /** One person's hides only (the Change dialog). */ userId?: number } = $props();
	let hides = $state<Hide[] | null>(null);
	let error = $state('');
	let busy = $state(false);

	async function load() {
		busy = true;
		try {
			hides = await loadHides(api, userId);
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			busy = false;
		}
	}
	$effect(() => {
		void userId;
		load();
	});
</script>

{#snippet body()}
	{#if error}
		<LoadError hasData={!!hides} message={error} onretry={load} {busy} />
	{/if}
	{#if hides === null}
		{#if !error}<p class="small muted" aria-busy="true">{t('discover.hidden.loading')}</p>{/if}
	{:else if hides.length === 0}
		<p class="small muted">{userId === undefined ? t('discover.hidden.empty') : t('discover.hidden.emptyPerson', { name: people.get(userId) ?? '' })}</p>
	{:else}
		<HiddenList {hides} {people} onshown={(id) => (hides = (hides ?? []).filter((h) => h.id !== id))} />
	{/if}
{/snippet}

{#if userId === undefined}
	<SettingsCard id="hidden-card" title={t('discover.hidden.cardTitle')}>
		<div class="top">{@render body()}</div>
	</SettingsCard>
{:else}
	<section class="person-hides" aria-labelledby="hidden-person-h">
		<h3 id="hidden-person-h" class="label">{t('discover.hidden.cardTitle')}</h3>
		{@render body()}
	</section>
{/if}

<style>
	.top {
		margin-top: var(--s-3);
	}
	.person-hides {
		margin-top: var(--s-4);
	}
	.person-hides h3 {
		margin: 0 0 var(--s-1);
	}
	p {
		margin: 0;
	}
</style>
