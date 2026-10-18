<script lang="ts">
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { to } from '#lib/paths.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import type { PickRulesView } from '#lib/types.ts';

	let rules = $state<PickRulesView[] | null>(null);
	let error = $state('');
	const kindWords: Record<string, string> = { movie: 'Movies', show: 'TV shows', music: 'Music' };
	const kinds = (r: PickRulesView) => (r.kinds.length ? r.kinds.map((k) => kindWords[k] ?? k).join(', ') : 'Every kind');

	onMount(async () => {
		try {
			rules = (await api.get<{ pick_rules: PickRulesView[] }>('/pick-rules')).pick_rules;
		} catch (e) {
			error = message(e);
		}
	});
</script>

<svelte:head><title>Pick rules · HolyArrs</title></svelte:head>

<PageHeader title="Pick rules" subtitle="Pick rules choose which Release HolyArrs takes for a title, such as 1080p or 4K." />

{#if error}
	<p class="notice bad" role="alert">{error}</p>
{:else if !rules}
	<div class="sk" style="height: 180px"></div>
{:else}
	<SettingsCard id="pick-rules-card" title="Your Pick rules" description="Each Library folder has its own Pick rules, and a title can use other ones.">
		{#if rules.length === 0}
			<EmptyState compact icon="star" title="No Pick rules yet" text="HolyArrs makes its ready-made Pick rules when it starts." />
		{:else}
			<ul class="rows" data-testid="pick-rules">
				{#each rules as r (r.id)}
					<li data-testid="pick-rule">
						<span class="grow"><b>{r.name}</b><br /><span class="xs muted">{kinds(r)}</span></span>
						{#if r.preset}<span class="chip">Ready-made</span>{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</SettingsCard>

	<SettingsCard id="pick-rules-where" title="Where Pick rules are chosen">
		<ul class="where">
			<li><a href={to('/settings/library-folders')}>Library folders</a>: each folder has its own Pick rules, which every new title there gets.</li>
			<li><a href={to('/settings/wishlist#ask-rules')}>Wishlist rules</a>: the rules approved asks get.</li>
			<li><a href={to('/settings/outside-lists')}>Outside lists</a>: the rules titles from a list get.</li>
			<li>A title's own page: change the rules for that one title.</li>
		</ul>
		<p class="hint">Changing what a Pick rule takes is not in the app yet.</p>
	</SettingsCard>
{/if}

<style>
	.where { display: flex; flex-direction: column; gap: 8px; font: var(--t-callout); }
	.where a { color: var(--accent-text); font-weight: 600; }
</style>
