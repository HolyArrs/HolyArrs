<script lang="ts" module>
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	export interface SavedKey {
		kind: string;
		address: string;
		allow_local_network: boolean;
	}
	export interface ReadAnswer {
		snapshot: import('./move.ts').Snapshot;
		needs_reentry?: string[];
		notes?: string[];
	}
</script>

<script lang="ts">
	import { getContext, tick } from 'svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import TestTag from '#lib/ui/TestTag.svelte';
	import StepCapsule from './StepCapsule.svelte';
	import { apps, appName, placeholderFor, type AppKind } from './move.ts';
	import { api, ApiError } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '../core/toasts.svelte.ts';
	import { reachWords } from '../downloads/index.ts';

	let {
		open = $bindable(false),
		start = 'radarr',
		startAddress = '',
		again = false,
		saved = [],
		known = [],
		reading = $bindable(''),
		onread
	}: {
		open?: boolean;
		start?: AppKind;
		/** Edit: the address of the copy being read again ('' for a backup or an older read). */
		startAddress?: string;
		again?: boolean;
		saved?: SavedKey[];
		/** Addresses already read; a saved key for one of them is not offered when adding another app. */
		known?: string[];
		reading?: string;
		onread: (r: ReadAnswer, kind: AppKind, usedSaved: boolean) => void;
	} = $props();

	let kind = $state<AppKind>('radarr');
	let way = $state<'api' | 'backup'>('api');
	let address = $state('');
	let key = $state('');
	let username = $state('');
	let signWord = $state('');
	let backupPath = $state('');
	let local = $state(true);
	let useSaved = $state(true);
	let error = $state('');
	let probe = $state<{ ok: boolean; text: string } | null>(null);
	let checking = $state(false);
	let form = $state<HTMLFormElement>();

	const hidden = getContext<AppKind[] | undefined>('move-hide-apps') ?? [];
	const shown = apps.filter((a) => !hidden.includes(a.kind));
	// The last typed sign-in per copy of an app (kind and address).
	const lastRead: Record<string, { address: string; key: string; username: string; signWord: string }> = {};
	let ctrl: AbortController | null = null;
	let wasOpen = false;
	let startedAt = 0;
	let stopped = false;

	const qbit = $derived(kind === 'qbittorrent');
	// Edit uses the key kept for that copy; adding offers a kept key only for a copy not read yet.
	const savedKey = $derived(
		again
			? saved.find((x) => x.kind === kind && (startAddress ? x.address === startAddress : true))
			: saved.find((x) => x.kind === kind && !known.includes(x.address))
	);
	const withSaved = $derived(way === 'api' && !!savedKey && useSaved);
	const keptAgain = $derived(again && !!savedKey);
	const busy = $derived(reading !== '');
	const title = $derived(again ? t('migrate.edit', { name: appName(kind) }) : t('migrate.addAnApp'));

	$effect(() => {
		if (open && !wasOpen) {
			if (!busy) {
				kind = start;
				way = 'api';
				useSaved = true;
				const last = again ? lastRead[`${start} ${startAddress}`] : undefined;
				address = again ? startAddress || (saved.find((x) => x.kind === start)?.address ?? last?.address ?? '') : '';
				key = last?.key ?? '';
				username = last?.username ?? '';
				signWord = last?.signWord ?? '';
				backupPath = '';
				local = true;
				error = '';
				probe = null;
			}
			setTimeout(() => document.getElementById(again || busy ? 'move-address' : 'move-app')?.focus(), 0);
		}
		wasOpen = open;
	});

	function changed() {
		error = '';
		probe = null;
	}
	function body(): Record<string, unknown> {
		const b: Record<string, unknown> = { kind };
		if (way === 'backup') {
			b.path = backupPath.trim();
			return b;
		}
		if (withSaved) {
			b.use_saved_key = true;
			if (savedKey) b.url = savedKey.address;
			return b;
		}
		const a = address.trim();
		b.url = a && !/^[a-z][a-z0-9+.-]*:\/\//i.test(a) ? `http://${a}` : a;
		b.allow_local_network = local;
		if (qbit && (username || signWord)) {
			b.username = username;
			b.password = signWord;
		}
		if (key) b.key = key;
		return b;
	}
	function quiet(err: unknown): boolean {
		return (err instanceof ApiError && err.body.code === 'reauth') || (err instanceof DOMException && err.name === 'AbortError');
	}
	function missing(): boolean {
		if (way === 'api' && !withSaved && !address.trim()) {
			error = t('migrate.addTheAddress');
			document.getElementById('move-address')?.focus();
			return true;
		}
		return !!form && !form.reportValidity();
	}
	async function check() {
		if (missing()) return;
		checking = true;
		changed();
		try {
			const r = await api.post<{ app: { kind: string; version?: string } }>('/migrate/probe', body());
			probe = { ok: true, text: `${appName(kind)}${r.app?.version ? ' ' + r.app.version : ''}` };
		} catch (err) {
			if (!quiet(err)) {
				error = reachWords(message(err));
				probe = { ok: false, text: error };
			}
		} finally {
			checking = false;
		}
		if (error) focusAddress();
	}
	async function focusAddress() {
		await tick();
		document.getElementById('move-address')?.focus();
	}
	async function read(e: SubmitEvent) {
		e.preventDefault();
		if (busy || missing()) return;
		const k = kind;
		const b = body();
		const usedSaved = !!b.use_saved_key;
		let keep = false;
		ctrl = new AbortController();
		startedAt = Date.now();
		stopped = false;
		reading = k;
		changed();
		try {
			const r = await api.request<ReadAnswer>('/migrate/read', { method: 'POST', body: b, signal: ctrl.signal });
			if (!usedSaved) lastRead[`${k} ${r.snapshot.address ?? ''}`] = { address, key, username, signWord };
			key = '';
			signWord = '';
			open = false;
			onread(r, k, usedSaved);
		} catch (err) {
			if (quiet(err)) {
				if (stopped) {
					keep = true;
					if (open) error = t('migrate.readingStopped');
				}
				return;
			}
			if (open) error = reachWords(message(err));
			else toasts.push(t('migrate.couldnTRead', { name: appName(k) }), 'fail', { body: reachWords(message(err)) });
		} finally {
			reading = '';
			ctrl = null;
			if (!keep) {
				key = '';
				signWord = '';
			}
		}
		if (open && error) focusAddress();
	}
	export function stop(e?: MouseEvent) {
		if (!ctrl) return;
		e?.preventDefault();
		if (e && e.detail === 0 && Date.now() - startedAt < 1000) return;
		stopped = true;
		ctrl.abort();
	}
