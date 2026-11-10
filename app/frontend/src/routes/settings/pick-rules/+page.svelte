<script lang="ts">
	import Words from '#lib/ui/Words.svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
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
	const kinds = (r: PickRulesView) => (r.kinds.length ? r.kinds.map((k) => kindChoices.find(([c]) => c === k)?.[1] ?? k).join(', ') : t('settings.pickRules.everyKind'));

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
			toasts.push(t('settings.pickRules.removed', { name: r.name }), 'ok');
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

<svelte:head><title>{t('settings.pickRules.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.pickRules.title')}>
	{#snippet actions()}
		{#if mayEdit}<Button variant="primary" icon="plus" href={to('/settings/pick-rules/new')}>{t('settings.pickRules.add')}</Button>{/if}
	{/snippet}
</PageHeader>

{#if error && !rules}
	<LoadError hasData={false} message={error} onretry={load} busy={loading} />
{:else if !rules}
	<SkSettings rows={3} />
{:else}
	{#if error}<LoadError hasData message={error} onretry={load} busy={loading} />{/if}
	<SettingsCard id="pick-rules-card" title={t('settings.pickRules.yours')} description={t('settings.pickRules.yoursDesc')}>
		{#if rules.length === 0}
			<EmptyState compact icon="star" title={t('settings.pickRules.none')} text={t('settings.pickRules.noneText')} action={mayEdit ? { label: t('settings.pickRules.add'), href: to('/settings/pick-rules/new'), icon: 'plus' } : undefined} />
		{:else}
			<ul class="rows" data-testid="pick-rules">
				{#each rules as r (r.id)}
					<li data-testid="pick-rule">
						<span class="grow">
							<b>{r.name}</b>
							<span class="xs muted meta">{[kinds(r), usedWords(r.in_use)].filter(Boolean).join(' · ')}</span>
						</span>
						{#if r.preset}<span class="chip">{t('settings.pickRules.readyMade')}</span>{/if}
						<span class="acts">
							<Button variant="plain" href={to(`/settings/pick-rules/${r.id}`)}>{mayEdit ? t('settings.pickRules.change') : t('settings.pickRules.open')}</Button>
							{#if mayEdit}<Menu
								label={t('settings.pickRules.moreFor', { name: r.name })}
								items={[
									{ label: t('settings.pickRules.copy'), icon: 'copy', href: to(`/settings/pick-rules/new?from=${r.id}`) },
									...(r.preset ? [] : [{ label: t('settings.pickRules.remove'), icon: 'trash' as const, destructive: true, onselect: () => askRemove(r) }])
								]}
							/>{/if}
						</span>
					</li>
				{/each}
			</ul>
		{/if}
	</SettingsCard>

	<OwnTraits {mayEdit} />

	<SettingsCard id="pick-rules-where" title={t('settings.pickRules.where')}>
		<ul class="where">
			<li><Words key="settings.pickRules.whereFolders" links={{ a: to('/settings/library-folders') }} /></li>
			<li><Words key="settings.pickRules.whereDefaults" links={{ a: to('/settings/title-defaults') }} /></li>
			<li><Words key="settings.pickRules.whereRequests" links={{ a: to('/settings/requests#ask-rules') }} /></li>
			<li><Words key="settings.pickRules.whereLists" links={{ a: to('/settings/outside-lists') }} /></li>
			<li>{t('settings.pickRules.whereTitle')}</li>
		</ul>
		{#if mayEdit}<p class="hint">{t('settings.pickRules.hint')}</p>
		{:else}<p class="hint" data-testid="read-only">{t('settings.pickRules.readOnly')}</p>{/if}
	</SettingsCard>
{/if}

<Sheet bind:open={removeOpen} title={t('settings.pickRules.removeTitle')} description={removing?.name} size="sm">
	<form id="remove-rules" onsubmit={remove}>
		<p class="small">{t('settings.pickRules.removeNote')}</p>
		{#if removing?.in_use}<p class="small muted">{t('settings.pickRules.removeUsed', { used: usedWords(removing.in_use) })}</p>{/if}
		{#if removeError}
			<p class="form-error" role="alert" data-testid="remove-error">{removeError}{#if removeUse}<br /><span class="muted">{t('settings.pickRules.sentence', { text: removeUse })}</span>{/if}</p>
		{/if}
	</form>
	{#snippet footer()}
		<Button variant="plain" onclick={() => (removeOpen = false)}>{t('settings.pickRules.cancel')}</Button>
		<Button variant="destructive" type="submit" form="remove-rules" busy={removeBusy}>{t('settings.pickRules.remove')}</Button>
	{/snippet}
</Sheet>

<style>
	.where { display: flex; flex-direction: column; gap: 8px; font: var(--t-callout); }
	.where :global(a) { color: var(--accent-text); font-weight: 600; }
	.meta { display: block; }
	.acts { display: flex; align-items: center; gap: 4px; flex: none; }
	@media (max-width: 520px) {
		.rows > li { flex-wrap: wrap; }
		.acts { width: 100%; justify-content: flex-end; }
	}
</style>
