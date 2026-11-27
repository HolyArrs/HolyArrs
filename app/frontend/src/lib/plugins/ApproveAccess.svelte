<script lang="ts">
	// What an installed plugin may reach that the owner chooses: a real folder for each folder it asks for, hosts of the owner's own
	// (Tailscale: a self-hosted Headscale; a bot-check plugin: their FlareSolverr), and the OK for an update that asks for more. Nothing is granted until saved.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { folderLabel, needLabel, scopeWarnings, takesOwnerHosts } from './view.ts';
	import type { Plugin, PluginResources } from './types.ts';

	let { p, reload }: { p: Plugin; reload: () => Promise<void> } = $props();
	// Older lists sent plain names; a plain name is a read-only folder.
	const folders = $derived((p.access?.folders ?? []).map((f) => (typeof f === 'string' ? { name: f, mode: 'read' as const } : f)));
	const needs = $derived(p.needs_approval ?? []);
	const warns = $derived(scopeWarnings(p.scope_groups));
	const ownHosts = $derived(takesOwnerHosts(p));
	let paths = $state<Record<string, string>>({});
	let hosts = $state('');
	let busy = $state(false);
	// The owner's caps for this plugin; an empty box keeps what it asks for.
	const limitKeys = [['memory_mib', 'plugins.limitMemory'], ['cpus', 'plugins.limitCpus'], ['processes', 'plugins.limitProcesses'], ['open_files', 'plugins.limitOpenFiles']] as const;
	let limits = $state<Record<string, string>>({});
	$effect(() => {
		paths = Object.fromEntries(folders.map((f) => [f.name, p.grants?.[f.name]?.path ?? '']));
		hosts = (p.owner_hosts ?? []).join(', ');
		limits = Object.fromEntries(limitKeys.map(([k]) => [k, String(p.owner_limits?.[k] ?? '')]));
	});
	const missing = $derived(folders.some((f) => f.required && !paths[f.name]?.trim()));
	const shown = $derived(!!p.installed);

	async function save() {
		if (busy || missing) return;
		busy = true;
		try {
			const body: { folders: Record<string, string>; owner_hosts?: string[]; limits?: PluginResources } = { folders: { ...paths } };
			if (ownHosts) body.owner_hosts = hosts.split(/[\s,]+/).filter(Boolean);
			const own: PluginResources = {};
			for (const [k] of limitKeys) {
				const n = Number.parseInt(limits[k] ?? '', 10);
				if (n > 0) own[k] = n;
			}
			body.limits = own;
			await api.post(`/plugins/${encodeURIComponent(p.id)}/approve`, body);
			toasts.push(t('plugins.accessSaved'), 'ok');
		} catch (e) {
			toasts.push(t('plugins.accessFailed'), 'fail', { body: message(e) });
		} finally {
			await reload();
			busy = false;
		}
	}
</script>

