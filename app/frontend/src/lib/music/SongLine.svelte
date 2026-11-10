<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { tmdbSized } from '#lib/ui/library-art.ts';
	import { coverFor } from './music-pages.ts';
	// A song line: 56 tall with 44 px art, title and artist, the explicit "E", and one action, Ask album (HolyArrs asks for albums; it does not play).
	let { title, artist, image = null, explicit = false, href, askHref }: { title: string; artist: string; image?: string | null; explicit?: boolean; href: string; askHref: string } = $props();
</script>

<div class="ui-song" data-testid="song-line">
	<a class="main" {href}>
		<span class="art">{#if image}<img src={tmdbSized(coverFor(image, 44).src, 'poster', 44)} alt="" loading="lazy" decoding="async" />{/if}</span>
		<span class="words">
			<span class="title">{title}</span>
			<span class="by">{#if explicit}<span class="e" aria-label={t('music.explicit')}>E</span>{/if}{artist}</span>
		</span>
	</a>
	<a class="ask" href={askHref}>{t('music.askAlbum')}</a>
</div>

<style>
	.ui-song { display: flex; align-items: center; gap: 8px; height: 56px; padding: 0 8px; border-radius: 7px; min-width: 0; }
	.ui-song:hover { background: rgba(255, 255, 255, 0.12); }
	.main { display: flex; align-items: center; gap: 12px; min-width: 0; flex: 1; color: var(--text); text-decoration: none; }
	.art { flex: none; width: 44px; height: 44px; border-radius: 4px; overflow: hidden; background: var(--surface-3); }
	.art img { width: 100%; height: 100%; object-fit: cover; }
	.words { display: flex; flex-direction: column; min-width: 0; }
	.title, .by { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.title { font-size: 14px; font-weight: 600; }
	.by { font-size: 13px; color: var(--text-muted); display: flex; align-items: center; gap: 6px; }
	.e { flex: none; display: inline-grid; place-items: center; width: 14px; height: 14px; border-radius: 2px; background: var(--text-muted); color: var(--surface-1, #000); font-size: 9px; font-weight: 700; }
	.ask { flex: none; font-size: 12px; font-weight: 600; color: var(--text); text-decoration: none; padding: 6px 10px; border-radius: 999px; border: 1px solid var(--border, rgb(255 255 255 / 0.2)); }
	.ask:focus-visible, .main:focus-visible { outline: none; box-shadow: var(--focus-ring); }
</style>
