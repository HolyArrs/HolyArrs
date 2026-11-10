<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// Follow toggle for a title or artist, synced with the server.
	import ActionButton from './ActionButton.svelte';
	import { followHint, followLabel, followPath, setFollow, type FollowTarget } from './follow.ts';
	import { api, message } from '../core/api.ts';
	import { toasts } from '../core/toasts.svelte.ts';

	let { titleId, tmdb, kind }: { titleId?: number; tmdb?: string; kind: 'movie' | 'show' | 'artist' } = $props();

	let on = $state(false);
	let busy = $state(false);

	// A local title id wins; otherwise the tmdb id is used, and artists have no target.
	const target = $derived<FollowTarget | null>(titleId ? { titleId } : tmdb ? { tmdb, kind: kind === 'movie' ? 'movie' : 'show' } : null);

	// Reset to off, then load the real state; a failed lookup stays off.
	$effect(() => {
		const t = target;
		on = false;
		if (t) api.get<{ following: boolean }>(followPath(t)).then((r) => (on = r.following), () => {});
	});

	async function flip() {
		busy = true;
		try {
			if (target) on = await setFollow(api, target, !on);
		} catch (err) {
			toasts.push(t('ui.followButton.couldntUpdate'), 'fail', { body: message(err) });
		} finally {
			busy = false;
		}
	}
</script>

<ActionButton icon="bell" pill label={followLabel(on)} title={followHint(kind, on, !titleId)} pressed={on} {busy} onclick={flip} data-testid="follow" />
