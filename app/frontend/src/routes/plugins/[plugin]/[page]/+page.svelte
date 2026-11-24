<script lang="ts">
	// A plugin's own page: a sandboxed frame with an opaque origin and a strict policy. It reaches HolyArrs
	// only by postMessage; each call runs as the plugin and needs both its scope and your own permission.
	// Video is the app's own: the page says play({file_id}) and HolyArrs shows its player over the page with
	// your own sign-in, sending back events (time, ended, error). The page never gets a stream address.
	import { page } from '$app/state';
	import { api } from '#lib/core/api.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import IconButton from '#lib/ui/IconButton.svelte';
	import { bridgeCall, playerCall, pluginPages, type PlayerCall } from '#lib/ui/plugin-cards.ts';

	const plugin = $derived(page.params.plugin ?? '');
	const pageId = $derived(page.params.page ?? '');
	let src = $state('');
	let title = $state('');
	let failed = $state(false);
	let frame = $state<HTMLIFrameElement>();
	let video = $state<HTMLVideoElement>();
	let closeBtn = $state<HTMLElement>();
	let streamUrl = $state('');
	let lastTick = 0;

	$effect(() => {
		const pl = plugin;
		const pg = pageId;
		failed = false;
		src = '';
		streamUrl = '';
		void pluginPages().then((ps) => (title = ps.find((p) => p.plugin === pl && p.id === pg)?.title ?? pg));
		api.post<{ url: string }>(`/plugin-pages/${encodeURIComponent(pl)}/${encodeURIComponent(pg)}/open`)
			.then((r) => (src = r.url))
			.catch(() => (failed = true));
	});

	function tell(msg: Record<string, unknown>) {
		frame?.contentWindow?.postMessage(msg, '*');
	}
	function event(type: string, extra: Record<string, unknown> = {}) {
		tell({ player: 'event', type, ...extra });
	}
	function closePlayer() {
		if (!streamUrl) return;
		streamUrl = '';
		event('closed');
		frame?.focus();
	}

	async function runPlayer(call: PlayerCall) {
		const reply = (status: number) => tell({ id: call.id, status, body: null });
		if (call.op === 'play') {
			try {
				const r = await api.post<{ url: string }>(`/plugin-pages/${encodeURIComponent(plugin)}/play`, { file_id: call.file_id });
				streamUrl = r.url;
				reply(200);
				queueMicrotask(() => closeBtn?.focus());
			} catch {
				reply(403);
				event('error', { reason: 'refused' });
			}
			return;
		}
		if (call.op === 'close') closePlayer();
		else if (video && call.op === 'pause') video.pause();
		else if (video && call.op === 'resume') void video.play().catch(() => {});
		else if (video && call.op === 'seek' && call.time !== undefined) video.currentTime = call.time;
		reply(200);
	}

	$effect(() => {
		const on = (e: MessageEvent) => {
			const win = frame?.contentWindow;
			const play = playerCall(e, win);
			if (play) return void runPlayer(play);
			const call = bridgeCall(e, win);
			if (!call || !win) return;
			api.post<{ status: number; body: unknown }>(`/plugin-pages/${encodeURIComponent(plugin)}/call`, { method: call.method, path: call.path, body: call.body })
				.then((r) => win.postMessage({ id: call.id, status: r.status, body: r.body }, '*'))
				.catch(() => win.postMessage({ id: call.id, status: 403, body: null }, '*'));
		};
		window.addEventListener('message', on);
		return () => window.removeEventListener('message', on);
	});

	function onKey(e: KeyboardEvent) {
		if (streamUrl && e.key === 'Escape') {
			e.preventDefault();
			closePlayer();
		}
	}
	function onTime() {
		const now = Date.now();
		if (!video || now - lastTick < 1000) return;
		lastTick = now;
		event('time', { time: video.currentTime, duration: Number.isFinite(video.duration) ? video.duration : 0 });
	}
</script>

<svelte:head><title>{title || plugin}</title></svelte:head>
<svelte:window onkeydown={onKey} />

<div class="wrap" data-testid="plugin-page">
	{#if failed}
		<p class="ended">{t('plugins.pageEnded')}</p>
	{:else if src}
		<iframe bind:this={frame} {src} title={title || plugin} sandbox="allow-scripts allow-forms" referrerpolicy="no-referrer" allow=""></iframe>
	{/if}
</div>

{#if streamUrl}
	<div class="player" role="dialog" aria-modal="true" aria-label={title || plugin} data-testid="plugin-player">
		<div class="box">
			<div class="bar"><IconButton bind:node={closeBtn} icon="x" label={t('accounts.security.close')} size="lg" onclick={closePlayer} data-testid="plugin-player-close" /></div>
			<!-- svelte-ignore a11y_media_has_caption -->
			<video
				bind:this={video}
				src={streamUrl}
				controls
				autoplay
				playsinline
				preload="auto"
				onplay={() => event('play')}
				onpause={() => event('pause')}
				ontimeupdate={onTime}
				onended={() => event('ended')}
				onerror={() => event('error', { reason: 'media' })}
			></video>
		</div>
	</div>
{/if}

<style>
	.wrap { display: flex; flex-direction: column; min-height: calc(100dvh - 120px); padding: 0 16px; }
	iframe { flex: 1; width: 100%; min-height: 70dvh; border: 0; border-radius: 16px; background: var(--surface-1); }
	.ended { color: var(--text-muted); padding: 24px 0; }
	.player { position: fixed; inset: 0; z-index: 50; display: grid; place-items: center; padding: 16px; background: color-mix(in srgb, #000 78%, transparent); }
	.box { width: min(100%, 960px); display: flex; flex-direction: column; gap: 8px; }
	.bar { display: flex; justify-content: flex-end; }
	video { width: 100%; max-height: calc(100dvh - 96px); border-radius: 12px; background: #000; }
</style>
