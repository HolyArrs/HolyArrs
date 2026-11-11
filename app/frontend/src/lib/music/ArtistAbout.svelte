<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import ArtPlaceholder from '#lib/ui/ArtPlaceholder.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import { minimalLook } from '#lib/core/look.svelte.ts';
	import { artistKind, changeWords, countryName, fullCount, type ArtistCoreData, type Catalog } from './music-pages.ts';

	// The About card (photo, rank disc, two lines of the bio) and the sheet it opens.
	let { open = $bindable(false), hero, picture, wideFace, wideFailed, photoFailed, listen, listenWords, catalog, genres }: {
		open?: boolean;
		hero: ArtistCoreData;
		picture: string | null | undefined;
		wideFace: string | null;
		wideFailed: boolean;
		photoFailed: boolean;
		listen: ReturnType<typeof import('./music-pages.ts').listening>;
		listenWords: string;
		catalog: Catalog | null;
		genres: string;
	} = $props();
	const cardImg = $derived(wideFace && !wideFailed ? wideFace : picture);
	const smallCount = $derived(catalog ? catalog.eps + catalog.singles : 0);
	// One or two sentences of the bio for the card; the full text is in the sheet.
	const bioLine = $derived(hero?.bio?.extract ? hero.bio.extract.split(/(?<=[.!?])\s+/).slice(0, 2).join(' ') : '');
	// The wide photo and the square photo take turns with a slow zoom, like the people page.
	const aboutStills = $derived([wideFace && !wideFailed ? wideFace : '', picture && !photoFailed ? picture : ''].filter((x, i, a): x is string => !!x && a.indexOf(x) === i));
	let still = $state(0);
	// The minimal look: a plain text card and a sheet without the big photo.
	const minimal = $derived(minimalLook());
	// The wide stills, else the one portrait.
	const shots = $derived(aboutStills.length ? aboutStills : picture ? [picture] : []);
	$effect(() => {
		const n = aboutStills.length;
		if (n < 2 || minimal || matchMedia('(prefers-reduced-motion: reduce)').matches) return;
		const t = setInterval(() => (still = (still + 1) % n), 7000);
		return () => clearInterval(t);
	});
</script>

