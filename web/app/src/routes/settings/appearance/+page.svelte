<script lang="ts">
	import { theme, setTheme, type ThemeChoice } from '#lib/theme.svelte.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import { DEFAULT_RATINGS, RATING_SOURCES, ratingPicks, resetRatingPicks, setRatingPick } from '#lib/rating-picks.svelte.ts';
	import { ABOUT_ITEMS, aboutIsDefault, aboutPicks, resetAboutPicks, setAboutPick } from '#lib/about-picks.svelte.ts';
	const aboutGroups = ['Information', 'Made by', 'Ratings', 'Feel'] as const;
	const isDefault = $derived(ratingPicks.list.length === DEFAULT_RATINGS.length && DEFAULT_RATINGS.every((r) => ratingPicks.list.includes(r)));

	let themeChoice = $state<ThemeChoice>(theme.choice);
	const themes: { value: ThemeChoice; label: string; icon: 'sun' | 'moon' | 'settings' }[] = [
		{ value: 'light', label: 'Light', icon: 'sun' },
		{ value: 'dark', label: 'Dark', icon: 'moon' },
		{ value: 'system', label: 'Automatic', icon: 'settings' }
	];
</script>

<svelte:head><title>Appearance · HolyArrs</title></svelte:head>

<PageHeader title="Appearance" subtitle="How HolyArrs looks in this browser. Each person and each browser picks their own." />

<SettingsCard id="theme-card" title="Theme" description="Light, dark, or follow this device. The change shows straight away and is kept in this browser." icon="appearance">
	<div id="theme" tabindex="-1">
		<SegmentedControl label="Theme" options={themes} bind:value={themeChoice} onchange={(v) => setTheme(v)} />
	</div>
	<p class="hint">Automatic turns dark when this device does, for example at night.</p>
</SettingsCard>

<SettingsCard id="ratings-card" title="Ratings" description="Which ratings movie and show pages show. Three read at a glance: viewers on IMDb, then critics and viewers on Rotten Tomatoes. Kept in this browser." icon="star">
	<ul class="picks" id="ratings" tabindex="-1">
		{#each RATING_SOURCES as r (r.id)}
			<li>
				<label class="switch" data-testid="rating-pick-{r.id}">
					<input type="checkbox" checked={ratingPicks.list.includes(r.id)} onchange={(e) => setRatingPick(r.id, e.currentTarget.checked)} />
					<span class="sw" aria-hidden="true"></span>
					<span><b>{r.name}</b><br /><span class="hint">{r.about}</span></span>
				</label>
			</li>
		{/each}
	</ul>
	<p class="hint">A title with none of these still shows one score, so the page is never without one.</p>
	{#if !isDefault}<button type="button" class="btn ghost" onclick={resetRatingPicks}>Back to the default three</button>{/if}
</SettingsCard>

<SettingsCard id="about-card" title="About on title pages" description="Which lines the About section at the bottom of movie and show pages shows. Kept short by default; year, length, age rating and genres are already at the top. Kept in this browser." icon="info">
	{#each aboutGroups as g (g)}
		<h3 class="grp">{g}</h3>
		<ul class="picks">
			{#each ABOUT_ITEMS.filter((i) => i.group === g) as it (it.id)}
				<li>
					<label class="switch" data-testid="about-pick-{it.id}">
						<input type="checkbox" checked={aboutPicks.list.includes(it.id)} onchange={(e) => setAboutPick(it.id, e.currentTarget.checked)} />
						<span class="sw" aria-hidden="true"></span>
						<span><b>{it.name}</b></span>
					</label>
				</li>
			{/each}
		</ul>
	{/each}
	{#if !aboutIsDefault()}<button type="button" class="btn ghost" onclick={resetAboutPicks}>Back to the defaults</button>{/if}
</SettingsCard>

<SettingsCard id="motion-card" title="Motion" description="HolyArrs follows this device's own Reduce motion setting: when it is on, screens change without sliding or growing.">
	<p class="hint">Change it in your device's accessibility settings.</p>
</SettingsCard>

<style>
	.grp { margin: 14px 0 6px; color: var(--text-muted); font: 600 12px/1.2 var(--font-ui); letter-spacing: .06em; text-transform: uppercase; }
	.grp:first-child { margin-top: 0; }
	.picks { display: flex; flex-direction: column; gap: 4px; margin: 0 0 8px; padding: 0; list-style: none; }
</style>
