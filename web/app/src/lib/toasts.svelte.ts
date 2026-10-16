// Small pop-up messages at the bottom of the screen.

export type ToastTone = 'ok' | 'fail' | 'info';
export interface Toast {
	id: number;
	text: string;
	tone: ToastTone;
}

export function createToasts(ms = 4200) {
	const list = $state<Toast[]>([]);
	let next = 1;

	function dismiss(id: number) {
		const i = list.findIndex((t) => t.id === id);
		if (i >= 0) list.splice(i, 1);
	}

	function push(text: string, tone: ToastTone = 'info'): number {
		const id = next++;
		list.push({ id, text, tone });
		if (list.length > 4) list.splice(0, list.length - 4);
		setTimeout(() => dismiss(id), tone === 'fail' ? ms * 2 : ms);
		return id;
	}

	return { list, push, dismiss };
}

export const toasts = createToasts();
