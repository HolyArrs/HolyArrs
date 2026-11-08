<script lang="ts">
	// Draws a QR code the server made (rows of "1" dark, "0" light) as one SVG path, with the 4-module quiet zone phones need to find it.
	let { rows, label }: { rows: string[]; label: string } = $props();
	const quiet = 4;
	const size = $derived(rows.length + quiet * 2);
	const d = $derived(
		rows
			.flatMap((row, y) => [...row].map((c, x) => (c === '1' ? `M${x + quiet} ${y + quiet}h1v1h-1z` : '')))
			.join('')
	);
</script>

<svg class="qr" viewBox="0 0 {size} {size}" role="img" aria-label={label} shape-rendering="crispEdges">
	<rect width={size} height={size} fill="#fff" />
	<path {d} fill="#000" />
</svg>

<style>
	/* Black on white in every theme: phone cameras read that best. */
	.qr { display: block; width: min(100%, 220px); height: auto; border-radius: 6px; }
</style>
