<!-- A sentence whose ltr() parts (paths, folder and file names) render in their own left-to-right element: the isolate marks alone do not keep a path's slashes in place in right-to-left text. -->
<script lang="ts">
	let { text }: { text: string } = $props();
	const marks = new RegExp('[' + String.fromCharCode(0x2066, 0x2069) + ']');
	const parts = $derived(text.split(marks));
</script>

{#each parts as part, i (i)}{#if i % 2}<bdi dir="ltr">{part}</bdi>{:else}{part}{/if}{/each}
