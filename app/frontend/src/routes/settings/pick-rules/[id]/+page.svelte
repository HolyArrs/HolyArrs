<script lang="ts">
	import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import SaveBar from '#lib/ui/SaveBar.svelte';
	import Field from '#lib/ui/Field.svelte';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import TagInput from '#lib/ui/TagInput.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { addBlockedGroup, applySize, applyStepSizes, blankBody, deviceFamilies, checkBody, copyName, fieldKey, frenchVersions, isMusic, isOwnTrait, kindChoices, languages, maxPoints, neverChoices, orderChoices, preferChoices, properChoices, sizeFields, stepLabel, stepSizeRows, tidyBody, toggleKind, traitLabel, traitsFor, usedWords, type RulesKind, type SizeFields, type StepSizeRow } from '#lib/downloads/index.ts';
	import type { OwnTrait, PickRulesBody, PickRulesDetail, PickRulesUse, TrashProfile, TrashResult } from '#lib/downloads/index.ts';
	import { tick, untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api, ApiError } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { to } from '#lib/core/paths.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { session } from '#lib/core/session.svelte.ts';

	// One set of Pick rules: name and kinds, quality steps and Good enough, traits and points, Must have and Never, size, waiting and language. "new" starts a set; ?from=<id> starts it as a copy of another.

	interface Form {
		name: string;
		kinds: string[];
		body: PickRulesBody;
		size: SizeFields;
		steps: StepSizeRow[];
	}

	// People without the settings permission see the rules but cannot change them.
	const mayEdit = $derived(session.can('settings'));
	let form = $state<Form>({ name: '', kinds: ['movie', 'show'], body: blankBody('movie'), size: { min: '', max: '' }, steps: [] });
	let id = $state<number | null>(null);
	let preset = $state(false);
	let savedName = $state('');
	let inUse = $state<PickRulesUse | null>(null);
	// Your own traits, offered after the built-in ones.
	let mine = $state<OwnTrait[]>([]);
	let sizeStart = $state<SizeFields>({ min: '', max: '' });
	let stepsStart = $state<StepSizeRow[]>([]);
	let saved = $state('');
	let loaded = $state(false);
	let loadError = $state('');
	let gone = $state(false);
	let busy = $state(false);
	let tried = $state(false);
	let serverErrors = $state<Record<string, string>>({});
	let formError = $state('');
	// The address this page shows; after a new set is saved it moves to the set's own address.
	let shownKey = '';

	const key = $derived(`${page.params.id}|${page.url.searchParams.get('from') ?? ''}`);
	$effect(() => {
		const k = key;
		untrack(() => {
			if (k !== shownKey) void load();
		});
	});

	const music = $derived(isMusic(form.kinds));
	const ladderOn = $derived(form.body.steps.filter((s) => s.allowed));
	// Moves a quality step one place up (1, toward best) or down (-1); the list is stored low to high.
	function moveStep(quality: string, by: 1 | -1) {
		const steps = form.body.steps;
		const i = steps.findIndex((s) => s.quality === quality);
		const j = i + by;
		if (i < 0 || j < 0 || j >= steps.length) return;
		[steps[i], steps[j]] = [steps[j], steps[i]];
	}
	const own = $derived(traitsFor(form.kinds, mine));
	const checked = $derived(checkBody(form.name, form.kinds, tidyBody(form.body), mine));
	const label = (t: string) => traitLabel(t, mine);
	const errors = $derived<Record<string, string>>({ ...(tried ? checked : {}), ...serverErrors });
	// Allowed steps without a size of their own yet, offered to add one.
	const sizeChoices = $derived(ladderOn.filter((s) => !form.steps.some((r) => r.step === s.quality)));
	const dirty = $derived(loaded && !gone && !loadError && (id === null || JSON.stringify(form) !== saved));
	const kindWords = $derived(form.kinds.map((k) => kindChoices.find(([c]) => c === k)?.[1] ?? k).join(', '));

	function start(f: Form) {
		f.body.tags ??= [];
		form = f;
		sizeStart = { ...f.size };
		stepsStart = structuredClone(f.steps);
		saved = JSON.stringify(f);
		tried = false;
		serverErrors = {};
		formError = '';
	}

	async function load() {
		shownKey = key;
		const param = page.params.id ?? '';
		const from = page.url.searchParams.get('from');
		loaded = false;
		loadError = '';
		gone = false;
		try {
			mine = (await api.get<{ own_traits: OwnTrait[] }>('/own-traits')).own_traits;
			if (param === 'new') {
				id = null;
				preset = false;
				inUse = null;
				savedName = '';
				if (from) {
					const d = await api.get<PickRulesDetail>(`/pick-rules/${encodeURIComponent(from)}`);
					start({ name: copyName(d.name), kinds: [...d.kinds], body: structuredClone(d.body), ...sizesOf(d.body) });
				} else {
					start({ name: '', kinds: ['movie', 'show'], body: blankBody('movie'), ...sizesOf(blankBody('movie')) });
				}
			} else {
				const d = await api.get<PickRulesDetail>(`/pick-rules/${encodeURIComponent(param)}`);
				show(d);
			}
		} catch (e) {
			if (e instanceof ApiError && e.status === 404) gone = true;
			else loadError = message(e);
		} finally {
			loaded = true;
		}
	}

	function show(d: PickRulesDetail) {
		id = d.id;
		preset = d.preset;
		savedName = d.name;
		inUse = d.in_use;
		start({ name: d.name, kinds: [...d.kinds], body: structuredClone(d.body), ...sizesOf(d.body) });
	}

	function setKind(kind: RulesKind, on: boolean) {
		const next = toggleKind(form.kinds, kind, on);
		// Video and music use different steps and traits: switching starts them again.
		if (isMusic(next) !== isMusic(form.kinds) && next.length) {
			const body = blankBody(next[0] as RulesKind);
			form = { ...form, kinds: next, body, ...sizesOf(body) };
		} else {
			form.kinds = next;
		}
	}

	function addTrait() {
		const free = own.find((t) => !form.body.traits.some((x) => x.trait === t));
		if (free) form.body.traits = [...form.body.traits, { trait: free, points: 0 }];
	}

	/** Drops one trait and moves focus to the next row's trait, else to Add a trait. */
	async function removeTrait(i: number) {
		form.body.traits = form.body.traits.filter((_, j) => j !== i);
		await tick();
		(document.getElementById(`trait-${i}`) ?? document.getElementById('pr-add-trait'))?.focus();
	}

	/** Keeps focus on a row's trait after its key changes and the row is drawn again. */
	async function traitChanged(i: number) {
		await tick();
		document.getElementById(`trait-${i}`)?.focus();
	}

	// TRaSH Guides: pick a profile, apply it; its scores become synced traits with points here.
	let trashApp = $state<'sonarr' | 'radarr'>('sonarr');
	let trashProfiles = $state<TrashProfile[] | null>(null);
	let trashPick = $state('');
	let trashBusy = $state(false);
	let trashLoading = $state(false);
	let trashError = $state('');
	let trashResult = $state<TrashResult | null>(null);
	const trashChoice = $derived(trashProfiles?.find((p) => p.id === trashPick));
	const trashApps = $derived([
		{ value: 'sonarr' as const, label: t('settings.pickRuleEdit.trashShows') },
		{ value: 'radarr' as const, label: t('settings.pickRuleEdit.trashMovies') }
	]);

	async function loadTrash() {
		trashLoading = true;
		trashError = '';
		try {
			const app = trashApp;
			const list = (await api.get<{ profiles: TrashProfile[] }>(`/trash/profiles?app=${app}`)).profiles;
			if (app !== trashApp) return;
			trashProfiles = list;
			const synced = form.body.synced;
			trashPick = synced && synced.app === app && list.some((p) => p.id === synced.profile_id) ? synced.profile_id : (list[0]?.id ?? '');
		} catch (e) {
			trashError = message(e);
		} finally {
			trashLoading = false;
		}
	}

	async function applyTrash(app: 'sonarr' | 'radarr', profileId: string) {
		if (id === null) return;
		trashBusy = true;
		trashError = '';
		try {
			const r = await api.post<TrashResult>(`/pick-rules/${id}/trash`, { app, profile_id: profileId });
			mine = (await api.get<{ own_traits: OwnTrait[] }>('/own-traits')).own_traits;
			show(r.pick_rules);
			trashResult = r;
			toasts.push(t('settings.pickRuleEdit.trashApplied', { name: r.pick_rules.body.synced?.name ?? t('settings.pickRuleEdit.trashProfile') }), 'ok', { body: t('settings.pickRuleEdit.trashScored', { n: r.scored.length }) });
		} catch (e) {
			trashError = message(e);
		} finally {
			trashBusy = false;
		}
	}

	function addGroup(el: HTMLInputElement) {
		form.body.blocked_groups = addBlockedGroup(form.body.blocked_groups, el.value);
		el.value = '';
	}
	function addTo(list: 'must_have' | 'never', trait: string) {
		if (trait && !form.body[list].includes(trait)) form.body[list] = [...form.body[list], trait];
	}

	function toggleLanguage(code: string) {
		const w = form.body.language.wanted;
		form.body.language.wanted = w.includes(code) ? w.filter((l) => l !== code) : [...w, code];
	}

	function toggleDevice(v: string) {
		const w = form.body.devices ?? [];
		form.body.devices = w.includes(v) ? w.filter((x) => x !== v) : [...w, v];
	}
	function toggleFrench(v: string) {
		const w = form.body.language.french_versions ?? [];
		form.body.language.french_versions = w.includes(v) ? w.filter((x) => x !== v) : [...w, v];
	}

	function setSize(which: 'min' | 'max', v: string) {
		form.size[which] = v.trim() === '' ? '' : Number(v);
	}

	function sizesOf(body: PickRulesBody): { size: SizeFields; steps: StepSizeRow[] } {
		return { size: sizeFields(body.size_limits), steps: stepSizeRows(body.size_limits, body.steps.map((s) => s.quality)) };
	}

	function setStepSize(i: number, which: 'min' | 'preferred' | 'max', v: string) {
		form.steps[i][which] = v.trim() === '' ? '' : Number(v);
	}

	// A new step size starts from the Every step limit; the list keeps ladder order.
	function addStepSize(step: string) {
		if (!step) return;
		const ladder = form.body.steps.map((s) => s.quality);
		form.steps = [...form.steps, { step, min: form.size.min, preferred: '' as const, max: form.size.max }].sort((a, b) => ladder.indexOf(a.step) - ladder.indexOf(b.step));
	}

	// Each field that shows its own error line, and the element to focus for it. Any other server error shows under the form.
	const fieldIds: Record<string, string> = {
		name: 'pr-name',
		kinds: 'pr-kinds',
		steps: 'pr-steps',
		good_enough_at: 'pr-good-enough',
		good_enough_points: 'pr-good-enough-points',
		traits: 'pr-traits',
		must_have: 'pr-must-add',
		never: 'pr-never-add',
		size_limits: 'pr-size-min',
		prefer: 'pr-prefer',
		wait_minutes_torrent: 'pr-wait-torrent',
		wait_days: 'pr-lang-wait',
		wait_minutes_usenet: 'pr-wait-usenet',
		language: 'pr-languages',
		min_points: 'pr-min-points',
		upgrade_min_points: 'pr-upgrade',
		order: 'pr-order',
		proper_policy: 'pr-proper',
		max_age_days: 'pr-proper-age'
	};
	/** Marks a control invalid and points it at its error line (the Field's own "<id>-error"). */
	const bad = (k: string) => (errors[k] ? { 'aria-invalid': true, 'aria-describedby': `${fieldIds[k]}-error` } : {});

	function focusFirst(keys: string[]) {
		const target = keys.map((k) => fieldIds[k]).find(Boolean);
		const el = target ? document.getElementById(target) : null;
		const details = el?.closest('details');
		if (details && !details.open) details.open = true;
		el?.scrollIntoView({ block: 'center' });
		(el?.matches('input, select, button') ? el : el?.querySelector<HTMLElement>('input, select, button'))?.focus({ preventScroll: true });
	}

	async function save(e: SubmitEvent) {
		e.preventDefault();
		tried = true;
		serverErrors = {};
		formError = '';
		const body = applyStepSizes(applySize(tidyBody(form.body), form.size, sizeStart), form.steps, stepsStart);
		const problems = checkBody(form.name, form.kinds, body, mine);
		if (Object.keys(problems).length) {
			focusFirst(Object.keys(problems));
			return;
		}
		busy = true;
		try {
			const payload = { name: form.name.trim(), kinds: form.kinds, body };
			if (id === null) {
				const d = await api.post<PickRulesDetail>('/pick-rules', payload);
				show(d);
				toasts.push(t('settings.pickRuleEdit.made', { name: d.name }), 'ok');
				shownKey = `${d.id}|`;
				await goto(to(`/settings/pick-rules/${d.id}`), { replace: true, reset: false });
			} else {
				show(await api.put<PickRulesDetail>(`/pick-rules/${id}`, payload));
				toasts.push(t('settings.pickRuleEdit.saved'), 'ok');
			}
		} catch (err) {
			// A trait of yours removed meanwhile: show its chip as removed too.
			if (err instanceof ApiError && err.status === 400) mine = await api.get<{ own_traits: OwnTrait[] }>('/own-traits').then((r) => r.own_traits, () => mine);
			const field = err instanceof ApiError && typeof err.body.field === 'string' ? fieldKey(err.body.field) : '';
			if (field && field in fieldIds) {
				serverErrors = { [field]: message(err) };
				focusFirst([field]);
			} else {
				formError = message(err);
				if (err instanceof ApiError && err.status === 409 && err.body.in_use && typeof err.body.in_use === 'object') formError += ` ${usedWords(err.body.in_use as PickRulesUse)}.`;
			}
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>{id === null ? t('settings.pickRuleEdit.add') : savedName} · HolyArrs</title></svelte:head>

<PageHeader
	title={id === null ? t('settings.pickRuleEdit.add') : savedName || t('settings.pickRuleEdit.quality')}
	subtitle={id === null ? t('settings.pickRuleEdit.subtitle') : inUse ? usedWords(inUse) + '.' : undefined}
	back={{ href: to('/settings/pick-rules'), label: t('settings.pickRuleEdit.quality') }}
>
	{#snippet actions()}
		{#if id !== null && mayEdit}<Button icon="copy" href={to(`/settings/pick-rules/new?from=${id}`)}>{t('settings.pickRuleEdit.copy')}</Button>{/if}
	{/snippet}
</PageHeader>

<PageLoader loading={!loaded} label={t('settings.pickRuleEdit.loading')}>
{#if id === null && !mayEdit}
	<EmptyState icon="star" title={t('settings.pickRuleEdit.needsPermission')} text={t('settings.pickRuleEdit.needsPermissionText')} action={{ label: t('settings.pickRuleEdit.all'), href: to('/settings/pick-rules') }} />
{:else if gone}
	<EmptyState icon="star" title={t('settings.pickRuleEdit.notFound')} text={t('settings.pickRuleEdit.notFoundText')} action={{ label: t('settings.pickRuleEdit.all'), href: to('/settings/pick-rules') }} />
{:else if loadError}
	<LoadError hasData={false} message={loadError} onretry={load} />
{:else}
	{#if !mayEdit}<p class="hint" data-testid="read-only">{t('settings.pickRuleEdit.readOnly')}</p>{/if}
	<form class="sc-form" id="pick-rules-form" onsubmit={save} aria-label={t('settings.pickRuleEdit.quality')} novalidate>
	<fieldset class="group cards" disabled={!mayEdit}>
	<legend class="sr-only">{mayEdit ? t('settings.pickRuleEdit.quality') : t('settings.pickRuleEdit.qualityReadOnly')}</legend>
		<SettingsCard id="rules-name" title={t('settings.pickRuleEdit.nameKinds')}>
			{#if preset}
				<p class="ro" data-testid="preset-name"><b>{form.name}</b><span class="muted">{` · ${kindWords}`}</span></p>
				<p class="hint">{t('settings.pickRuleEdit.fixed')}</p>
			{:else}
				<Field id="pr-name" label={t('settings.pickRuleEdit.name')} error={errors.name}>
					<input id="pr-name" class="input" bind:value={form.name} maxlength="200" autocomplete="off" {...bad('name')} />
				</Field>
				<fieldset class="group" id="pr-kinds" tabindex="-1" aria-describedby={errors.kinds ? 'pr-kinds-error' : 'kinds-hint'}>
					<legend class="label">{t('settings.pickRuleEdit.for')}</legend>
					<div class="boxes">
						{#each kindChoices as [k, l] (k)}
							<label class="checkline box"><input type="checkbox" checked={form.kinds.includes(k)} onchange={(e) => setKind(k, e.currentTarget.checked)} /> <span>{l}</span></label>
						{/each}
					</div>
					<span class="hint" id="kinds-hint">{t('settings.pickRuleEdit.switching')}</span>
					{#if errors.kinds}<span class="field-error" id="pr-kinds-error" role="alert">{errors.kinds}</span>{/if}
				</fieldset>
			{/if}
		</SettingsCard>

		<SettingsCard id="rules-steps" title={t('settings.pickRuleEdit.steps')} description={t('settings.pickRuleEdit.stepsDesc')}>
			<fieldset class="group" id="pr-steps" tabindex="-1">
				<legend class="sr-only">{t('settings.pickRuleEdit.stepsLegend')}</legend>
				<ul class="steps" data-testid="steps">
					{#each [...form.body.steps].reverse() as s, i (s.quality)}
						<li>
							<label class="switch">
								<input type="checkbox" role="switch" bind:checked={s.allowed} />
								<span class="sw" aria-hidden="true"></span>
								<span class="step-name">{stepLabel(s.quality)}{#if s.allowed && s.quality === form.body.good_enough_at}<span class="chip ok">{t('settings.pickRuleEdit.goodEnough')}</span>{/if}</span>
							</label>
							{#if mayEdit}
								<span class="move">
									<IconButton icon="chevronUp" size="sm" label={t('downloads.act.up')} tooltip={false} disabled={i === 0} onclick={() => moveStep(s.quality, 1)} data-testid="step-up-{s.quality}" />
									<IconButton icon="chevronDown" size="sm" label={t('downloads.act.down')} tooltip={false} disabled={i === form.body.steps.length - 1} onclick={() => moveStep(s.quality, -1)} data-testid="step-down-{s.quality}" />
								</span>
							{/if}
						</li>
					{/each}
				</ul>
				{#if errors.steps}<span class="field-error" id="pr-steps-error" role="alert">{errors.steps}</span>{/if}
			</fieldset>
			<Field id="pr-good-enough" label={t('settings.pickRuleEdit.goodEnoughAt')} hint={t('settings.pickRuleEdit.goodEnoughHint')} error={errors.good_enough_at}>
				<select id="pr-good-enough" class="input" bind:value={form.body.good_enough_at} {...bad('good_enough_at')}>
					{#each [...ladderOn].reverse() as s (s.quality)}<option value={s.quality}>{stepLabel(s.quality)}</option>{/each}
				</select>
			</Field>
			<Field id="pr-good-enough-points" label={t('settings.pickRuleEdit.goodEnoughPoints')} hint={t('settings.pickRuleEdit.goodEnoughPointsHint')} error={errors.good_enough_points}>
				<input id="pr-good-enough-points" class="input" type="number" min={-maxPoints} max={maxPoints} step="1" bind:value={form.body.good_enough_points} {...bad('good_enough_points')} />
			</Field>
		</SettingsCard>

		{#snippet options(list: string[])}
			{#each list.filter((t) => !isOwnTrait(t)) as t (t)}<option value={t}>{label(t)}</option>{/each}
			{#if list.some(isOwnTrait)}<optgroup label={t('settings.pickRuleEdit.yourTraits')}>{#each list.filter(isOwnTrait) as t (t)}<option value={t}>{label(t)}</option>{/each}</optgroup>{/if}
		{/snippet}
		<SettingsCard id="rules-traits" title={t('settings.pickRuleEdit.traits')} description={t('settings.pickRuleEdit.traitsDesc')}>
			{#if form.body.traits.length === 0}
				<p class="hint">{t('settings.pickRuleEdit.noTraits')}</p>
			{:else}
				<ul class="traits" id="pr-traits" data-testid="traits">
					{#each form.body.traits as tr, i (tr.trait)}
						<li>
							<label class="sr-only" for="trait-{i}">{t('settings.pickRuleEdit.traitN', { n: i + 1 })}</label>
							<select id="trait-{i}" class="input" bind:value={tr.trait} onchange={() => traitChanged(i)}>
								{@render options(own.filter((x) => x === tr.trait || !form.body.traits.some((y) => y.trait === x)))}
							</select>
							<label class="sr-only" for="points-{i}">{t('settings.pickRuleEdit.pointsFor', { trait: label(tr.trait) })}</label>
							<input id="points-{i}" class="input points" type="number" min={-maxPoints} max={maxPoints} step="1" bind:value={tr.points} />
							<IconButton icon="trash" label={t('settings.pickRuleEdit.removeTrait', { trait: label(tr.trait) })} tooltip={false} onclick={() => removeTrait(i)} />
						</li>
					{/each}
				</ul>
			{/if}
			{#if errors.traits}<span class="field-error" id="pr-traits-error" role="alert">{errors.traits}</span>{/if}
			<div class="trait-actions">
				<Button id="pr-add-trait" icon="plus" onclick={addTrait} disabled={form.body.traits.length >= own.length}>{t('settings.pickRuleEdit.addTrait')}</Button>
				<Button variant="secondary" href={to('/settings/pick-rules#own-traits')}>{t('settings.pickRuleEdit.ownTrait')}</Button>
			</div>
			{#snippet advanced()}
				<Field id="pr-order" label={t('settings.pickRuleEdit.whichWins')} hint={orderChoices.find(([v]) => v === form.body.order)?.[2]} error={errors.order}>
					<select id="pr-order" class="input" bind:value={form.body.order}>
						{#each orderChoices as [v, l] (v)}<option value={v}>{l}</option>{/each}
					</select>
				</Field>
				<Field id="pr-upgrade" label={t('settings.pickRuleEdit.upgrade')} hint={t('settings.pickRuleEdit.upgradeHint')} error={errors.upgrade_min_points}>
					<input id="pr-upgrade" class="input" type="number" min={-maxPoints} max={maxPoints} step="1" bind:value={form.body.upgrade_min_points} {...bad('upgrade_min_points')} />
				</Field>
				<Field id="pr-min-points" label={t('settings.pickRuleEdit.minPoints')} error={errors.min_points}>
					<input id="pr-min-points" class="input" type="number" min={-maxPoints} max={maxPoints} step="1" bind:value={form.body.min_points} {...bad('min_points')} />
				</Field>
				<Field id="pr-proper" label={t('settings.pickRuleEdit.fixedCopies')} hint={properChoices.find(([v]) => v === form.body.proper_policy.mode)?.[2]} error={errors.proper_policy}>
					<select id="pr-proper" class="input" bind:value={form.body.proper_policy.mode}>
						{#each properChoices as [v, l] (v)}<option value={v}>{l}</option>{/each}
					</select>
				</Field>
				{#if form.body.proper_policy.mode === 'take_any' || form.body.proper_policy.mode === 'same_group_only'}
					<Field id="pr-proper-age" label={t('settings.pickRuleEdit.properAge')} hint={t('settings.pickRuleEdit.properAgeHint')} error={errors.max_age_days}>
						<input id="pr-proper-age" class="input" type="number" min="0" max="36500" step="1" bind:value={form.body.proper_policy.max_age_days} placeholder="0" {...bad('max_age_days')} />
					</Field>
				{/if}
			{/snippet}
		</SettingsCard>

		{#if !music && id !== null}
			<SettingsCard id="rules-trash" title={t('settings.pickRuleEdit.trashTitle')} description={t('settings.pickRuleEdit.trashDesc')}>
				{#if form.body.synced}
					<div class="synced-row" data-testid="trash-synced">
						<span class="grow">
							<b>{form.body.synced.name}</b>
							<span class="xs muted">{t('settings.pickRuleEdit.trashSynced', { app: form.body.synced.app === 'radarr' ? t('settings.pickRuleEdit.trashMovies') : t('settings.pickRuleEdit.trashShows'), when: new Date(form.body.synced.at).toLocaleString(appLocale()) })}</span>
						</span>
						{#if mayEdit}<Button icon="refresh" busy={trashBusy} disabled={dirty} onclick={() => form.body.synced && applyTrash(form.body.synced.app, form.body.synced.profile_id)} data-testid="trash-resync">{t('settings.pickRuleEdit.trashSyncAgain')}</Button>{/if}
					</div>
				{/if}
				{#if mayEdit}
					<div class="field">
						<span class="label" id="trash-app-label">{t('settings.pickRuleEdit.trashFor')}</span>
						<SegmentedControl options={trashApps} bind:value={trashApp} label={t('settings.pickRuleEdit.trashFor')} testid="trash-app" onchange={() => ((trashProfiles = null), void loadTrash())} />
					</div>
					{#if trashProfiles === null}
						<div><Button busy={trashLoading} onclick={loadTrash} data-testid="trash-load">{t('settings.pickRuleEdit.trashShow')}</Button></div>
					{:else if trashProfiles.length === 0}
						<p class="hint">{t('settings.pickRuleEdit.trashNone')}</p>
					{:else}
						<Field id="trash-profile" label={t('settings.pickRuleEdit.trashProfile')} hint={trashChoice?.description}>
							<select id="trash-profile" class="input" bind:value={trashPick} data-testid="trash-profile">
								{#each trashProfiles as p (p.id)}<option value={p.id}>{p.name}</option>{/each}
							</select>
						</Field>
						<p class="hint">{t('settings.pickRuleEdit.trashReplaces')}</p>
						<div><Button variant="primary" busy={trashBusy} disabled={dirty || !trashPick} onclick={() => applyTrash(trashApp, trashPick)} data-testid="trash-apply">{t('settings.pickRuleEdit.trashApply')}</Button></div>
						{#if dirty}<p class="hint">{t('settings.pickRuleEdit.trashSaveFirst')}</p>{/if}
					{/if}
				{/if}
				{#if trashError}<p class="form-error" role="alert">{trashError}</p>{/if}
				{#if trashResult}
					<div class="trash-result" data-testid="trash-result">
						<p class="small">{t('settings.pickRuleEdit.trashResult', { n: trashResult.scored.length, added: trashResult.added, updated: trashResult.updated })}</p>
						{#if trashResult.skipped.length}
							<details>
								<summary class="small">{t('settings.pickRuleEdit.trashLeftOut', { n: trashResult.skipped.length })}</summary>
								<ul class="skipped" data-testid="trash-skipped">
									{#each trashResult.skipped as s (s.name)}
										<li class="xs"><b>{s.name}</b> <span class="muted">{s.kinds?.length ? t('settings.pickRuleEdit.trashTests', { kinds: s.kinds.map((k) => serverWords(k)).join(', ') }) : serverWords(s.reason)}</span></li>
									{/each}
								</ul>
							</details>
						{/if}
					</div>
				{/if}
			</SettingsCard>
		{/if}

		<SettingsCard id="rules-must-never" title={t('settings.pickRuleEdit.mustNever')}>
			<div class="field">
				<span class="label" id="must-label">{t('settings.pickRuleEdit.mustHave')}</span>
				<div class="chips" role="group" aria-labelledby="must-label" data-testid="must-have">
					{#each form.body.must_have as t (t)}<Chip label={label(t)} onremove={() => (form.body.must_have = form.body.must_have.filter((x) => x !== t))} />{:else}<span class="hint">{t('settings.pickRuleEdit.nothingYet')}</span>{/each}
				</div>
				<label class="sr-only" for="pr-must-add">{t('settings.pickRuleEdit.addMust')}</label>
				<select id="pr-must-add" class="input" value="" onchange={(e) => (addTo('must_have', e.currentTarget.value), (e.currentTarget.value = ''))} {...bad('must_have')}>
					<option value="">{t('settings.pickRuleEdit.addMust')}</option>
					{@render options(own.filter((t) => !form.body.must_have.includes(t)))}
				</select>
				{#if errors.must_have}<span class="field-error" id="pr-must-add-error" role="alert">{errors.must_have}</span>{/if}
			</div>
			<div class="field">
				<span class="label" id="never-label">{t('settings.pickRuleEdit.never')}</span>
				<div class="chips" role="group" aria-labelledby="never-label" data-testid="never">
					{#each form.body.never as t (t)}<Chip label={label(t)} onremove={() => (form.body.never = form.body.never.filter((x) => x !== t))} />{:else}<span class="hint">{t('settings.pickRuleEdit.nothingYet')}</span>{/each}
				</div>
				<label class="sr-only" for="pr-never-add">{t('settings.pickRuleEdit.addNever')}</label>
				<select id="pr-never-add" class="input" value="" onchange={(e) => (addTo('never', e.currentTarget.value), (e.currentTarget.value = ''))} {...bad('never')}>
					<option value="">{t('settings.pickRuleEdit.addNever')}</option>
					{@render options(neverChoices(form.kinds, mine).filter((t) => !form.body.never.includes(t)))}
				</select>
				{#if errors.never}<span class="field-error" id="pr-never-add-error" role="alert">{errors.never}</span>{/if}
			</div>
			<div class="field">
				<span class="label" id="groups-label">{t('settings.pickRuleEdit.blockedGroups')}</span>
				<div class="chips" role="group" aria-labelledby="groups-label" data-testid="blocked-groups">
					{#each form.body.blocked_groups ?? [] as g (g)}<Chip label={g} onremove={() => (form.body.blocked_groups = (form.body.blocked_groups ?? []).filter((x) => x !== g))} />{:else}<span class="hint">{t('settings.pickRuleEdit.nothingYet')}</span>{/each}
				</div>
				<label class="sr-only" for="pr-group-add">{t('settings.pickRuleEdit.addBlockedGroup')}</label>
				<input id="pr-group-add" class="input" type="text" autocomplete="off" spellcheck="false" maxlength="200" placeholder={t('settings.pickRuleEdit.addBlockedGroup')} aria-describedby="pr-group-hint" onkeydown={(e) => { if (e.key === 'Enter' && !e.isComposing) { e.preventDefault(); addGroup(e.currentTarget); } }} onblur={(e) => addGroup(e.currentTarget)} data-testid="blocked-groups-input" />
				<span class="hint" id="pr-group-hint">{t('settings.pickRuleEdit.blockedGroupsHint')}</span>
			</div>
			{#if !music}
				<div class="field">
					<span class="label" id="devices-label">{t('settings.pickRuleEdit.devices')}</span>
					<div class="chips" role="group" id="pr-devices" aria-labelledby="devices-label" aria-describedby="devices-hint" data-testid="devices">
						{#each deviceFamilies as d (d)}<Chip label={t(`settings.pickRuleEdit.device.${d}`)} selected={form.body.devices?.includes(d) ?? false} onclick={() => toggleDevice(d)} />{/each}
					</div>
					<span class="hint" id="devices-hint">{t('settings.pickRuleEdit.devicesHint')}</span>
				</div>
			{/if}
		</SettingsCard>

		<SettingsCard id="rules-size" title={t('settings.pickRuleEdit.size')} description={music ? t('settings.pickRuleEdit.sizeMusic') : t('settings.pickRuleEdit.sizeVideo')}>
			<div class="pair">
				<Field id="pr-size-min" label={t('settings.pickRuleEdit.smallest')} error={errors.size_limits}>
					<input id="pr-size-min" class="input" type="number" min="0" step="0.01" inputmode="decimal" placeholder={t('settings.pickRuleEdit.noLimit')} value={form.size.min} oninput={(e) => setSize('min', e.currentTarget.value)} {...bad('size_limits')} />
				</Field>
				<Field id="pr-size-max" label={t('settings.pickRuleEdit.largest')}>
					<input id="pr-size-max" class="input" type="number" min="0" step="0.01" inputmode="decimal" placeholder={t('settings.pickRuleEdit.noLimit')} value={form.size.max} oninput={(e) => setSize('max', e.currentTarget.value)} />
				</Field>
			</div>
			<p class="hint">{t('settings.pickRuleEdit.sizeNote')}</p>
			{#if form.steps.length}
				<p class="hint">{t('settings.pickRuleEdit.stepSizesNote')}</p>
				<ul class="traits step-sizes" data-testid="own-sizes">
					{#each form.steps as r, i (r.step)}
						<li>
							<span class="step-size-name">{stepLabel(r.step)}</span>
							<label class="sr-only" for="size-min-{i}">{stepLabel(r.step)}: {t('settings.pickRuleEdit.smallest')}</label>
							<input id="size-min-{i}" class="input points" type="number" min="0" step="0.01" inputmode="decimal" placeholder={t('settings.pickRuleEdit.noLimit')} value={r.min} oninput={(e) => setStepSize(i, 'min', e.currentTarget.value)} />
							<label class="sr-only" for="size-pref-{i}">{stepLabel(r.step)}: {t('settings.pickRuleEdit.preferred')}</label>
							<input id="size-pref-{i}" class="input points" type="number" min="0" step="0.01" inputmode="decimal" placeholder={t('settings.pickRuleEdit.noPreferred')} value={r.preferred} oninput={(e) => setStepSize(i, 'preferred', e.currentTarget.value)} />
							<label class="sr-only" for="size-max-{i}">{stepLabel(r.step)}: {t('settings.pickRuleEdit.largest')}</label>
							<input id="size-max-{i}" class="input points" type="number" min="0" step="0.01" inputmode="decimal" placeholder={t('settings.pickRuleEdit.noLimit')} value={r.max} oninput={(e) => setStepSize(i, 'max', e.currentTarget.value)} />
							<IconButton icon="trash" label={t('settings.pickRuleEdit.removeStepSize', { step: stepLabel(r.step) })} tooltip={false} onclick={() => (form.steps = form.steps.filter((x) => x.step !== r.step))} />
						</li>
					{/each}
				</ul>
			{/if}
			{#if sizeChoices.length}
				<div class="trait-actions">
					<label class="sr-only" for="pr-size-add">{t('settings.pickRuleEdit.addStepSize')}</label>
					<select id="pr-size-add" class="input" value="" onchange={(e) => (addStepSize(e.currentTarget.value), (e.currentTarget.value = ''))}>
						<option value="">{t('settings.pickRuleEdit.addStepSize')}</option>
						{#each sizeChoices as s (s.quality)}<option value={s.quality}>{stepLabel(s.quality)}</option>{/each}
					</select>
				</div>
			{/if}
			<label class="checkline">
				<input id="pr-replace-small" type="checkbox" bind:checked={form.body.replace_under_min} />
				<span><b>{t('settings.pickRuleEdit.replaceSmall')}</b><br /><span class="hint">{t('settings.pickRuleEdit.replaceSmallHint')}</span></span>
			</label>
		</SettingsCard>

		<SettingsCard id="rules-wait" title={t('settings.pickRuleEdit.waiting')} description={t('settings.pickRuleEdit.waitingDesc')}>
			<div class="field" id="pr-prefer">
				<span class="label">{t('settings.pickRuleEdit.prefer')}</span>
				<div><SegmentedControl label={t('settings.pickRuleEdit.prefer')} options={preferChoices} bind:value={form.body.protocol.prefer} /></div>
				<span class="hint">{t('settings.pickRuleEdit.winsTie')}</span>
			</div>
			<div class="pair">
				<Field id="pr-wait-torrent" label={t('settings.pickRuleEdit.waitTorrent')} error={errors.wait_minutes_torrent}>
					<input id="pr-wait-torrent" class="input" type="number" min="0" max="10080" step="1" bind:value={form.body.protocol.wait_minutes_torrent} {...bad('wait_minutes_torrent')} />
				</Field>
				<Field id="pr-wait-usenet" label={t('settings.pickRuleEdit.waitUsenet')} error={errors.wait_minutes_usenet}>
					<input id="pr-wait-usenet" class="input" type="number" min="0" max="10080" step="1" bind:value={form.body.protocol.wait_minutes_usenet} {...bad('wait_minutes_usenet')} />
				</Field>
			</div>
			<label class="checkline">
				<input id="pr-skip-wait" type="checkbox" bind:checked={form.body.protocol.skip_wait_at_good_enough} />
				<span><b>{t('settings.pickRuleEdit.skipWait')}</b></span>
			</label>
		</SettingsCard>

		{#if !music}
			<SettingsCard id="rules-sources" title={t('settings.pickRuleEdit.sourcesTitle')} description={t('settings.pickRuleEdit.sourcesDesc')}>
				<Field id="pr-tags" label={t('settings.pickRuleEdit.tags')}>
					<TagInput id="pr-tags" bind:value={form.body.tags} testid="pr-tags" />
				</Field>
			</SettingsCard>
		{/if}

		<SettingsCard id="rules-language" title={t('settings.pickRuleEdit.language')} description={t('settings.pickRuleEdit.languageDesc')}>
			<div class="field">
				<span class="label" id="lang-label">{t('settings.pickRuleEdit.languages')}</span>
				<div class="chips" role="group" id="pr-languages" aria-labelledby="lang-label">
					{#each languages as [code, l] (code)}<Chip label={l} selected={form.body.language.wanted.includes(code)} onclick={() => toggleLanguage(code)} />{/each}
				</div>
				{#if errors.language}<span class="field-error" id="pr-languages-error" role="alert">{errors.language}</span>{/if}
			</div>
			{#if form.body.language.wanted.includes('fr')}
				<div class="field">
					<span class="label" id="fr-version-label">{t('settings.pickRuleEdit.frenchVersions')}</span>
					<div class="chips" role="group" id="pr-french-versions" aria-labelledby="fr-version-label" aria-describedby="fr-version-hint">
						{#each frenchVersions as fv (fv)}<Chip label={fv} selected={form.body.language.french_versions?.includes(fv) ?? false} onclick={() => toggleFrench(fv)} />{/each}
					</div>
					<span class="hint" id="fr-version-hint">{t('settings.pickRuleEdit.frenchVersionsHint')}</span>
				</div>
			{/if}
			<label class="checkline">
				<input id="pr-original" type="checkbox" bind:checked={form.body.language.original_ok} />
				<span><b>{t('settings.pickRuleEdit.original')}</b><br /><span class="hint">{t('settings.pickRuleEdit.originalHint')}</span></span>
			</label>
			{#if form.body.language.original_ok && form.body.language.wanted.length}
				<Field id="pr-lang-wait" label={t('settings.pickRuleEdit.langWait')} hint={t('settings.pickRuleEdit.langWaitHint')} error={errors.wait_days}>
					<input id="pr-lang-wait" class="input" type="number" min="0" max="90" step="1" inputmode="numeric" placeholder="0" bind:value={form.body.language.wait_days} {...bad('wait_days')} />
				</Field>
			{/if}
		</SettingsCard>

	</fieldset>
		{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
	</form>
	{#if mayEdit}<SaveBar {dirty} form="pick-rules-form" saving={busy} ondiscard={() => start(JSON.parse(saved))} />{/if}
{/if}
</PageLoader>

<style>
	/* The fieldset only turns the form read only; it adds no box of its own. */
	.all { display: contents; border: 0; padding: 0; margin: 0; }
	.synced-row { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
	.synced-row .grow { display: flex; flex-direction: column; min-width: 0; flex: 1; }
	.skipped { margin: 8px 0 0; padding-left: 18px; display: flex; flex-direction: column; gap: 4px; }
	.loading { display: flex; flex-direction: column; gap: 16px; }
	.ro { font: var(--t-body); }
	.group { border: 0; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 6px; }
	.group:focus { outline: none; }
	.cards { min-width: 0; gap: 12px; }
	.boxes { display: flex; flex-wrap: wrap; gap: 4px 20px; }
	.box { align-items: center; padding: 0; }
	.box input { margin-top: 0; }
	.steps { display: flex; flex-direction: column; }
	.steps li { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
	.steps li + li { border-top: 1px solid var(--line); }
	.switch { flex: 1; min-width: 0; font: var(--t-callout); }
	.move { display: flex; gap: 2px; flex: none; }
	.traits { display: flex; flex-direction: column; gap: 8px; }
	.traits li { display: flex; align-items: center; gap: 8px; }
	.traits select { flex: 1; min-width: 0; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
	/* One control height per row: select, points and the remove button. */
	.traits .input { height: 32px; min-height: 32px; padding-block: 0; }
	.points { width: 140px; flex: none; }
	.step-name { display: inline-flex; align-items: center; flex-wrap: wrap; gap: 8px; min-width: 0; }
	.trait-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; padding-block: 12px; }
	/* Number boxes fill the same control column as the pickers above them. */
	.cards :global(.field > input.input[type='number']) { max-width: none; }
	.pair { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 12px; }
	.pair :global(.field + .field) { margin-top: 0; }
	.step-sizes { margin-top: 12px; }
	.step-sizes .points { width: 110px; min-width: 0; }
	.step-size-name { flex: 1; min-width: 0; font: var(--t-callout); }
	.chips { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; min-height: 32px; }
	.field-error { color: var(--fail-text); font: var(--t-footnote); }
	/* Phones: points stay on the trait's row, narrower. */
	@media (max-width: 600px) {
		.traits .input { height: 40px; min-height: 40px; }
		/* The trait gets the whole first line so its name is never cut; points and remove sit below. */
		.traits li { flex-wrap: wrap; }
		.traits select, .step-size-name { flex: 1 1 100%; }
		.points, .step-sizes .points { flex: 1; width: auto; }
	}
	@media (pointer: coarse) { .traits .input { height: 44px; min-height: 44px; } }
</style>
