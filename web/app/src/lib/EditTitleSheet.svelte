<script lang="ts">
	import { untrack } from 'svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import { api, message } from '#lib/api.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import TagInput from '#lib/ui/TagInput.svelte';
	import { forgetTags } from '#lib/tags.ts';
	import type { LibraryFolder, PickRulesView, TitleView } from '#lib/types.ts';

	// Edit a movie or show from its Library tab: keep up to date, which seasons
	// to keep (a quick choice that sets every season at once), new seasons,
	// quality, episode order, tags and folder. Nothing changes until Save.
	let { open = $bindable(false), title, onsaved }: { open?: boolean; title: TitleView; onsaved?: () => void } = $props();

	type Preset = 'asis' | 'all' | 'latest' | 'missing' | 'future' | 'none';
	let keep = $state(true);
	let preset = $state<Preset>('asis');
	let newSeasons = $state('all');
	let rules = $state(0);
	let order = $state('aired');
	let folder = $state(0);
	let tags = $state<string[]>([]);
	let ruleList = $state<PickRulesView[]>([]);
	let folders = $state<LibraryFolder[]>([]);
	let busy = $state(false);
	let error = $state('');

	$effect(() => {
		if (!open) return;
		untrack(() => {
			keep = title.keep_up_to_date;
			preset = 'asis';
			newSeasons = title.show?.new_seasons || 'all';
			rules = title.pick_rules_id ?? 0;
			order = title.show?.episode_order || 'aired';
			folder = title.library_folder_id ?? 0;
			tags = [...(title.tags ?? [])];
			error = '';
		});
		void api.get<{ pick_rules: PickRulesView[] }>('/pick-rules').then((r) => (ruleList = r.pick_rules.filter((x) => x.kinds.includes(title.kind))), () => {});
		void api.get<{ library_folders: LibraryFolder[] }>('/library-folders').then((r) => (folders = r.library_folders.filter((x) => x.kind === (title.kind === 'artist' ? 'music' : title.kind))), () => {});
	});

	const seasons = $derived((title.show?.seasons ?? []).filter((s) => s.number > 0));
	const latest = $derived(Math.max(0, ...seasons.map((s) => s.number)));
	// Which seasons each choice keeps up to date.
	function wants(p: Preset, n: number, missing: number, notOut: number): boolean | null {
		if (p === 'asis') return null;
		if (p === 'all') return true;
		if (p === 'none') return false;
		if (p === 'latest') return n === latest;
		if (p === 'missing') return missing > 0;
		return notOut > 0; // future: only seasons still airing
	}
	const presetNote = $derived.by(() => {
		if (preset === 'asis') return '';
		const n = seasons.filter((s) => wants(preset, s.number, s.missing, s.not_out_yet)).length;
		return n === 0 ? 'No season will be kept up to date.' : `${n} of ${seasons.length} seasons will be kept up to date.`;
	});
	const moving = $derived(folder !== 0 && folder !== (title.library_folder_id ?? 0));

	async function save() {
		busy = true;
		error = '';
		try {
			const body: Record<string, unknown> = {};
			if (keep !== title.keep_up_to_date) body.keep_up_to_date = keep;
			if (rules && rules !== title.pick_rules_id) body.pick_rules_id = rules;
			if (tags.join(',') !== (title.tags ?? []).join(',')) body.tags = tags;
			if (title.show) {
				if (newSeasons !== title.show.new_seasons) body.new_seasons = newSeasons;
				if (order !== title.show.episode_order) body.episode_order = order;
			}
			if (Object.keys(body).length) await api.put(`/titles/${title.id}`, body);
			if (body.tags) forgetTags();
			for (const s of seasons) {
				const w = wants(preset, s.number, s.missing, s.not_out_yet);
				if (w !== null && w !== s.keep_up_to_date) await api.put(`/titles/${title.id}/seasons/${s.number}`, { keep_up_to_date: w });
			}
			if (moving) {
				await api.post(`/titles/${title.id}/move`, { library_folder_id: folder, apply: true });
				toasts.push('Moving the files to the new folder now.', 'ok');
			}
			toasts.push('Saved.', 'ok');
			open = false;
			onsaved?.();
		} catch (e) {
			error = message(e);
		} finally {
			busy = false;
		}
	}
</script>

