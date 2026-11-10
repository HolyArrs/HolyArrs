<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import { genreTintToken } from './genre-tint.ts';
	import { to } from '../core/paths.ts';

	// The Music genre tiles (Explore's Genres row and the Genres button's picker): a 16:9 tile in the genre's own colour with its artist photos laid over it in grey, crossfading one to the next. Each tile runs on its own random timer.
	type Genre = { key: string; name: string; href: string; art: string; images?: string[]; hue: number; tint?: string };
	let {
		genres,
		allHref,
		compact = false,
		onopen,
		onpick,
		selected,
		title,
		all: withAll = true
	}: {
		genres: Genre[];
		allHref: string;
		compact?: boolean;
		onopen?: () => void;
		/** A pick handled on the page: the genre's key, or null for All genres. */
		onpick?: (key: string | null) => void;
		/** The active genre's key ('' for All genres); undefined marks none. */
		selected?: string;
		/** The row's title (Explore more: Decades, Moods, Charts). */
		title?: string;
		/** Whether the All genres tile shows. */
		all?: boolean;
	} = $props();

	function chosen(e: MouseEvent, key: string | null) {
		if (onpick) {
			e.preventDefault();
			onpick(key);
		}
		onopen?.();
	}
	const tint = (g: Genre) => g.tint ?? `var(${genreTintToken(slugOf(g.key), g.hue)})`;

	// Arrow keys move through the picker's grid: left and right by one, up and down by a row.
	function gridKeys(e: KeyboardEvent) {
		const keys = ['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Home', 'End'];
		if (!keys.includes(e.key)) return;
		const grid = e.currentTarget as HTMLElement;
		const list = [...grid.querySelectorAll<HTMLElement>(':scope > a.tile')];
		const i = list.indexOf(document.activeElement as HTMLElement);
		if (i < 0) return;
		const cols = Math.max(1, list.filter((el) => el.offsetTop === list[0].offsetTop).length);
		const step: Record<string, number> = { ArrowLeft: -1, ArrowRight: 1, ArrowUp: -cols, ArrowDown: cols, Home: -i, End: list.length - 1 - i };
		const to = i + step[e.key];
		if (to < 0 || to >= list.length) return;
		e.preventDefault();
		list[to].focus();
		list[to].scrollIntoView({ block: 'nearest' });
	}

	const slugOf = (key: string) => key.replace(/^genre:/, '');
	const photos = (g: Genre) => (g.images?.length ? g.images : g.art ? [g.art] : []).map((u) => to(u));
	const collage = $derived(genres.slice(0, 4).map((g) => ({ key: g.key, src: photos(g)[0] ?? '', tone: tint(g) })));
	let failedAll = $state<Record<string, boolean>>({});

	// One tile's slideshow, as an action on its element.
	function show(node: HTMLElement, srcs: string[]) {
		const imgs = Array.from(node.querySelectorAll<HTMLImageElement>('img.ph'));
		if (imgs.length < 2) return;
		const ready = imgs.map((_, i) => i === 0);
		const dead = imgs.map(() => false);
		let cur = 0;
		let timer: ReturnType<typeof setTimeout> | undefined;
		let seen = false;
		const reduce = matchMedia('(prefers-reduced-motion: reduce)');
		const awake = () => seen && !document.hidden && !reduce.matches;
		const load = (i: number) => new Promise<boolean>((resolve) => {
			if (ready[i]) return resolve(true);
			if (dead[i]) return resolve(false);
			const pre = new Image();
			pre.src = srcs[i];
			pre.decode().then(() => { imgs[i].src = srcs[i]; ready[i] = true; resolve(true); }, () => { dead[i] = true; resolve(false); });
		});
		const swap = (to: number) => {
			imgs[cur].classList.remove('on');
			imgs[to].classList.add('on');
			cur = to;
		};
		const wait = (first: boolean) => {
			clearTimeout(timer);
			if (!awake()) return;
			// 6 to 8 seconds; the first wait gets a random head start so tiles never change together.
			const ms = 6000 + Math.random() * 2000 + (first ? Math.random() * 6000 : 0);
			timer = setTimeout(async () => {
				for (let step = 1; step < imgs.length; step++) {
					const next = (cur + step) % imgs.length;
					if (await load(next)) { if (awake()) swap(next); break; }
				}
				wait(false);
			}, ms);
		};
		const io = new IntersectionObserver((es) => { seen = es.some((e) => e.isIntersecting); if (seen) wait(true); else clearTimeout(timer); });
		io.observe(node);
		const refresh = () => (awake() ? wait(true) : clearTimeout(timer));
		document.addEventListener('visibilitychange', refresh);
		reduce.addEventListener('change', refresh);
		return { destroy() { clearTimeout(timer); io.disconnect(); document.removeEventListener('visibilitychange', refresh); reduce.removeEventListener('change', refresh); } };
	}
