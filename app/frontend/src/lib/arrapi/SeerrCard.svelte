<script lang="ts">
	import Words from '#lib/ui/Words.svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
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

	const sideWord: Record<Side, { app: string }> = {
		sonarr: { app: 'Sonarr' },
		radarr: { app: 'Radarr' }
	};
	const sideWhat = (side: Side) => t(side === 'sonarr' ? 'arrapi.seerr.shows' : 'arrapi.seerr.movies');
	const sideWhatLower = (side: Side) => t(side === 'sonarr' ? 'arrapi.seerr.showsLower' : 'arrapi.seerr.moviesLower');

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
			toasts.push(t('arrapi.seerr.saveFail'), 'fail', { body: message(err) });
			await load();
		} finally {
			busy = '';
		}
	}

	async function makeKey(side: Side) {
		if (a?.sides.find((s) => s.side === side)?.key && !confirm(t('arrapi.seerr.replaceConfirm', { app: sideWord[side].app }))) return;
		busy = 'key-' + side;
		try {
			const r = await api.post<{ key: string }>(`/settings/old-app-api/keys/${side}`);
			fresh = { side, key: r.key };
			await load();
		} catch (err) {
			// A 403 asking for the password again is handled by Settings itself.
			if (!(err instanceof ApiError && err.body.code === 'reauth')) toasts.push(t('arrapi.seerr.makeFail'), 'fail', { body: message(err) });
		} finally {
			busy = '';
		}
	}

	async function removeKey(side: Side) {
		if (!confirm(t('arrapi.seerr.removeConfirm', { app: sideWord[side].app, what: sideWhatLower(side) }))) return;
		busy = 'del-' + side;
		try {
			await api.del(`/settings/old-app-api/keys/${side}`);
			if (fresh?.side === side) fresh = null;
			toasts.push(t('arrapi.seerr.removed'), 'ok');
			await load();
		} catch (err) {
			if (!(err instanceof ApiError && err.body.code === 'reauth')) toasts.push(t('arrapi.seerr.removeFail'), 'fail', { body: message(err) });
		} finally {
			busy = '';
		}
	}

	async function copy(text: string, done: string) {
		try {
			await navigator.clipboard.writeText(text);
			toasts.push(done, 'ok');
		} catch {
			toasts.push(t('arrapi.seerr.copyFail'), 'fail', { body: t('arrapi.seerr.copyFailBody') });
		}
	}
</script>

{#if !gone}
	<SettingsCard id="seerr-card" title={t('arrapi.seerr.title')} description={t('arrapi.seerr.description')} icon="inbox">
		{#if error && !a}
			<LoadError hasData={false} message={error} onretry={load} />
		{:else if !a}
			<p class="hint">{t('arrapi.seerr.loading')}</p>
		{:else}
			{#if error}<LoadError hasData message={error} onretry={load} />{/if}
			<div class="srow">
				<div class="txt"><label for="seerr-on">{t('arrapi.seerr.answer')}</label></div>
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
							<span class="label">{t('arrapi.seerr.addAs', { what: sideWhat(s.side), app: sideWord[s.side].app })}</span>
							<span class="hint"><Words key="arrapi.seerr.urlBase" vars={{ path: s.path }} codeClass="mono" /></span>
							{#if s.key}
								<span class="hint" data-testid="seerr-key-{s.side}">{s.key.last_used_at
									? t('arrapi.seerr.keyUsed', { hint: s.key.hint.replace(/^[.…]+/, ''), owner: s.key.owner, ago: ago(s.key.last_used_at) })
									: t('arrapi.seerr.keyNew', { hint: s.key.hint.replace(/^[.…]+/, ''), owner: s.key.owner })}</span>
								{#if !s.key.owner_active}<span class="hint warn">{t('arrapi.seerr.ownerOff', { owner: s.key.owner })}</span>{/if}
							{:else}
								<span class="hint">{t('arrapi.seerr.noKey')}</span>
							{/if}
						</div>
						<div class="ctl">
							<Button variant="plain" type="button" onclick={() => copy(s.path, t('arrapi.seerr.urlCopied'))} aria-label={t('arrapi.seerr.copyUrl', { app: sideWord[s.side].app })}><Icon name="copy" size={16} /></Button>
							{#if s.key}
								<Button variant="plain" type="button" onclick={() => removeKey(s.side)} aria-busy={busy === 'del-' + s.side}>{t('arrapi.seerr.removeKey')}</Button>
							{/if}
							<Button type="button" onclick={() => makeKey(s.side)} aria-busy={busy === 'key-' + s.side}>{s.key ? t('arrapi.seerr.newKey') : t('arrapi.seerr.makeKey')}</Button>
						</div>
						{#if fresh?.side === s.side}
							<div class="below fresh" role="status" data-testid="seerr-fresh">
								<p class="small"><b>{t('arrapi.seerr.freshTitle', { app: sideWord[s.side].app })}</b>{t('arrapi.seerr.freshBody')}</p>
								<code class="mono key">{fresh.key}</code>
								<div class="row acts">
									<Button icon="copy" type="button" onclick={() => copy(fresh?.key ?? '', t('arrapi.seerr.keyCopied'))}>{t('arrapi.seerr.copy')}</Button>
									<Button variant="plain" type="button" onclick={() => (fresh = null)}>{t('arrapi.seerr.close')}</Button>
								</div>
							</div>
						{/if}
					</div>
				{/each}
				<div class="srow">
					<div class="txt"><label for="seerr-delete">{t('arrapi.seerr.deleteFiles')}</label><span class="hint">{t('arrapi.seerr.deleteFilesHint')}</span></div>
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
