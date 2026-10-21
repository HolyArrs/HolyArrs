<script lang="ts">
	// A person circle placeholder (the PersonCircle size): circle, name, role.
	let { size = 96 }: { size?: number } = $props();
</script>

<div class="sk-person" style="--size: {size}px" aria-hidden="true">
	<span class="sk c"></span>
	<span class="sk sk-line n"></span>
	<span class="sk sk-line r"></span>
</div>

<style>
	.sk-person { display: flex; flex-direction: column; align-items: center; gap: 6px; width: calc(var(--size) + 16px); }
	.c { width: var(--size); height: var(--size); border-radius: 50%; margin-bottom: 2px; }
	.n { width: 80%; height: 12px; }
	.r { width: 55%; height: 10px; }
</style>
