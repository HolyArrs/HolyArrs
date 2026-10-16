<script lang="ts">
	import { to } from '#lib/paths.ts';
	import Icon from '#lib/Icon.svelte';
	import RenamePreview from '#lib/RenamePreview.svelte';
	import { session } from '#lib/session.svelte.ts';
</script>

<svelte:head><title>Rename files · HolyArrs</title></svelte:head>

<a class="back small muted" href={to(`/library`)}><Icon name="back" size={16} /> Library</a>

<header class="page-head">
	<div>
		<h1>Rename files</h1>
		<p>
			Every file in the Library whose name does not follow the naming pattern, with its name now and the name it gets. Nothing changes until you press
			Rename.{#if session.can('settings')}{' '}The patterns are in <a href={to(`/settings/naming`)}>Settings &gt; Naming and files</a>.{/if}
		</p>
	</div>
</header>

<section class="card">
	<RenamePreview />
</section>

<style>
	.back {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		min-height: 40px;
		margin-bottom: var(--s-2);
	}
</style>
