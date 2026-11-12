<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import KidMark from './KidMark.svelte';
	import { friendLine, friendsNote, type PlexFriendsView } from './plex-friends.ts';
	import { onMount } from 'svelte';
	// Under Include Plex friends: who is read with the owner's Plex link, when, and what came of it.
	let { on }: { on: boolean } = $props();
	let view = $state<PlexFriendsView | null>(null);
	let error = $state('');
	const note = $derived(friendsNote(view, on));

	onMount(async () => {
		try {
			view = await api.get<PlexFriendsView>('/settings/asks/plex-friends');
		} catch (e) {
			error = message(e);
		}
	});
</script>

{#if on}
	<div class="friends" data-testid="plex-friends-list">
		{#if error}
			<p class="form-error" role="alert">{error}</p>
		{:else if note}
			<p class="hint" data-testid="plex-friends-note">{note}</p>
		{/if}
		{#if view?.owner_linked && view.people.length}
			<ul>
				{#each view.people as f (f.user_id)}
					<li data-testid="plex-friend">
						<span class="who"><b>{f.name}</b>{#if f.kid}<KidMark label={t('accounts.preset.kid')} size="sm" />{/if}</span>
						<span class="xs muted">{friendLine(f)}</span>
						{#if f.error}<span class="xs problem">{serverWords(f.error)}</span>{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</div>
{/if}

<style>
	.friends { padding-inline-start: 28px; }
	ul { margin: 0; padding: 0; list-style: none; }
	li { display: flex; flex-direction: column; gap: 2px; min-width: 0; padding: 8px 0; border-top: 1px solid var(--line); overflow-wrap: anywhere; }
	li:first-child { border-top: 0; }
	.who { display: flex; align-items: center; gap: var(--s-2); min-width: 0; }
	.problem { color: var(--fail-text); }
</style>
