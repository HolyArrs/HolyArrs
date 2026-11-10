<script lang="ts">
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

	const matchChoices: { value: OwnTrait['match']; label: string }[] = [
		{ value: 'name', label: 'Words in the name' },
		{ value: 'group', label: 'Release group' }
	];
	// Traits synced from TRaSH Guides sit apart: there can be dozens, and only a sync changes them.
	const handMade = $derived((list ?? []).filter((t) => !t.source));
	const synced = $derived((list ?? []).filter((t) => t.source === 'trash'));
	const specWord = { name: 'name', group: 'group', source: 'source', resolution: 'resolution' } as const;
	const specSummary = (t: OwnTrait) => (t.specs ?? []).map((s) => `${s.negate ? 'not ' : ''}${s.name} (${specWord[s.kind] ?? s.kind})`).join(', ');
	const summary = (t: OwnTrait) => `${t.match === 'group' ? 'Release group' : 'Name has'}: ${t.terms.join(', ')}`;

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

	function openSheet(t: OwnTrait | null) {
		editing = t;
		name = t?.name ?? '';
		match = t?.match ?? 'name';
		termsText = t?.terms.join('\n') ?? '';
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
			const t = editing ? await api.put<OwnTrait>(`/own-traits/${editing.id}`, body) : await api.post<OwnTrait>('/own-traits', body);
			const rest = (list ?? []).filter((x) => x.id !== t.id);
			list = [...rest, t].sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }));
			open = false;
			toasts.push(editing ? `${t.name} saved` : `${t.name} made`, 'ok', editing ? undefined : { body: 'Ready for quality sets' });
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
		const t = removing;
		removeBusy = true;
		removeError = '';
		try {
			const r = await api.del<{ pick_rules_changed: number }>(`/own-traits/${t.id}`);
			list = (list ?? []).filter((x) => x.id !== t.id);
			removeOpen = false;
			const n = r?.pick_rules_changed ?? 0;
			toasts.push(`${t.name} removed`, 'ok', n ? { body: `Also taken out of ${n === 1 ? '1 set' : `${n} sets`}` } : undefined);
		} catch (err) {
			removeError = message(err);
		} finally {
			removeBusy = false;
		}
	}
</script>

<SettingsCard id="own-traits" title="Your traits" description="Words or release groups to look for in Release names">
	{#if error && !list}
		<LoadError hasData={false} message={error} onretry={load} busy={loading} />
	{:else if list}
		{#if error}<LoadError hasData message={error} onretry={load} busy={loading} />{/if}
		{#if handMade.length === 0}
			<EmptyState compact icon="star" title="No traits of your own" text="Make one for a word like a subtitle language, or for release groups you like or avoid." action={mayEdit ? { label: 'Make a trait', icon: 'plus', onclick: () => openSheet(null) } : undefined} />
		{:else}
			<ul class="rows" data-testid="own-traits">
				{#each handMade as t (t.id)}
					<li data-testid="own-trait">
						<span class="grow">
							<b>{t.name}</b>
							<span class="xs muted meta">{summary(t)}</span>
						</span>
						{#if mayEdit}<span class="acts">
								<Button variant="plain" onclick={() => openSheet(t)}>Change</Button>
								<Menu label="More for {t.name}" items={[{ label: 'Remove', icon: 'trash', destructive: true, onselect: () => ((removing = t), (removeError = ''), (removeOpen = true)) }]} />
							</span>{/if}
					</li>
				{/each}
			</ul>
			{#if mayEdit}<div><Button icon="plus" onclick={() => openSheet(null)} data-testid="own-trait-add">Make a trait</Button></div>{/if}
		{/if}
		{#if synced.length}
			<details class="synced" data-testid="synced-traits">
				<summary>From TRaSH Guides ({synced.length})</summary>
				<p class="xs muted">Sync a set of Pick rules again to update these</p>
				<ul class="rows">
					{#each synced as t (t.id)}
						<li data-testid="synced-trait">
							<span class="grow">
								<b>{t.name}</b>
								<span class="xs muted meta">{specSummary(t)}</span>
							</span>
							{#if mayEdit}<span class="acts">
									<Menu label="More for {t.name}" items={[{ label: 'Remove', icon: 'trash', destructive: true, onselect: () => ((removing = t), (removeError = ''), (removeOpen = true)) }]} />
								</span>{/if}
						</li>
					{/each}
				</ul>
			</details>
		{/if}
	{/if}
</SettingsCard>

<Sheet bind:open title={editing ? 'Change trait' : 'Make a trait'} description={editing?.name} size="sm">
	<form id="own-trait-form" class="form" onsubmit={save} novalidate>
		<Field id="ot-name" label="Name" error={errors.name}>
			<input id="ot-name" class="input" maxlength="60" autocomplete="off" bind:value={name} aria-invalid={errors.name ? 'true' : undefined} aria-describedby={errors.name ? 'ot-name-error' : 'ot-name-hint'} />
		</Field>
		<div class="field">
			<span class="label" id="ot-match-label">Look at</span>
			<SegmentedControl options={matchChoices} bind:value={match} label="Look at" testid="ot-match" />
			<span class="hint">{match === 'group' ? 'The release group, the name after the last dash, must be one of these words, or fit one of the patterns.' : 'The Release name must have one of these anywhere in it.'}</span>
		</div>
		<Field id="ot-terms" label={match === 'group' ? 'Release groups' : 'Words'} hint="One per line. /pattern/ for regex." error={errors.terms}>
			<textarea id="ot-terms" class="input mono" rows="4" spellcheck="false" autocomplete="off" bind:value={termsText} placeholder={match === 'group' ? 'HOLYARRS' : 'NLSubs'} aria-invalid={errors.terms ? 'true' : undefined} aria-describedby={errors.terms ? 'ot-terms-error' : 'ot-terms-hint'}></textarea>
		</Field>
		{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
	</form>
	{#snippet footer()}
		<Button variant="plain" onclick={() => (open = false)}>Cancel</Button>
		<Button variant="primary" type="submit" form="own-trait-form" {busy}>{editing ? 'Save' : 'Make trait'}</Button>
	{/snippet}
</Sheet>

<Sheet bind:open={removeOpen} title="Remove this trait?" description={removing?.name} size="sm">
	<form id="remove-own-trait" onsubmit={remove}>
		<p class="small">It's removed from every Pick rule too</p>
		{#if removeError}<p class="form-error" role="alert">{removeError}</p>{/if}
	</form>
	{#snippet footer()}
		<Button variant="plain" onclick={() => (removeOpen = false)}>Cancel</Button>
		<Button variant="destructive" type="submit" form="remove-own-trait" busy={removeBusy}>Remove</Button>
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
