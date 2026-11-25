<script lang="ts">
	import Words from '#lib/ui/Words.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import { DEFAULT_MCP_GROUPS, MCP_GROUPS, aiKeyBody, aiKeyGroupsBody, aiURL, isAIKey, type McpGroup } from '#lib/ai/index.ts';
	import { onMount, tick } from 'svelte';
	import { api, ApiError } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { ago, date } from '#lib/core/format.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { getBase } from '#lib/core/paths.ts';
	import type { ApiKey } from '#lib/core/types.ts';

	let loaded = $state(false);
	let error = $state('');
	let enabled = $state(false);
	let saving = $state(false);
	let keys = $state<ApiKey[]>([]);
	let keysNote = $state('');
	let label = $state('');
	let making = $state(false);
	// What a new key may do.
	let groups = $state<string[]>([...DEFAULT_MCP_GROUPS]);
	let fresh = $state<{ id: number; key: string; label: string } | null>(null);
	let byHand = $state<HTMLElement>();
	let revoking = $state<ApiKey | null>(null);
	let revokeOpen = $state(false);
	let revokeBusy = $state(false);
	let editing = $state<ApiKey | null>(null);
	let editOpen = $state(false);
	let editGroups = $state<string[]>([]);
	let editBusy = $state(false);

	const isOwner = $derived(session.s.role === 'owner');
	const canEdit = $derived(session.s.role === 'owner' || session.s.role === 'admin');
	const url = $derived(typeof window === 'undefined' ? '/api/v1/mcp' : aiURL(window.location.origin, getBase()));
	const aiKeys = $derived(keys.filter(isAIKey));

	function keyNote(e: unknown): string {
		return e instanceof ApiError && e.status === 403 ? t('settings.ai.signInKeys') : message(e);
	}

	async function load() {
		error = '';
		try {
			enabled = (await api.get<{ enabled: boolean }>('/settings/ai')).enabled;
		} catch (e) {
			error = message(e);
		}
		try {
			keys = (await api.get<{ keys: ApiKey[] }>('/auth/keys')).keys;
			keysNote = '';
		} catch (e) {
			keysNote = keyNote(e);
		}
		loaded = true;
	}

	onMount(load);

	async function setEnabled(v: boolean) {
		if (saving) return;
		const before = enabled;
		enabled = v;
		saving = true;
		try {
			enabled = (await api.put<{ enabled: boolean }>('/settings/ai', { enabled: v })).enabled;
			toasts.push(enabled ? t('settings.ai.on') : t('settings.ai.off'), 'ok');
		} catch (e) {
			enabled = before;
			toasts.push(t('settings.ai.couldntSave'), 'fail', { body: message(e) });
		} finally {
			saving = false;
		}
	}

	async function makeKey(e: SubmitEvent) {
		e.preventDefault();
		if (label.trim()) await newKey(label.trim(), groups);
		label = '';
		groups = [...DEFAULT_MCP_GROUPS];
	}

	async function newKey(name: string, ticked: readonly string[]) {
		making = true;
		try {
			const r = await api.post<{ id: number; key: string; label: string }>('/auth/keys', aiKeyBody(name, ticked));
			fresh = { id: r.id, key: r.key, label: r.label };
			keys = (await api.get<{ keys: ApiKey[] }>('/auth/keys')).keys;
		} catch (err) {
			toasts.push(t('settings.ai.makeFailed'), 'fail', { body: keyNote(err) });
		} finally {
			making = false;
		}
	}

	async function doneWithKey() {
		fresh = null;
		await tick();
		byHand?.focus();
	}

	function askRevoke(k: ApiKey) {
		revoking = k;
		revokeOpen = true;
	}

	async function revoke(k: ApiKey) {
		if (revokeBusy) return;
		revokeBusy = true;
		revokeOpen = false;
		try {
			await api.del(`/auth/keys/${k.id}`);
			keys = keys.filter((x) => x.id !== k.id);
			if (fresh?.id === k.id) fresh = null;
			toasts.push(t('settings.ai.revoked', { name: k.label }), 'ok');
		} catch (e) {
			toasts.push(t('settings.ai.revokeFailed'), 'fail', { body: keyNote(e) });
		} finally {
			revokeBusy = false;
		}
	}

	function askEdit(k: ApiKey) {
		editing = k;
		editGroups = [...(k.mcp_groups ?? [])];
		editOpen = true;
	}

	async function saveEdit(e: SubmitEvent) {
		e.preventDefault();
		const k = editing;
		if (!k || editBusy) return;
		editBusy = true;
		try {
			const r = await api.put<{ mcp_groups: string[] }>(`/auth/keys/${k.id}`, aiKeyGroupsBody(editGroups));
			keys = keys.map((x) => (x.id === k.id ? { ...x, mcp_groups: r.mcp_groups } : x));
			editOpen = false;
			toasts.push(t('settings.ai.groupsSaved', { name: k.label }), 'ok');
		} catch (err) {
			toasts.push(t('settings.ai.groupsSaveFailed', { name: k.label }), 'fail', { body: keyNote(err) });
		} finally {
			editBusy = false;
		}
	}

	async function copy(text: string) {
		try {
			await navigator.clipboard.writeText(text);
			toasts.push(t('settings.ai.textCopied'), 'ok');
		} catch {
			toasts.push(t('settings.ai.copyFailed'), 'fail', { body: t('settings.ai.selectInstead') });
		}
	}

	const groupName = (g: McpGroup) =>
		({ see: t('settings.ai.groupSee'), add: t('settings.ai.groupAdd'), control: t('settings.ai.groupControl'), settings: t('settings.ai.groupSettings'), delete: t('settings.ai.groupDelete'), admin: t('settings.ai.groupAdmin'), approve: t('settings.ai.groupApprove'), plugins: t('settings.ai.groupPlugins') })[g];
	const shortName = (g: McpGroup) =>
		({ see: t('settings.ai.shortSee'), add: t('settings.ai.shortAdd'), control: t('settings.ai.shortControl'), settings: t('settings.ai.shortSettings'), delete: t('settings.ai.shortDelete'), admin: t('settings.ai.shortAdmin'), approve: t('settings.ai.shortApprove'), plugins: t('settings.ai.shortPlugins') })[g];
	const groupShort = (k: ApiKey) => MCP_GROUPS.filter((g) => k.mcp_groups?.includes(g)).map(shortName).join(' · ');
