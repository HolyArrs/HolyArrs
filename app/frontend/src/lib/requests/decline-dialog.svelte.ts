// The decline dialog: the reason chips, the note and the confirm.
import { tick } from 'svelte';
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import type { Ask } from '#lib/core/types.ts';

export function createDeclineDialog(ctx: any) {
	let declineOpen = $state(false);
	let declineList = $state<Ask[]>([]);
	let declineWhy = $state('');
	let whyBox = $state<HTMLTextAreaElement>();
	// One-tap reasons for the people who asked; the box stays free to type.
	const reasonKeys = ['alreadyHave', 'notForKids', 'askedBefore', 'noSpace'];
	function askDecline(list: Ask[]) {
		declineList = list;
		declineWhy = '';
		declineOpen = true;
		// The reason box takes focus, so the sheet opens ready to type (no ring on its close button).
		void tick().then(() => setTimeout(() => whyBox?.focus({ preventScroll: true }), 60));
	}
	// A reason chip adds its words to what is typed; tapping it again takes them out.
	const hasWhy = (why: string) => declineWhy.includes(why);
	function tapWhy(why: string) {
		const text = declineWhy.trim();
		if (hasWhy(why)) {
			declineWhy = text
				.split(/\.\s*/)
				.map((s) => s.trim())
				.filter((s) => s && s !== why)
				.join('. ');
			if (declineWhy === text) declineWhy = text.replace(why, '').replace(/\s{2,}/g, ' ').trim();
		} else declineWhy = text ? `${text.replace(/\.$/, '')}. ${why}` : why;
		whyBox?.focus({ preventScroll: true });
	}
	function confirmDecline(e: SubmitEvent) {
		e.preventDefault();
		// The button that opened the sheet leaves with the ask: focus goes to the window, or the next card.
		const ids = new Set(declineList.map((a) => a.id));
		const cards = [...document.querySelectorAll<HTMLElement>('.bc[data-ask]')];
		const at = cards.findIndex((c) => ids.has(Number(c.dataset.ask)));
		const nextCard = cards.slice(at + 1).find((c) => !ids.has(Number(c.dataset.ask))) ?? cards.slice(0, Math.max(at, 0)).find((c) => !ids.has(Number(c.dataset.ask)));
		ctx.act(declineList, 'turn_down', declineWhy.trim());
		declineOpen = false;
		setTimeout(() => {
			const a = document.activeElement;
			if (a && a !== document.body && a.isConnected && !a.closest('[inert]')) return;
			const win = document.querySelector<HTMLElement>('dialog.win[open]');
			(win ?? nextCard?.querySelector<HTMLElement>('.open') ?? null)?.focus({ preventScroll: true });
		}, 80);
	}

	return {
		askDecline,
		get declineOpen() { return declineOpen; },
		set declineOpen(v) { declineOpen = v; },
		get declineList() { return declineList; },
		set declineList(v) { declineList = v; },
		confirmDecline,
		get whyBox() { return whyBox; },
		set whyBox(v) { whyBox = v; },
		get declineWhy() { return declineWhy; },
		set declineWhy(v) { declineWhy = v; },
		get reasons() { return reasonKeys.map((k) => t(`requests.decline.reason.${k}`)); },
		hasWhy,
		tapWhy,
	};
}
