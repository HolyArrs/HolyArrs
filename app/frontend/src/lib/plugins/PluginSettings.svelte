<script lang="ts">
	// A plugin's own settings, inside its dialog: the fields its manifest declares, grouped, with Back and Save pinned below.
	// Secrets are never shown back: a set secret shows as "Saved"; Replace types a new one, Clear drops it.
	// A compact table in five refined looks T1-T5; the lab's Design preview bar switches them, a release build uses T1.
	// Every control has one height that matches the row; help text wraps so nothing is cut.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { shownName, shownVersion, type Plugin, type SettingsView } from './index.ts';

	type Field = SettingsView['fields'][number];
	type SecId = 'conn' | 'perm' | 'sec';

	let { p, onback }: { p: Plugin; onback: () => void } = $props();

	let view = $state<SettingsView | null>(null);
	let vals = $state<Record<string, string>>({});
	let secrets = $state<Record<string, string>>({});
	let clear = $state<Record<string, boolean>>({});
	let err = $state('');
	let saving = $state(false);
	const path = $derived(`/plugins/${encodeURIComponent(p.id)}/settings`);

	// Fields fall into three plain groups: how it is reached, what it may do, and its secrets.
	const sections = $derived.by(() => {
		const f = view?.fields ?? [];
		const all: { id: SecId; title: string; list: Field[] }[] = [
			{ id: 'conn', title: t('plugins.groupConnection'), list: f.filter((x) => x.type !== 'toggle' && x.type !== 'secret') },
			{ id: 'perm', title: t('plugins.groupPermissions'), list: f.filter((x) => x.type === 'toggle') },
			{ id: 'sec', title: t('plugins.groupSecurity'), list: f.filter((x) => x.type === 'secret') }
		];
		return all.filter((s) => s.list.length);
	});

	const whole = (v: unknown) => v == null || String(v).trim() === '' || /^\d+$/.test(String(v).trim());
	const bad = $derived(view ? view.fields.filter((f) => f.type === 'number' && !whole(vals[f.key])).map((f) => f.key) : []);

	async function load() {
		err = '';
		try {
			view = await api.get<SettingsView>(path);
			vals = { ...view.values };
			secrets = {};
			clear = {};
		} catch (e) {
			err = message(e);
		}
	}
	$effect(() => {
		void path;
		void load();
	});

	function replace(id: string, key: string) {
		clear[key] = false;
		const el = document.getElementById(id) as HTMLInputElement | null;
		el?.focus();
		el?.select();
	}

	async function save(e: SubmitEvent) {
		e.preventDefault();
		if (!view || saving || bad.length) return;
		saving = true;
		const values: Record<string, string> = Object.fromEntries(Object.entries(vals).map(([k, v]) => [k, v == null ? '' : String(v).trim()]));
		for (const f of view.fields) if (f.type === 'secret' && secrets[f.key]) values[f.key] = secrets[f.key];
		try {
			view = await api.put<SettingsView>(path, { values, clear: Object.keys(clear).filter((k) => clear[k]) });
			vals = { ...view.values };
			secrets = {};
			clear = {};
			toasts.push(t('plugins.settingsSaved', { name: shownName(p) }), 'ok');
			// Saved, but the running plugin did not start again with the new values.
			if (view.start_error) toasts.push(t('plugins.startFailed', { name: shownName(p) }), 'fail', { body: view.start_error });
		} catch (e) {
			toasts.push(t('plugins.settingsFailed'), 'fail', { body: message(e) });
		} finally {
			saving = false;
		}
	}
</script>

