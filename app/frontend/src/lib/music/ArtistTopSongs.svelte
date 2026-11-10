<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import ArtPlaceholder from '#lib/ui/ArtPlaceholder.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Menu from '#lib/ui/Menu.svelte';
	import type { MenuItem } from '#lib/ui/Menu.svelte';
	import { albumHref, appPicture, coverFor, length, type PageSong } from './music-pages.ts';

	// The artist's top songs: ten, in two columns of five. Each row opens its album.
	let { songs, mbid, items, year, badArt = $bindable() }: { songs: PageSong[]; mbid: string; items: (s: PageSong) => MenuItem[]; year: (s: PageSong) => string; badArt: Set<string> } = $props();
</script>

	<section class="block top" data-testid="top-songs">
		<h2>{t('music.topSongs')}</h2>
		<ol class="songs">
			{#each songs as s, i (s.title + i)}
				{@const yr = year(s)}
				<li class="song">
					{#if s.album_mbid}
						<a class="num" href={albumHref(s.album_mbid, mbid)} aria-label={t('music.open', { name: s.album })}><span class="n">{i + 1}</span><span class="go" aria-hidden="true"><Icon name="arrow" size={16} /></span></a>
					{:else}
						<span class="num"><span class="n">{i + 1}</span></span>
					{/if}
					<span class="cov">{#if s.image && !badArt.has('top:' + s.title)}<img src={coverFor(appPicture(s.image), 48).src} alt="" width="48" height="48" loading={i < 5 ? 'eager' : 'lazy'} decoding="async" onerror={() => (badArt = new Set([...badArt, 'top:' + s.title]))} />{:else}<ArtPlaceholder name={s.title} shape="square" />{/if}</span>
					<span class="s-words">
						<span class="s-name">{#if s.explicit}<span class="e" aria-label={t('music.explicit')}>E</span>{/if}{s.title}</span>
						<span class="s-album">{[s.album, yr].filter(Boolean).join(' · ')}</span>
					</span>
					<span class="s-len">{length(s.length_ms)}</span>
					{#if s.album_mbid}<span class="s-more"><Menu label={t('music.moreFor', { title: s.title })} items={items(s)} width={240} /></span>{:else}<span></span>{/if}
				</li>
			{/each}
		</ol>
	</section>

<style>
	.block { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
	.songs { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); grid-template-rows: repeat(5, auto); grid-auto-flow: column; gap: 0 24px; margin: 0; padding: 0; list-style: none; }
	.song { display: grid; grid-template-columns: 32px 48px minmax(0, 1fr) auto 36px; align-items: center; gap: 12px; height: 64px; padding: 0 8px; border-radius: 10px; transition: background var(--dur-fast) var(--ease-out), translate var(--dur) var(--ease-out), box-shadow var(--dur) var(--ease-out); }
	.song:hover, .song:focus-within { background: var(--surface-2); translate: 0 -1px; box-shadow: var(--shadow-card); }
	.num { position: relative; display: grid; place-items: center; width: 32px; height: 32px; border-radius: 50%; color: var(--text-muted); font: 600 15px/1 var(--font-ui); font-variant-numeric: tabular-nums; }
	.num .go { position: absolute; inset: 0; display: grid; place-items: center; opacity: 0; color: var(--text); transition: opacity var(--dur-fast) var(--ease-out); }
	.song:hover a.num .n, .song:focus-within a.num .n { opacity: 0; }
	.song:hover a.num .go, .song:focus-within a.num .go { opacity: 1; }
	a.num:hover { background: var(--surface-3); }
	a.num:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	/* Touch phones: a centred 44px hit area on the song-number link. */
	@media (max-width: 767px) and (pointer: coarse) { a.num::after { content: ''; position: absolute; left: 50%; top: 50%; width: 44px; height: 44px; translate: -50% -50%; } }
	.cov { position: relative; display: block; flex: none; width: 48px; height: 48px; overflow: hidden; border-radius: 6px; background: var(--surface-2); }
	.cov img { width: 100%; height: 100%; object-fit: cover; }
	.s-words { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
	.s-name { font: var(--t-callout); font-weight: 600; color: var(--text); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.s-album { color: var(--text-muted); font: var(--t-footnote); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.s-len { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.s-more { display: grid; place-items: center; opacity: .6; transition: opacity var(--dur-fast) var(--ease-out); }
	.song:hover .s-more, .song:focus-within .s-more { opacity: 1; }
	@media (hover: none) { .s-more { opacity: 1; } }
	.e { display: inline-grid; place-items: center; width: 15px; height: 15px; margin-right: 6px; border-radius: 3px; background: var(--text-muted); color: var(--bg); font: 700 10px/1 var(--font-ui); vertical-align: 1px; }
	@media (prefers-reduced-motion: reduce) { .song:hover { translate: none; } }
	@media (max-width: 1000px) {
		.songs { grid-template-columns: minmax(0, 1fr); grid-template-rows: none; grid-auto-flow: row; }
	}
	@media (max-width: 767px) {
		.song { grid-template-columns: 24px 48px minmax(0, 1fr) 36px; gap: 10px; padding: 0 4px; }
		.s-len { display: none; }
		.num { width: 24px; }
	}
</style>
