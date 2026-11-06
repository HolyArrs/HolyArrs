<script lang="ts">
	import { theme, setTheme, type ThemeChoice } from '#lib/theme.svelte.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { DEFAULT_RATINGS, MAX_RATINGS, RATING_SOURCES, ratingPicks, resetRatingPicks, setRatingPick } from '#lib/rating-picks.svelte.ts';
	import { ABOUT_ITEMS, aboutIsDefault, aboutPicks, resetAboutPicks, setAboutPick } from '#lib/about-picks.svelte.ts';
	const aboutGroups = ['Information', 'Production', 'Links', 'Ratings', 'Feel'] as const;
	const isDefault = $derived(!ratingPicks.custom && ratingPicks.list.length === DEFAULT_RATINGS.length && DEFAULT_RATINGS.every((r) => ratingPicks.list.includes(r)));

	let themeChoice = $state<ThemeChoice>(theme.choice);
	const themes: { value: ThemeChoice; label: string; icon: 'sun' | 'moon' | 'settings' }[] = [
		{ value: 'light', label: 'Light', icon: 'sun' },
		{ value: 'dark', label: 'Dark', icon: 'moon' },
		{ value: 'system', label: 'Automatic', icon: 'settings' }
	];
</script>

<svelte:head><title>Appearance · HolyArrs</title></svelte:head>

<PageHeader title="Appearance" />

<SettingsCard id="theme-card" title="Theme" description="Saved in this browser" icon="appearance">
	<div id="theme" tabindex="-1">
		<SegmentedControl label="Theme" options={themes} bind:value={themeChoice} onchange={(v) => setTheme(v)} />
	</div>
</SettingsCard>

<SettingsCard id="ratings-card" title="Ratings" description="Up to three, beside the year on title pages" icon="star">
	<ul class="picks" id="ratings" tabindex="-1">
		{#each RATING_SOURCES as r (r.id)}
			<li>
				<label class="switch" data-testid="rating-pick-{r.id}">
					<input type="checkbox" checked={ratingPicks.list.includes(r.id)} disabled={!ratingPicks.list.includes(r.id) && ratingPicks.list.length >= MAX_RATINGS} onchange={(e) => setRatingPick(r.id, e.currentTarget.checked)} />
					<span class="sw" aria-hidden="true"></span>
					<span><b>{r.name}</b><br /><span class="hint">{r.about}</span></span>
				</label>
			</li>
		{/each}
	</ul>
	{#if !isDefault}<Button variant="plain" size="sm" onclick={resetRatingPicks}>Reset</Button>{/if}
</SettingsCard>

<SettingsCard id="about-card" title="About on title pages" description="Lines in the About section of title pages" icon="info">
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
	{#if !aboutIsDefault()}<Button variant="plain" size="sm" onclick={resetAboutPicks}>Reset</Button>{/if}
</SettingsCard>

<SettingsCard id="motion-card" title="Motion" description="Follows your device's Reduce motion setting">
</SettingsCard>

<style>
	.grp { margin: 14px 0 6px; color: var(--text-muted); font: 600 12px/1.2 var(--font-ui); letter-spacing: .06em; text-transform: uppercase; }
	.grp:first-child { margin-top: 0; }
	.picks { display: flex; flex-direction: column; gap: 4px; margin: 0 0 8px; padding: 0; list-style: none; }
</style>