{#snippet control(f: Field, id: string, desc: string | undefined)}
	{#if f.type === 'toggle'}
		<label class="switch">
			<input {id} type="checkbox" aria-describedby={desc} checked={vals[f.key] === 'true'} onchange={(e) => (vals[f.key] = e.currentTarget.checked ? 'true' : 'false')} />
			<span class="sw" aria-hidden="true"></span>
		</label>
	{:else if f.type === 'select'}
		<select {id} class="input" aria-describedby={desc} bind:value={vals[f.key]}>
			{#each f.options ?? [] as o (o)}<option value={o}>{o}</option>{/each}
		</select>
	{:else if f.type === 'secret'}
		{@const set = !!view?.secrets_set[f.key]}
			<div class="secret" class:set class:cleared={clear[f.key]}>
				<span class="sin">
					{#if set && !clear[f.key] && !secrets[f.key]}<span class="lock" aria-hidden="true"><Icon name="check" size={14} /></span>{/if}
					<input {id} class="input" type="password" autocomplete="off" aria-describedby={desc} disabled={!!clear[f.key]} placeholder={set && !clear[f.key] ? t('plugins.secretSaved') : ''} bind:value={secrets[f.key]} />
				</span>
				{#if set}
					<span class="sbtn">
						{#if !clear[f.key]}<Button variant="plain" size="sm" type="button" onclick={() => replace(id, f.key)}>{t('plugins.secretReplace')}</Button>{/if}
						<Button variant="plain" size="sm" type="button" aria-pressed={!!clear[f.key]} onclick={() => (clear[f.key] = !clear[f.key])}>{clear[f.key] ? t('plugins.secretKeep') : t('plugins.secretClear')}</Button>
					</span>
				{/if}
			</div>
	{:else}
		<input {id} class="input" class:num={f.type === 'number'} type="text" inputmode={f.type === 'number' ? 'numeric' : undefined} aria-invalid={bad.includes(f.key) || undefined} aria-describedby={desc} bind:value={vals[f.key]} />
	{/if}
{/snippet}

<form class="ps" onsubmit={save} data-testid="plugin-settings-form">
	<header class="phd"><h3 class="ph" dir="auto">{t('plugins.settingsName', { name: shownName(p) })}</h3><p class="psub xs muted" dir="auto">{shownVersion(p)}{#if p.author} · {p.author}{/if}</p></header>
	{#if err}
		<LoadError hasData={false} message={err} onretry={load} />
	{:else if view}
		{#if !view.fields.length}<p class="small muted">{t('plugins.settingsNone')}</p>{/if}
		<div class="body">
			<div class="secs">
				{#each sections as s (s.id)}
					<section class="sec s-{s.id}" aria-labelledby="ps-{p.id}-h-{s.id}">
						<h4 class="sh" id="ps-{p.id}-h-{s.id}">{s.title}</h4>
						<div class="rows fit-grid" style:--fit-col="340px" style:--fit-gap="0 16px">
							{#each s.list as f (f.key)}
								{@const id = `ps-${p.id}-${f.key}`}
								{@const isBad = bad.includes(f.key)}
								{@const desc = [f.help ? `${id}-h` : '', isBad ? `${id}-e` : ''].filter(Boolean).join(' ') || undefined}
								<div class="prow t-{f.type}" class:bad={isBad}>
									<div class="txt">
										<label class="label" for={id} dir="auto">{f.label}</label>
										{#if f.help}<span class="hint" id="{id}-h" dir="auto">{f.help}</span>{/if}
									</div>
									<div class="ctl">{@render control(f, id, desc)}</div>
									{#if isBad}<span class="err" id="{id}-e" role="alert">{t('plugins.numberWhole')}</span>{/if}
								</div>
							{/each}
						</div>
					</section>
				{/each}
			</div>
		</div>
	{/if}
	<div class="pact">
		<Button variant="plain" type="button" onclick={onback} data-testid="plugin-settings-back">{t('plugins.back')}</Button>
		<Button variant="primary" type="submit" busy={saving} disabled={!view || !view.fields.length || bad.length > 0} data-testid="plugin-settings-save">{t('plugins.save')}</Button>
	</div>
</form>

<style>
	/* Shared frame (all looks): one control height (--h) that matches the row, help wraps under the label, Back and Save pinned
	   without a divider. Looks T1-T5 change only surfaces, borders and where the section names sit. */
	.ps { --h: 30px; --row: 40px; --ctl-w: 220px; display: flex; flex-direction: column; gap: 10px; padding: var(--s-4) var(--s-4) 0; min-width: 0; }
	.phd { display: grid; gap: 2px; min-width: 0; }
	.ph { margin: 0; font-size: 17px; font-weight: 600; letter-spacing: normal; overflow-wrap: anywhere; }
	.psub { margin: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.body { display: grid; min-width: 0; }
	.secs { display: grid; gap: 10px; min-width: 0; }
	.sec { display: grid; gap: 6px; min-width: 0; }
	.sh { margin: 0; padding-inline: 2px; font-size: 11px; font-weight: 600; letter-spacing: 0.02em; color: var(--text-muted); }
	.rows { display: grid; min-width: 0; }
	.prow { display: grid; grid-template-columns: minmax(0, 1fr) auto; grid-template-areas: 'lab ctl' 'hint hint' 'err err'; align-items: center; column-gap: var(--s-4); row-gap: 2px; min-height: var(--row); padding: 4px 12px; min-width: 0; transition: background-color 150ms ease; }
	.txt { display: contents; }
	.label { grid-area: lab; }
	.hint { grid-area: hint; }
	.ctl { grid-area: ctl; display: flex; justify-content: flex-end; align-items: center; min-width: 0; }
	.label { font-size: 13px; font-weight: 600; line-height: 18px; overflow-wrap: anywhere; min-width: 0; }
	.hint { color: var(--text-muted); font-size: 12px; line-height: 15px; overflow-wrap: anywhere; min-width: 0; }
	.err { grid-area: err; color: var(--fail, #e5484d); font: var(--t-footnote); padding-top: 4px; }
	.ctl .input { box-sizing: border-box; width: var(--ctl-w); max-width: 100%; height: var(--h); min-height: var(--h); padding-block: 0; font-size: 13px; }
	.ctl .input.num { width: 96px; text-align: start; font-variant-numeric: tabular-nums; }
	.ctl .switch { height: var(--h); min-height: var(--h); display: inline-flex; align-items: center; }
	.ctl .input:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }
	.prow.bad .input { border-color: var(--fail, #e5484d); }
	.secret { display: flex; align-items: center; gap: 6px; width: 100%; justify-content: flex-end; min-width: 0; }
	.sin { position: relative; display: flex; flex: 0 1 var(--ctl-w); min-width: 0; }
	.sin .input { width: 100%; }
	.secret.set .sin .input::placeholder { color: var(--ok, var(--accent)); font-weight: 600; opacity: 1; }
	.lock { position: absolute; inset-inline-end: 10px; top: 50%; translate: 0 -50%; display: grid; color: var(--ok, var(--accent)); pointer-events: none; }
	.secret.cleared .sin .input { text-decoration: line-through; opacity: 0.6; }
	/* Replace / Clear: small bordered buttons of the control height, so they weigh the same as the field. */
	.sbtn { display: flex; gap: 4px; flex: none; }
	.sbtn :global(.ui-btn) { height: var(--h); min-height: var(--h); padding: 0 10px; font-size: 12px; border-radius: 8px; box-shadow: 0 0 0 1px var(--line-strong); }
	.pact { position: sticky; bottom: 0; z-index: 1; display: flex; justify-content: flex-end; gap: var(--s-2); margin-inline: calc(-1 * var(--s-4)); padding: 8px var(--s-4) 12px; background: linear-gradient(to bottom, transparent, var(--surface-1) 30%); }
	.pact :global(.ui-btn) { height: var(--h); min-height: var(--h); padding: 0 14px; font-size: 13px; border-radius: 999px; }

	/* Quiet inset rows: each row its own soft inset tile; the section name is a small label above. */
	.rows { gap: 4px; }
	.prow { border-radius: 10px; background: color-mix(in srgb, var(--surface-2) 70%, transparent); box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--line) 60%, transparent); }
	.prow:hover, .prow:focus-within { background: var(--surface-2); }


	/* The rows go into as many columns as fit (the shared .fit-grid), so even a long form fits one screen. */
	.ps { container-type: inline-size; }
	@container (min-width: 900px) {
		.ps { --ctl-w: 160px; }
	}

	/* Narrow screens: text fields drop under their label; toggles and short numbers stay beside it. */
	@media (max-width: 600px) {
		.ps { --ctl-w: 100%; }
		.prow:is(.t-text, .t-secret, .t-select) { grid-template-columns: minmax(0, 1fr); grid-template-areas: 'lab' 'hint' 'ctl' 'err'; row-gap: 6px; }
		.prow:is(.t-text, .t-secret, .t-select) .ctl, .prow .secret { justify-content: flex-start; }
		.sin { flex: 1 1 auto; }
	}
	@media (prefers-reduced-motion: reduce) { .prow { transition: none; } }
</style>
