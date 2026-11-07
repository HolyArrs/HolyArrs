<script lang="ts">
	import ActionButton from './ActionButton.svelte';
	import { api, message } from '../core/api.ts';
	import { followHint, followLabel, followPath, setFollow, type FollowTarget } from './follow.ts';
	import { toasts } from '../core/toasts.svelte.ts';

	let { titleId, tmdb, kind }: { titleId?: number; tmdb?: string; kind: 'movie' | 'show' | 'artist' } = $props();
	const target = $derived<FollowTarget | null>(titleId ? { titleId } : tmdb ? { tmdb, kind: kind === 'movie' ? 'movie' : 'show' } : null);

	let on = $state(false);
	let busy = $state(false);
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
			toasts.push(`Couldn't update follow`, 'fail', { body: message(err) });
		} finally {
			busy = false;
		}
	}
</script>

<ActionButton icon="bell" pill label={followLabel(on)} title={followHint(kind, on, !titleId)} pressed={on} {busy} onclick={flip} data-testid="follow" />
