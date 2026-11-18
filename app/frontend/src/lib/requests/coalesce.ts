/** Wraps fn so a burst of calls (a live event per removed request) runs it once, ms after the last one; cancel drops a call that is waiting (the page was left). */
export function coalesce(fn: () => void, ms: number): (() => void) & { cancel: () => void } {
	let timer: ReturnType<typeof setTimeout> | undefined;
	const soon = () => {
		clearTimeout(timer);
		timer = setTimeout(fn, ms);
	};
	soon.cancel = () => clearTimeout(timer);
	return soon;
}
