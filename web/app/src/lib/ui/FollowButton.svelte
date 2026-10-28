<script lang="ts">
	// The title page bell: Follow a movie, a show or an artist to see its
	// dates on your calendar and hear about new episodes, seasons or albums.
	// It asks for nothing.
	import ActionButton from './ActionButton.svelte';
	import { api, message } from '../api.ts';
	import { followHint, followLabel, followPath, setFollow, type FollowTarget } from '../follow.ts';
	import { toasts } from '../toasts.svelte.ts';

	// A movie or show by its TMDB id: one in the Library is followed as its title; one
	// not in it yet waits, and you hear when it lands.
	let { titleId, tmdb, kind }: { titleId?: number; tmdb?: string; kind: 'movie' | 'show' | 'artist' } = $props();
	const target = $derived<FollowTarget | null>(titleId ? { titleId } : tmdb ? { tmdb, kind: kind === 'movie' ? 'movie' : 'show' } : null);

	let on = $state(false);
	let busy = $state(false);
	$effect(() => {
		const t = target;
		if (t) api.get<{ following: boolean }>(followPath(t)).then((r) => (on = r.following), () => {});
	});

	async function flip() {
		busy = true;
		try {
			if (target) on = await setFollow(api, target, !on);
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			busy = false;
		}
	}
</script>

<ActionButton icon="bell" pill label={followLabel(on)} title={followHint(kind, on, !titleId)} pressed={on} {busy} onclick={flip} data-testid="follow" />