<Sheet bind:open title={`Edit ${title.name}`} description="Nothing changes until you press Save." placement="right" size="md">
	<div class="et" data-testid="edit-sheet">
		<div class="et-rows">
			<button type="button" class="et-row" role="switch" aria-checked={keep} onclick={() => (keep = !keep)}>
				<span><b>Keep up to date</b><small>Grab what is missing and upgrade when a better copy shows up</small></span>
				<span class="et-switch" class:on={keep} aria-hidden="true"></span>
			</button>
		</div>

		{#if title.show}
			<label class="et-field">
				<span><b>Seasons to keep</b><small>Sets every season at once. You can still change one season on its own.</small></span>
				<select bind:value={preset}>
					<option value="asis">Leave as they are</option>
					<option value="all">All seasons</option>
					<option value="missing">Seasons with missing episodes</option>
					<option value="future">Seasons still airing</option>
					<option value="latest">Latest season only</option>
					<option value="none">No seasons</option>
				</select>
				{#if presetNote}<small class="et-note">{presetNote}</small>{/if}
			</label>
			<label class="et-field">
				<span><b>New seasons</b><small>What happens when a new season is announced</small></span>
				<select bind:value={newSeasons}>
					<option value="all">Keep them up to date</option>
					<option value="none">Leave them alone</option>
				</select>
			</label>
		{/if}

		<label class="et-field">
			<span><b>Quality</b><small>The rules that decide which release wins</small></span>
			<select bind:value={rules}>
				{#each ruleList as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
			</select>
		</label>

		{#if title.show}
			<label class="et-field">
				<span><b>Episode order</b><small>How episodes are numbered when naming and matching files</small></span>
				<select bind:value={order}>
					<option value="aired">As aired (S01E01)</option>
					<option value="daily">By date (daily shows)</option>
					<option value="absolute">Absolute (anime)</option>
				</select>
			</label>
		{/if}

		<div class="et-field">
			<label for="et-tags"><span><b>Tags</b><small>Sends this title's downloads and searches to the Downloaders and Sources with the same tag</small></span></label>
			<TagInput id="et-tags" bind:value={tags} testid="et-tags" />
		</div>

		<label class="et-field">
			<span><b>Folder</b><small class="et-path">{title.folder_path ?? ''}</small></span>
			<select bind:value={folder}>
				{#each folders as f (f.id)}<option value={f.id}>{f.label} · {f.path}</option>{/each}
			</select>
			{#if moving}<small class="et-note warn">Saving moves its files to the new folder.</small>{/if}
		</label>

		{#if error}<p class="et-err" role="alert">{error}</p>{/if}
		<div class="et-actions">
			<button type="button" class="et-btn" onclick={() => (open = false)}>Cancel</button>
			<button type="button" class="et-btn primary" disabled={busy} onclick={save}>{busy ? 'Saving…' : 'Save'}</button>
		</div>
	</div>
</Sheet>

<style>
	.et { display: grid; gap: 14px; }
	.et-rows { display: grid; border-radius: 14px; background: var(--surface-2); overflow: hidden; }
	.et-row { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 14px 16px; border: 0; background: none; color: var(--text); text-align: left; cursor: pointer; font: inherit; }
	.et-row span:first-child, .et-field > span, .et-field > label > span { display: grid; gap: 3px; }
	.et-row b, .et-field b { font: 600 15px/1.2 var(--font-ui); }
	.et-row small, .et-field small { color: var(--text-muted); font: 500 13px/1.35 var(--font-ui); }
	.et-switch { position: relative; flex: none; width: 44px; height: 26px; border-radius: 99px; background: color-mix(in srgb, var(--text) 18%, transparent); transition: background var(--dur-fast) var(--ease-out); }
	.et-switch::after { content: ''; position: absolute; top: 3px; left: 3px; width: 20px; height: 20px; border-radius: 99px; background: #fff; box-shadow: 0 1px 3px rgb(0 0 0 / 25%); transition: transform var(--dur-fast) var(--ease-out); }
	.et-switch.on { background: #30a46c; }
	.et-switch.on::after { transform: translateX(18px); }
	.et-field { display: grid; gap: 8px; padding: 14px 16px; border-radius: 14px; background: var(--surface-2); }
	.et-field select { height: 40px; padding: 0 12px; border: 1px solid color-mix(in srgb, var(--text) 14%, transparent); border-radius: 10px; background: var(--surface-1, var(--bg)); color: var(--text); font: 500 14px/1 var(--font-ui); }
	.et-path { font-family: var(--font-mono, ui-monospace, monospace) !important; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.et-note { color: var(--text-muted); }
	.et-note.warn { color: #d4a72c !important; }
	.et-err { margin: 0; color: var(--danger, #e5484d); font: 500 14px/1.4 var(--font-ui); }
	.et-actions { display: flex; justify-content: flex-end; gap: 8px; }
	.et-btn { height: 40px; padding: 0 20px; border: 0; border-radius: 999px; background: color-mix(in srgb, var(--text) 8%, transparent); color: var(--text); font: 600 14px/1 var(--font-ui); cursor: pointer; }
	.et-btn.primary { background: var(--text); color: var(--bg); }
	.et-btn:disabled { opacity: .5; }
</style>
