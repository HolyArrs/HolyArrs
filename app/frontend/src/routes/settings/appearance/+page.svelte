<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { DEFAULT_RATINGS, MAX_RATINGS, RATING_SOURCES, ratingPicks, resetRatingPicks, setRatingPick } from '#lib/ui/rating-picks.svelte.ts';
	import { theme, setTheme, type ThemeChoice, backdrop, setBackdrop, type Backdrop, ABOUT_ITEMS, aboutIsDefault, aboutPicks, resetAboutPicks, setAboutPick, Table, LanguagePicker } from '#lib/accounts/index.ts';
	import { onMount } from 'svelte';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { setAutoplaySetting } from '#lib/core/trailer/autoplay.svelte.ts';
	import { look, setLook, minimalLook, type Look } from '#lib/core/look.svelte.ts';
	const aboutGroups = ['Information', 'Production', 'Links', 'Ratings', 'Feel'] as const;
	const isDefault = $derived(!ratingPicks.custom && ratingPicks.list.length === DEFAULT_RATINGS.length && DEFAULT_RATINGS.every((r) => ratingPicks.list.includes(r)));

	let themeChoice = $state<ThemeChoice>(theme.choice);
	let backdropChoice = $state<Backdrop>(backdrop.value);
	const backdrops = $derived<{ value: Backdrop; label: string }[]>([
		{ value: 'plain', label: t('settings.appearance.plain') },
		{ value: 'posters', label: t('settings.appearance.posterWall') }
	]);
	let lookChoice = $state<Look>(look.value);
	const looks = $derived<{ value: Look; label: string }[]>([
		{ value: 'full', label: t('settings.appearance.lookFull') },
		{ value: 'minimal', label: t('settings.appearance.lookMinimal') }
	]);
	const themes = $derived<{ value: ThemeChoice; label: string; icon: 'sun' | 'moon' | 'settings' }[]>([
		{ value: 'light', label: t('settings.appearance.light'), icon: 'sun' },
		{ value: 'dark', label: t('settings.appearance.dark'), icon: 'moon' },
		{ value: 'system', label: t('settings.appearance.automatic'), icon: 'settings' }
	]);

	// App-wide looks (settings people only): trailers that play by themselves and the sign-in page art.
	let autoplay = $state<boolean | null>(null);
	let signinArt = $state(false);
	let busy = $state(false);
	let appError = $state('');
	async function loadApp() {
		try {
			const s = await api.get<{ autoplay_trailers: boolean; signin_library_art?: boolean }>('/settings/ui');
			autoplay = s.autoplay_trailers;
			signinArt = s.signin_library_art === true;
		} catch {
			autoplay = null;
		}
	}
	async function saveApp(field: 'autoplay_trailers' | 'signin_library_art', on: boolean, box: HTMLInputElement) {
		busy = true;
		appError = '';
		try {
			const s = await api.put<{ autoplay_trailers?: boolean; signin_library_art?: boolean }>('/settings/ui', { [field]: on });
			if (field === 'autoplay_trailers') {
				autoplay = s.autoplay_trailers ?? on;
				setAutoplaySetting(autoplay);
				toasts.push(autoplay ? t('settings.appearance.autoplayOn') : t('settings.appearance.autoplayOff'), 'ok');
			} else {
				signinArt = s.signin_library_art ?? on;
				toasts.push(signinArt ? t('settings.appearance.artLibrary') : t('settings.appearance.artTrending'), 'ok');
			}
		} catch (e) {
			// Put the box back and say why next to it, so a failed save is never silent.
			box.checked = !on;
			appError = t('settings.appearance.notSaved', { reason: message(e) });
			toasts.push(t('settings.appearance.couldntSave'), 'fail', { body: message(e) });
		} finally {
			busy = false;
		}
	}

	// Explore row titles: smart titles against plain ones, counted on this server only (owner).
	type TitleStat = { layer: string; shown: number; opens: number; asks: number; rate: string };
	let titleStats = $state<TitleStat[] | null>(null);
	const layerWords = $derived<Record<string, string>>({
		occasion: t('settings.appearance.layerOccasion'),
		time: t('settings.appearance.layerTime'),
		genre: t('settings.appearance.layerGenre'),
		taste: t('settings.appearance.layerTaste'),
		reason: t('settings.appearance.layerReason'),
		plain: t('settings.appearance.layerPlain')
	});
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
		if (session.s.role === 'owner' && !session.held('row_title_stats')) void loadTitleStats();
	});
</script>

<svelte:head><title>{t('settings.appearance.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.appearance.title')} />

<SettingsCard id="theme-card" title={t('settings.appearance.theme')} description={t('settings.appearance.savedHere')} icon="appearance">
	<div id="theme" tabindex="-1">
		<SegmentedControl label={t('settings.appearance.theme')} options={themes} bind:value={themeChoice} onchange={(v) => setTheme(v)} />
	</div>
</SettingsCard>