</script>

{#snippet freshBox()}
	{#if fresh}
		<div class="fresh" role="status">
			<p class="small"><b>{t('settings.ai.copyKeyNow')}</b></p>
			<p class="small">{t('settings.ai.shownOnce', { name: fresh.label })}</p>
			<div class="fresh-row">
				<code class="key grow">{fresh.key}</code>
				<Button icon="copy" type="button" onclick={() => fresh && copy(fresh.key)}>{t('settings.ai.copy')}</Button>
				<Button variant="plain" type="button" onclick={doneWithKey}>{t('settings.ai.close')}</Button>
			</div>
		</div>
	{/if}
{/snippet}

<svelte:head><title>{t('settings.ai.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.ai.title')} />

{#if !loaded}
	<SkSettings rows={3} />
{:else if error}
	<LoadError hasData={false} message={error} onretry={load} busy={false} />
{:else}
	<SettingsCard id="ai-switch" title={t('settings.ai.title')}>
		<p class="small muted" style="margin: var(--s-2) 0 var(--s-3)">
			{t('settings.ai.intro')}
		</p>
		<label class="switch">
			<input type="checkbox" checked={enabled} disabled={!isOwner || saving} aria-busy={saving} onchange={(e) => setEnabled(e.currentTarget.checked)} />
			<span class="sw" aria-hidden="true"></span>
			<span>{t('settings.ai.letConnect')}</span>
		</label>
		{#if !isOwner}<p class="hint">{t('settings.ai.ownerOnly')}</p>{/if}
		<ul class="facts small muted">
			<li>{t('settings.ai.fact1')}</li>
			<li>{t('settings.ai.fact2')}</li>
			<li>{t('settings.ai.fact3')}</li>
		</ul>
	</SettingsCard>

	<SettingsCard id="ai-connect" title={t('settings.ai.connect')}>
		<div class="small addr-label">{t('settings.ai.address')}</div>
		<div class="addr-row">
			<code class="addr grow" data-testid="ai-address">{url}</code>
			<Button icon="copy" type="button" aria-label={t('settings.ai.copyAddress')} onclick={() => copy(url)}>{t('settings.ai.copy')}</Button>
		</div>
		<p class="xs muted">{t('settings.ai.errorExpected')}</p>
		{#if !enabled}<p class="notice warn" role="status"><Icon name="info" size={18} /><span><Words key="settings.ai.turnOnFirst" /></span></p>{/if}
	</SettingsCard>

	<SettingsCard id="ai-keys" title={t('settings.ai.linked')}>
		<p class="small muted" style="margin: var(--s-2) 0 var(--s-4)">{t('settings.ai.linkedNote')}</p>
		{#if keysNote}
			<p class="notice warn">{keysNote}</p>
		{:else}
			{@render freshBox()}
			{#if aiKeys.length === 0}
				<p class="empty-row">{t('settings.ai.noneLinked')}</p>
			{:else}
				<ul class="rows">
					{#each aiKeys as k (k.id)}
						<li>
							<div class="grow">
								<div class="name">{k.label} <span class="mono xs muted">{k.hint}</span></div>
								{#if k.mcp_groups}<div class="xs muted" data-testid="ai-key-groups">{t('settings.ai.canDo', { list: groupShort(k) || t('settings.ai.groupNone') })}</div>{/if}
								<div class="xs muted">{t('settings.ai.made', { date: date(k.created_at) })} · {k.last_used_at ? t('settings.ai.lastUsed', { ago: ago(k.last_used_at) }) : t('settings.ai.neverUsed')}</div>
							</div>
							{#if canEdit && k.mcp_groups}<Button variant="plain" type="button" data-testid="ai-key-edit" aria-label={t('settings.ai.editName', { name: k.label })} onclick={() => askEdit(k)}>{t('settings.ai.edit')}</Button>{/if}
							<Button variant="destructive" type="button" onclick={() => askRevoke(k)}>{t('settings.ai.revoke')}</Button>
						</li>
					{/each}
				</ul>
			{/if}
			<details class="more" style="margin-top: var(--s-4)">
				<summary bind:this={byHand}>{t('settings.ai.byHand')}</summary>
				<form onsubmit={makeKey}>
					<fieldset class="groups" data-testid="ai-key-groups-form" aria-describedby="ai-groups-fixed">
						<legend>{t('settings.ai.groupsLegend')}</legend>
						{#each MCP_GROUPS as g (g)}
							<label class="checkline">
								<input type="checkbox" name="mcp_groups" value={g} bind:group={groups} />
								{@render groupText(g)}
							</label>
						{/each}
						<p class="hint" id="ai-groups-fixed">{t('settings.ai.groupsFixed')}</p>
					</fieldset>
					<div class="row">
						<label class="sr-only" for="ai-key-label">{t('settings.ai.keyName')}</label>
						<input id="ai-key-label" class="input grow" style="max-width: 320px" bind:value={label} placeholder={t('settings.ai.keyExample')} maxlength="64" />
						<Button icon="key" type="submit" disabled={making || !label.trim()} aria-busy={making}>{t('settings.ai.makeAiKey')}</Button>
					</div>
				</form>
			</details>
		{/if}
	</SettingsCard>

	<Sheet bind:open={editOpen} size="sm" title={editing ? t('settings.ai.editName', { name: editing.label }) : t('settings.ai.edit')} description={t('settings.ai.editDesc')}>
		<form onsubmit={saveEdit}>
			<fieldset class="groups" data-testid="ai-key-edit-form">
				<legend>{t('settings.ai.groupsLegend')}</legend>
				{#each MCP_GROUPS as g (g)}
					<label class="checkline">
						<input type="checkbox" name="edit_mcp_groups" value={g} bind:group={editGroups} />
						{@render groupText(g)}
					</label>
				{/each}
			</fieldset>
			<div class="confirm-actions">
				<Button variant="plain" type="button" onclick={() => (editOpen = false)}>{t('settings.ai.keepIt')}</Button>
				<Button type="submit" disabled={editBusy} aria-busy={editBusy}>{t('settings.ai.saveGroups')}</Button>
			</div>
		</form>
	</Sheet>

	<Sheet bind:open={revokeOpen} size="sm" title={revoking ? t('settings.ai.revokeName', { name: revoking.label }) : t('settings.ai.revoke')} description={t('settings.ai.revokeDesc')}>
		<div class="confirm-actions">
			<Button variant="plain" type="button" onclick={() => (revokeOpen = false)}>{t('settings.ai.keepIt')}</Button>
			<Button variant="destructive" type="button" disabled={revokeBusy} onclick={() => revoking && void revoke(revoking)}>{t('settings.ai.revoke')}</Button>
		</div>
	</Sheet>
{/if}

{#snippet groupText(g: McpGroup)}
	{#if g === 'admin'}
		<span>{groupName(g)}<br /><span class="hint">{t('settings.ai.adminHint')}</span></span>
	{:else if g === 'approve'}
		<span>{groupName(g)}<br /><span class="hint">{t('settings.ai.approveHint')}</span></span>
	{:else}
		<span>{groupName(g)}</span>
	{/if}
{/snippet}

<style>
	.facts {
		margin: var(--s-3) 0 0;
		padding-left: 1.2em;
		display: grid;
		gap: 4px;
	}
	.addr-row {
		display: flex;
		align-items: center;
		gap: var(--s-2);
		flex-wrap: wrap;
		margin: 2px 0 0;
	}
	.addr,
	.key {
		padding: 8px var(--s-3);
		background: var(--surface-2);
		border-radius: var(--r-sm);
		font-size: var(--text-sm);
		min-width: 0;
		overflow-wrap: anywhere;
	}
	.notice {
		margin-top: var(--s-2);
	}
	.groups {
		margin-bottom: var(--s-3);
	}
	.groups .hint {
		margin: var(--s-1) 0 0;
	}
	.more summary {
		cursor: pointer;
		padding: 6px 0;
	}
	.fresh {
		padding: var(--s-4);
		border: 1px solid var(--ha-red);
		background: var(--ha-red-soft);
		border-radius: var(--r-btn);
		margin-bottom: var(--s-4);
	}
	.fresh p {
		margin: 0 0 var(--s-1);
	}
	.fresh-row {
		display: flex;
		align-items: center;
		gap: var(--s-2);
		flex-wrap: wrap;
		margin-top: var(--s-2);
	}
	.confirm-actions {
		display: flex;
		justify-content: flex-end;
		flex-wrap: wrap;
		gap: 8px;
		margin-top: 6px;
	}
	/* AI address card: no divider between a label and its value. */
	:global(#ai-connect > .body) { gap: 8px !important; padding-top: 16px !important; padding-bottom: 18px !important; }
	:global(#ai-connect > .body > :not(.notice)) { box-shadow: none !important; padding-block: 0 !important; }
	/* The notice keeps the house notice box: its own padding, a gap above, the info icon. */
	:global(#ai-connect > .body > .notice) { margin: 4px 18px 0 !important; align-items: center; }
	.addr-label { margin-top: 0; font: 600 15px/1.3 var(--font-ui); }
	.addr { flex: 1; padding: 10px 14px; border-radius: 12px; font-size: 14px; }
</style>
