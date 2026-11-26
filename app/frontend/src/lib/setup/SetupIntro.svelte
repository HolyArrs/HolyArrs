<script lang="ts">
	// Welcome on the sign-in page's first run: what HolyArrs does, who built it, and Get started.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import Icon, { type IconName } from '#lib/ui/Icon.svelte';
	import { pixelTrail } from '#lib/accounts/index.ts';
	import { setupLang, languageName } from './PickLanguage.svelte';

	let { onstart }: { onstart: () => void } = $props();
	// Welcome's feature rows: what HolyArrs does, one line each.
	const introFeats: { key: string; icon: IconName }[] = [
		{ key: 'welcome.intro.f1', icon: 'download' },
		{ key: 'welcome.intro.f2', icon: 'library' },
		{ key: 'welcome.intro.f3', icon: 'requests' },
		{ key: 'welcome.intro.f4', icon: 'subtitle' }
	];
	// The sentence around the name, split at a marker so any language's word order works.
	const by = $derived(t('welcome.intro.byName', { name: '\u0001' }).split('\u0001'));
</script>

<h1 class="title" tabindex="-1">{t('welcome.intro.title')}</h1>
<p class="lead intro-lead">{t('welcome.intro.lead')}</p>
<ul class="intro-feats">
	{#each introFeats as f (f.key)}<li><span class="feat-icon" aria-hidden="true"><Icon name={f.icon} size={20} /></span><span>{t(f.key)}</span></li>{/each}
</ul>
<p class="lead intro-by">{by[0]}<a href="https://github.com/CoderLuii" target="_blank" rel="noopener noreferrer">CoderLuii</a>{by[1] ?? ''}</p>
<p class="lead intro-start">{t('welcome.intro.start')}</p>
{#if setupLang.failed}<p class="small muted intro-later" role="status" data-testid="setup-lang-later">{t('welcome.lang.later', { name: languageName(setupLang.failed) })}</p>{/if}
<div class="submit intro-go" use:pixelTrail={{ selector: '.ui-btn' }}><Button variant="primary" size="lg" full type="button" data-testid="setup-start" onclick={onstart}>{t('welcome.intro.go')}</Button></div>

<style>
	@media (prefers-reduced-motion: no-preference) {
		:global(.intro) .title, :global(.intro) .lead {
			animation: part-in var(--dur-slow) var(--ease-out) calc(90ms + var(--i, 1) * 70ms) both;
		}
	}
	:global(.gl-wait) .title, :global(.gl-wait) .lead { animation-play-state: paused; }
	@keyframes part-in { from { opacity: 0; transform: translateY(6px); } }
	:global(.light) .title { color: #111; }
	:global(.light) .lead { color: rgb(0 0 0 / 0.74); }
	.title { font: var(--t-title1); letter-spacing: var(--t-title1-track); text-align: center; color: #fff; text-wrap: balance; }
	.lead { margin: var(--s-2) 0 var(--gap); color: rgb(255 255 255 / 0.68); font: var(--t-callout); text-align: center; text-wrap: pretty; }
	/* Welcome: the summary and the byline sit close together above the Get started line. */
	.lead.intro-lead { margin-bottom: var(--s-5); }
	.intro-feats { list-style: none; margin: 0 auto; padding: 0; display: grid; gap: 10px; width: min(100%, 420px); }
	.intro-feats li { display: flex; align-items: center; gap: 12px; min-height: 48px; padding: 0 14px; border-radius: var(--g-r, var(--r-card)); background: var(--g-bg, rgb(255 255 255 / 0.06)); border: 1px solid var(--g-line, rgb(255 255 255 / 0.1)); color: #fff; font: 500 15px/1.3 var(--font-ui); }
	:global(.light) .intro-feats li { color: #111; background: rgb(255 255 255 / 0.55); border-color: rgb(0 0 0 / 0.1); }
	.feat-icon { flex: none; display: grid; place-items: center; width: 20px; opacity: 0.8; }
	.lead.intro-by { margin: var(--s-5) 0 0; font: var(--t-footnote); }
	.intro-by a { color: inherit; font-weight: 600; text-decoration: underline; text-underline-offset: 2px; }
	.lead.intro-start { margin: var(--s-2) 0 var(--s-4); }
	.intro-go { display: flex; flex-direction: column; }
	.title:focus { outline: none; }
	.intro-later { margin: 0 0 var(--s-2); text-align: center; font: var(--t-footnote); }
	/* Get started: the label sits above the pixel trail, like Continue on the other steps. */
	.intro-go :global(.ui-btn) { position: relative; overflow: hidden; isolation: isolate; }
	.intro-go :global(.ui-btn > :not(canvas)) { position: relative; z-index: 1; }
	/* Welcome is one group, centred in the card: no gap between the rows and Get started. */
	:global(.step.intro-step) { display: flex; flex-direction: column; justify-content: center; }
	:global(.step.intro-step) .intro-start { margin-top: 40px; margin-bottom: 20px; }
	/* The first screen reads bigger: headline, summary and rows a step up. */
	:global(.step.intro-step) .intro-lead { font-size: 17px; max-width: 560px; margin-inline: auto; }
	:global(.step.intro-step) .intro-feats { width: min(100%, 460px); }
	:global(.step.intro-step) .intro-feats li { min-height: 54px; font-size: 16px; }
	@media (max-width: 600px) {
		:global(.step.intro-step) .intro-lead { font-size: 15px; }
		:global(.step.intro-step) .intro-feats li { min-height: 48px; font-size: 15px; }
	}
	@media (min-width: 1100px) {
		:global(.panel.setup) .lead:not(.intro-start):not(.intro-by) + :global(*) { margin-top: clamp(20px, 3vh, 44px); }
		:global(.panel.setup) .title { font-size: clamp(32px, 3.6vh, 40px); line-height: 1.15; }
		:global(.panel.setup) .lead { font-size: 17px; }
		:global(.step.intro-step) .intro-start { margin: auto 0 8px; }
		:global(.step.intro-step) .intro-feats { margin-top: 28px; }
		:global(.step.intro-step) .title { font-size: clamp(32px, 3.6vh, 40px); }
	}
	@media (max-width: 600px) {
		.title { font-size: 20px; line-height: 1.15; }
		.lead { margin: var(--s-1) 0 var(--gap); }
	}
</style>
