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
		<p class="eyebrow">Library management</p>
		<h1>Rename files</h1>
		<p>
			Every file in the Library whose name does not follow the naming pattern, with its name now and the name it gets. Nothing changes until you press
			Rename.{#if session.can('settings')}{' '}The patterns are in <a href={to(`/settings/naming`)}>Settings &gt; Naming and files</a>.{/if}
		</p>
	</div>
</header>

<section class="card rename-workspace" aria-label="Rename preview">
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
	.eyebrow { margin: 0 0 var(--s-2); font-size: 11px; font-weight: var(--weight-semibold); letter-spacing: .09em; text-transform: uppercase; color: var(--text-muted); }
	.rename-workspace { overflow: hidden; box-shadow: none; }
	.rename-workspace :global(.btn) { min-height: 40px; }
	@media (max-width: 600px) {
		.back { min-height: 44px; }
		.rename-workspace :global(.btn) { min-height: 44px; }
	}
</style>
