<script lang="ts" module>
	/** An old app's key saved from the secrets file (owner only); never the key itself. */
	export interface SavedKey {
		kind: string;
		address: string;
		allow_local_network: boolean;
	}
	/** What a read answers. */
	export interface ReadAnswer {
		snapshot: import('../move.ts').Snapshot;
		needs_reentry?: string[];
		notes?: string[];
	}
</script>

<script lang="ts">
	import Sheet from '../ui/Sheet.svelte';
	import Button from '../ui/Button.svelte';
	import StepCapsule from '../ui/StepCapsule.svelte';
	import TestTag from '../ui/TestTag.svelte';
	import { api, ApiError, message } from '../api.ts';
	import { toasts } from '../toasts.svelte.ts';
	import { getContext } from 'svelte';
	import { apps, appName, placeholderFor, type AppKind } from '../move.ts';
	// The first-run wizard hides the apps it already set up (Prowlarr, downloaders).
	const hidden = getContext<AppKind[] | undefined>('move-hide-apps') ?? [];
	const shown = apps.filter((a) => !hidden.includes(a.kind));

	// Add an app, or read one again: check the connection, then read it.
	// Closing the sheet does not stop a read; Stop reading does (the server
	// stops too, as the request ends).
	let {
		open = $bindable(false),
		start = 'radarr',
		again = false,
		saved = [],
		reading = $bindable(''),
		onread
	}: {
		open?: boolean;
		/** The app the sheet opens on. */
		start?: AppKind;
		/** Reading an app again (the title says so). */
		again?: boolean;
		saved?: SavedKey[];
		/** The app being read now, or ''. */
		reading?: string;
		onread: (r: ReadAnswer, kind: AppKind, usedSaved: boolean) => void;
	} = $props();

	let kind = $state<AppKind>('radarr');
	// Reading from a backup file waits for a later release (Luis 2026-11-05); the server still has it.
	let way = $state<'api' | 'backup'>('api');
	let address = $state('');
	let key = $state('');
	let username = $state('');
	let signWord = $state('');
	let backupPath = $state('');
	// The owner types the address, and old apps live on the home network: always allowed.
	let local = $state(true);
	let useSaved = $state(true);
	let error = $state('');
	// The answer of Check connection, shown as a tag.
	let probe = $state<{ ok: boolean; text: string } | null>(null);
	let checking = $state(false);
	let ctrl: AbortController | null = null;
	let form = $state<HTMLFormElement>();

	const qbit = $derived(kind === 'qbittorrent');
	const savedKey = $derived(saved.find((x) => x.kind === kind));
	const withSaved = $derived(way === 'api' && !!savedKey && useSaved);
	const busy = $derived(reading !== '');
	const title = $derived(again ? `Read ${appName(kind)} again` : 'Add an app');

	// Each open starts clean, on the app it was opened for.
	let wasOpen = false;
	$effect(() => {
		if (open && !wasOpen) {
			if (!busy) {
				kind = start;
				way = 'api';
				useSaved = true;
				address = again ? (saved.find((x) => x.kind === start)?.address ?? '') : '';
				backupPath = '';
				local = true;
				error = '';
				probe = null;
			}
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
			return b;
		}
		b.url = address.trim();
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

	async function check() {
		if (form && !form.reportValidity()) return;
		checking = true;
		changed();
		try {
			const r = await api.post<{ app: { kind: string; version?: string } }>('/migrate/probe', body());
			probe = { ok: true, text: `${appName(kind)}${r.app?.version ? ' ' + r.app.version : ''}` };
		} catch (err) {
			if (!quiet(err)) {
				error = message(err);
				probe = { ok: false, text: error };
			}
		} finally {
			checking = false;
		}
	}

	async function read(e: SubmitEvent) {
		e.preventDefault();
		if (busy) return;
		const k = kind;
		const b = body();
		const usedSaved = !!b.use_saved_key;
		ctrl = new AbortController();
		reading = k;
		changed();
		try {
			const r = await api.request<ReadAnswer>('/migrate/read', { method: 'POST', body: b, signal: ctrl.signal });
			key = '';
			signWord = '';
			open = false;
			onread(r, k, usedSaved);
		} catch (err) {
			if (quiet(err)) return;
			if (open) error = message(err);
			else toasts.push(`${appName(k)} was not read: ${message(err)}`, 'fail');
		} finally {
			reading = '';
			ctrl = null;
			key = '';
			signWord = '';
		}
	}

	/** stop ends the read: the request is aborted, so the server stops reading. */
	export function stop() {
		ctrl?.abort();
	}
</script>

<Sheet bind:open {title} size="md" placement="auto">
	<form id="move-read-form" bind:this={form} onsubmit={read}>
		<div class="srow">
			<div class="txt"><label for="move-app">App</label></div>
			<div class="ctl">
				<select id="move-app" class="input" bind:value={kind} onchange={changed} disabled={busy}>
					{#each shown as a (a.kind)}<option value={a.kind}>{a.name}</option>{/each}
				</select>
			</div>
		</div>
		{#if way === 'api'}
			{#if savedKey}
				<div class="srow" data-testid="use-saved-key">
					<div class="txt"><label for="move-saved">Use the saved key</label><span class="hint">Sent only to {savedKey.address}.</span></div>
					<div class="ctl">
						<label class="switch"><input id="move-saved" type="checkbox" bind:checked={useSaved} onchange={changed} /><span class="sw" aria-hidden="true"></span></label>
					</div>
				</div>
			{/if}
			<div class="srow">
				<div class="txt">
					{#if withSaved}<span class="label">Address</span>{:else}<label for="move-address">Address</label>{/if}
				</div>
				<div class="ctl">
					{#if withSaved}
						<span class="value mono addr" data-testid="saved-address">{savedKey?.address}</span>
					{:else}
						<input id="move-address" class="input mono" type="url" bind:value={address} oninput={changed} placeholder={placeholderFor(kind)} required autocomplete="off" spellcheck="false" />
					{/if}
				</div>
			</div>
			{#if !withSaved}
				{#if qbit}
					<div class="srow">
						<div class="txt"><label for="move-user">User name</label></div>
						<div class="ctl"><input id="move-user" class="input" bind:value={username} oninput={changed} autocomplete="off" required={!key} /></div>
					</div>
					<div class="srow">
						<div class="txt"><label for="move-pass">Password</label></div>
						<div class="ctl"><input id="move-pass" class="input" type="password" bind:value={signWord} oninput={changed} autocomplete="new-password" required={!key} /></div>
					</div>
					<div class="srow">
						<div class="txt"><label for="move-key">API key</label><span class="hint">Instead of a sign-in, qBittorrent 5.2+</span></div>
						<div class="ctl"><input id="move-key" class="input mono" type="password" bind:value={key} oninput={changed} autocomplete="new-password" spellcheck="false" /></div>
					</div>
				{:else}
					<div class="srow">
						<div class="txt"><label for="move-key">API key</label><span class="hint">Settings › General</span></div>
						<div class="ctl"><input id="move-key" class="input mono" type="password" bind:value={key} oninput={changed} autocomplete="new-password" spellcheck="false" required /></div>
					</div>
				{/if}
			{/if}
		{/if}
	</form>
	{#if busy}
		<div class="cap" role="status"><StepCapsule tone="go" live fill={0.4} status="processing" word="Reading {appName(reading)}" detail="this can take a few minutes" testid="read-capsule" /></div>
	{/if}
	{#if probe}<p class="probe" data-testid={probe.ok ? 'probe-ok' : undefined}><TestTag ok={probe.ok} text={probe.text} app={appName(kind)} /></p>{/if}
	{#if error}<p class="form-error" role="alert">{error}</p>{/if}
	{#snippet footer()}
		<Button variant="plain" onclick={() => (open = false)}>Cancel</Button>
		<Button onclick={check} busy={checking} disabled={busy}>Check connection</Button>
		<!-- One button that changes, so focus stays on it while reading. -->
		<Button variant="primary" type={busy ? 'button' : 'submit'} form={busy ? undefined : 'move-read-form'} onclick={busy ? stop : undefined}>{busy ? 'Stop reading' : `Read ${appName(kind)}`}</Button>
	{/snippet}
</Sheet>

<style>
	/* Its own row layout, so it looks the same outside Settings (the first-run wizard). */
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
	.probe, .form-error { margin-top: 12px; }
	.probe { margin-bottom: 0; display: flex; }
	select.input { width: 260px; }
	@media (max-width: 600px) {
		select.input { width: min(260px, 46vw); }
	}
</style>
