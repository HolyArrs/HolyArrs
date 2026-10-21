<script lang="ts">
	import PageLoader from '#lib/ui/PageLoader.svelte';
	// One set of Pick rules: name and kinds, quality steps and Good enough,
	// traits and points, Must have and Never, size, waiting and language.
	// "new" starts a set; ?from=<id> starts it as a copy of another.
	import { tick, untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api, ApiError, message } from '#lib/api.ts';
	import { to } from '#lib/paths.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import {
		applySize,
		blankBody,
		checkBody,
		copyName,
		fieldKey,
		isMusic,
		kindChoices,
		languages,
		neverChoices,
		orderChoices,
		preferChoices,
		properChoices,
		sizeFields,
		stepLabel,
		tidyBody,
		toggleKind,
		traitLabel,
		traitsFor,
		usedWords,
		type RulesKind,
		type SizeFields
	} from '#lib/pick-rules.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import SaveBar from '#lib/ui/SaveBar.svelte';
	import Field from '#lib/ui/Field.svelte';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import type { PickRulesBody, PickRulesDetail, PickRulesUse } from '#lib/types.ts';

	interface Form {
		name: string;
		kinds: string[];
		body: PickRulesBody;
		size: SizeFields;
	}

	// People without the settings permission see the rules but cannot change them.
	const mayEdit = $derived(session.can('settings'));
	let form = $state<Form>({ name: '', kinds: ['movie', 'show'], body: blankBody('movie'), size: { min: '', max: '' } });
	let id = $state<number | null>(null);
	let preset = $state(false);
	let savedName = $state('');
	let inUse = $state<PickRulesUse | null>(null);
	let sizeStart = $state<SizeFields>({ min: '', max: '' });
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
	const own = $derived(traitsFor(form.kinds));
	const checked = $derived(checkBody(form.name, form.kinds, tidyBody(form.body)));
	const errors = $derived<Record<string, string>>({ ...(tried ? checked : {}), ...serverErrors });
	const otherSizes = $derived(Object.keys(form.body.size_limits).filter((k) => k !== '*').length);
	const dirty = $derived(loaded && !gone && !loadError && (id === null || JSON.stringify(form) !== saved));
	const kindWords = $derived(form.kinds.map((k) => kindChoices.find(([c]) => c === k)?.[1] ?? k).join(', '));

	function start(f: Form) {
		form = f;
		sizeStart = { ...f.size };
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
			if (param === 'new') {
				id = null;
				preset = false;
				inUse = null;
				savedName = '';
				if (from) {
					const d = await api.get<PickRulesDetail>(`/pick-rules/${encodeURIComponent(from)}`);
					start({ name: copyName(d.name), kinds: [...d.kinds], body: structuredClone(d.body), size: sizeFields(d.body.size_limits) });
				} else {
					start({ name: '', kinds: ['movie', 'show'], body: blankBody('movie'), size: sizeFields(blankBody('movie').size_limits) });
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
		start({ name: d.name, kinds: [...d.kinds], body: structuredClone(d.body), size: sizeFields(d.body.size_limits) });
	}

	function setKind(kind: RulesKind, on: boolean) {
		const next = toggleKind(form.kinds, kind, on);
		// Video and music use different steps and traits: switching starts them again.
		if (isMusic(next) !== isMusic(form.kinds) && next.length) {
			const body = blankBody(next[0] as RulesKind);
			form = { ...form, kinds: next, body, size: sizeFields(body.size_limits) };
		} else {
			form.kinds = next;
		}
	}

	function addTrait() {
		const free = own.find((t) => !form.body.traits.some((x) => x.trait === t));
		if (free) form.body.traits = [...form.body.traits, { trait: free, points: 0 }];
	}

	/** removeTrait drops one trait and moves focus to the next row's trait, else to Add a trait. */
	async function removeTrait(i: number) {
		form.body.traits = form.body.traits.filter((_, j) => j !== i);
		await tick();
		(document.getElementById(`trait-${i}`) ?? document.getElementById('pr-add-trait'))?.focus();
	}

	/** traitChanged keeps focus on a row's trait after its key changes and the row is drawn again. */
	async function traitChanged(i: number) {
		await tick();
		document.getElementById(`trait-${i}`)?.focus();
	}

	function addTo(list: 'must_have' | 'never', trait: string) {
		if (trait && !form.body[list].includes(trait)) form.body[list] = [...form.body[list], trait];
	}

	function toggleLanguage(code: string) {
		const w = form.body.language.wanted;
		form.body.language.wanted = w.includes(code) ? w.filter((l) => l !== code) : [...w, code];
	}

	function setSize(which: 'min' | 'max', v: string) {
		form.size[which] = v.trim() === '' ? '' : Number(v);
	}

	// Each field that shows its own error line, and the element to focus for it.
	// Any other server error shows under the form.
	const fieldIds: Record<string, string> = {
		name: 'pr-name',
		kinds: 'pr-kinds',
		steps: 'pr-steps',
		good_enough_at: 'pr-good-enough',
		traits: 'pr-traits',
		must_have: 'pr-must-add',
		never: 'pr-never-add',
		size_limits: 'pr-size-min',
		prefer: 'pr-prefer',
		wait_minutes_torrent: 'pr-wait-torrent',
		wait_minutes_usenet: 'pr-wait-usenet',
		language: 'pr-languages',
		min_points: 'pr-min-points',
		upgrade_min_points: 'pr-upgrade',
		order: 'pr-order',
		proper_policy: 'pr-proper',
		max_age_days: 'pr-proper-age'
	};
	/** bad marks a control invalid and points it at its error line (the Field's own "<id>-error"). */
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
		const body = applySize(tidyBody(form.body), form.size, sizeStart);
		const problems = checkBody(form.name, form.kinds, body);
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
				toasts.push(`${d.name} made.`, 'ok');
				shownKey = `${d.id}|`;
				await goto(to(`/settings/pick-rules/${d.id}`), { replace: true, reset: false });
			} else {
				show(await api.put<PickRulesDetail>(`/pick-rules/${id}`, payload));
				toasts.push('Pick rules saved.', 'ok');
			}
		} catch (err) {
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

<svelte:head><title>{id === null ? 'New Pick rules' : savedName} · HolyArrs</title></svelte:head>

<PageHeader
	title={id === null ? 'New Pick rules' : savedName || 'Pick rules'}
	subtitle={id === null ? 'Choose the steps, traits, sizes and waits a new set takes.' : inUse ? usedWords(inUse) + '.' : undefined}
	back={{ href: to('/settings/pick-rules'), label: 'Pick rules' }}
>
	{#snippet actions()}
		{#if id !== null && mayEdit}<Button icon="copy" href={to(`/settings/pick-rules/new?from=${id}`)}>Make a copy</Button>{/if}
	{/snippet}
</PageHeader>

<PageLoader loading={!loaded} label="Loading the rule">
{#if id === null && !mayEdit}
	<EmptyState icon="star" title="Making Pick rules takes the settings permission" text="You can see the Pick rules there are." action={{ label: 'All Pick rules', href: to('/settings/pick-rules') }} />
{:else if gone}
	<EmptyState icon="star" title="These Pick rules are gone" text="They may have been removed on another device." action={{ label: 'All Pick rules', href: to('/settings/pick-rules') }} />
{:else if loadError}
	<LoadError hasData={false} message={loadError} onretry={load} />
{:else}
	{#if !mayEdit}<p class="hint" data-testid="read-only">You can see these Pick rules. Changing them takes the settings permission.</p>{/if}
	<form class="sc-form" id="pick-rules-form" onsubmit={save} aria-label="Pick rules" novalidate>
	<fieldset class="group" disabled={!mayEdit}>
	<legend class="sr-only">{mayEdit ? 'Pick rules' : 'Pick rules, read only'}</legend>
		<SettingsCard id="rules-name" title="Name and kinds" description="What these rules are called and what they are for.">
			{#if preset}
				<p class="ro" data-testid="preset-name"><b>{form.name}</b><span class="muted">{` · ${kindWords}`}</span></p>
				<p class="hint">Ready-made Pick rules keep their name and kinds, because HolyArrs finds them by name. What they take can change. Make a copy to start your own.</p>
			{:else}
				<Field id="pr-name" label="Name" error={errors.name}>
					<input id="pr-name" class="input" bind:value={form.name} maxlength="200" autocomplete="off" {...bad('name')} />
				</Field>
				<fieldset class="group" id="pr-kinds" tabindex="-1" aria-describedby={errors.kinds ? 'pr-kinds-error' : 'kinds-hint'}>
					<legend class="label">For</legend>
					<div class="boxes">
						{#each kindChoices as [k, l] (k)}
							<label class="checkline box"><input type="checkbox" checked={form.kinds.includes(k)} onchange={(e) => setKind(k, e.currentTarget.checked)} /> <span>{l}</span></label>
						{/each}
					</div>
					<span class="hint" id="kinds-hint">Music Pick rules are for music alone. Switching between video and music starts the steps and traits again.</span>
					{#if errors.kinds}<span class="field-error" id="pr-kinds-error" role="alert">{errors.kinds}</span>{/if}
				</fieldset>
			{/if}
		</SettingsCard>

		<SettingsCard id="rules-steps" title="Quality steps" description="Which steps HolyArrs may take, best at the top. It keeps looking for a better copy until a file reaches Good enough.">
			<fieldset class="group" id="pr-steps" tabindex="-1">
				<legend class="sr-only">Steps HolyArrs may take</legend>
				<ul class="steps" data-testid="steps">
					{#each [...form.body.steps].reverse() as s (s.quality)}
						<li>
							<label class="switch">
								<input type="checkbox" role="switch" bind:checked={s.allowed} />
								<span class="sw" aria-hidden="true"></span>
								<span>{stepLabel(s.quality)}</span>
							</label>
							{#if s.allowed && s.quality === form.body.good_enough_at}<span class="chip ok">Good enough</span>{/if}
						</li>
					{/each}
				</ul>
				{#if errors.steps}<span class="field-error" id="pr-steps-error" role="alert">{errors.steps}</span>{/if}
			</fieldset>
			<Field id="pr-good-enough" label="Good enough at" hint="HolyArrs stops looking for a better copy once a file reaches this step." error={errors.good_enough_at}>
				<select id="pr-good-enough" class="input" bind:value={form.body.good_enough_at} {...bad('good_enough_at')}>
					{#each [...ladderOn].reverse() as s (s.quality)}<option value={s.quality}>{stepLabel(s.quality)}</option>{/each}
				</select>
			</Field>
		</SettingsCard>

		<SettingsCard id="rules-traits" title="Traits and points" description="Traits a Release name shows earn or lose points. More points win between Releases on the same step.">
			{#if form.body.traits.length === 0}
				<p class="hint">No traits yet: every Release on a step counts the same.</p>
			{:else}
				<ul class="traits" id="pr-traits" data-testid="traits">
					{#each form.body.traits as t, i (t.trait)}
						<li>
							<label class="sr-only" for="trait-{i}">Trait {i + 1}</label>
							<select id="trait-{i}" class="input" bind:value={t.trait} onchange={() => traitChanged(i)}>
								{#each own.filter((x) => x === t.trait || !form.body.traits.some((y) => y.trait === x)) as x (x)}<option value={x}>{traitLabel(x)}</option>{/each}
							</select>
							<label class="sr-only" for="points-{i}">Points for {traitLabel(t.trait)}</label>
							<input id="points-{i}" class="input points" type="number" min="-10000" max="10000" step="1" bind:value={t.points} />
							<IconButton icon="trash" label="Remove {traitLabel(t.trait)}" tooltip={false} onclick={() => removeTrait(i)} />
						</li>
					{/each}
				</ul>
			{/if}
			{#if errors.traits}<span class="field-error" id="pr-traits-error" role="alert">{errors.traits}</span>{/if}
			<div><Button id="pr-add-trait" icon="plus" onclick={addTrait} disabled={form.body.traits.length >= own.length}>Add a trait</Button></div>
			{#snippet advanced()}
				<Field id="pr-order" label="Which wins" hint={orderChoices.find(([v]) => v === form.body.order)?.[2]} error={errors.order}>
					<select id="pr-order" class="input" bind:value={form.body.order}>
						{#each orderChoices as [v, l] (v)}<option value={v}>{l}</option>{/each}
					</select>
				</Field>
				<Field id="pr-upgrade" label="Points a better copy must add" hint="A Release on the same step replaces the file only with at least this many more points." error={errors.upgrade_min_points}>
					<input id="pr-upgrade" class="input narrow" type="number" min="-10000" max="10000" step="1" bind:value={form.body.upgrade_min_points} {...bad('upgrade_min_points')} />
				</Field>
				<Field id="pr-min-points" label="Fewest points a Release needs" hint="A Release with fewer points is passed over." error={errors.min_points}>
					<input id="pr-min-points" class="input narrow" type="number" min="-10000" max="10000" step="1" bind:value={form.body.min_points} {...bad('min_points')} />
				</Field>
				<Field id="pr-proper" label="Fixed copies" hint={properChoices.find(([v]) => v === form.body.proper_policy.mode)?.[2]} error={errors.proper_policy}>
					<select id="pr-proper" class="input" bind:value={form.body.proper_policy.mode}>
						{#each properChoices as [v, l] (v)}<option value={v}>{l}</option>{/each}
					</select>
				</Field>
				{#if form.body.proper_policy.mode === 'take_any' || form.body.proper_policy.mode === 'same_group_only'}
					<Field id="pr-proper-age" label="Only within this many days of the import" hint="0 means no limit." error={errors.max_age_days}>
						<input id="pr-proper-age" class="input narrow" type="number" min="0" max="36500" step="1" bind:value={form.body.proper_policy.max_age_days} placeholder="0" {...bad('max_age_days')} />
					</Field>
				{/if}
			{/snippet}
		</SettingsCard>

		<SettingsCard id="rules-must-never" title="Must have and Never" description="A Release without every must-have trait is passed over. One with any Never trait is never taken.">
			<div class="field">
				<span class="label" id="must-label">Must have</span>
				<div class="chips" role="group" aria-labelledby="must-label" data-testid="must-have">
					{#each form.body.must_have as t (t)}<Chip label={traitLabel(t)} onremove={() => (form.body.must_have = form.body.must_have.filter((x) => x !== t))} />{:else}<span class="hint">Nothing yet.</span>{/each}
				</div>
				<label class="sr-only" for="pr-must-add">Add to Must have</label>
				<select id="pr-must-add" class="input" value="" onchange={(e) => (addTo('must_have', e.currentTarget.value), (e.currentTarget.value = ''))} {...bad('must_have')}>
					<option value="">Add to Must have…</option>
					{#each own.filter((t) => !form.body.must_have.includes(t)) as t (t)}<option value={t}>{traitLabel(t)}</option>{/each}
				</select>
				{#if errors.must_have}<span class="field-error" id="pr-must-add-error" role="alert">{errors.must_have}</span>{/if}
			</div>
			<div class="field">
				<span class="label" id="never-label">Never</span>
				<div class="chips" role="group" aria-labelledby="never-label" data-testid="never">
					{#each form.body.never as t (t)}<Chip label={traitLabel(t)} onremove={() => (form.body.never = form.body.never.filter((x) => x !== t))} />{:else}<span class="hint">Nothing yet.</span>{/each}
				</div>
				<label class="sr-only" for="pr-never-add">Add to Never</label>
				<select id="pr-never-add" class="input" value="" onchange={(e) => (addTo('never', e.currentTarget.value), (e.currentTarget.value = ''))} {...bad('never')}>
					<option value="">Add to Never…</option>
					{#each neverChoices(form.kinds).filter((t) => !form.body.never.includes(t)) as t (t)}<option value={t}>{traitLabel(t)}</option>{/each}
				</select>
				{#if errors.never}<span class="field-error" id="pr-never-add-error" role="alert">{errors.never}</span>{/if}
			</div>
		</SettingsCard>

		<SettingsCard id="rules-size" title="Size" description={music ? 'How big a Release may be, per hour of music, for every step.' : 'How big a Release may be, per hour of video, for every step.'}>
			<div class="pair">
				<Field id="pr-size-min" label="Smallest (GB per hour)" error={errors.size_limits}>
					<input id="pr-size-min" class="input narrow" type="number" min="0" step="0.01" inputmode="decimal" placeholder="No limit" value={form.size.min} oninput={(e) => setSize('min', e.currentTarget.value)} {...bad('size_limits')} />
				</Field>
				<Field id="pr-size-max" label="Largest (GB per hour)">
					<input id="pr-size-max" class="input narrow" type="number" min="0" step="0.01" inputmode="decimal" placeholder="No limit" value={form.size.max} oninput={(e) => setSize('max', e.currentTarget.value)} />
				</Field>
			</div>
			<p class="hint">Leave a box empty for no limit. A 2-hour movie at 6 GB per hour is 12 GB.{#if otherSizes} {otherSizes === 1 ? 'One step has' : `${otherSizes} steps have`} sizes of {otherSizes === 1 ? 'its' : 'their'} own, which stay as they are.{/if}</p>
		</SettingsCard>

		<SettingsCard id="rules-wait" title="Waiting" description="Wait a while after a Release turns up, in case a better one follows.">
			<div class="field" id="pr-prefer">
				<span class="label">Prefer</span>
				<div><SegmentedControl label="Prefer" options={preferChoices} bind:value={form.body.protocol.prefer} /></div>
				<span class="hint">When a torrent and a Usenet Release are equally good, this one wins.</span>
			</div>
			<div class="pair">
				<Field id="pr-wait-torrent" label="Wait for torrents (minutes)" error={errors.wait_minutes_torrent}>
					<input id="pr-wait-torrent" class="input narrow" type="number" min="0" max="10080" step="1" bind:value={form.body.protocol.wait_minutes_torrent} {...bad('wait_minutes_torrent')} />
				</Field>
				<Field id="pr-wait-usenet" label="Wait for Usenet (minutes)" error={errors.wait_minutes_usenet}>
					<input id="pr-wait-usenet" class="input narrow" type="number" min="0" max="10080" step="1" bind:value={form.body.protocol.wait_minutes_usenet} {...bad('wait_minutes_usenet')} />
				</Field>
			</div>
			<label class="checkline">
				<input id="pr-skip-wait" type="checkbox" bind:checked={form.body.protocol.skip_wait_at_good_enough} />
				<span><b>Skip the wait once good enough</b><br /><span class="hint">A Release that is already good enough is taken at once.</span></span>
			</label>
		</SettingsCard>

		<SettingsCard id="rules-language" title="Language" description="Which spoken languages a Release may have.">
			<div class="field">
				<span class="label" id="lang-label">Languages to take</span>
				<div class="chips" role="group" id="pr-languages" aria-labelledby="lang-label">
					{#each languages as [code, l] (code)}<Chip label={l} selected={form.body.language.wanted.includes(code)} onclick={() => toggleLanguage(code)} />{/each}
				</div>
				{#if errors.language}<span class="field-error" id="pr-languages-error" role="alert">{errors.language}</span>{/if}
			</div>
			<label class="checkline">
				<input id="pr-original" type="checkbox" bind:checked={form.body.language.original_ok} />
				<span><b>The original language is fine</b><br /><span class="hint">A Release in the title's own language is taken too, even when it is not picked above.</span></span>
			</label>
		</SettingsCard>

	</fieldset>
		{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
	</form>
	{#if mayEdit}<SaveBar {dirty} form="pick-rules-form" saving={busy} label={id === null ? 'Make these Pick rules' : 'Save'} ondiscard={() => start(JSON.parse(saved))} />{/if}
{/if}
</PageLoader>

<style>
	/* The fieldset only turns the form read only; it adds no box of its own. */
	.all { display: contents; border: 0; padding: 0; margin: 0; }
	.loading { display: flex; flex-direction: column; gap: 16px; }
	.ro { font: var(--t-body); }
	.group { border: 0; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 6px; }
	.group:focus { outline: none; }
	.boxes { display: flex; flex-wrap: wrap; gap: 4px 20px; }
	.box { align-items: center; padding: 0; }
	.box input { margin-top: 0; }
	.steps { display: flex; flex-direction: column; }
	.steps li { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
	.steps li + li { border-top: 1px solid var(--line); }
	.switch { flex: 1; min-width: 0; font: var(--t-callout); }
	.traits { display: flex; flex-direction: column; gap: 8px; }
	.traits li { display: flex; align-items: center; gap: 8px; }
	.traits select { flex: 1; min-width: 0; }
	.points { width: 110px; flex: none; }
	.narrow { max-width: 180px; }
	.pair { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 12px; }
	.pair :global(.field + .field) { margin-top: 0; }
	.chips { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; min-height: 32px; }
	.field-error { color: var(--fail-text); font: var(--t-footnote); }
	/* Phones: the trait gets its own line, its points and remove go below it. */
	@media (max-width: 520px) {
		.traits { gap: 16px; }
		.traits li { flex-wrap: wrap; }
		.traits select { flex-basis: 100%; }
		.points { flex: 1; width: auto; }
	}
</style>
