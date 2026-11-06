<script lang="ts">
	import { theme, setTheme, type ThemeChoice } from '#lib/theme.svelte.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { DEFAULT_RATINGS, MAX_RATINGS, RATING_SOURCES, ratingPicks, resetRatingPicks, setRatingPick } from '#lib/rating-picks.svelte.ts';
	import { ABOUT_ITEMS, aboutIsDefault, aboutPicks, resetAboutPicks, setAboutPick } from '#lib/about-picks.svelte.ts';
	import Table from '#lib/ui/Table.svelte';
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { session } from '#lib/session.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { setAutoplaySetting } from '#lib/autoplay.svelte.ts';
	const aboutGroups = ['Information', 'Production', 'Links', 'Ratings', 'Feel'] as const;
	const isDefault = $derived(!ratingPicks.custom && ratingPicks.list.length === DEFAULT_RATINGS.length && DEFAULT_RATINGS.every((r) => ratingPicks.list.includes(r)));

	let themeChoice = $state<ThemeChoice>(theme.choice);
	const themes: { value: ThemeChoice; label: string; icon: 'sun' | 'moon' | 'settings' }[] = [
		{ value: 'light', label: 'Light', icon: 'sun' },
		{ value: 'dark', label: 'Dark', icon: 'moon' },
		{ value: 'system', label: 'Automatic', icon: 'settings' }
	];

	// App-wide looks (settings people only; moved here from This app):
	// trailers that play by themselves and the sign-in page art.
	let autoplay = $state<boolean | null>(null);
	let signinArt = $state(false);
	let busy = $state(false);
	async function loadApp() {
		try {
			const s = await api.get<{ autoplay_trailers: boolean; signin_library_art?: boolean }>('/settings/ui');
			autoplay = s.autoplay_trailers;
			signinArt = s.signin_library_art === true;
		} catch {
			autoplay = null;
		}
	}
	async function saveApp(field: 'autoplay_trailers' | 'signin_library_art', on: boolean) {
		busy = true;
		try {
			const s = await api.put<{ autoplay_trailers?: boolean; signin_library_art?: boolean }>('/settings/ui', { [field]: on });
			if (field === 'autoplay_trailers') {
				autoplay = s.autoplay_trailers ?? on;
				setAutoplaySetting(autoplay);
				toasts.push(autoplay ? 'Trailers play by themselves' : 'Trailers no longer play by themselves', 'ok');
			} else {
				signinArt = s.signin_library_art ?? on;
				toasts.push(signinArt ? 'The sign-in page shows Library art' : 'The sign-in page shows trending art', 'ok');
			}
		} catch (e) {
			toasts.push(message(e), 'fail');
			await loadApp();
		} finally {
			busy = false;
		}
	}

	// Explore row titles: smart titles against plain ones, counted on this server only (owner).
	type TitleStat = { layer: string; shown: number; opens: number; asks: number; rate: string };
	let titleStats = $state<TitleStat[] | null>(null);
	const layerWords: Record<string, string> = { occasion: 'Occasion', time: 'Time of day', genre: 'Genre mix', taste: 'Your taste', reason: 'Row reason', plain: 'Plain title' };
	async function loadTitleStats() {
		try {
			const a = await api.get<{ layers: Omit<TitleStat, 'rate'>[] }>('/explore/title-stats');
			titleStats = a.layers.map((l) => ({ ...l, layer: layerWords[l.layer] ?? l.layer, rate: l.shown ? `${(((l.opens + l.asks) / l.shown) * 100).toFixed(1)}%` : '–' }));
		} catch {
			titleStats = null;
		}
	}
	onMount(() => {
		if (session.can('settings')) void loadApp();
		if (session.s.role === 'owner') void loadTitleStats();
	});
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

{#if autoplay !== null}
	<SettingsCard id="autoplay-card" title="For everyone" description="Applies to every person on this server" icon="tv">
		<div data-testid="autoplay-setting">
			<label class="checkline">
				<input id="autoplay-trailers" type="checkbox" checked={autoplay} disabled={busy} onchange={(e) => saveApp('autoplay_trailers', e.currentTarget.checked)} />
				<span><b>Play trailers by themselves</b><br /><span class="hint">Muted, on Explore. Never for Kids, Reduce motion or slow connections.</span></span>
			</label>
			<label class="checkline" data-testid="signin-art-setting">
				<input id="signin-art" type="checkbox" checked={signinArt} disabled={busy} onchange={(e) => saveApp('signin_library_art', e.currentTarget.checked)} />
				<span><b>Show Library art on the sign-in page</b><br /><span class="hint">Anyone can see them, with no rating filter</span></span>
			</label>
		</div>
	</SettingsCard>
{/if}

{#if session.s.role === 'owner' && titleStats}
	<SettingsCard id="row-titles-card" title="Explore row titles" description="Last 30 days">
		<Table
			caption="Explore row titles"
			columns={[{ key: 'layer', label: 'Title from' }, { key: 'shown', label: 'Shown', align: 'end' }, { key: 'opens', label: 'Opens', align: 'end' }, { key: 'asks', label: 'Requests', align: 'end' }, { key: 'rate', label: 'Per row', align: 'end' }]}
			rows={titleStats}
			rowKey={(r) => r.layer}
			empty="No Explore rows counted yet."
		/>
	</SettingsCard>
{/if}

<SettingsCard id="motion-card" title="Motion" description="Follows your device's Reduce motion setting">
</SettingsCard>

<style>
	.grp { margin: 14px 0 6px; color: var(--text-muted); font: 600 12px/1.2 var(--font-ui); letter-spacing: .06em; text-transform: uppercase; }
	.grp:first-child { margin-top: 0; }
	.picks { display: flex; flex-direction: column; gap: 4px; margin: 0 0 8px; padding: 0; list-style: none; }
</style>
