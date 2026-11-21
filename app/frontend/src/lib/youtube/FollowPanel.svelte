<script lang="ts">
	// The person's follows: quality, keep-last, Shorts, mirroring a playlist, what the first download took, the last check, pause and unfollow (keeping or deleting the videos).
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { message } from '#lib/core/api.ts';
	import { ago } from '#lib/core/format.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { ask } from '#lib/ui/ask.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import { ytEditFollow, ytUnfollow, type FollowEdit } from './api.ts';
	import { ALL_VIDEOS, type YtFollow } from './view.ts';

	let {
		open = $bindable(false),
		follows,
		focus = null,
		onchanged,
		onunfollowed,
		onshow
	}: {
		open?: boolean;
		follows: YtFollow[];
		focus?: number | null;
		onchanged: (f: YtFollow) => void;
		onunfollowed: (id: number, files: 'keep' | 'delete') => void;
		onshow: (id: number) => void;
	} = $props();
	const id = $props.id();

	const heights = [2160, 1440, 1080, 720, 480, 360];
	let busy = $state<number | null>(null);
	let asking = $state<number | null>(null);

	// The follow a duplicate link pointed at comes into view.
	$effect(() => {
		if (!open || focus === null) return;
		requestAnimationFrame(() => document.getElementById(`${id}-f${focus}`)?.scrollIntoView({ block: 'nearest' }));
	});

	function firstWords(f: YtFollow): string {
		return f.mirror || f.backfill === null || f.backfill >= ALL_VIDEOS ? t('youtube.follows.firstAll') : t('youtube.follows.firstNewest', { n: f.backfill });
	}

	async function edit(f: YtFollow, change: FollowEdit) {
		busy = f.id;
		try {
			onchanged(await ytEditFollow(f.id, change));
			toasts.push(t('youtube.follows.saved'), 'ok');
		} catch (e) {
			toasts.push(t('youtube.follows.saveFailed'), 'fail', { body: message(e) });
			// Puts the controls back to what is saved.
			onchanged({ ...f });
		} finally {
			busy = null;
		}
	}

	// Turning mirroring on can delete videos at the next check, so it asks first; turning it off never deletes anything.
	async function setMirror(f: YtFollow, on: boolean) {
		if (on && !(await ask(t('youtube.follows.mirrorAsk', { name: f.title }), { yes: t('youtube.confirm.mirror'), danger: true }))) return onchanged({ ...f });
		await edit(f, { mirror: on });
	}

	function keepNumber(f: YtFollow, raw: string) {
		const n = Math.round(Number(raw));
		if (!Number.isFinite(n) || n < 1 || n === f.keep_last) return onchanged({ ...f });
		void edit(f, { keep_last: n });
	}

	async function unfollow(f: YtFollow, files: 'keep' | 'delete') {
		busy = f.id;
		try {
			await ytUnfollow(f.id, files);
			asking = null;
			onunfollowed(f.id, files);
			toasts.push(t('youtube.follows.unfollowed', { name: f.title }), 'ok');
		} catch (e) {
			toasts.push(t('youtube.follows.saveFailed'), 'fail', { body: message(e) });
		} finally {
			busy = null;
		}
	}
</script>

