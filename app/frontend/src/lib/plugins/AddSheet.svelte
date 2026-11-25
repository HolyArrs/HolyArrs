<script lang="ts">
	// The Add sheet of the plugins page: two ways to make a plugin.
	// "Build with AI" shows a one-line prompt and copies it; "Build with code" links to the guide and the template.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import ClampText from './ClampText.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { AI_PROMPT, DEV_STEPS } from './ai-prompt.ts';
	import type { PluginOverview } from './types.ts';
	let { open = $bindable(false), ov = null, reload = async () => {} }: { open?: boolean; ov?: PluginOverview | null; reload?: () => Promise<void> } = $props();

	// Developer mode: unsigned plugins from a folder on this server, in the same sandbox.
	let devBusy = $state(false);
	async function setDev(on: boolean) {
		if (devBusy) return;
		devBusy = true;
		try {
			await api.put('/plugins/dev-mode', { on });
			toasts.push(t(on ? 'plugins.devTurnedOn' : 'plugins.devTurnedOff'), 'ok');
		} catch (e) {
			toasts.push(t('plugins.devFailed'), 'fail', { body: message(e) });
		} finally {
			await reload();
			devBusy = false;
		}
	}

	let pick = $state<'' | 'ai' | 'dev'>('');
	// '' | 'ok' (copied) | 'fail' (the text is selected for a manual copy); per item key.
	let copied = $state<Record<string, '' | 'ok' | 'fail'>>({});
	let promptBox = $state<HTMLElement | null>(null);
	const timers: Record<string, ReturnType<typeof setTimeout>> = {};

	$effect(() => {
		if (!open) pick = '';
	});

	async function copy(key: string, text: string, el?: HTMLElement | null) {
		try {
			await navigator.clipboard.writeText(text);
			copied[key] = 'ok';
		} catch {
			copied[key] = 'fail';
			if (el instanceof HTMLTextAreaElement) {
				el.focus();
				el.select();
			} else if (el) {
				const r = document.createRange();
				r.selectNodeContents(el);
				const s = getSelection();
				s?.removeAllRanges();
				s?.addRange(r);
			}
		}
		clearTimeout(timers[key]);
		if (copied[key] === 'ok') timers[key] = setTimeout(() => (copied[key] = ''), 2000);
	}

	async function chooseAi() {
		pick = 'ai';
		await Promise.resolve();
		await copy('prompt', AI_PROMPT, promptBox);
	}

</script>

