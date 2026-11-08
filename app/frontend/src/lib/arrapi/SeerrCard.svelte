<script lang="ts">
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import Button from '#lib/ui/Button.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { onMount } from 'svelte';
	import { api, ApiError } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { ago } from '#lib/core/format.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';

	// Seerr and Overseerr send approved requests to Sonarr and Radarr; this card switches on HolyArrs's small API that answers them in their place. Each side gets its own key, shown once.
	type Side = 'sonarr' | 'radarr';
	interface Key { side: Side; hint: string; owner: string; owner_active: boolean; created_at: string; last_used_at?: string }
	interface SideView { side: Side; path: string; url: string; key?: Key }
	interface Answer { enabled: boolean; delete_files: boolean; sides: SideView[] }

	const sideWord: Record<Side, { what: string; app: string }> = {
		sonarr: { what: 'Shows', app: 'Sonarr' },
		radarr: { what: 'Movies', app: 'Radarr' }
	};

	let a = $state<Answer | null>(null);
	let gone = $state(false); // An older server without this API: no card.
	let error = $state('');
	let busy = $state('');
	let fresh = $state<{ side: Side; key: string } | null>(null);

	async function load() {
		error = '';
		try {
			a = await api.get<Answer>('/settings/old-app-api');
		} catch (err) {
			if (err instanceof ApiError && err.status === 404) gone = true;
			else error = message(err);
		}
	}
	onMount(load);

	async function put(body: Partial<Pick<Answer, 'enabled' | 'delete_files'>>, what: string, el?: HTMLInputElement) {
		busy = what;
		try {
			a = await api.put<Answer>('/settings/old-app-api', body);
		} catch (err) {
			if (el) el.checked = !el.checked;
			toasts.push("Couldn't save", 'fail', { body: message(err) });
			await load();
		} finally {
			busy = '';
		}
	}

	async function makeKey(side: Side) {
		if (a?.sides.find((s) => s.side === side)?.key && !confirm(`Replace the ${sideWord[side].app} key? The current one stops working.`)) return;
		busy = 'key-' + side;
		try {
			const r = await api.post<{ key: string }>(`/settings/old-app-api/keys/${side}`);
			fresh = { side, key: r.key };
			await load();
		} catch (err) {
			// A 403 asking for the password again is handled by Settings itself.
			if (!(err instanceof ApiError && err.body.code === 'reauth')) toasts.push("Couldn't make key", 'fail', { body: message(err) });
		} finally {
			busy = '';
		}
	}

	async function removeKey(side: Side) {
		if (!confirm(`Remove the ${sideWord[side].app} key? Seerr stops sending ${sideWord[side].what.toLowerCase()} requests.`)) return;
		busy = 'del-' + side;
		try {
			await api.del(`/settings/old-app-api/keys/${side}`);
			if (fresh?.side === side) fresh = null;
			toasts.push('Key removed', 'ok');
			await load();
		} catch (err) {
			if (!(err instanceof ApiError && err.body.code === 'reauth')) toasts.push("Couldn't remove key", 'fail', { body: message(err) });
		} finally {
			busy = '';
		}
	}

	async function copy(text: string, what: string) {
		try {
			await navigator.clipboard.writeText(text);
			toasts.push(`${what} copied`, 'ok');
		} catch {
			toasts.push("Couldn't copy", 'fail', { body: 'Select it instead' });
		}
	}
</script>

