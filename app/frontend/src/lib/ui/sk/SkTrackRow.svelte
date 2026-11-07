<script lang="ts">
	// A track or episode row placeholder: number, name and a short end column.
	let { height = 48 }: { height?: number } = $props();
</script>

<div class="sk-track" style:min-height="{height}px" aria-hidden="true">
	<span class="sk sk-line n"></span>
	<span class="sk sk-line t"></span>
	<span class="sk sk-line d"></span>
</div>

<style>
	.sk-track { display: flex; align-items: center; gap: 14px; padding: 0 12px; border-bottom: 1px solid var(--line); }
	.n { width: 18px; flex: none; }
	.t { flex: 1; max-width: 360px; }
	.d { width: 40px; margin-left: auto; flex: none; }
</style>
