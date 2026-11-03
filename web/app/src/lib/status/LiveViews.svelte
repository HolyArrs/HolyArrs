<script lang="ts">
	// The Timeline plate's right half: one live feed, shown in whichever of
	// the ten shapes ?live= picks (preview; one stays once Luis has chosen).
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { LiveJobs } from './livejobs.svelte.ts';
	import { liveViewOf } from './liveviews.ts';
	import type { StatusRow } from './status.ts';
	import LiveRadar from './live/LiveRadar.svelte';
	import LiveArc from './live/LiveArc.svelte';
	import LiveTape from './live/LiveTape.svelte';
	import LivePulse from './live/LivePulse.svelte';
	import LiveDeck from './live/LiveDeck.svelte';
	import LiveGauges from './live/LiveGauges.svelte';
	import LiveDay from './live/LiveDay.svelte';
	import LiveForecast from './live/LiveForecast.svelte';
	import LiveNarrator from './live/LiveNarrator.svelte';
	import LiveMatrix from './live/LiveMatrix.svelte';

	let { upcoming, recent }: { upcoming: StatusRow[]; recent: StatusRow[] } = $props();

	const lj = new LiveJobs();
	$effect(() => lj.learn([...upcoming, ...recent]));
	onMount(() => {
		lj.start();
		return () => lj.stop();
	});
	const view = $derived(liveViewOf(page.url.searchParams.get('live')));
	const views = { radar: LiveRadar, arc: LiveArc, tape: LiveTape, pulse: LivePulse, deck: LiveDeck, gauges: LiveGauges, day: LiveDay, forecast: LiveForecast, narrator: LiveNarrator, matrix: LiveMatrix };
	const View = $derived(views[view]);
</script>

<div class="lv" data-testid="status-live" data-view={view} data-running={lj.running.length}>
	<View {lj} {upcoming} {recent} />
</div>

<style>
	.lv { display: flex; flex-direction: column; min-width: 0; min-height: 100%; }
</style>