{#if !gone}
	<SettingsCard id="seerr-card" title="Requests from Seerr and Overseerr" description="Add HolyArrs to Seerr or Overseerr as Sonarr and Radarr. Approved requests land in your Library." icon="inbox">
		{#if error && !a}
			<LoadError hasData={false} message={error} onretry={load} />
		{:else if !a}
			<p class="hint">Loading…</p>
		{:else}
			{#if error}<LoadError hasData message={error} onretry={load} />{/if}
			<div class="srow">
				<div class="txt"><label for="seerr-on">Answer Seerr and Overseerr</label></div>
				<div class="ctl">
					<label class="switch">
						<input id="seerr-on" type="checkbox" checked={a.enabled} disabled={busy === 'on'} aria-busy={busy === 'on'} onchange={(e) => put({ enabled: e.currentTarget.checked }, 'on', e.currentTarget)} data-testid="seerr-on" />
						<span class="sw" aria-hidden="true"></span>
					</label>
				</div>
			</div>
			{#if a.enabled}
				{#each a.sides as s (s.side)}
					<div class="srow side" data-testid="seerr-side-{s.side}">
						<div class="txt">
							<span class="label">{sideWord[s.side].what}: add as a {sideWord[s.side].app} server</span>
							<span class="hint">URL base <code class="mono">{s.path}</code>, with HolyArrs's address and port.</span>
							{#if s.key}
								<span class="hint" data-testid="seerr-key-{s.side}">Key ending in {s.key.hint.replace(/^[.…]+/, '')}, made by {s.key.owner}{s.key.last_used_at ? `, last used ${ago(s.key.last_used_at)}` : ', not used yet'}.</span>
								{#if !s.key.owner_active}<span class="hint warn">{s.key.owner}'s account is off or may no longer manage the Library, so this key is refused. Make a new one.</span>{/if}
							{:else}
								<span class="hint">No key</span>
							{/if}
						</div>
						<div class="ctl">
							<Button variant="plain" type="button" onclick={() => copy(s.path, 'URL base')} aria-label="Copy the {sideWord[s.side].app} URL base"><Icon name="copy" size={16} /></Button>
							{#if s.key}
								<Button variant="plain" type="button" onclick={() => removeKey(s.side)} aria-busy={busy === 'del-' + s.side}>Remove key</Button>
							{/if}
							<Button type="button" onclick={() => makeKey(s.side)} aria-busy={busy === 'key-' + s.side}>{s.key ? 'New key' : 'Make key'}</Button>
						</div>
						{#if fresh?.side === s.side}
							<div class="below fresh" role="status" data-testid="seerr-fresh">
								<p class="small"><b>{sideWord[s.side].app} key</b>: copy it now and paste it into Seerr or Overseerr as the API key. It is shown only once.</p>
								<code class="mono key">{fresh.key}</code>
								<div class="row acts">
									<Button icon="copy" type="button" onclick={() => copy(fresh?.key ?? '', 'Key')}>Copy</Button>
									<Button variant="plain" type="button" onclick={() => (fresh = null)}>Close</Button>
								</div>
							</div>
						{/if}
					</div>
				{/each}
				<div class="srow">
					<div class="txt"><label for="seerr-delete">Let these apps delete files</label><span class="hint">Seerr stores keys in plain text. Leave off unless needed.</span></div>
					<div class="ctl">
						<label class="switch">
							<input id="seerr-delete" type="checkbox" checked={a.delete_files} disabled={busy === 'delete'} aria-busy={busy === 'delete'} onchange={(e) => put({ delete_files: e.currentTarget.checked }, 'delete', e.currentTarget)} data-testid="seerr-delete" />
							<span class="sw" aria-hidden="true"></span>
						</label>
					</div>
				</div>
			{/if}
		{/if}
	</SettingsCard>
{/if}

<style>
	.side .ctl { flex-wrap: wrap; justify-content: flex-end; }
	.warn { color: var(--wait-text, var(--text)); }
	.fresh { display: flex; flex-direction: column; gap: 8px; padding: var(--s-4); border: 1px solid var(--ha-red); background: var(--ha-red-soft); border-radius: var(--r-btn); }
	.key { word-break: break-all; font-size: 14px; }
	.acts { gap: 8px; }
	@media (max-width: 600px) {
		.side { grid-template-columns: minmax(0, 1fr); }
		.side .ctl { justify-self: start; justify-content: flex-start; }
	}
</style>