{#snippet copyBtn(key: string, text: string, target: () => HTMLElement | null)}
	<button type="button" class="cbtn" onclick={() => copy(key, text, target())} aria-label="{t('plugins.copy')}: {t('plugins.addAiTitle')}" data-testid="plugins-add-copy-{key}">
		<Icon name={copied[key] === 'ok' ? 'check' : 'copy'} size={14} /><span>{copied[key] === 'ok' ? t('plugins.copied') : t('plugins.copy')}</span>
	</button>
{/snippet}

<Sheet bind:open size="md" title={t('plugins.addTitle')}>
	<div class="addsheet" data-testid="plugins-add-sheet">
		<p class="small muted">{t('plugins.addIntro')}</p>
		<div class="choices" role="group" aria-label={t('plugins.addTitle')}>
			<button type="button" class="choice" aria-expanded={pick === 'ai'} onclick={chooseAi} data-testid="plugins-add-ai">
				<span class="ci" aria-hidden="true"><Icon name="sparkle" size={18} /></span>
				<span class="ct"><b>{t('plugins.addAiTitle')}</b><span class="xs muted">{t('plugins.addAiBody')}</span></span>
			</button>
			<button type="button" class="choice" aria-expanded={pick === 'dev'} onclick={() => (pick = 'dev')} data-testid="plugins-add-dev">
				<span class="ci" aria-hidden="true"><Icon name="pencil" size={18} /></span>
				<span class="ct"><b>{t('plugins.addDevTitle')}</b><span class="xs muted">{t('plugins.addDevBody')}</span></span>
			</button>
		</div>

		{#if pick === 'ai'}
			<section class="panel" data-testid="plugins-add-ai-panel" aria-label={t('plugins.addAiTitle')}>
				<div class="phead">
					<span class="status small" role="status">
						{#if copied.prompt === 'ok'}<Icon name="check" size={14} />{t('plugins.addPromptCopied')}{:else if copied.prompt === 'fail'}{t('plugins.addCopyFailed')}{/if}
					</span>
					{@render copyBtn('prompt', AI_PROMPT, () => promptBox)}
				</div>
				<ClampText lines={10}><code class="prompt mono" bind:this={promptBox} data-testid="plugins-add-prompt">{AI_PROMPT}</code></ClampText>
			</section>
		{:else if pick === 'dev'}
			<section class="panel" data-testid="plugins-add-dev-panel" aria-label={t('plugins.addDevTitle')}>
				<div class="links">
					<a class="guide small" href={DEV_STEPS.guide} target="_blank" rel="noopener noreferrer" data-testid="plugins-add-guide"><Icon name="source" size={15} />{t('plugins.addGuide')}</a>
					<a class="guide small" href={DEV_STEPS.template} target="_blank" rel="noopener noreferrer" data-testid="plugins-add-template"><Icon name="source" size={15} />{t('plugins.addTemplate')}</a>
				</div>
				{#if ov?.can_run}
					<div class="devmode" data-testid="plugins-dev-mode">
						<b class="small">{t('plugins.devTitle')}</b>
						<p class="xs muted">{t('plugins.devBody')}</p>
						{#if ov.dev_mode && ov.dev_dir}<p class="xs devdir" dir="ltr" data-testid="plugins-dev-dir">{t('plugins.devFolder', { dir: ov.dev_dir })}</p>{/if}
						<div><Button variant={ov.dev_mode ? 'secondary' : 'primary'} busy={devBusy} disabled={devBusy} onclick={() => setDev(!ov?.dev_mode)} data-testid="plugins-dev-toggle">{t(ov.dev_mode ? 'plugins.devOff' : 'plugins.devOn')}</Button></div>
					</div>
				{/if}
			</section>
		{/if}
	</div>
</Sheet>

<style>
	.addsheet { display: grid; gap: 12px; min-width: 0; }
	.addsheet p { margin: 0; }
	.devmode { display: grid; gap: 6px; margin-top: 12px; padding-top: 12px; border-top: 1px solid var(--line); min-width: 0; }
	.devdir { overflow-wrap: anywhere; font-family: var(--mono, monospace); }
	.choices { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; }
	@media (max-width: 520px) { .choices { grid-template-columns: minmax(0, 1fr); } }
	.choice { display: flex; align-items: flex-start; gap: 10px; min-height: 64px; padding: 12px; border: 0; border-radius: 12px; background: var(--surface-2); box-shadow: 0 0 0 1px var(--line); color: var(--text); font: inherit; text-align: start; cursor: pointer; transition: background-color 150ms ease, box-shadow 150ms ease; }
	.choice:hover { background: var(--surface-3, var(--line)); }
	.choice[aria-expanded='true'] { box-shadow: 0 0 0 2px var(--accent); }
	.choice:focus-visible { outline: 2px solid var(--focus, var(--accent)); outline-offset: 2px; }
	.ci { flex: none; display: grid; place-items: center; width: 32px; height: 32px; border-radius: 9px; background: color-mix(in srgb, var(--accent) 16%, var(--surface-1)); color: var(--accent); }
	.ct { display: grid; gap: 2px; min-width: 0; }
	.ct b { font-size: 14px; font-weight: 600; }
	.panel { display: grid; gap: 8px; min-width: 0; }
	.phead { display: flex; align-items: center; justify-content: space-between; gap: 8px; min-height: 32px; }
	.status { display: inline-flex; align-items: center; gap: 4px; color: var(--ok, var(--accent)); font-weight: 600; }
	.prompt { display: block; box-sizing: border-box; width: 100%; padding: 10px 12px; overflow-wrap: anywhere; user-select: all; border-radius: 10px; border: 1px solid var(--line-strong); background: var(--surface-2); color: var(--text); font-size: 12px; line-height: 1.5; }
	.cbtn { flex: none; display: inline-flex; align-items: center; gap: 5px; height: 30px; padding: 0 12px; border: 0; border-radius: 999px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line-strong); color: var(--text); font: inherit; font-size: 12px; font-weight: 600; cursor: pointer; }
	.cbtn:hover { background: var(--surface-2); }
	.cbtn:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
	.links { display: flex; flex-wrap: wrap; gap: 8px 16px; }
	.guide { justify-self: start; display: inline-flex; align-items: center; gap: 6px; color: var(--text); font-weight: 600; }
</style>
