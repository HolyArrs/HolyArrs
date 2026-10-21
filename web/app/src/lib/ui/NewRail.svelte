<script lang="ts">
	// This week: a date rail at the top of New & Popular. Days 3 back to 7
	// ahead, each with a big day word on the left and its titles to the right:
	// what landed (on which service), what came out, which episode aired, and
	// what is coming (with the Follow bell, or "Coming" when it is in the
	// Library already). Scrolls sideways; swipes on phones.
	import { api } from '../api.ts';
	import { setFollow, followLabel, followHint, movieFollowHint } from '../follow.ts';
	import { dayOffset, railBell, railLabel, type RailDay, type RailItem } from '../new-popular.ts';
	import { sized } from '../tmdb-image.ts';
	import Icon from '../Icon.svelte';

	let {
		days,
		href,
		logo
	}: {
		days: RailDay[];
		href: (it: RailItem) => string | undefined;
		logo: (service: number | undefined) => string | undefined;
	} = $props();

	const today = new Date();
	let following = $state(new Map<string, boolean>());
	let busy = $state('');
	const keyOf = (it: RailItem) => `${it.kind}:${it.tmdb_id}`;
	const isOn = (it: RailItem) => following.get(keyOf(it)) ?? it.following;

	async function flip(it: RailItem) {
		const k = keyOf(it);
		if (busy) return;
		busy = k;
		try {
			const on = await setFollow(api, { tmdb: it.tmdb_id, kind: it.kind }, !isOn(it));
			following = new Map([...following, [k, on]]);
		} catch {
			/* the bell stays as it was */
		} finally {
			busy = '';
		}
	}
	const hint = (it: RailItem) => (it.kind === 'movie' ? movieFollowHint(it.name, isOn(it)) : followHint('show', isOn(it), true));
</script>

