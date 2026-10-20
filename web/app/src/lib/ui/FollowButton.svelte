<script lang="ts">
	// The title page bell: Follow a show or an artist to hear about new
	// episodes, seasons or albums. It asks for nothing.
	import ActionButton from './ActionButton.svelte';
	import { api, message } from '../api.ts';
	import { followHint, followLabel, setFollow } from '../follow.ts';
	import { toasts } from '../toasts.svelte.ts';

	let { titleId, kind }: { titleId: number; kind: 'show' | 'artist' } = $props();

	let on = $state(false);
	let busy = $state(false);
	$effect(() => {
		const id = titleId;
		api.get<{ following: boolean }>(`/titles/${id}/follow`).then((r) => (on = r.following), () => {});
	});

	async function flip() {
		busy = true;
		try {
			on = await setFollow(api, titleId, !on);
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			busy = false;
		}
	}
</script>

<ActionButton icon="bell" pill label={followLabel(on)} title={followHint(kind, on)} pressed={on} {busy} onclick={flip} data-testid="follow" />
