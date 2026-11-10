<script lang="ts">
	import Words from '#lib/ui/Words.svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import { checkMetadata, metadataHealth, metadataDownText } from '#lib/metadata/index.ts';
	import { onMount } from 'svelte';
	import { to } from '#lib/core/paths.ts';

	let loaded = $state(false);
	let checking = $state(false);

	onMount(async () => {
		await checkMetadata();
		loaded = true;
	});

	async function recheck() {
		checking = true;
		await checkMetadata(true);
		checking = false;
	}
</script>

<svelte:head><title>{t('settings.tmdb.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.tmdb.title')} />

{#if !loaded}
	<SkSettings rows={2} />
{:else}
	<SettingsCard id="metadata-card" title={t('settings.tmdb.server')}>
		<div class="srow">
			<div class="txt"><span class="label">{metadataHealth.up ? t('settings.tmdb.connected') : t('settings.tmdb.notAnswering')}</span>{#if !metadataHealth.up}<span class="hint" role="status">{metadataDownText()}</span>{/if}</div>
			<div class="ctl state">
				<span class="st" class:fail={!metadataHealth.up}><i></i>{metadataHealth.up ? t('settings.tmdb.answering') : t('settings.tmdb.down')}</span>
				<Button variant="secondary" size="sm" onclick={recheck} disabled={checking} aria-busy={checking}>{t('settings.tmdb.checkAgain')}</Button>
			</div>
		</div>
	</SettingsCard>

	<!-- Keep in step with the metadata privacy doc (tmdb-privacy.test.ts checks both). -->
	<SettingsCard id="metadata-sent" title={t('settings.tmdb.sentTitle')} description={t('settings.tmdb.sentDesc')}>
		<ul class="sent">
			<li><b>{t('settings.tmdb.tokenLabel')}</b> {t('settings.tmdb.tokenText')}</li>
			<li><b>{t('settings.tmdb.lookup')}</b> {t('settings.tmdb.lookupText')}</li>
			<li><b>{t('settings.tmdb.display')}</b> {t('settings.tmdb.displayText')}</li>
			<li><b>{t('settings.tmdb.headers')}</b> <Words key="settings.tmdb.headersText" /></li>
			<li><b>{t('settings.tmdb.country')}</b> <Words key="settings.tmdb.countryText" links={{ a: to('/settings/this-app') + '#count-install' }} /></li>
		</ul>
	</SettingsCard>

	<SettingsCard id="metadata-never" title={t('settings.tmdb.neverTitle')}>
		<ul class="sent">
			<li>{t('settings.tmdb.never1')}</li>
			<li>{t('settings.tmdb.never2')}</li>
			<li>{t('settings.tmdb.never3')}</li>
			<li>{t('settings.tmdb.never4')}</li>
		</ul>
	</SettingsCard>
{/if}

<style>
	.sent { margin: 0; padding-block: 0 !important; list-style: none; box-shadow: none !important; }
	.sent > li { padding: 13px 0; color: var(--text-muted); font: var(--t-subhead); font-size: 14px; }
	.sent > li + li { box-shadow: inset 0 1px 0 var(--line); }
	.sent b { color: var(--text); font-weight: 600; }
	.sent :global(code) { font-size: .9em; overflow-wrap: anywhere; }
	.state { display: flex; align-items: center; gap: 14px; }
	.st { display: inline-flex; align-items: center; gap: 6px; color: var(--text-muted); font: 500 13px/1 var(--font-ui); }
	.st i { width: 7px; height: 7px; border-radius: 50%; background: var(--ok); box-shadow: 0 0 0 3px color-mix(in srgb, var(--ok) 18%, transparent); }
	.st.fail i { background: var(--fail); box-shadow: 0 0 0 3px color-mix(in srgb, var(--fail) 22%, transparent); }
</style>