<section class="block about">
	<h2>{t('music.about')}</h2>
	<button type="button" class="acard" class:minimal aria-haspopup="dialog" aria-label={t('music.aboutReadMore', { name: hero.name })} onclick={() => (open = true)} onpointermove={(e) => { const b = e.currentTarget.getBoundingClientRect(); e.currentTarget.style.setProperty('--mx', `${((e.clientX - b.left) / b.width) * 100}%`); e.currentTarget.style.setProperty('--my', `${((e.clientY - b.top) / b.height) * 100}%`); }} data-testid="artist-about">
		{#if minimal}{:else if aboutStills.length}{#each aboutStills as src, i (src)}<img class="ab-frame" class:on={i === still % aboutStills.length} {src} alt="" loading="lazy" decoding="async" />{/each}<span class="a-glow" aria-hidden="true"></span>{:else if cardImg}<img class="ab-frame on" src={cardImg} alt="" loading="lazy" decoding="async" />{:else}<span class="a-none" aria-hidden="true"><ArtPlaceholder name={hero.name ?? ''} shape="wide" /></span>{/if}
		{#if listen?.rank}<span class="disc" aria-hidden="true"><b>#{listen.rank}</b><small>{t('music.inTheWorld')}</small></span>{/if}
		<span class="a-foot">
			{#if listenWords || catalog?.span || genres}<span class="a-lis">{[listenWords, catalog?.span, genres.split(', ').slice(0, 2).join(', ')].filter(Boolean).join(' · ')}</span>{/if}
			{#if bioLine}<span class="a-bio">{bioLine}</span>{:else}<span class="a-bio">{hero.name}</span>{/if}
			<span class="a-more">{t('music.moreAbout', { name: hero.name })}</span>
		</span>
	</button>
</section>

<Sheet bind:open title={t('music.about2', { name: hero.name })} size="lg">
	<div class="sheet" data-testid="artist-about-sheet">
			{#if shots.length && !minimal}
				<figure class="col-photo">
					{#each shots as src, i (src)}<img class:on={i === still % shots.length} {src} alt="" decoding="async" />{/each}
					{#if shots.length > 1}
						<button type="button" class="col-nav prev" aria-label={t('music.previousPhoto')} onclick={() => (still = (still - 1 + shots.length) % shots.length)}><Icon name="chevronLeft" size={20} /></button>
						<button type="button" class="col-nav next" aria-label={t('music.nextPhoto')} onclick={() => (still = (still + 1) % shots.length)}><Icon name="chevronRight" size={20} /></button>
					{/if}
				</figure>
			{/if}
			<div class="col">
				<aside class="col-side">
					{#if listen?.rank}<span class="col-disc"><b>#{listen.rank}</b><small>{t('music.inTheWorld')}</small></span>{/if}
					{#if listen}<p class="col-big"><b>{fullCount(listen.n)}</b><span>{listen.unit}</span></p>{/if}
					{#if listen?.today}<p class="col-big"><b class="chg" class:up={listen.today > 0} class:down={listen.today < 0}>{changeWords(listen.today, '')}</b><span>{t('music.today')}</span></p>{/if}
					<dl class="col-list">
						{#if catalog?.albums}<div><dt>{catalog.albums === 1 ? t('music.album') : t('music.albums')}</dt><dd>{catalog.albums}</dd></div>{/if}
						{#if smallCount}<div><dt>{t('music.singlesEps')}</dt><dd>{smallCount}</dd></div>{/if}
						{#if hero.country}<div><dt>{t('music.from')}</dt><dd>{countryName(hero.country)}</dd></div>{/if}
						{#if catalog?.since}<div><dt>{t('music.active')}</dt><dd>{catalog.since} – {hero.status === 'ended' ? (catalog.span?.split('–')[1] ?? '') : t('music.now')}</dd></div>{/if}
						{#if hero.type}<div><dt>{t('music.kind')}</dt><dd>{artistKind(hero.type)}</dd></div>{/if}
					</dl>
				</aside>
				<div class="col-main">
					{#if hero.bio?.extract}<div class="col-bio">{#each hero.bio.extract.split(/\n+/).filter(Boolean) as t, i (i)}<p>{t}</p>{/each}</div>{/if}
					<p class="col-by">{#if picture && !photoFailed}<img src={picture} alt="" />{/if}{hero.name}</p>
					{#if genres}<div class="chips">{#each genres.split(', ') as g (g)}<span>{g}</span>{/each}</div>{/if}
				</div>
			</div>
	</div>
</Sheet>

<style>
	h2 { margin: 0; font: var(--t-title2); }
	.block { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
	.chg.up { color: var(--ok-text); font-weight: 600; }
	.chg.down { color: var(--fail-text); font-weight: 600; }
	:global(.artist.tinted) .chg.up { color: #9df5b4; }
	:global(.artist.tinted) .chg.down { color: #ffb3b3; }
	.about { scroll-margin-top: 80px; }
	.acard { position: relative; display: block; width: 100%; height: clamp(300px, 32vw, 440px); padding: 0; border: 0; border-radius: var(--r-card); overflow: hidden; background: #1c1d21; color: #fff; text-align: left; cursor: pointer; isolation: isolate; box-shadow: 0 0 0 1px rgb(255 255 255 / 8%), 0 24px 60px rgb(0 0 0 / .4); transition: translate var(--dur-slow) var(--ease-out), box-shadow var(--dur-slow) var(--ease-out); }
	.acard:hover { translate: 0 var(--lift); }
	.acard:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.ab-frame { position: absolute; inset: 0; z-index: -2; width: 100%; height: 100%; object-fit: cover; object-position: center 25%; opacity: 0; scale: 1.02; transition: opacity 1.2s var(--ease-out), scale 8s linear; }
	.ab-frame.on { opacity: 1; scale: 1.12; }
	.acard::after { content: ''; position: absolute; inset: 0; z-index: -1; background: linear-gradient(to top, color-mix(in srgb, var(--ar-bg, #000) 70%, #000) 0%, color-mix(in srgb, var(--ar-bg, #000) 35%, transparent) 45%, transparent 75%), linear-gradient(to right, rgb(0 0 0 / .35), transparent 60%); }
	.a-glow { position: absolute; inset: 0; z-index: -1; pointer-events: none; background: radial-gradient(60% 50% at var(--mx, 70%) var(--my, 20%), rgb(255 255 255 / .1), transparent 70%); opacity: 0; transition: opacity var(--dur) var(--ease-out); }
	.acard:hover .a-glow { opacity: 1; }
	.a-none { position: absolute; inset: 0; z-index: -2; overflow: hidden; }
	.disc { position: absolute; top: 20px; left: 20px; display: grid; place-content: center; justify-items: center; width: 92px; height: 92px; border-radius: 50%; background: var(--accent-fill); color: var(--text-on-red); box-shadow: 0 8px 24px rgb(0 0 0 / .3); text-align: center; }
	.disc b { font: 800 26px/1 var(--font-display); }
	.disc small { margin-top: 3px; font: 600 11px/1.1 var(--font-ui); }
	.a-foot { position: absolute; left: 28px; right: 28px; bottom: 26px; display: flex; flex-direction: column; gap: 8px; }
	.a-lis { font: 700 12px/1.3 var(--font-ui); text-transform: uppercase; letter-spacing: .08em; opacity: .8; font-variant-numeric: tabular-nums; }
	.a-bio { max-width: 64ch; color: rgb(255 255 255 / .9); font: 500 19px/1.45 var(--font-ui); text-shadow: 0 1px 12px rgb(0 0 0 / .35); display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
	.a-more { align-self: flex-start; margin-top: 6px; padding: 9px 16px; border-radius: 999px; background: rgb(255 255 255 / 14%); box-shadow: inset 0 0 0 1px rgb(255 255 255 / 16%); backdrop-filter: blur(20px) saturate(160%); font: 600 14px/1 var(--font-ui); transition: background var(--dur-fast) var(--ease-out); }
	.acard:hover .a-more { background: rgb(255 255 255 / 24%); }
	/* Minimal: a plain card in the theme's colours, the text in the flow. */
	.acard.minimal { display: flex; align-items: center; gap: 20px; height: auto; padding: 20px; background: var(--surface-1); color: var(--text); box-shadow: 0 0 0 1px var(--line); }
	.acard.minimal::after { content: none; }
	.acard.minimal:focus-visible { box-shadow: var(--focus-ring); }
	.acard.minimal .disc { position: static; flex: none; box-shadow: none; }
	.acard.minimal .a-foot { position: static; min-width: 0; }
	.acard.minimal .a-bio { color: var(--text); text-shadow: none; }
	.acard.minimal .a-more { background: var(--surface-2); box-shadow: inset 0 0 0 1px var(--line); backdrop-filter: none; -webkit-backdrop-filter: none; }
	.acard.minimal:hover .a-more { background: var(--line); }
	:global(html[data-look='minimal'] .ui-sheet:has(.sheet[data-testid='artist-about-sheet'])) { --text: inherit; --text-muted: inherit; --surface-1: inherit; --surface-2: inherit; --line: inherit; --border: inherit; color-scheme: inherit; color: var(--text); background: var(--surface-1); box-shadow: var(--shadow-modal); }
	:global(html[data-look='minimal']) .chips span { background: var(--surface-2); box-shadow: inset 0 0 0 1px var(--line); }
	:global(html[data-look='minimal']) .col-bio p { color: var(--text); }
	@media (prefers-reduced-motion: reduce) { .acard:hover { translate: none; } .ab-frame { transition: opacity 1.2s; scale: 1; } }
	:global(.ui-sheet:has(.sheet[data-testid='artist-about-sheet'])) { --w: 1040px; max-height: min(92dvh, 980px); --text: #fff; --text-muted: rgb(235 235 245 / .62); --surface-1: rgb(255 255 255 / 7%); --surface-2: rgb(255 255 255 / 12%); --line: rgb(255 255 255 / 10%); --border: rgb(255 255 255 / 10%); color-scheme: dark; color: #fff; background: radial-gradient(120% 70% at 50% 0%, color-mix(in srgb, var(--tint-bg, #2a2c33) 85%, #fff 6%), color-mix(in srgb, var(--tint-bg, #1b1c20) 55%, #050506) 70%); box-shadow: 0 0 0 1px rgb(255 255 255 / 8%), 0 40px 120px rgb(0 0 0 / .6); }
	.sheet { display: flex; flex-direction: column; gap: 24px; }
	.chg { font-style: normal; }
	.chips { display: flex; flex-wrap: wrap; gap: 8px; }
	.chips span { padding: 8px 14px; border-radius: 999px; background: rgb(255 255 255 / 9%); box-shadow: inset 0 0 0 1px rgb(255 255 255 / 8%); font: 600 13px/1 var(--font-ui); text-transform: capitalize; }
	.col-photo { position: relative; margin: 0; height: clamp(260px, 44vh, 420px); border-radius: 18px; overflow: hidden; background: #000; }
	.col-photo img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: contain; opacity: 0; transition: opacity .6s var(--ease-out); }
	.col-photo img.on { opacity: 1; }
	.col-nav { position: absolute; top: 50%; translate: 0 -50%; width: 40px; height: 40px; border: 0; border-radius: 50%; display: grid; place-items: center; background: rgb(0 0 0 / .45); color: #fff; cursor: pointer; backdrop-filter: blur(12px); }
	.col-nav:hover { background: rgb(0 0 0 / .7); }
	.col-nav.prev { left: 14px; } .col-nav.next { right: 14px; }
	.col { display: grid; grid-template-columns: 230px minmax(0, 1fr); gap: 40px; }
	.col-side { display: flex; flex-direction: column; gap: 22px; min-width: 0; }
	.col-disc { display: grid; place-content: center; justify-items: center; width: 84px; height: 84px; border-radius: 50%; background: var(--accent-fill); color: var(--text-on-red); text-align: center; }
	.col-disc b { font: 800 24px/1 var(--font-display); }
	.col-disc small { font: 600 10px/1.1 var(--font-ui); margin-top: 2px; }
	.col-big { margin: 0; display: flex; flex-direction: column; gap: 4px; }
	.col-big b { font: 800 30px/1 var(--font-display); letter-spacing: -.02em; font-variant-numeric: tabular-nums; }
	.col-big span { color: var(--text-muted); font: 500 13px/1.2 var(--font-ui); text-transform: capitalize; }
	.col-list { margin: 6px 0 0; display: flex; flex-direction: column; gap: 14px; }
	.col-list div { display: flex; flex-direction: column; gap: 2px; }
	.col-list dt { order: 2; color: var(--text-muted); font: 500 13px/1.2 var(--font-ui); }
	.col-list dd { order: 1; margin: 0; font: 700 14px/1.3 var(--font-ui); }
	.col-main { display: flex; flex-direction: column; gap: 22px; min-width: 0; }
	.col-bio { display: flex; flex-direction: column; gap: 14px; }
	.col-bio p { margin: 0; color: rgb(235 235 245 / .8); font: 500 16px/1.65 var(--font-ui); max-width: 68ch; }
	.col-by { margin: 0; display: flex; align-items: center; gap: 10px; color: var(--text-muted); font: 500 13px/1 var(--font-ui); }
	.col-by img { width: 28px; height: 28px; border-radius: 50%; object-fit: cover; }
	@media (max-width: 767px) {
		.col { grid-template-columns: minmax(0, 1fr); gap: 28px; }
		.col-list { flex-direction: row; flex-wrap: wrap; gap: 14px 28px; }
		.a-bio { font-size: 16px; }
		.a-foot { left: 18px; right: 18px; bottom: 18px; }
	}
	@media (max-width: 767px) {
		.acard { height: 360px; }
		.disc { width: 76px; height: 76px; top: 14px; left: 14px; }
		.disc b { font-size: 21px; }
		.disc small { font-size: 10px; }
		.a-foot { left: 16px; right: 16px; bottom: 16px; }
		.acard.minimal { height: auto; gap: 14px; padding: 16px; }
	}
</style>
