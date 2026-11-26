<script lang="ts">
	// Paste a Release name: the server reads it, matches it to a Library title and
	// judges it with each copy's quality set, the same way a search does.
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import Field from '#lib/ui/Field.svelte';
	import Button from '#lib/ui/Button.svelte';
	import ReleaseTags from './ReleaseTags.svelte';
	import { verdict, verdictTone } from './releases.ts';
	import { nameCheckTags, type NameCheckResult } from './name-check.ts';
	import { api, message } from '#lib/core/api.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import type { PickRulesView } from '#lib/core/types.ts';

	let { rules }: { rules: PickRulesView[] } = $props();
	let name = $state('');
	let size = $state('');
	let rulesId = $state(0);
	let busy = $state(false);
	let error = $state('');
	let result = $state<NameCheckResult | null>(null);

	const sizeBytes = $derived.by(() => {
		const gb = Number(size.replace(',', '.'));
		return size.trim() && Number.isFinite(gb) && gb > 0 ? Math.round(gb * 1024 ** 3) : 0;
	});

	async function check(e: SubmitEvent) {
		e.preventDefault();
		if (!name.trim()) return;
		busy = true;
		error = '';
		try {
			result = await api.post<NameCheckResult>('/releases/name-check', { name: name.trim(), size: sizeBytes, pick_rules_id: rulesId });
		} catch (err) {
			result = null;
			error = message(err);
		} finally {
			busy = false;
		}
	}
</script>

<SettingsCard id="name-check" title={t('settings.pickRules.check.title')} description={t('settings.pickRules.check.desc')}>
	<form class="nc-form" onsubmit={check} data-testid="name-check">
		<Field id="nc-name" label={t('settings.pickRules.check.name')}>
			<input id="nc-name" class="input" maxlength="1000" autocomplete="off" spellcheck="false" bind:value={name} placeholder="Movie.Name.2024.1080p.BluRay.x264-GROUP" />
		</Field>
		<Field id="nc-rules" label={t('settings.pickRules.check.rules')}>
			<select id="nc-rules" class="input" bind:value={rulesId}>
				<option value={0}>{t('settings.pickRules.check.copies')}</option>
				{#each rules as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
			</select>
		</Field>
		<Field id="nc-size" label={t('settings.pickRules.check.size')}>
			<input id="nc-size" class="input" inputmode="decimal" autocomplete="off" bind:value={size} />
		</Field>
		<div class="nc-go"><Button variant="primary" type="submit" {busy} disabled={!name.trim()}>{t('settings.pickRules.check.check')}</Button></div>
	</form>

	{#if error}<p class="form-error" role="alert">{t('settings.pickRules.check.failed')}: {error}</p>{/if}

	{#if result}
		{@const p = result.parsed}
		<div class="nc-out" aria-live="polite" data-testid="name-check-result">
			<section>
				<h3 class="nc-h">{t('settings.pickRules.check.readAs')}</h3>
				<p class="nc-name"><b>{p.title?.value || '—'}</b>{#if p.year?.value}{' '}<span class="muted">({p.year.value})</span>{/if}</p>
				<p class="nc-tags"><ReleaseTags release={result} />{#each nameCheckTags(result) as tag (tag)}<span class="nc-tag">{tag}</span>{/each}</p>
			</section>
			<section>
				{#if result.title}
					<p class="nc-match" data-testid="name-check-title"><b>{t('settings.pickRules.check.matches', { name: result.title.year ? `${result.title.name} (${result.title.year})` : result.title.name })}</b></p>
					{#if result.title.detail}<p class="small muted">{serverWords(result.title.detail)}</p>{/if}
				{/if}
				{#if result.note === 'no_title' && !result.copies.length}<p class="small muted" data-testid="name-check-none">{t('settings.pickRules.check.noTitle')}</p>{/if}
				{#if result.note === 'no_album' && result.title}<p class="small muted">{t('settings.pickRules.check.noAlbum', { name: result.title.name })}</p>{/if}
				{#if result.title && !result.note && !result.copies.length}<p class="small muted">{t('settings.pickRules.check.noCopies')}</p>{/if}
			</section>
			{#if result.copies.length}
				<ul class="nc-copies">
					{#each result.copies as c, i (i)}
						<li class="nc-copy" class:off={!c.passed} data-testid="name-check-copy">
							<p class="nc-head">
								<b>{[c.version, c.album, c.episodes].filter(Boolean).join(' · ') || c.pick_rules}</b>
								<span class="nc-verdict {verdictTone({ state: 'judged', outcome: c.outcome })}">{verdict({ state: 'judged', outcome: c.outcome })}</span>
								{#if c.passed}<span class="nc-tag">{t('mediafiles.release.points', { n: c.points })}</span>{/if}
								{#if c.has_file}<span class="nc-tag">{t('settings.pickRules.check.hasFile')}</span>{/if}
							</p>
							{#if c.version}<p class="xs muted">{c.pick_rules}</p>{/if}
							<ul class="nc-why">{#each c.reasons as w, j (j)}<li class={w.effect}>{serverWords(w.text)}</li>{/each}</ul>
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	{/if}
</SettingsCard>

<style>
	.nc-form { display: grid; gap: 12px; }
	.nc-go { display: flex; justify-content: flex-end; }
	.nc-out { display: grid; gap: 16px; margin-top: 16px; }
	.nc-out section, .nc-out p { margin: 0; }
	.nc-out section { display: grid; gap: 6px; }
	.nc-h { margin: 0; color: var(--text-muted); font: 600 12.5px/1.2 var(--font-ui); }
	.nc-name { font: 600 15px/1.35 var(--font-ui); overflow-wrap: anywhere; }
	.nc-tags, .nc-head { display: flex; flex-wrap: wrap; align-items: center; gap: 5px; }
	.nc-tag, .nc-verdict { display: inline-flex; align-items: center; height: 20px; padding: 0 7px; border-radius: 5px; border: 1px solid color-mix(in srgb, var(--text) 28%, transparent); color: var(--text-muted); font: 700 11px/1 var(--font-ui); letter-spacing: .03em; white-space: nowrap; }
	.nc-verdict { border: 0; color: var(--text); background: color-mix(in srgb, var(--text) 7%, transparent); }
	.nc-verdict.ok { color: var(--ok-text); background: color-mix(in srgb, var(--ok) 14%, transparent); }
	.nc-verdict.fail { color: var(--fail-text); background: color-mix(in srgb, var(--fail) 12%, transparent); }
	.nc-verdict.wait { color: var(--wait-text); background: color-mix(in srgb, var(--wait) 14%, transparent); }
	.nc-head b { margin-right: 4px; overflow-wrap: anywhere; }
	.nc-match { font: 600 14px/1.35 var(--font-ui); }
	.nc-copies { display: grid; gap: 8px; margin: 0; padding: 0; list-style: none; }
	.nc-copy { display: grid; gap: 6px; padding: 14px 16px; border-radius: 14px; background: var(--surface-2); }
	.nc-why { display: grid; gap: 4px; margin: 0; padding: 0; list-style: none; color: var(--text-muted); font: 500 12.5px/1.35 var(--font-ui); overflow-wrap: anywhere; }
	.nc-why .fail { color: var(--fail-text); }
	.nc-why .hold, .nc-why .warn { color: var(--wait-text); }
	.nc-why .add, .nc-why .pass { color: var(--text); }
	@media (max-width: 600px) {
		.nc-go :global(button) { width: 100%; }
		.nc-copy { padding: 12px 12px 12px 14px; }
	}
</style>
