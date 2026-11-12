<!-- Hide from browsing: a title or one of its genres, for everyone or one person; the hides on it already, with Show again. -->
<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Field from '#lib/ui/Field.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import { api, message } from '#lib/core/api.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import HiddenList from './HiddenList.svelte';
	import { addHide, hideFor, hideName, showAgain, type Hide } from './hidden.ts';

	let {
		open = $bindable(false),
		kind,
		tmdb,
		name,
		genres,
		hides,
		people,
		onchange
	}: {
		open?: boolean;
		kind: 'movie' | 'show';
		tmdb: string;
		name: string;
		genres: string[];
		hides: Hide[];
		/** Everyone but the owner, by id. */
		people: ReadonlyMap<number, string>;
		onchange: (hides: Hide[]) => void;
	} = $props();

	// '' is this title; otherwise a genre.
	let what = $state('');
	let forWho = $state<'everyone' | 'one'>('everyone');
	let person = $state(0);
	let saving = $state(false);
	let error = $state('');

	$effect(() => {
		if (!open) return;
		what = '';
		forWho = 'everyone';
		person = 0;
		error = '';
	});

	const peopleList = $derived([...people].map(([id, n]) => ({ id, name: n })));
	const missingPerson = $derived(forWho === 'one' && !person);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (missingPerson) {
			error = t('discover.hidden.pickPerson');
			return;
		}
		saving = true;
		error = '';
		try {
			const h = await addHide(api, { kind, ...(what ? { genre: what } : { tmdb_id: tmdb, name }), user_id: forWho === 'one' ? person : 0 });
			const next = [...hides.filter((x) => x.id !== h.id), h];
			onchange(next);
			open = false;
			const words = { name: hideName(h), who: hideFor(h, people) };
			toasts.push(h.user_id ? t('discover.hidden.hiddenFor', words) : t('discover.hidden.hiddenEveryone', words), 'ok', {
				action: {
					label: t('discover.hidden.undo'),
					run: () => {
						showAgain(api, h.id).then(
							() => onchange(next.filter((x) => x.id !== h.id)),
							(err: unknown) => toasts.push(message(err), 'fail')
						);
					}
				}
			});
		} catch (err) {
			error = message(err);
		} finally {
			saving = false;
		}
	}
</script>

<Sheet bind:open title={t('discover.hidden.sheetTitle', { name })} size="sm">
	<form id="hide-form" class="hf" onsubmit={submit}>
		<p class="small muted">{t('discover.hidden.sheetBody')}</p>
		{#if hides.length}
			<section aria-labelledby="hide-now">
				<h3 id="hide-now" class="label">{t('discover.hidden.hiddenNow')}</h3>
				<HiddenList {hides} {people} onshown={(id) => onchange(hides.filter((x) => x.id !== id))} />
			</section>
		{/if}
		{#if genres.length}
			<Field id="hide-what" label={t('discover.hidden.what')}>
				<select id="hide-what" class="input" bind:value={what} data-testid="hide-what">
					<option value="">{t('discover.hidden.onlyThis', { name })}</option>
					{#each genres as g (g)}<option value={g}>{kind === 'show' ? t('discover.hidden.allGenreShows', { genre: g }) : t('discover.hidden.allGenreMovies', { genre: g })}</option>{/each}
				</select>
			</Field>
		{/if}
		{#if peopleList.length}
			<div class="field">
				<span class="label" id="hide-for">{t('discover.hidden.for')}</span>
				<SegmentedControl size="sm" label={t('discover.hidden.for')} options={[{ value: 'everyone', label: t('discover.hidden.everyone') }, { value: 'one', label: t('discover.hidden.onePerson') }]} bind:value={forWho} testid="hide-for" />
			</div>
			{#if forWho === 'one'}
				<Field id="hide-person" label={t('discover.hidden.person')} error={missingPerson && error ? error : undefined}>
					<select id="hide-person" class="input" bind:value={person} data-testid="hide-person" aria-invalid={missingPerson && !!error}>
						<option value={0} disabled>{t('discover.hidden.pickPerson')}</option>
						{#each peopleList as p (p.id)}<option value={p.id}>{p.name}</option>{/each}
					</select>
				</Field>
			{/if}
		{/if}
		{#if error && !missingPerson}<p class="field-error" role="alert">{error}</p>{/if}
	</form>
	{#snippet footer()}
		<Button variant="plain" onclick={() => (open = false)}>{t('discover.hidden.cancel')}</Button>
		<Button variant="primary" type="submit" form="hide-form" busy={saving} data-testid="hide-submit">{t('discover.hidden.hide')}</Button>
	{/snippet}
</Sheet>

<style>
	.hf {
		display: flex;
		flex-direction: column;
		gap: var(--s-4);
		min-width: 0;
	}
	.hf p {
		margin: 0;
		text-wrap: pretty;
	}
	/* The form's gap already spaces fields; the global .field + .field margin would double it. */
	.hf > :global(.field + .field) {
		margin-top: 0;
	}
	h3.label {
		margin: 0 0 var(--s-1);
	}
</style>