</script>

<Sheet bind:open {title} size="md" placement="auto">
	<form id="move-read-form" bind:this={form} onsubmit={read} novalidate>
		<div class="srow">
			<div class="txt"><label for="move-app">{t('migrate.app')}</label></div>
			<div class="ctl">
				<select id="move-app" class="input" bind:value={kind} onchange={changed} disabled={busy || again}>
					{#each shown as a (a.kind)}<option value={a.kind}>{a.name}</option>{/each}
				</select>
			</div>
		</div>
		{#if way === 'api'}
			{#if savedKey && !keptAgain}
				<div class="srow" data-testid="use-saved-key">
					<div class="txt"><label for="move-saved">{t('migrate.useTheSavedKey')}</label><span class="hint">{t('migrate.sentOnlyTo', { address: savedKey.address })}</span></div>
					<div class="ctl">
						<label class="switch"><input id="move-saved" type="checkbox" bind:checked={useSaved} onchange={changed} /><span class="sw" aria-hidden="true"></span></label>
					</div>
				</div>
			{/if}
			<div class="srow">
				<div class="txt">
					{#if withSaved}<span class="label">{t('migrate.address')}</span>{:else}<label for="move-address">{t('migrate.address')}</label>{/if}
					{#if withSaved && keptAgain}<span class="hint">{t('migrate.savedKey')}</span>{/if}
				</div>
				<div class="ctl">
					{#if withSaved}
						<span class="value mono addr" data-testid="saved-address">{savedKey?.address}</span>
					{:else}
						<input id="move-address" class="input mono" type="text" inputmode="url" bind:value={address} oninput={changed} placeholder={placeholderFor(kind).replace('http://', '')} autocomplete="off" spellcheck="false" />
					{/if}
				</div>
			</div>
			{#if withSaved && keptAgain}
				<p class="other"><button type="button" class="link" onclick={() => { useSaved = false; changed(); }}>{t('migrate.useADifferentKey')}</button></p>
			{/if}
			{#if !withSaved}
				{#if qbit}
					<div class="srow">
						<div class="txt"><label for="move-user">{t('migrate.userName')}</label></div>
						<div class="ctl"><input id="move-user" class="input" bind:value={username} oninput={changed} autocomplete="off" required={!key} /></div>
					</div>
					<div class="srow">
						<div class="txt"><label for="move-pass">{t('migrate.passwordLabel')}</label></div>
						<div class="ctl"><input id="move-pass" class="input" type="password" bind:value={signWord} oninput={changed} autocomplete="new-password" required={!key} /></div>
					</div>
					<div class="srow">
						<div class="txt"><label for="move-key">{t('migrate.apiKeyLabel')}</label><span class="hint">{t('migrate.replacesUserNameAnd')}</span></div>
						<div class="ctl"><input id="move-key" class="input mono" type="password" bind:value={key} oninput={changed} autocomplete="new-password" spellcheck="false" /></div>
					</div>
				{:else}
					<div class="srow">
						<div class="txt"><label for="move-key">{t('migrate.apiKeyLabel')}</label><span class="hint">{t('migrate.settingsGeneral', { name: appName(kind) })}</span></div>
						<div class="ctl"><input id="move-key" class="input mono" type="password" bind:value={key} oninput={changed} autocomplete="new-password" spellcheck="false" required /></div>
					</div>
				{/if}
			{/if}
		{/if}
	</form>
	{#if busy}
		<div class="cap" role="status"><StepCapsule tone="go" live fill={0.4} status="processing" word={t('migrate.adding', { name: appName(reading) })} detail={t('migrate.canTakeAFew')} testid="read-capsule" /></div>
	{/if}
	{#if probe}<p class="probe" data-testid={probe.ok ? 'probe-ok' : undefined}><TestTag ok={probe.ok} text={probe.text} app={appName(kind)} /></p>{/if}
	{#if error}<p class="form-error" role="alert">{error}</p>{/if}
	{#snippet footer()}
		<Button variant="secondary" onclick={() => (open = false)}>{t('migrate.cancel')}</Button>
		<Button onclick={check} busy={checking} disabled={busy}>{t('migrate.test')}</Button>
		<Button variant="primary" type={busy ? 'button' : 'submit'} form={busy ? undefined : 'move-read-form'} onclick={busy ? stop : undefined}>{busy ? t('migrate.stop') : again ? t('migrate.save') : t('migrate.add', { name: appName(kind) })}</Button>
	{/snippet}
</Sheet>

<style>
	.srow { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: center; column-gap: 16px; min-height: 52px; padding: 7px 0; border-top: 1px solid var(--line); }
	.srow:first-child { border-top: 0; }
	.srow > .txt { display: flex; flex-direction: column; gap: 1px; min-width: 0; }
	.srow > .txt > :is(.label, label) { font: var(--t-callout); color: var(--text); }
	.srow > .txt > .hint { color: var(--text-muted); font: var(--t-footnote); }
	.srow > .ctl { justify-self: end; display: flex; align-items: center; gap: 8px; min-width: 0; }
	.srow > .ctl > .input { width: 260px; }
	@media (max-width: 600px) {
		.srow > .ctl > .input { width: min(260px, 46vw); }
	}
	.addr { display: block; max-width: min(260px, 46vw); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.cap { margin-top: 12px; }
	.other { margin: 4px 0 0; text-align: right; }
	.link { background: none; border: 0; padding: 6px 0; min-height: 44px; color: var(--text-muted); font: var(--t-footnote); text-decoration: underline; cursor: pointer; }
	.link:hover { color: var(--text); }
	.probe, .form-error { margin-top: 12px; }
	.probe { margin-bottom: 0; display: flex; }
	select.input { width: 260px; }
	@media (max-width: 600px) {
		select.input { width: min(260px, 46vw); }
	}
</style>