{#if shown}
	<section class="approve" data-testid="plugin-approve" aria-labelledby={`approve-${p.id}`}>
		<div class="head">
			<h3 id={`approve-${p.id}`}>{t('plugins.accessTitle')}</h3>
			{#if needs.length}
				<p class="small need" role="status"><Icon name="info" size={15} /><span>{t('plugins.accessNeeded')}</span></p>
				<ul class="nchips xs">{#each needs as n (n)}<li class="nchip" dir="auto" title={needLabel(n, p)}><Icon name="plus" size={12} /><span class="nt">{needLabel(n, p)}</span></li>{/each}</ul>
			{/if}
		</div>
		<!-- The red warnings (network.forward and the like) show where the access is approved too, not only in the details. -->
		{#each warns as w (w)}<p class="small warn" role="note" data-testid="approve-warning"><Icon name="alert" size={15} /><span>{w}</span></p>{/each}
		{#if folders.length || ownHosts}<div class="boxes">
		{#each folders as f (f.name)}
			<label class="row">
				<span class="small lab">{folderLabel(f)}</span>
				{#if f.why}<span class="xs muted why">{t('plugins.folderWhy', { why: f.why })}</span>{/if}
				<input class="input" type="text" dir="ltr" spellcheck="false" autocomplete="off" placeholder={`/data/${f.name}`} bind:value={paths[f.name]} data-testid={`plugin-folder-${f.name}`} />
			</label>
		{/each}
		{#if ownHosts}
			<label class="row wide">
				<span class="small lab">{t('plugins.ownerHosts')}<span class="xs muted"> · {t('plugins.ownerHostsHelp')}</span></span>
				<input class="input" type="text" dir="ltr" spellcheck="false" autocomplete="off" placeholder={p.id === 'tailscale' ? 'headscale.example.com:8080' : 'flaresolverr:8191'} bind:value={hosts} data-testid="plugin-owner-hosts" />
			</label>
		{/if}
		</div>{/if}
		<fieldset class="limits boxes">
			<legend class="small lab">{t('plugins.limitsTitle')}</legend>
			{#each limitKeys as [k, label] (k)}
				<label class="row lim">
					<span class="small lab">{t(label)}</span>
					<input class="input" type="number" inputmode="numeric" min="1" dir="ltr" bind:value={limits[k]} data-testid={`plugin-limit-${k}`} />
				</label>
			{/each}
			{#if p.installed}
			<div class="acts">
				<Button variant={needs.length ? 'primary' : 'plain'} busy={busy} disabled={busy || missing} onclick={save} data-testid="plugin-approve-save">{missing ? t('plugins.chooseFolders') : needs.length ? t('plugins.accessApprove') : t('plugins.accessSave')}</Button>
			</div>
		{/if}
		</fieldset>
		{#if p.blocked_count}<p class="xs muted" data-testid="plugin-blocked">{t('plugins.blockedCount', { n: p.blocked_count })}</p>{/if}
	</section>
{/if}

<style>
	/* A card like About and Can access: chips for the new access, then one even grid of boxes (folders and limits), Approve at the end. */
	.approve { grid-column: 1 / -1; display: grid; gap: 10px; min-width: 0; }
	.approve h3 { margin: 0; font-size: 12px; font-weight: 600; color: var(--text-muted); letter-spacing: normal; }
	.need { display: flex; gap: var(--s-2); align-items: flex-start; margin: 0; color: var(--wait); }
	.need :global(svg) { flex: none; margin-top: 2px; }
	.nchips { display: flex; flex-wrap: wrap; gap: 6px; margin: 0; padding: 0; list-style: none; }
	.nchip { flex: 0 1 auto; min-width: 0; display: inline-flex; align-items: center; gap: 4px; max-width: 100%; padding: 3px 10px 3px 8px; border-radius: 999px; background: var(--surface-2); box-shadow: 0 0 0 1px var(--line); }
	.boxes { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(120px, 100%), 1fr)); gap: 8px; min-width: 0; margin: 0; padding: 0; border: 0; }
	.limits legend { grid-column: 1 / -1; padding: 0; margin-bottom: 4px; font-size: 12px; font-weight: 600; color: var(--text-muted); }
	.limits legend.lab { font-size: 12px; }
	.row { display: grid; gap: 4px; align-content: start; min-width: 0; }
	/* The own-server box spans the card so its example address shows in full. */
	.wide { grid-column: 1 / -1; }
	.lab { font-size: 12px; overflow-wrap: anywhere; }
	.why { overflow-wrap: anywhere; }
	.row .input { width: 100%; min-width: 0; height: 36px; min-height: 36px; box-sizing: border-box; font-size: 13px; }
	/* Limits: four boxes and Save on one line across the card; labels wrap only between words; 2x2 when the card is narrow. */
	.approve { container-type: inline-size; }
	.limits { grid-template-columns: repeat(4, minmax(0, 1fr)); align-items: end; }
	.limits:has(.acts) { grid-template-columns: repeat(4, minmax(0, 1fr)) auto; }
	.lim .lab { overflow-wrap: normal; }
	.acts { display: flex; align-items: flex-end; justify-content: flex-end; min-width: 0; }
	@container (max-width: 460px) {
		.limits, .limits:has(.acts) { grid-template-columns: repeat(2, minmax(0, 1fr)); }
		.acts { grid-column: 1 / -1; }
	}
	/* No folders to choose: no empty row (Svelte leaves text in it, so :empty never matches). */
	.approve > .boxes:not(:has(*)) { display: none; }
	.nchip { list-style: none; }
	.nchip :global(svg) { flex: none; }
	/* Requested access is never cut short: a long host wraps instead of ending in an ellipsis. */
	.nt { min-width: 0; overflow-wrap: anywhere; }
	/* The heading, the notice and the new access share one line while they fit, then wrap. */
	.head { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 12px; min-width: 0; }
	.warn { display: flex; gap: var(--s-2); align-items: flex-start; margin: 0; color: var(--fail-text, var(--fail)); }
	.warn :global(svg) { flex: none; margin-top: 2px; }
</style>
