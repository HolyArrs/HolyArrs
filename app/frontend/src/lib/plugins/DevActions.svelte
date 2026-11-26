<script lang="ts">
	// The panel actions of a developer plugin (Install from folder or Reload, Stop, Send a test event, Remove)
	// and, for a listed plugin, the Get test versions choice.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { api } from '#lib/core/api.ts';
	import { pluginState, shownName } from './view.ts';
	import type { Plugin, PluginOverview } from './types.ts';
	import type { Snippet } from 'svelte';

	type Act = (p: Plugin, run: () => Promise<unknown>, done: string, fail: string) => unknown;
	let { sel, ov, busy, removing, act, askRemove, part, settings }: { sel: Plugin; ov: PluginOverview | null; busy: string; removing: Plugin | null; act: Act; askRemove: (p: Plugin) => void; part: 'dev' | 'test'; settings?: Snippet } = $props();
	const path = $derived(`/plugins/${encodeURIComponent(sel.id)}`);
	const running = $derived(pluginState(sel) === 'running');
	const name = $derived(shownName(sel));
</script>

{#if part === 'dev'}
	<Button variant="primary" aria-label="{sel.installed ? t('plugins.devReload') : t('plugins.devInstall')}: {name}" disabled={!ov?.can_run || !ov?.dev_mode || !!busy || !!sel.dev_error} busy={busy === sel.id && !removing} onclick={() => act(sel, () => api.post(`${path}/dev-install`), 'plugins.devReloaded', 'plugins.devReloadFailed')} data-testid="plugin-dev-install">{t(sel.installed ? 'plugins.devReload' : 'plugins.devInstall')}</Button>
	{@render settings?.()}
	{#if sel.installed && running}<Button aria-label={t('plugins.stopName', { name })} disabled={!!busy} onclick={() => act(sel, () => api.post(`${path}/stop`), 'plugins.stopped', 'plugins.stopFailed')} data-testid="plugin-stop">{t('plugins.stop')}</Button>{/if}
	{#if sel.installed && running && sel.access?.events?.length}<Button disabled={!!busy} onclick={() => act(sel, () => api.post(`${path}/test-event`, { event: sel.access?.events?.[0] }), 'plugins.devTestSent', 'plugins.devTestFailed')} data-testid="plugin-dev-test-event">{t('plugins.devTestEvent')}</Button>{/if}
	{#if sel.installed}<Button aria-label={t('plugins.removeName', { name })} disabled={!!busy} busy={busy === sel.id && removing?.id === sel.id} onclick={() => askRemove(sel)}>{t('plugins.remove')}</Button>{/if}
	<span class="unsigned xs" title={t('plugins.devUnsignedHint')} data-testid="plugin-unsigned"><Icon name="alert" size={13} />{t('plugins.devUnsigned')}</span>
	{#if sel.dev_error}<p class="xs err" data-testid="plugin-dev-error"><Icon name="alert" size={13} /><span>{sel.dev_error}</span></p>{/if}
{:else if !sel.dev && (sel.test_version || sel.test_on || sel.test)}
	<p class="xs muted testv" data-testid="plugin-test-versions">{#if sel.test_version}<span>{t('plugins.testAvailable', { version: sel.test_version })}</span>{/if}<Button size="sm" disabled={!!busy} onclick={() => act(sel, () => api.put(`${path}/test-versions`, { on: !sel.test_on }), sel.test_on ? 'plugins.testOffToast' : 'plugins.testOnToast', 'plugins.testFailed')} data-testid="plugin-test-toggle">{t(sel.test_on ? 'plugins.testStop' : 'plugins.testGet')}</Button></p>
{/if}

<style>
	.unsigned { display: inline-flex; align-items: center; gap: 4px; height: 24px; padding: 0 10px; border-radius: 999px; background: color-mix(in srgb, var(--warn, var(--accent)) 16%, var(--surface-2)); font-weight: 600; white-space: nowrap; }
	.err { flex-basis: 100%; display: flex; align-items: flex-start; gap: 6px; margin: 0; color: var(--fail-text); }
	.err :global(svg) { flex: none; margin-top: 1px; }
	.testv { flex-basis: 100%; display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin: 0; }
</style>