<Sheet bind:open title={t('youtube.follows.title')} size="md">
	{#if follows.length === 0}
		<p class="muted">{t('youtube.follows.none')}</p>
	{:else}
		<ul class="rows follows" data-testid="youtube-follows">
			{#each follows as f (f.id)}
				<li id="{id}-f{f.id}" class:focus={focus === f.id} data-testid="youtube-follow">
					<div class="grow stack-s">
						<div class="head">
							<b class="ellipsis">{f.title}</b>
							<span class="small muted">{f.kind === 'channel' ? t('youtube.follows.channel') : t('youtube.follows.playlist')}</span>
							{#if !f.enabled}<span class="chip wait">{t('youtube.follows.paused')}</span>{/if}
						</div>
						<div class="controls">
							<label class="ctl">
								<span class="small muted">{t('youtube.follows.quality')}</span>
								<select class="input" value={f.quality_max_height} disabled={busy === f.id} onchange={(e) => edit(f, { quality_max_height: Number(e.currentTarget.value) })}>
									{#each heights as h (h)}<option value={h}>{t('youtube.follows.upTo', { p: h })}</option>{/each}
								</select>
							</label>
							{#if !f.mirror}
							<label class="checkline">
								<input type="checkbox" checked={f.keep_last !== null} disabled={busy === f.id} onchange={(e) => edit(f, { keep_last: e.currentTarget.checked ? (f.backfill !== null && f.backfill < ALL_VIDEOS ? f.backfill : 20) : null })} />
								<span>{f.keep_last !== null ? t('youtube.confirm.keepNewest') : t('youtube.follows.keepAll')}</span>
							</label>
							{#if f.keep_last !== null}
								<input class="input num" type="number" min="1" step="1" aria-label={t('youtube.confirm.keepNewest')} value={f.keep_last} disabled={busy === f.id} onchange={(e) => keepNumber(f, e.currentTarget.value)} />
							{/if}
							{/if}
							{#if f.kind === 'channel'}
								<label class="checkline">
									<input type="checkbox" checked={f.include_shorts} disabled={busy === f.id} onchange={(e) => edit(f, { include_shorts: e.currentTarget.checked })} />
									<span>{t('youtube.confirm.shorts')}</span>
								</label>
							{/if}
						</div>
						{#if f.kind === 'playlist'}
							<label class="checkline">
								<input type="checkbox" role="switch" checked={f.mirror} disabled={busy === f.id} onchange={(e) => setMirror(f, e.currentTarget.checked)} data-testid="youtube-follow-mirror" />
								<span>{t('youtube.confirm.mirror')}<br /><span class="hint">{t('youtube.confirm.mirrorHint')}</span></span>
							</label>
						{/if}
						<dl class="facts-s small">
							<dt class="muted">{t('youtube.follows.first')}</dt>
							<dd>{firstWords(f)}</dd>
							<dt class="muted">{t('youtube.follows.lastCheck')}</dt>
							<dd>{f.last_checked_at ? ago(f.last_checked_at) : t('youtube.follows.never')}</dd>
						</dl>
						{#if f.last_error}<p class="err small">{t('youtube.follows.checkFailed')} {f.last_error}</p>{/if}
						{#if asking === f.id}
							<div class="ask" role="alertdialog" aria-labelledby="{id}-a{f.id}">
								<p class="small" id="{id}-a{f.id}">{t('youtube.follows.unfollowAsk', { name: f.title })}</p>
								<div class="row">
									<Button size="sm" busy={busy === f.id} disabled={busy === f.id} onclick={() => unfollow(f, 'keep')} data-testid="youtube-unfollow-keep">{t('youtube.follows.keepFiles')}</Button>
									<Button size="sm" variant="destructive" disabled={busy === f.id} onclick={() => unfollow(f, 'delete')} data-testid="youtube-unfollow-delete">{t('youtube.follows.deleteFiles')}</Button>
									<Button size="sm" variant="plain" onclick={() => (asking = null)}>{t('youtube.confirm.cancel')}</Button>
								</div>
							</div>
						{:else}
							<div class="row">
								<Button size="sm" variant="plain" onclick={() => onshow(f.id)}>{t('youtube.follows.showVideos')}</Button>
								<Button size="sm" variant="plain" icon={f.enabled ? 'pause' : 'play'} disabled={busy === f.id} onclick={() => edit(f, { enabled: !f.enabled })}>
									{f.enabled ? t('youtube.follows.pause') : t('youtube.follows.resume')}
								</Button>
								<Button size="sm" variant="plain" onclick={() => (asking = f.id)} data-testid="youtube-unfollow">{t('youtube.follows.unfollow')}</Button>
							</div>
						{/if}
					</div>
				</li>
			{/each}
		</ul>
	{/if}
</Sheet>

<style>
	.follows { margin: 0; padding: 0; list-style: none; }
	.follows > li { align-items: flex-start; }
	.follows > li.focus { background: var(--surface-2); border-radius: var(--r-card); padding-inline: var(--s-3); }
	.stack-s { display: grid; gap: var(--s-2); }
	.head { display: flex; align-items: center; gap: var(--s-2); min-width: 0; }
	.controls { display: flex; flex-wrap: wrap; align-items: center; gap: var(--s-2) var(--s-4); }
	.ctl { display: flex; align-items: center; gap: var(--s-2); }
	.ctl .input { width: auto; }
	.num { width: 6em; }
	.facts-s { display: grid; grid-template-columns: max-content 1fr; gap: 2px var(--s-4); margin: 0; }
	.facts-s dd { margin: 0; }
	.err { margin: 0; color: var(--fail-text); overflow-wrap: anywhere; }
	.ask { display: grid; gap: var(--s-2); padding: var(--s-3); border: 1px solid var(--line); border-radius: var(--r-card); }
	.ask p { margin: 0; }
</style>
