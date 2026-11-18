// In-app yes/no question for Remove, Restore, Stop and similar actions, in place of the browser's own confirm() box.
// `yes` names the action on the button ("Remove", "Restore"); `danger` paints it red for actions that lose something.
export type AskOptions = { yes?: string; danger?: boolean };
type Asked = { message: string; yes?: string; danger: boolean; resolve: (yes: boolean) => void };

export const asking = $state<{ now: Asked | null }>({ now: null });

// Resolves true when the person confirms, false when they cancel or close the sheet.
export function ask(message: string, opts: AskOptions = {}): Promise<boolean> {
	asking.now?.resolve(false);
	return new Promise((resolve) => {
		asking.now = { message, yes: opts.yes, danger: opts.danger ?? false, resolve };
	});
}

export function answer(yes: boolean) {
	const a = asking.now;
	asking.now = null;
	a?.resolve(yes);
}
