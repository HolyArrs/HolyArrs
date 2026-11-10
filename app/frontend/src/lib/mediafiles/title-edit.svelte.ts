// The movie page's edit dialog: the quality rules, the release gate and keeping it up to date.
import { api, message } from '#lib/core/api.ts';
import { toasts } from '#lib/core/toasts.svelte.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import type { PickRulesView, TitleView } from '#lib/core/types.ts';

export function createTitleEdit(ctx: any) {
	let editDialog: HTMLDialogElement | undefined = $state();
	let rules = $state<PickRulesView[]>([]);
	let eRules = $state(0);
	let eGate = $state('released');
	let eDelay = $state(0);
	let eKeep = $state(true);
	let editError = $state('');
	let editing = $state(false);
	async function openEdit() {
		if (!ctx.title) return;
		editError = '';
		eRules = ctx.title.pick_rules_id ?? 0;
		eGate = ctx.title.release_gate;
		eDelay = ctx.title.gate_delay_days;
		eKeep = ctx.title.keep_up_to_date;
		editing = true;
		editDialog?.showModal();
		if (!rules.length) {
			try {
				rules = (await api.get<{ pick_rules: PickRulesView[] }>('/pick-rules')).pick_rules.filter((r) => !r.kinds.length || r.kinds.includes('movie'));
			} catch (e) {
				editError = message(e);
			}
		}
	}
	async function saveEdit(e: SubmitEvent) {
		e.preventDefault();
		ctx.saving = true;
		editError = '';
		try {
			const body: Record<string, unknown> = { release_gate: eGate, gate_delay_days: Number(eDelay) || 0, keep_up_to_date: eKeep };
			if (eRules) body.pick_rules_id = eRules;
			const r = await api.put<{ title: TitleView }>(`/titles/${ctx.id}`, body);
			ctx.title = r.title;
			editDialog?.close();
			toasts.push(t('mediafiles.title.saved', { name: ctx.title?.name ?? t('mediafiles.title.titleWord') }), 'ok');
		} catch (err) {
			editError = message(err);
		} finally {
			ctx.saving = false;
		}
	}

	return {
		get rules() { return rules; },
		set rules(v) { rules = v; },
		openEdit,
		get editDialog() { return editDialog; },
		set editDialog(v) { editDialog = v; },
		get editing() { return editing; },
		set editing(v) { editing = v; },
		saveEdit,
		get eRules() { return eRules; },
		set eRules(v) { eRules = v; },
		get eGate() { return eGate; },
		set eGate(v) { eGate = v; },
		get eDelay() { return eDelay; },
		set eDelay(v) { eDelay = v; },
		get eKeep() { return eKeep; },
		set eKeep(v) { eKeep = v; },
		get editError() { return editError; },
		set editError(v) { editError = v; },
	};
}