{#if days.length > 0}
	<section class="rail" aria-labelledby="rail-title" data-testid="new-rail">
		<h2 id="rail-title" class="rail-title">This week</h2>
		<div class="track" role="list">
			{#each days as d (d.day)}
				{@const label = railLabel(d.day, today)}
				{@const off = dayOffset(d.day, today)}
				<div class="day" class:past={off < 0} class:now={off === 0} role="listitem" data-day={d.day}>
					<div class="when" aria-label="{label.big}, {label.small}">
						<span class="big">{label.big}</span>
						<span class="small">{label.small}</span>
					</div>
					<ul class="titles">
						{#each d.items as it (keyOf(it))}
							{@const bell = railBell(it)}
							{@const link = href(it)}
							{@const svc = logo(it.streaming?.service)}
							<li class="t">
								<a class="poster" href={link} aria-label="{it.name}, {it.what}">
									{#if it.poster}<img src={sized(it.poster, 'poster', 104) || it.poster} alt="" loading="lazy" />{:else}<span class="ph">{it.name}</span>{/if}
									{#if svc}<img class="svc" src={svc} alt="" title={it.streaming?.name} loading="lazy" />{/if}
								</a>
								<div class="line">
									<span class="name" title={it.name}>{it.name}</span>
									<span class="what">{it.what}</span>
								</div>
								{#if bell === 'bell'}
									<button type="button" class="bell" class:on={isOn(it)} aria-pressed={isOn(it)} aria-label="{followLabel(isOn(it))} {it.name}" title={hint(it)} disabled={busy === keyOf(it)} onclick={() => void flip(it)} data-testid="rail-follow">
										<Icon name={isOn(it) ? 'bellOn' : 'bell'} size={16} />
									</button>
								{:else if bell === 'coming'}
									<span class="coming">Coming</span>
								{/if}
							</li>
						{/each}
					</ul>
				</div>
			{/each}
		</div>
	</section>
{/if}

<style>
	.rail {
		--rail-bg: color-mix(in srgb, var(--surface-2, #1c1c1e) 72%, transparent);
		--rail-line: color-mix(in srgb, var(--text, #fff) 10%, transparent);
		position: relative;
		border-radius: 20px;
		background: var(--rail-bg);
		-webkit-backdrop-filter: saturate(160%) blur(24px);
		backdrop-filter: saturate(160%) blur(24px);
		box-shadow: 0 0 0 0.5px var(--rail-line), 0 18px 40px rgb(0 0 0 / 0.18);
		padding: 16px 0 14px;
		overflow: hidden;
	}
	.rail-title { margin: 0 0 10px; padding: 0 20px; font: var(--weight-semibold, 600) 13px/1 var(--font-ui); letter-spacing: .14em; text-transform: uppercase; color: var(--text-2, #9a9a9a); }
	.track {
		display: flex;
		gap: 0;
		overflow-x: auto;
		scroll-snap-type: x proximity;
		overscroll-behavior-x: contain;
		scrollbar-width: none;
		padding: 0 12px;
		-webkit-overflow-scrolling: touch;
	}
	.track::-webkit-scrollbar { display: none; }
	.day { flex: none; display: flex; gap: 14px; padding: 4px 16px 4px 8px; scroll-snap-align: start; border-right: 0.5px solid var(--rail-line); }
	.day:last-child { border-right: 0; }
	.when { flex: none; width: 74px; display: flex; flex-direction: column; justify-content: flex-start; gap: 6px; padding-top: 2px; }
	.big { font: 400 30px/.92 Anton, var(--font-display, var(--font-ui)), sans-serif; letter-spacing: .01em; color: var(--text, #fff); text-transform: uppercase; }
	.now .big { color: #ff4a52; }
	.past .big { color: color-mix(in srgb, var(--text, #fff) 62%, transparent); }
	.small { font: 600 11px/1.2 var(--font-ui); color: var(--text-2, #9a9a9a); letter-spacing: .02em; }
	.titles { list-style: none; margin: 0; padding: 0; display: flex; gap: 12px; }
	.t { position: relative; width: 104px; display: flex; flex-direction: column; gap: 6px; }
	.poster { position: relative; display: block; width: 104px; aspect-ratio: 2 / 3; border-radius: 10px; overflow: hidden; background: color-mix(in srgb, var(--text, #fff) 8%, transparent); transition: transform 220ms cubic-bezier(.2,.8,.2,1); }
	.poster:hover, .poster:focus-visible { transform: translateY(-2px); }
	.poster img:not(.svc) { width: 100%; height: 100%; object-fit: cover; display: block; }
	.ph { display: flex; align-items: center; justify-content: center; height: 100%; padding: 8px; text-align: center; font: 600 12px/1.2 var(--font-ui); color: var(--text-2, #9a9a9a); }
	.svc { position: absolute; left: 6px; bottom: 6px; width: 22px; height: 22px; border-radius: 6px; object-fit: cover; box-shadow: 0 2px 6px rgb(0 0 0 / .4); }
	.line { display: flex; flex-direction: column; gap: 2px; min-width: 0; padding-right: 26px; }
	.name { font: 600 12px/1.25 var(--font-ui); color: var(--text, #fff); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.what { font: 500 11px/1.25 var(--font-ui); color: var(--text-2, #9a9a9a); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.bell, .coming { position: absolute; right: 0; bottom: 2px; }
	.bell { width: 24px; height: 24px; display: grid; place-items: center; border: 0; border-radius: 999px; background: color-mix(in srgb, var(--text, #fff) 10%, transparent); color: var(--text, #fff); cursor: pointer; }
	.bell.on { background: #ff4a52; color: #fff; }
	.bell:focus-visible, .poster:focus-visible { outline: 2px solid var(--focus, #0a84ff); outline-offset: 2px; }
	.coming { font: 700 9px/1 var(--font-ui); letter-spacing: .1em; text-transform: uppercase; color: var(--text-2, #9a9a9a); padding: 4px 0; }
	@media (prefers-reduced-motion: reduce) { .poster { transition: none; } .poster:hover, .poster:focus-visible { transform: none; } .track { scroll-behavior: auto; } }
	@media (max-width: 767px) {
		.rail { border-radius: 16px; padding: 12px 0 10px; }
		.rail-title { padding: 0 16px; }
		.when { width: 58px; }
		.big { font-size: 24px; }
		.t, .poster { width: 88px; }
		.track { scroll-snap-type: x mandatory; }
	}
</style>
