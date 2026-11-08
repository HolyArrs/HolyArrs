<script lang="ts">
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Menu from '#lib/ui/Menu.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import { OwnTraits } from '#lib/downloads/index.ts';
	import { kindChoices, usedWords } from '#lib/downloads/index.ts';
	import type { PickRulesUse } from '#lib/downloads/index.ts';
	import { onMount } from 'svelte';
	import { api, ApiError } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { to } from '#lib/core/paths.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import type { PickRulesView } from '#lib/core/types.ts';

	// People without the settings permission see the rules but cannot change them.
	const mayEdit = $derived(session.can('settings'));
	let rules = $state<PickRulesView[] | null>(null);
	let error = $state('');
	let loading = $state(false);
	const kinds = (r: PickRulesView) => (r.kinds.length ? r.kinds.map((k) => kindChoices.find(([c]) => c === k)?.[1] ?? k).join(', ') : 'Every kind');

	// Remove asks first; the server refuses rules that something still uses.
	let removing = $state<PickRulesView | null>(null);
	let removeOpen = $state(false);
	let removeBusy = $state(false);
	let removeError = $state('');
	let removeUse = $state('');

	async function load() {
		loading = true;
		try {
			rules = (await api.get<{ pick_rules: PickRulesView[] }>('/pick-rules')).pick_rules;
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			loading = false;
		}
	}
	onMount(load);

	function askRemove(r: PickRulesView) {
		removing = r;
		removeError = '';
		removeUse = '';
		removeOpen = true;
	}

	async function remove(e: SubmitEvent) {
		e.preventDefault();
		if (!removing) return;
		const r = removing;
		removeBusy = true;
		removeError = '';
		removeUse = '';
		try {
			await api.del(`/pick-rules/${r.id}`);
			rules = (rules ?? []).filter((x) => x.id !== r.id);
			removeOpen = false;
			toasts.push(`${r.name} removed`, 'ok');
		} catch (err) {
			removeError = message(err);
			if (err instanceof ApiError && err.status === 409) {
				const use = err.body.in_use;
				removeUse = typeof use === 'number' || (use && typeof use === 'object') ? usedWords(use as PickRulesUse | number) : '';
			}
		} finally {
			removeBusy = false;
		}
	}
</script>

<svelte:head><title>Quality · HolyArrs</title></svelte:head>

<PageHeader title="Quality">
	{#snippet actions()}
		{#if mayEdit}<Button variant="primary" icon="plus" href={to('/settings/pick-rules/new')}>Add a quality set</Button>{/if}
	{/snippet}
</PageHeader>

{#if error && !rules}
	<LoadError hasData={false} message={error} onretry={load} busy={loading} />
{:else if !rules}
	<SkSettings rows={3} />
{:else}
	{#if error}<LoadError hasData message={error} onretry={load} busy={loading} />{/if}
	<SettingsCard id="pick-rules-card" title="Your quality sets" description="Each Library folder has its own; a title can override">
		{#if rules.length === 0}
			<EmptyState compact icon="star" title="No quality sets" text="HolyArrs makes its ready-made quality sets when it starts." action={mayEdit ? { label: 'Add a quality set', href: to('/settings/pick-rules/new'), icon: 'plus' } : undefined} />
		{:else}
			<ul class="rows" data-testid="pick-rules">
				{#each rules as r (r.id)}
					<li data-testid="pick-rule">
						<span class="grow">
							<b>{r.name}</b>
							<span class="xs muted meta">{[kinds(r), usedWords(r.in_use)].filter(Boolean).join(' · ')}</span>
						</span>
						{#if r.preset}<span class="chip">Ready-made</span>{/if}
						<span class="acts">
							<Button variant="plain" href={to(`/settings/pick-rules/${r.id}`)}>{mayEdit ? 'Change' : 'Open'}</Button>
							{#if mayEdit}<Menu
								label="More for {r.name}"
								items={[
									{ label: 'Make a copy', icon: 'copy', href: to(`/settings/pick-rules/new?from=${r.id}`) },
									...(r.preset ? [] : [{ label: 'Remove', icon: 'trash' as const, destructive: true, onselect: () => askRemove(r) }])
								]}
							/>{/if}
						</span>
					</li>
				{/each}
			</ul>
		{/if}
	</SettingsCard>

	<OwnTraits {mayEdit} />

	<SettingsCard id="pick-rules-where" title="Where quality is chosen">
		<ul class="where">
			<li><a href={to('/settings/library-folders')}>Library folders</a>: each folder has its own quality set, which every new title there gets.</li>
			<li><a href={to('/settings/title-defaults')}>Movies and TV defaults</a>: the rules a new movie or show gets.</li>
			<li><a href={to('/settings/requests#ask-rules')}>Request rules</a>: the rules approved requests get.</li>
			<li><a href={to('/settings/outside-lists')}>Outside lists</a>: the rules titles from a list get.</li>
			<li>A title's page, for that title only</li>
		</ul>
		{#if mayEdit}<p class="hint">Copy a ready-made set to start your own</p>
		{:else}<p class="hint" data-testid="read-only">View only. Changing needs the Settings permission.</p>{/if}
	</SettingsCard>
{/if}

<Sheet bind:open={removeOpen} title="Remove this quality set?" description={removing?.name} size="sm">
	<form id="remove-rules" onsubmit={remove}>
		<p class="small">Can't be undone. Files stay.</p>
		{#if removing?.in_use}<p class="small muted">{usedWords(removing.in_use)}. Pick other rules there first.</p>{/if}
		{#if removeError}
			<p class="form-error" role="alert" data-testid="remove-error">{removeError}{#if removeUse}<br /><span class="muted">{removeUse}.</span>{/if}</p>
		{/if}
	</form>
	{#snippet footer()}
		<Button variant="plain" onclick={() => (removeOpen = false)}>Cancel</Button>
		<Button variant="destructive" type="submit" form="remove-rules" busy={removeBusy}>Remove</Button>
	{/snippet}
</Sheet>

<style>
	.where { display: flex; flex-direction: column; gap: 8px; font: var(--t-callout); }
	.where a { color: var(--accent-text); font-weight: 600; }
	.meta { display: block; }
	.acts { display: flex; align-items: center; gap: 4px; flex: none; }
	@media (max-width: 520px) {
		.rows > li { flex-wrap: wrap; }
		.acts { width: 100%; justify-content: flex-end; }
	}
</style>