<SettingsCard id="look-card" title={t('settings.appearance.look')} description={t('settings.appearance.lookDesc')} icon="appearance">
	<div id="look" tabindex="-1">
		<SegmentedControl label={t('settings.appearance.look')} options={looks} bind:value={lookChoice} onchange={(v) => setLook(v)} />
	</div>
</SettingsCard>

<SettingsCard id="language-card" title={t('accounts.language.label')} description={t('accounts.language.hint')} icon="appearance">
	<LanguagePicker id="language" />
</SettingsCard>

<SettingsCard id="backdrop-card" title={t('settings.appearance.background')} description={t('settings.appearance.backgroundDesc')} icon="appearance">
	<div id="backdrop" tabindex="-1">
		<SegmentedControl label={t('settings.appearance.background')} options={backdrops} bind:value={backdropChoice} onchange={(v) => setBackdrop(v)} />
		{#if minimalLook()}<p class="look-hint" data-testid="wall-off-minimal">{t('settings.appearance.wallOffMinimal')}</p>{/if}
	</div>
</SettingsCard>

<SettingsCard id="ratings-card" title={t('settings.appearance.ratings')} description={t('settings.appearance.ratingsDesc')} icon="star">
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
	{#if !isDefault}<Button variant="plain" size="sm" onclick={resetRatingPicks}>{t('settings.appearance.reset')}</Button>{/if}
</SettingsCard>

<SettingsCard id="about-card" title={t('settings.appearance.about')} description={t('settings.appearance.aboutDesc')} icon="info">
	{#each aboutGroups as g (g)}
		<h3 class="grp">{t('settings.appearance.group.' + g.toLowerCase())}</h3>
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
	{#if !aboutIsDefault()}<Button variant="plain" size="sm" onclick={resetAboutPicks}>{t('settings.appearance.reset')}</Button>{/if}
</SettingsCard>

{#if autoplay !== null}
	<SettingsCard id="autoplay-card" title={t('settings.appearance.everyone')} description={t('settings.appearance.everyoneDesc')} icon="tv">
		<div data-testid="autoplay-setting">
			<label class="checkline">
				<input id="autoplay-trailers" type="checkbox" checked={autoplay} disabled={busy} onchange={(e) => saveApp('autoplay_trailers', e.currentTarget.checked, e.currentTarget)} />
				<span><b>{t('settings.appearance.autoplay')}</b><br /><span class="hint">{t('settings.appearance.autoplayHint')}</span></span>
			</label>
			<label class="checkline" data-testid="signin-art-setting">
				<input id="signin-art" type="checkbox" checked={signinArt} disabled={busy} onchange={(e) => saveApp('signin_library_art', e.currentTarget.checked, e.currentTarget)} />
				<span><b>{t('settings.appearance.signinArt')}</b><br /><span class="hint">{t('settings.appearance.signinArtHint')}</span></span>
			</label>
			{#if appError}<p class="app-error" role="alert" data-testid="autoplay-error">{appError}</p>{/if}
		</div>
	</SettingsCard>
{/if}

{#if session.s.role === 'owner' && titleStats}
	<SettingsCard id="row-titles-card" title={t('settings.appearance.rowTitles')} description={t('settings.appearance.last30')}>
		<Table
			caption={t('settings.appearance.rowTitles')}
			columns={[{ key: 'layer', label: t('settings.appearance.colFrom') }, { key: 'shown', label: t('settings.appearance.colShown'), align: 'end' }, { key: 'opens', label: t('settings.appearance.colOpens'), align: 'end' }, { key: 'asks', label: t('settings.appearance.colRequests'), align: 'end' }, { key: 'rate', label: t('settings.appearance.colRate'), align: 'end' }]}
			rows={titleStats}
			rowKey={(r) => r.layer}
			empty={t('settings.appearance.noRows')}
		/>
	</SettingsCard>
{/if}

<SettingsCard id="motion-card" title={t('settings.appearance.motion')}>
	<p class="motion-line">{t('settings.appearance.motionNote')}</p>
</SettingsCard>

<style>
	.app-error { margin: 8px 0 0; color: var(--fail-text, var(--fail)); font: var(--t-subhead); overflow-wrap: anywhere; }
	/* Group names inside the sheet: room above, no divider lines through or under them. */
	.grp { margin: 0; padding-top: 22px !important; padding-bottom: 2px !important; box-shadow: none !important; color: var(--text-muted); font: 600 11px/1.2 var(--font-ui); letter-spacing: .08em; text-transform: uppercase; }
	.grp:first-child { padding-top: 16px !important; }
	/* One row height and one divider for every switch row. */
	.picks { display: flex; flex-direction: column; gap: 0; margin: 0; padding-block: 0 !important; list-style: none; box-shadow: none !important; }
	.picks > li { padding: 6px 0; }
	.picks > li + li { box-shadow: inset 0 1px 0 var(--line); }
	.look-hint { margin: 8px 0 0; color: var(--text-muted); font: var(--t-subhead); }
	.motion-line { margin: 0; padding-block: 14px; color: var(--text-muted); font: var(--t-subhead); }
</style>
