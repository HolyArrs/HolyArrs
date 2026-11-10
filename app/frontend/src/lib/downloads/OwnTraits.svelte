<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// Your own traits: words or release groups you name once, then give points, make a must-have or put in Never in any Pick rules. A sheet makes or changes one; removing one also takes it out of every Pick rule.
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Menu from '#lib/ui/Menu.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Field from '#lib/ui/Field.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import { checkOwnTrait, ownTerms } from './pick-rules.ts';
	import type { OwnTrait } from './types.ts';
	import { onMount } from 'svelte';
	import { api, ApiError } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';

	let { mayEdit }: { mayEdit: boolean } = $props();

	let list = $state<OwnTrait[] | null>(null);
	let error = $state('');
	let loading = $state(false);

	async function load() {
		loading = true;
		try {
			list = (await api.get<{ own_traits: OwnTrait[] }>('/own-traits')).own_traits;
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			loading = false;
		}
	}
	onMount(load);

	const matchChoices = $derived<{ value: OwnTrait['match']; label: string }[]>([
		{ value: 'name', label: t('downloads.ot.words') },
		{ value: 'group', label: t('downloads.ot.group') }
	]);
	// Traits synced from TRaSH Guides sit apart: there can be dozens, and only a sync changes them.
	const handMade = $derived((list ?? []).filter((o) => !o.source));
	const synced = $derived((list ?? []).filter((o) => o.source === 'trash'));
	const specWord = (k: string) => ({ name: t('downloads.ot.kindName'), group: t('downloads.ot.kindGroup'), source: t('downloads.ot.kindSource'), resolution: t('downloads.ot.kindResolution') })[k] ?? k;
	const specSummary = (o: OwnTrait) => (o.specs ?? []).map((s) => t(s.negate ? 'downloads.ot.specNot' : 'downloads.ot.spec', { name: s.name, kind: specWord(s.kind) })).join(', ');
	const summary = (o: OwnTrait) => t(o.match === 'group' ? 'downloads.ot.sumGroup' : 'downloads.ot.sumName', { terms: o.terms.join(', ') });

	// The sheet that makes or changes one trait.
	let editing = $state<OwnTrait | null>(null);
	let open = $state(false);
	let name = $state('');
	let match = $state<OwnTrait['match']>('name');
	let termsText = $state('');
	let tried = $state(false);
	let busy = $state(false);
	let serverErrors = $state<Record<string, string>>({});
	let formError = $state('');
	const checked = $derived(checkOwnTrait(name, ownTerms(termsText)));
	const errors = $derived<Record<string, string>>({ ...(tried ? checked : {}), ...serverErrors });

	function openSheet(o: OwnTrait | null) {
		editing = o;
		name = o?.name ?? '';
		match = o?.match ?? 'name';
		termsText = o?.terms.join('\n') ?? '';
		tried = false;
		serverErrors = {};
		formError = '';
		open = true;
	}

	async function save(e: SubmitEvent) {
		e.preventDefault();
		tried = true;
		serverErrors = {};
		formError = '';
		if (Object.keys(checked).length) {
			document.getElementById(checked.name ? 'ot-name' : 'ot-terms')?.focus();
			return;
		}
		busy = true;
		const body = { name: name.trim(), match, terms: ownTerms(termsText) };
		try {
			const saved = editing ? await api.put<OwnTrait>(`/own-traits/${editing.id}`, body) : await api.post<OwnTrait>('/own-traits', body);
			const rest = (list ?? []).filter((x) => x.id !== saved.id);
			list = [...rest, saved].sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }));
			open = false;
			toasts.push(editing ? t('downloads.ot.saved', { name: saved.name }) : t('downloads.ot.made', { name: saved.name }), 'ok', editing ? undefined : { body: t('downloads.ot.ready') });
		} catch (err) {
			const field = err instanceof ApiError && typeof err.body.field === 'string' ? err.body.field : '';
			if (field === 'name' || field === 'terms' || field === 'match') serverErrors = { [field]: message(err) };
			else formError = message(err);
		} finally {
			busy = false;
		}
	}

	// Remove asks first and says what happens to Pick rules that use it.
	let removing = $state<OwnTrait | null>(null);
	let removeOpen = $state(false);
	let removeBusy = $state(false);
	let removeError = $state('');

	async function remove(e: SubmitEvent) {
		e.preventDefault();
		if (!removing) return;
		const gone = removing;
		removeBusy = true;
		removeError = '';
		try {
			const r = await api.del<{ pick_rules_changed: number }>(`/own-traits/${gone.id}`);
			list = (list ?? []).filter((x) => x.id !== gone.id);
			removeOpen = false;
			const n = r?.pick_rules_changed ?? 0;
			toasts.push(t('downloads.ot.removed', { name: gone.name }), 'ok', n ? { body: n === 1 ? t('downloads.ot.outOfOne') : t('downloads.ot.outOfMany', { n }) } : undefined);
		} catch (err) {
			removeError = message(err);
		} finally {
			removeBusy = false;
		}
	}
</script>

