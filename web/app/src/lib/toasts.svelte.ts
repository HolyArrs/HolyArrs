// Small pop-up messages at the bottom of the screen.

export type ToastTone = 'ok' | 'fail' | 'info';
export interface Toast {
	id: number;
	text: string;
	tone: ToastTone;
	/** An optional button, such as Undo; pressing it runs and closes the toast. */
	action?: ToastAction;
}
export interface ToastAction {
	label: string;
	run: () => void;
}

export function createToasts(ms = 4200) {
	const list = $state<Toast[]>([]);
	let next = 1;

	function dismiss(id: number) {
		const i = list.findIndex((t) => t.id === id);
		if (i >= 0) list.splice(i, 1);
	}

	function push(text: string, tone: ToastTone = 'info', opts: { action?: ToastAction; ms?: number } = {}): number {
		const id = next++;
		list.push({ id, text, tone, action: opts.action });
		// Keep at most 4: drop plain notes first, so an Undo stays reachable while its decision waits.
		while (list.length > 4) {
			const plain = list.findIndex((t) => !t.action);
			list.splice(plain >= 0 ? plain : 0, 1);
		}
		setTimeout(() => dismiss(id), opts.ms ?? (tone === 'fail' ? ms * 2 : ms));
		return id;
	}

	return { list, push, dismiss };
}

export const toasts = createToasts();
