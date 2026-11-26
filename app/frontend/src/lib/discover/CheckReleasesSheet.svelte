<script lang="ts">
	// Check releases for a movie not in the Library yet: searches only when asked, judged by the Pick rules picked here.
	import { untrack } from 'svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { api } from '#lib/core/api.ts';
	import { ReleaseSheet, savedDefaults, startChoices, type ReleaseCheck } from '#lib/mediafiles/index.ts';
	import type { LibraryFolder, LookupMovie, PickRulesView, ReleasePage, ReleaseView } from '#lib/core/types.ts';

	let { open = $bindable(false), movie, folders, rules, onpick }: { open?: boolean; movie: LookupMovie; folders: LibraryFolder[]; rules: PickRulesView[]; onpick: (r: ReleaseView, rulesId: number, checkId: number) => void } = $props();

	const movieRules = $derived(rules.filter((r) => !r.kinds.length || r.kinds.includes('movie')));
	let rulesId = $state(0);
	let checkId = $state(0);
	// Each open starts from the saved Pick rules and no results, so nothing searches until asked.
	// Only opening (or the offered rules changing) resets: it must not re-run when Pick rules are picked here (#714).
	$effect(() => {
		if (!open) return;
		const offered = movieRules;
		untrack(() => {
			checkId = 0;
			// Until the saved defaults answer (or if they fail), the first offered Pick rules stand, so a check never goes out without one.
			if (!offered.some((r) => r.id === rulesId)) rulesId = offered[0]?.id ?? 0;
			const opened = sheetKey;
			void savedDefaults.load().then((d) => {
				if (!checkId && sheetKey === opened) rulesId = startChoices(d, folders, rules, 'movie').rulesId || rulesId;
			}).catch(() => {});
		});
	});
	// Other Pick rules judge differently: the old results go and the sheet asks again.
	let sheetKey = $state(0);
	function setRules(id: number) {
		rulesId = id;
		checkId = 0;
		sheetKey++;
	}

	const empty: ReleasePage = { releases: [], total: 0, limit: 0, offset: 0, running_job_ids: [] };
	const check: ReleaseCheck = {
		load: () => (checkId ? api.get<ReleasePage>(`/releases/check/${checkId}`) : Promise.resolve(empty)),
		async search() {
			const r = await api.post<{ job_ids: number[] }>('/releases/check', {
				tmdb: Number(movie.ids.tmdb),
				imdb: movie.ids.imdb ?? '',
				name: movie.name,
				original_language: movie.original_language ?? '',
				year: movie.year || 0,
				pick_rules_id: rulesId
			});
			checkId = r.job_ids[0] ?? 0;
			return r.job_ids;
		},
		async pick(r) {
			onpick(r, rulesId, checkId);
		}
	};
</script>

{#key sheetKey}
	<ReleaseSheet bind:open titleId={0} name={movie.name} {check}>
		{#snippet head()}
			<label class="cr-rules">
				<span>{t('requests.add.rules')}</span>
				<select value={rulesId} onchange={(e) => setRules(Number(e.currentTarget.value))} data-testid="check-rules">
					{#each movieRules as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
				</select>
			</label>
		{/snippet}
	</ReleaseSheet>
{/key}

<style>
	.cr-rules { display: inline-flex; align-items: center; gap: 8px; color: var(--text-muted); font: 600 12.5px/1 var(--font-ui); }
	.cr-rules select { height: 34px; max-width: 100%; padding: 0 12px; border: 0; border-radius: 999px; background: color-mix(in srgb, var(--text) 7%, transparent); color: var(--text); font: 600 13px/1 var(--font-ui); }
	@media (pointer: coarse) { .cr-rules select { height: 44px; } }
</style>
