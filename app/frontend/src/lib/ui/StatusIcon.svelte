<script lang="ts" module>
	// Animated status glyphs, one per request state.
	export type StatusKey = 'pending' | 'upcoming' | 'searching' | 'downloading' | 'importing' | 'processing' | 'ready' | 'failed' | 'declined';
	type Glyph = { fill?: string; d: string; move?: string; mark?: string };
	// fill is the faint backing shape, d the outline, move the part that animates, mark the check that draws in
	const CIRCLE = 'M12 2a10 10 0 1 1 0 20 10 10 0 0 1 0-20z';
	const glyphs: Record<StatusKey, Glyph> = {
		pending: { fill: CIRCLE, d: CIRCLE, move: 'M12 6v6l4 2' },
		upcoming: { fill: 'M5 4h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z', d: 'M5 4h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2zM16 2v4M8 2v4M3 10h18' },
		searching: { d: '', move: 'M11 3a8 8 0 1 1 0 16 8 8 0 0 1 0-16zM21 21l-4.3-4.3', fill: 'M11 3a8 8 0 1 1 0 16 8 8 0 0 1 0-16z' },
		downloading: { fill: 'M3 15h18v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z', d: 'M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4', move: 'M12 3v12M7 10l5 5 5-5' },
		importing: {
			fill: 'M21 8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z',
			d: 'M21 8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16zM7.5 4.27l9 5.15M3.3 7l8.7 5 8.7-5M12 22V12'
		},
		processing: { d: '', move: 'M21 12a9 9 0 1 1-6.219-8.56' },
		ready: { fill: CIRCLE, d: CIRCLE, mark: 'M9 12l2 2 4-4' },
		failed: { fill: CIRCLE, d: CIRCLE + 'M12 8v4M12 16h.01' },
		declined: { fill: CIRCLE, d: CIRCLE + 'M15 9l-6 6M9 9l6 6' }
	};
	// Unknown words fall back to the busy spinner rather than a blank icon.
	export function statusKey(word: string): StatusKey {
		const w = word.split(' · ')[0].trim().toLowerCase();
		return (w in glyphs ? w : 'processing') as StatusKey;
	}
</script>

<script lang="ts">
	let { status, size = 16, motion = true }: { status: StatusKey; size?: number; motion?: boolean } = $props();
	const g = $derived(glyphs[status]);
</script>

<svg class="si si-{status}" class:si-motion={motion} width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
	{#if g.fill}<path class="si-fill" class:si-move={status === 'searching'} d={g.fill} fill="currentColor" stroke="none" />{/if}
	{#if g.d}<path d={g.d} />{/if}
	{#if g.move}<path class="si-move" d={g.move} />{/if}
	{#if g.mark}<path class="si-mark" d={g.mark} pathLength="1" />{/if}
</svg>

<style>
	.si { display: block; flex: none; overflow: visible; }
	.si-fill { opacity: 0.2; }
	/* view-box origin lets the glyph part spin around the icon centre */
	.si-move { transform-box: view-box; transform-origin: 12px 12px; }
	/* motion only for people who have not asked to reduce it */
	@media (prefers-reduced-motion: no-preference) {
		.si-motion.si-downloading .si-move { animation: si-nudge 1.6s cubic-bezier(0.45, 0, 0.55, 1) infinite; }
		.si-motion.si-searching .si-move { transform-origin: 11px 11px; animation: si-sweep 2.4s ease-in-out infinite; }
		.si-motion.si-processing .si-move { animation: si-spin 1.6s linear infinite; }
		.si-motion.si-ready .si-mark { stroke-dasharray: 1; stroke-dashoffset: 1; animation: si-draw 520ms cubic-bezier(0.22, 1, 0.36, 1) 120ms forwards; }
	}
	@keyframes si-nudge { 0%, 100% { transform: translateY(-1px); } 50% { transform: translateY(1.5px); } }
	@keyframes si-sweep { 0%, 100% { transform: translate(-1px, 0) rotate(-8deg); } 50% { transform: translate(1px, 0.5px) rotate(8deg); } }
	@keyframes si-spin { to { transform: rotate(360deg); } }
	@keyframes si-draw { to { stroke-dashoffset: 0; } }
</style>
