<script lang="ts">
	// The Timeline plate's right half: the live job feed drawn as a day column.
	import LiveDay from './LiveDay.svelte';
	import { LiveJobs } from './live-jobs.svelte.ts';
	import type { StatusRow } from './rows.ts';
	import { onMount } from 'svelte';

	let { upcoming, recent }: { upcoming: StatusRow[]; recent: StatusRow[] } = $props();

	const lj = new LiveJobs();
	$effect(() => lj.learn([...upcoming, ...recent]));
	onMount(() => {
		lj.start();
		return () => lj.stop();
	});
</script>

<div class="lv" data-testid="status-live" data-running={lj.running.length}>
	<LiveDay {lj} {upcoming} {recent} />
</div>

<style>
	.lv { display: flex; flex-direction: column; min-width: 0; min-height: 100%; }
</style>
