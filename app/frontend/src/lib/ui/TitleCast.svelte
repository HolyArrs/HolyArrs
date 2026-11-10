<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// Cast and crew shelf for a title page, with a full list sheet.
	import Shelf from './Shelf.svelte';
	import PersonCircle from './PersonCircle.svelte';
	import Skeleton from './Skeleton.svelte';
	import Sheet from './Sheet.svelte';
	import type { PersonSummary } from './people.ts';
	import { api } from '../core/api.ts';
	import { to } from '../core/paths.ts';

	// the shelf shows one row; the sheet shows everyone
	const ROW = 12;

	let { titleId }: { titleId: number } = $props();
	type Credit = { person: PersonSummary; roles: string[]; characters: string[] };
	let people = $state<Credit[]>([]);
	let phase = $state<'loading' | 'ok' | 'error'>('loading');
	let all = $state(false);

	// the live flag drops answers that arrive after the title changed
	$effect(() => {
		const id = titleId;
		let live = true;
		phase = 'loading';
		people = [];
		api.get<{ people: Credit[] }>(`/titles/${id}/people`)
			.then((r) => { if (live) { people = r.people ?? []; phase = 'ok'; } })
			.catch(() => { if (live) { phase = 'error'; } });
		return () => { live = false; };
	});

	// show characters for actors, job roles for crew
	const role = (c: Credit) => (c.characters?.length ? c.characters.join(', ') : c.roles.join(', '));
</script>

{#if phase === 'loading'}
	<section class="tp-section" id="title-cast" aria-label={t('ui.titleCast.castAndCrewLoading')} aria-busy="true">
		<h2 class="tp-h2">{t('ui.titleCast.castAndCrew')}</h2>
		<div class="sk-row">{#each [0, 1, 2, 3, 4] as i (i)}<Skeleton shape="circle" width="96px" height="96px" />{/each}</div>
	</section>
{:else if phase === 'ok' && people.length}
	<Shelf title={t('ui.titleCast.castAndCrew')} item="112px" anchor="title-cast" whole onseeall={() => (all = true)}>
		{#each people.slice(0, ROW) as c (c.person.id)}
			<PersonCircle name={c.person.name} role={role(c)} image={c.person.profile} href={to(`/people/${encodeURIComponent(c.person.id)}`)} />
		{/each}
	</Shelf>
	<Sheet bind:open={all} title={t('ui.titleCast.castAndCrew')} size="lg">
		<ul class="all-grid">
			{#each people as c (c.person.id)}
				<li><PersonCircle name={c.person.name} role={role(c)} image={c.person.profile} href={to(`/people/${encodeURIComponent(c.person.id)}`)} /></li>
			{/each}
		</ul>
	</Sheet>
{:else if phase === 'ok'}
	<section class="tp-section" id="title-cast" aria-labelledby="h-cast">
		<h2 id="h-cast" class="tp-h2">{t('ui.titleCast.castAndCrew')}</h2>
		<p class="small muted">{t('ui.titleCast.noCastOrCrew')}</p>
	</section>
{/if}

<style>
	/* clip the skeleton row so it never causes sideways scroll */
	.sk-row { display: flex; gap: 16px; overflow: hidden; }
	.all-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(104px, 1fr)); gap: 16px 12px; margin: 0; padding: 0; list-style: none; }
</style>
