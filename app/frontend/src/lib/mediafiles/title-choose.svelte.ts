// The movie page's choose and turn down release dialogs.
import { chooseWarnings } from './releases.ts';
import { api, message, ApiError } from '#lib/core/api.ts';
import { toasts } from '#lib/core/toasts.svelte.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import type { Choice, ReleaseView, TitleView } from '#lib/core/types.ts';

export function createTitleChoose(ctx: { slotPick: number; title: TitleView | null; load: (which?: 'all' | 'title' | 'releases') => Promise<void>; relOffset: number }) {
	let chooseDialog: HTMLDialogElement | undefined = $state();
	let turnDialog: HTMLDialogElement | undefined = $state();
	let target = $state<ReleaseView | null>(null);
	let forceReasons = $state<string[]>([]);
	let chooseError = $state('');
	let choosing = $state(false);
	let turnReason = $state('');
	// The Slot a choice goes to: the picked Version, the one the Release matched, or the only one the movie has.
	const targetSlotId = $derived(ctx.slotPick || target?.slot_id || (ctx.title?.slots.length === 1 ? ctx.title.slots[0].id : 0));
	const targetSlot = $derived(ctx.title?.slots.find((s) => s.id === targetSlotId));
	const warnings = $derived(target ? chooseWarnings(target, targetSlot?.state) : []);
	const needsForce = $derived(!!target && (target.needs_force || forceReasons.length > 0));

	function askChoose(r: ReleaseView) {
		target = r;
		forceReasons = [];
		chooseError = '';
		chooseDialog?.showModal();
	}

	async function choose() {
		if (!target) return;
		choosing = true;
		chooseError = '';
		try {
			const body: Record<string, unknown> = { force: needsForce };
			if (targetSlotId) body.slot_id = targetSlotId;
			const ch = await api.post<Choice>(`/releases/${target.id}/choose`, body);
			chooseDialog?.close();
			toasts.push(t('mediafiles.title.sent'), 'ok', ch.warnings.length ? { body: ch.warnings.length === 1 ? ch.warnings[0] : t('mediafiles.title.warnings', { n: ch.warnings.length }) } : undefined);
			await ctx.load();
		} catch (e) {
			if (e instanceof ApiError && e.status === 409 && e.body.needs_force) {
				forceReasons = (e.body.reasons as string[] | undefined) ?? [t('mediafiles.title.didNotPass')];
			} else {
				chooseError = message(e);
			}
		} finally {
			choosing = false;
		}
	}

	function askTurnDown(r: ReleaseView) {
		target = r;
		turnReason = '';
		chooseError = '';
		turnDialog?.showModal();
	}

	async function turnDown(e: SubmitEvent) {
		e.preventDefault();
		if (!target) return;
		choosing = true;
		try {
			await api.post(`/releases/${target.id}/turn-down`, turnReason.trim() ? { reason: turnReason.trim() } : undefined);
			turnDialog?.close();
			toasts.push(t('mediafiles.title.turnedDown'), 'ok', { body: t('mediafiles.title.neverPicked') });
			await ctx.load('releases');
		} catch (err) {
			chooseError = message(err);
		} finally {
			choosing = false;
		}
	}

	function pickSlot(v: number) {
		ctx.slotPick = v;
		ctx.relOffset = 0;
		void ctx.load('releases');
	}

	return {
		get target() { return target; },
		set target(v) { target = v; },
		pickSlot,
		askChoose,
		askTurnDown,
		get chooseDialog() { return chooseDialog; },
		set chooseDialog(v) { chooseDialog = v; },
		get needsForce() { return needsForce; },
		get warnings() { return warnings; },
		get forceReasons() { return forceReasons; },
		set forceReasons(v) { forceReasons = v; },
		get chooseError() { return chooseError; },
		set chooseError(v) { chooseError = v; },
		choose,
		get choosing() { return choosing; },
		set choosing(v) { choosing = v; },
		get turnDialog() { return turnDialog; },
		set turnDialog(v) { turnDialog = v; },
		turnDown,
		get turnReason() { return turnReason; },
		set turnReason(v) { turnReason = v; },
	};
}
