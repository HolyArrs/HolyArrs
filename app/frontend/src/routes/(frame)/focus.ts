// Focus rings follow the keyboard. Browsers treat a clicked text field as :focus-visible, so <html data-input> records how the person last moved: "keyboard" after Tab, arrow keys outside a text field, or ⌘K / Ctrl+K; "pointer" after a mouse, pen or touch press. app.css hides the ring on text fields while it is "pointer" (they get a quiet tint instead).

const textField = (el: EventTarget | null) =>
	el instanceof HTMLElement && (el.isContentEditable || el instanceof HTMLTextAreaElement || (el instanceof HTMLInputElement && !['checkbox', 'radio', 'range', 'file', 'button', 'submit', 'reset', 'color'].includes(el.type)));

/** Whether a key press means the person is moving focus by keyboard. */
export function isNavigationKey(e: Pick<KeyboardEvent, 'key' | 'metaKey' | 'ctrlKey' | 'altKey'>, inTextField: boolean): boolean {
	if (e.key === 'Tab') return true;
	if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') return true;
	if (inTextField || e.altKey || e.metaKey || e.ctrlKey) return false;
	return ['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'Home', 'End', 'PageUp', 'PageDown', 'Enter', ' '].includes(e.key);
}

/** Keeps <html data-input> up to date; returns a cleanup. */
export function trackInputModality(root: HTMLElement = document.documentElement): () => void {
	root.dataset.input = 'pointer';
	const key = (e: KeyboardEvent) => {
		if (isNavigationKey(e, textField(e.target))) root.dataset.input = 'keyboard';
	};
	const pointer = () => (root.dataset.input = 'pointer');
	window.addEventListener('keydown', key, true);
	window.addEventListener('pointerdown', pointer, true);
	return () => {
		window.removeEventListener('keydown', key, true);
		window.removeEventListener('pointerdown', pointer, true);
	};
}