</script>

{#snippet tiles()}
	{#if compact && withAll}{@render all()}{/if}
	{#each genres as g (g.key)}
		{@const srcs = photos(g)}
		<a class="tile" class:bare={!srcs.length} class:on={selected === g.key} aria-current={selected === g.key ? 'page' : undefined} href={g.href} style:--g={tint(g)} onclick={(e) => chosen(e, g.key)}>
			<span class="show" use:show={srcs}>
				{#each srcs as s, i (s)}
					<!-- Only the first photo is requested now; the others load just before their fade. -->
					<img class="ph" class:on={i === 0} src={i === 0 ? s : undefined} alt="" decoding="async" draggable="false" onerror={(e) => ((e.currentTarget as HTMLImageElement).hidden = true)} />
				{/each}
			</span>
			<strong>{g.name}</strong>
		</a>
	{/each}
	{#if !compact && withAll}{@render all()}{/if}
{/snippet}

{#snippet all()}
	<a class="tile all" class:on={selected === ''} aria-current={selected === '' ? 'page' : undefined} href={allHref} onclick={(e) => chosen(e, null)}>
		<span class="quad" aria-hidden="true">
			{#each collage as c (c.key)}
				<span class="q" style:--g={c.tone}>{#if c.src && !failedAll[c.key]}<img src={c.src} alt="" loading="lazy" decoding="async" draggable="false" onerror={() => (failedAll[c.key] = true)} />{/if}</span>
			{/each}
		</span>
		<span class="txt"><small>{t('music.browse')}</small><strong>{t('music.allGenres')}<Icon name="chevronRight" size={compact ? 14 : 18} /></strong></span>
	</a>
{/snippet}

{#if compact}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="mgt compact" data-testid="music-genre-tiles" onkeydown={gridKeys}>{@render tiles()}</div>
{:else}
	<div data-testid="music-genre-tiles"><Shelf title={title ?? t('music.genres')} item="250px">{@render tiles()}</Shelf></div>
{/if}

<style>
	/* Deep hues around the HolyArrs red, one fixed colour per genre. Light mode steps a little brighter. */
	:global(:root) {
		--mg-crimson: #a31d2b; --mg-amber: #9a4a14; --mg-raspberry: #b0245a; --mg-plum: #6a1f5c; --mg-teal: #11585f; --mg-vermilion: #b3341d; --mg-wine: #6e1630;
		--mg-midnight: #2a2f6b; --mg-magenta: #8f1f7a; --mg-forest: #1f5a3a; --mg-indigo: #4b2370; --mg-blue: #1f4f7a; --mg-charcoal: #3b3438; --mg-rust: #7c3a1d;
	}
	:global(:root[data-theme='light']) {
		--mg-crimson: #c0283a; --mg-amber: #b05a1a; --mg-raspberry: #c92c6c; --mg-plum: #82297a; --mg-teal: #17707a; --mg-vermilion: #cc4024; --mg-wine: #8a1f3d;
		--mg-midnight: #3a4190; --mg-magenta: #a82a92; --mg-forest: #26734a; --mg-indigo: #6030a0; --mg-blue: #2a67a0; --mg-charcoal: #4f484c; --mg-rust: #944a24;
	}
	@media (prefers-color-scheme: light) {
		:global(:root:not([data-theme='dark'])) {
			--mg-crimson: #c0283a; --mg-amber: #b05a1a; --mg-raspberry: #c92c6c; --mg-plum: #82297a; --mg-teal: #17707a; --mg-vermilion: #cc4024; --mg-wine: #8a1f3d;
			--mg-midnight: #3a4190; --mg-magenta: #a82a92; --mg-forest: #26734a; --mg-indigo: #6030a0; --mg-blue: #2a67a0; --mg-charcoal: #4f484c; --mg-rust: #944a24;
		}
	}
	/* One grid on every tab: 5 across on a desktop, 3 on a tablet, 2 on a phone. */
	.mgt { display: grid; gap: 10px; grid-template-columns: repeat(5, minmax(0, 1fr)); }
	@media (max-width: 1023px) { .mgt { grid-template-columns: repeat(3, minmax(0, 1fr)); } }
	@media (max-width: 559px) { .mgt { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; } }
	.tile.on { box-shadow: 0 0 0 2px var(--bg, #0b0708), 0 0 0 4px #d0202a; }
	.tile {
		position: relative; isolation: isolate; display: block; aspect-ratio: 16 / 9; overflow: hidden; border-radius: 12px; color: #fff;
		background: linear-gradient(135deg, var(--g, var(--mg-charcoal)), color-mix(in srgb, var(--g, var(--mg-charcoal)) 55%, #000));
		transition: transform var(--dur-slow) var(--ease-out), box-shadow var(--dur-slow) var(--ease-out);
	}
	.show { position: absolute; inset: 0; z-index: -2; }
	/* The server's photos are already grey; they sit over the colour at .55 and trade places by opacity. */
	.ph { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; opacity: 0; transition: opacity var(--dur-hero) var(--ease-out); }
	.ph.on { opacity: .55; }
	/* A scrim from the bottom keeps the label at AA contrast over any photo. */
	.tile::after { content: ''; position: absolute; inset: 0; z-index: -1; background: linear-gradient(0deg, rgba(0, 0, 0, .62), rgba(0, 0, 0, .12) 55%, transparent); }
	strong { position: absolute; left: 12px; right: 10px; bottom: 9px; font: 700 clamp(14px, 1.25vw, 17px) / 1.15 var(--font-ui); letter-spacing: .005em; overflow-wrap: anywhere; display: flex; align-items: center; gap: 2px; }
	.bare strong { font-size: clamp(20px, 1.9vw, 26px); letter-spacing: -.01em; text-transform: capitalize; }
	.compact strong { left: 8px; right: 6px; bottom: 6px; font-size: 12px; overflow-wrap: normal; }
	.tile:is(:hover, :focus-visible) { transform: translateY(-2px); box-shadow: 0 10px 22px -12px rgba(0, 0, 0, .6); }
	.tile:active { transform: scale(.98); }
	.tile:focus-visible { outline: none; box-shadow: 0 0 0 2px var(--bg, #0b0708), 0 0 0 4px #d0202a; }
	.all { background: #120a0b; }
	.all::after { background: linear-gradient(0deg, rgba(0, 0, 0, .78), rgba(0, 0, 0, .45)); }
	.quad { position: absolute; inset: 0; z-index: -2; display: grid; grid-template-columns: 1fr 1fr; grid-template-rows: 1fr 1fr; gap: 2px; background: #120a0b; }
	.q { position: relative; overflow: hidden; background: linear-gradient(135deg, var(--g), color-mix(in srgb, var(--g) 55%, #000)); }
	.q img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; opacity: .55; }
	.txt { position: absolute; left: 12px; right: 10px; bottom: 9px; display: flex; flex-direction: column; gap: 4px; }
	.txt strong { position: static; }
	small { font: 600 10px/1 var(--font-ui); letter-spacing: .14em; text-transform: uppercase; color: #ffc9cb; }
	@media (prefers-reduced-motion: reduce) {
		.tile, .ph { transition: none; }
		.tile:is(:hover, :focus-visible), .tile:active { transform: none; }
	}
</style>
