<script lang="ts">
	// The title page bell: Follow a show or an artist to hear about new
	// episodes, seasons or albums. It asks for nothing.
	import ActionButton from './ActionButton.svelte';
	import { api, message } from '../api.ts';
	import { followHint, followLabel, followPath, setFollow, type FollowTarget } from '../follow.ts';
	import { toasts } from '../toasts.svelte.ts';

	// A show not in the Library is followed by its TMDB id: you hear when it lands.
	let { titleId, tmdb, kind }: { titleId?: number; tmdb?: string; kind: 'show' | 'artist' } = $props();
	const target = $derived<FollowTarget | null>(titleId ? { titleId } : tmdb ? { tmdb } : null);

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