<SettingsCard id="own-traits" title={t('downloads.ot.title')} description={t('downloads.ot.desc')}>
	{#if error && !list}
		<LoadError hasData={false} message={error} onretry={load} busy={loading} />
	{:else if list}
		{#if error}<LoadError hasData message={error} onretry={load} busy={loading} />{/if}
		{#if handMade.length === 0}
			<EmptyState compact icon="star" title={t('downloads.ot.none')} text={t('downloads.ot.noneText')} action={mayEdit ? { label: t('downloads.ot.make'), icon: 'plus', onclick: () => openSheet(null) } : undefined} />
		{:else}
			<ul class="rows" data-testid="own-traits">
				{#each handMade as o (o.id)}
					<li data-testid="own-trait">
						<span class="grow">
							<b>{o.name}</b>
							<span class="xs muted meta">{summary(o)}</span>
						</span>
						{#if mayEdit}<span class="acts">
								<Button variant="plain" onclick={() => openSheet(o)}>{t('sources.bridge.change')}</Button>
								<Menu label={t('downloads.ot.moreFor', { name: o.name })} items={[{ label: t('downloads.act.remove'), icon: 'trash', destructive: true, onselect: () => ((removing = o), (removeError = ''), (removeOpen = true)) }]} />
							</span>{/if}
					</li>
				{/each}
			</ul>
			{#if mayEdit}<div><Button icon="plus" onclick={() => openSheet(null)} data-testid="own-trait-add">{t('downloads.ot.make')}</Button></div>{/if}
		{/if}
		{#if synced.length}
			<details class="synced" data-testid="synced-traits">
				<summary>{t('downloads.ot.fromTrash', { n: synced.length })}</summary>
				<p class="xs muted">{t('downloads.ot.syncAgain')}</p>
				<ul class="rows">
					{#each synced as o (o.id)}
						<li data-testid="synced-trait">
							<span class="grow">
								<b>{o.name}</b>
								<span class="xs muted meta">{specSummary(o)}</span>
							</span>
							{#if mayEdit}<span class="acts">
									<Menu label={t('downloads.ot.moreFor', { name: o.name })} items={[{ label: t('downloads.act.remove'), icon: 'trash', destructive: true, onselect: () => ((removing = o), (removeError = ''), (removeOpen = true)) }]} />
								</span>{/if}
						</li>
					{/each}
				</ul>
			</details>
		{/if}
	{/if}
</SettingsCard>

<Sheet bind:open title={editing ? t('downloads.ot.changeTrait') : t('downloads.ot.make')} description={editing?.name} size="sm">
	<form id="own-trait-form" class="form" onsubmit={save} novalidate>
		<Field id="ot-name" label={t('downloads.fact.name')} error={errors.name}>
			<input id="ot-name" class="input" maxlength="60" autocomplete="off" bind:value={name} aria-invalid={errors.name ? 'true' : undefined} aria-describedby={errors.name ? 'ot-name-error' : 'ot-name-hint'} />
		</Field>
		<div class="field">
			<span class="label" id="ot-match-label">{t('downloads.ot.lookAt')}</span>
			<SegmentedControl options={matchChoices} bind:value={match} label={t('downloads.ot.lookAt')} testid="ot-match" />
			<span class="hint">{match === 'group' ? t('downloads.ot.hintGroup') : t('downloads.ot.hintName')}</span>
		</div>
		<Field id="ot-terms" label={match === 'group' ? t('downloads.ot.groups') : t('downloads.ot.wordsLabel')} hint={t('downloads.ot.termsHint')} error={errors.terms}>
			<textarea id="ot-terms" class="input mono" rows="4" spellcheck="false" autocomplete="off" bind:value={termsText} placeholder={match === 'group' ? 'HOLYARRS' : 'NLSubs'} aria-invalid={errors.terms ? 'true' : undefined} aria-describedby={errors.terms ? 'ot-terms-error' : 'ot-terms-hint'}></textarea>
		</Field>
		{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
	</form>
	{#snippet footer()}
		<Button variant="plain" onclick={() => (open = false)}>{t('downloads.sel.cancel')}</Button>
		<Button variant="primary" type="submit" form="own-trait-form" {busy}>{editing ? t('downloads.tv.save') : t('downloads.ot.makeTrait')}</Button>
	{/snippet}
</Sheet>

<Sheet bind:open={removeOpen} title={t('downloads.ot.removeTitle')} description={removing?.name} size="sm">
	<form id="remove-own-trait" onsubmit={remove}>
		<p class="small">{t('downloads.ot.removeNote')}</p>
		{#if removeError}<p class="form-error" role="alert">{removeError}</p>{/if}
	</form>
	{#snippet footer()}
		<Button variant="plain" onclick={() => (removeOpen = false)}>{t('downloads.sel.cancel')}</Button>
		<Button variant="destructive" type="submit" form="remove-own-trait" busy={removeBusy}>{t('downloads.act.remove')}</Button>
	{/snippet}
</Sheet>

<style>
	.form { display: flex; flex-direction: column; gap: 16px; }
	.synced > summary { cursor: pointer; font-weight: 600; padding: 4px 0; }
	.synced > p { margin: 4px 0 8px; }
	.meta { display: block; overflow-wrap: anywhere; }
	.acts { display: flex; align-items: center; gap: 4px; flex: none; }
	@media (max-width: 520px) {
		.rows > li { flex-wrap: wrap; }
		.acts { width: 100%; justify-content: flex-end; }
	}
</style>
