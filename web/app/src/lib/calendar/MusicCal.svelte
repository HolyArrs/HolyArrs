<script lang="ts">
	import type { CalendarEntry } from '#lib/types.ts';
	import { to } from '#lib/paths.ts';
	import { entryDay, entryHref, itemDot } from '#lib/calendar.ts';
	import { sized } from '#lib/tmdb-image.ts';
	import ArtPlaceholder from '#lib/ui/ArtPlaceholder.svelte';

	// The calendar for music only, in three layouts Luis is comparing:
	// 1 Fridays: one heading per release day and a grid of square covers (new releases).
	// 2 Shelves: one row per release day that scrolls sideways (the music home look).
	// 3 Radar: one list line per release, small cover, name, artist, kind (a release radar).
	// Each day keeps data-day so the page's scrolling (today, arrows, small month) still works.
	let { entries, today, layout }: { entries: CalendarEntry[]; today: string; layout: 1 | 2 | 3 } = $props();

	const days = $derived.by(() => {
		const by = new Map<string, CalendarEntry[]>();
		for (const e of entries) {
			const d = entryDay(e);
			by.set(d, [...(by.get(d) ?? []), e]);
		}
		return [...by.entries()].sort(([a], [b]) => a.localeCompare(b)).map(([day, list]) => ({ day, list }));
	});
	const at = (day: string) => new Date(day + 'T00:00:00');
	const dayName = (day: string) => at(day).toLocaleDateString(undefined, { weekday: 'long', month: 'short', day: 'numeric' });
	const kindWord = (e: CalendarEntry) => (e.album_type ? e.album_type[0].toUpperCase() + e.album_type.slice(1).toLowerCase() : 'Album');
	const title = (e: CalendarEntry) => e.album ?? e.name;
	const have = (e: CalendarEntry) => itemDot(e, today) === 'have';
	let failed = $state<string[]>([]);
</script>

{#snippet cover(e: CalendarEntry, size: number)}
	{@const src = e.poster ? sized(e.poster, 'poster', size) : ''}
	{#if src && !failed.includes(src)}<img {src} alt="" loading="lazy" decoding="async" onerror={() => (failed = [...failed, src])} />{:else}<ArtPlaceholder name={title(e)} shape="square" />{/if}
{/snippet}

<div class="mc mc-{layout}" data-testid="cal-music">
	{#each days as d (d.day)}
		<section class="day" class:today={d.day === today} class:past={d.day < today} data-day={d.day}>
			<header class="dh">
				<h3>{d.day === today ? 'Today' : dayName(d.day)}</h3>
				<span>{d.list.length} {d.list.length === 1 ? 'release' : 'releases'}</span>
			</header>
			{#if layout === 3}
				<ol class="radar">
					{#each d.list as e (e.part_id ?? e.title_id + title(e))}
						<li>
							<a href={to(entryHref(e))}>
								<span class="rc">{@render cover(e, 120)}</span>
								<span class="rw"><b>{title(e)}</b><small>{e.name}</small></span>
								<span class="rk">{kindWord(e)}</span>
								{#if have(e)}<span class="rh">In Library</span>{/if}
							</a>
						</li>
					{/each}
				</ol>
			{:else}
				<ul class="covers">
					{#each d.list as e (e.part_id ?? e.title_id + title(e))}
						<li>
							<a href={to(entryHref(e))}>
								<span class="pic">{@render cover(e, 400)}<i class="kind">{kindWord(e)}</i>{#if have(e)}<i class="have" title="In Library"></i>{/if}</span>
								<b>{title(e)}</b>
								<small>{e.name}</small>
							</a>
						</li>
					{/each}
				</ul>
			{/if}
		</section>
	{/each}
</div>

<style>
	.mc { display: flex; flex-direction: column; gap: 40px; }
	.dh { display: flex; align-items: baseline; gap: 12px; margin: 0 0 16px; padding-bottom: 10px; border-bottom: 1px solid var(--line); }
	.dh h3 { margin: 0; font: 800 26px/1.1 var(--font-display); letter-spacing: -.02em; }
	.dh span { color: var(--text-muted); font: 600 14px/1 var(--font-ui); }
	.today .dh h3 { color: var(--accent); }
	.past { opacity: .7; }
	a { color: var(--text); text-decoration: none; }

	/* Fridays: a grid of square covers. */
	.covers { display: grid; grid-template-columns: repeat(auto-fill, minmax(176px, 1fr)); gap: 24px 18px; margin: 0; padding: 0; list-style: none; }
	.covers a { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
	.pic { position: relative; display: block; aspect-ratio: 1; margin-bottom: 8px; overflow: hidden; border-radius: 10px; background: var(--surface-2); box-shadow: var(--shadow-card); }
	.pic img { width: 100%; height: 100%; object-fit: cover; transition: scale var(--dur-slow) var(--ease-out); }
	.covers a:hover img { scale: 1.04; }
	.kind { position: absolute; left: 8px; bottom: 8px; padding: 3px 8px; border-radius: 999px; background: rgb(0 0 0 / .55); backdrop-filter: blur(10px); color: #fff; font: 700 11px/1.2 var(--font-ui); font-style: normal; }
	.have { position: absolute; top: 8px; right: 8px; width: 12px; height: 12px; border-radius: 50%; background: #34c759; box-shadow: 0 0 0 2px rgb(0 0 0 / .4); }
	.covers b, .rw b { font: 700 14px/1.25 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.covers small, .rw small { color: var(--text-muted); font: var(--t-footnote); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

	/* Shelves: the same covers in one sideways row per day. */
	.mc-2 .covers { display: flex; gap: 16px; overflow-x: auto; padding-bottom: 8px; scroll-snap-type: x mandatory; scrollbar-width: none; }
	.mc-2 .covers li { flex: none; width: 180px; scroll-snap-align: start; }

	/* Radar: one line per release. */
	.radar { margin: 0; padding: 0; list-style: none; display: grid; grid-template-columns: repeat(auto-fill, minmax(420px, 1fr)); gap: 4px 24px; }
	.radar a { display: grid; grid-template-columns: 56px minmax(0, 1fr) auto auto; align-items: center; gap: 14px; padding: 8px; border-radius: 12px; transition: background var(--dur-fast) var(--ease-out); }
	.radar a:hover { background: var(--surface-1); }
	.rc { display: block; width: 56px; height: 56px; overflow: hidden; border-radius: 6px; background: var(--surface-2); }
	.rc img { width: 100%; height: 100%; object-fit: cover; }
	.rw { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
	.rk { color: var(--text-muted); font: 600 12px/1 var(--font-ui); text-transform: uppercase; letter-spacing: .06em; }
	.rh { padding: 4px 8px; border-radius: 999px; background: rgb(52 199 89 / 18%); color: #9df5b4; font: 700 11px/1 var(--font-ui); }

	@media (max-width: 767px) {
		.mc { gap: 28px; }
		.dh h3 { font-size: 21px; }
		.covers { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 18px 12px; }
		.mc-2 .covers { display: flex; }
		.mc-2 .covers li { width: 148px; }
		.radar { grid-template-columns: minmax(0, 1fr); }
		.radar a { grid-template-columns: 48px minmax(0, 1fr) auto; }
		.rc { width: 48px; height: 48px; }
		.rk { display: none; }
	}
</style>
