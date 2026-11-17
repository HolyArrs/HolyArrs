// In-app yes/no question for Remove, Delete and Replace actions, in place of the browser's own confirm() box.
type Asked = { message: string; resolve: (yes: boolean) => void };

export const asking = $state<{ now: Asked | null }>({ now: null });

// Resolves true when the person confirms, false when they cancel or close the sheet.
export function ask(message: string): Promise<boolean> {
	asking.now?.resolve(false);
	return new Promise((resolve) => {
		asking.now = { message, resolve };
	});
}

export function answer(yes: boolean) {
	const a = asking.now;
	asking.now = null;
	a?.resolve(yes);
}
