<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import { to } from '#lib/paths.ts';
	import type { AccountModel } from '../account.svelte.ts';

	// The shows and artists I follow, each with Unfollow.
	let { m }: { m: AccountModel } = $props();
</script>

{#if m.follows.length}
	<ul class="follows" data-testid="follows">
		{#each m.follows as f (f.title_id)}
			<li>
				<a href={to(`/library/${f.title_id}`)}>{f.name}{#if f.year}&nbsp;({f.year}){/if}</a>
				<span class="small muted">{f.kind === 'artist' ? 'Artist' : 'Show'}</span>
				<Button variant="plain" size="sm" onclick={() => m.unfollow(f)}>Unfollow</Button>
			</li>
		{/each}
	</ul>
{:else}
	<p class="small muted">You follow nothing yet. Follow from a title page.</p>
{/if}

<style>
	.follows { display: flex; flex-direction: column; }
	.follows li { display: flex; align-items: center; gap: var(--s-3); min-height: 44px; border-bottom: 1px solid var(--line); }
	.follows li:last-child { border-bottom: 0; }
	.follows a { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: 500; }
	.follows a:hover { text-decoration: underline; }
</style>
