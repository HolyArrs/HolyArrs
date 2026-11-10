// The move wizard's last steps: starting, finishing and undoing a move.
import { type Move } from './move.ts';
import { api, ApiError } from '#lib/core/api.ts';
import { plainMessage as message } from '#lib/core/settings.ts';
import { toasts } from '#lib/core/toasts.svelte.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export function createMoveRun(ctx: { moves: Move[]; picked: number[]; [key: string]: any }) {
	async function moveNow() {
		if (ctx.checking || ctx.starting) return;
		ctx.checking = true;
		ctx.actionError = '';
		try {
			// A change while it runs makes it old: run again (a few times at most).
			for (let i = 0; i < 3 && (!ctx.dry || ctx.stale || ctx.trying); i++) {
				await ctx.runTry();
				if (ctx.destroyed || ctx.tryFailed || ctx.gone) return;
			}
			if (!ctx.dry || ctx.stale || ctx.step !== 'choose') return;
			if (ctx.problems > 0) ctx.beforeOpen = true;
			else ctx.confirmMove = true;
		} finally {
			ctx.checking = false;
		}
	}
	function moveAnyway() {
		ctx.beforeOpen = false;
		void start();
	}
	async function startMove(e: SubmitEvent) {
		e.preventDefault();
		ctx.confirmMove = false;
		await start();
	}

	// Step 3: the move.
	async function start() {
		if (!ctx.dry || ctx.stale) return;
		let recheck = false;
		ctx.starting = true;
		ctx.actionError = '';
		try {
			const r = await api.post<{ move: Move }>('/migrate/moves', { ...ctx.choices(), plan_hash: ctx.planHash });
			ctx.current = r.move;
			ctx.progress = 0;
			ctx.progressNote = '';
			ctx.lost = false;
			ctx.moves = [r.move, ...ctx.moves.filter((m) => m.id !== r.move.id)];
			// Choose starts the move now (there is no Move step before it): show the Move step before following, so finished() lands on Done.
			ctx.step = 'move';
			ctx.beforeOpen = false;
			ctx.follower.follow(r.move.id);
			ctx.setUrl('move', r.move.id, false);
			// The Move button is gone now: keep focus on the step.
			ctx.focusStep();
		} catch (err) {
			if (ctx.quiet(err)) return;
			const text = message(err);
			if (err instanceof ApiError && err.status === 409 && /safe mode/i.test(text)) ctx.safeMode = true;
			else if (err instanceof ApiError && err.status === 409 && /changed/i.test(text)) {
				// The plan moved on since the test: check again and ask again.
				ctx.dryAt = -1;
				ctx.tryNotice = t('migrate.somethingChangedSoHolyarrs');
				// After finally: moveNow skips while starting is still true.
				recheck = true;
			} else if (err instanceof ApiError && err.status === 404) ctx.gone = true;
			else ctx.actionError = text;
		} finally {
			ctx.starting = false;
		}
		if (recheck) void moveNow();
	}

	/** Hears a move or an undo end. */
	function finished(m: Move) {
		ctx.moves = [m, ...ctx.moves.filter((x) => x.id !== m.id)];
		if (m.state === 'undone') toasts.push(t('migrate.moveUndone', { id: m.id }), 'ok');
		else if (m.undo_state === 'failed') toasts.push(t('migrate.couldnTUndoMove'), 'fail', { body: t('migrate.tryAgain') });
		else if (m.state === 'done') toasts.push(t('migrate.moveFinished', { id: m.id }), 'ok');
		else toasts.push(t('migrate.moveFailed', { id: m.id }), 'fail', { body: t('migrate.seeTheReport') });
		// A finished move forgets the kept keys.
		if (m.state === 'done') void ctx.loadSaved();
		if (m.state !== 'undone' && !m.undo_state) {
			// A finished move needs a new test before another one: the reads it used are switched off.
			ctx.dry = null;
			ctx.dryAt = -1;
			ctx.planHash = '';
			ctx.suggested = {};
			ctx.picked = ctx.picked.filter((id) => !m.snapshots.includes(id));
			ctx.changed();
			// Still on Choose for another round: it plans again with the reads left (or falls back to step 1).
			if (ctx.step === 'choose' || (ctx.step === 'move' && ctx.current?.id !== m.id)) ctx.enter('choose', false);
		}
		if (ctx.current?.id === m.id) {
			ctx.current = m;
			ctx.progress = 1;
			if (ctx.step === 'move' || ctx.step === 'done') {
				ctx.step = 'done';
				ctx.setUrl('done', m.id, true);
				ctx.focusStep();
			}
		}
	}

	// Step 4.
	async function undo(e: SubmitEvent) {
		e.preventDefault();
		ctx.confirmUndo = false;
		const m = ctx.current;
		if (!m || ctx.undoing) return;
		ctx.undoing = true;
		ctx.actionError = '';
		try {
			const r = await api.post<{ move: Move }>(`/migrate/moves/${m.id}/undo`);
			ctx.current = r.move;
			ctx.progress = 0;
			ctx.progressNote = '';
			ctx.lost = false;
			ctx.follower.follow(m.id);
			// The Undo button is gone now: keep focus on the step.
			ctx.focusStep();
		} catch (err) {
			if (ctx.quiet(err)) return;
			if (err instanceof ApiError && err.status === 409 && /safe mode/i.test(message(err))) ctx.safeMode = true;
			ctx.actionError = message(err);
		} finally {
			ctx.undoing = false;
		}
	}

	return {
		start,
		finished,
		moveNow,
		moveAnyway,
		startMove,
		undo,
	};
}
