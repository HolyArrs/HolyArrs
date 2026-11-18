import { afterEach, describe, expect, it, vi } from 'vitest';
import { readFileSync } from 'node:fs';
import { askInView, inView, watchFocusDrops } from '../core/motion.ts';

// The password-again prompt can open far from the button that needed it (on a phone, above the screen).
// When it opens it is scrolled to the middle of the view and its field takes the cursor.
function prompt(withField = true) {
	const field = { focus: vi.fn() };
	const el = {
		scrollIntoView: vi.fn(),
		querySelector: vi.fn((sel: string) => (withField && sel.includes('input') ? field : null))
	};
	return { el, field };
}
const motion = (reduce: boolean) => ({ matchMedia: () => ({ matches: reduce }) });

describe('Confirm prompt', () => {
	afterEach(() => vi.unstubAllGlobals());

	it('focuses its field and asks to be scrolled into view when it opens', () => {
		const { el, field } = prompt();
		askInView(el as unknown as HTMLElement, motion(false));
		expect(el.scrollIntoView).toHaveBeenCalledWith({ block: 'center', behavior: 'smooth' });
		expect(field.focus).toHaveBeenCalledWith({ preventScroll: true });
	});

	it('jumps instead of gliding when motion is reduced', () => {
		const { el } = prompt();
		askInView(el as unknown as HTMLElement, motion(true));
		expect(el.scrollIntoView).toHaveBeenCalledWith({ block: 'center', behavior: 'auto' });
	});

	// "Sign in again with Plex" signs out: a keyboard user pressing Space to scroll must not land on it.
	it('scrolls a box with only a button into view without putting the cursor on the button', () => {
		const button = { focus: vi.fn() };
		const el = { scrollIntoView: vi.fn(), querySelector: vi.fn((sel: string) => (/(^|,\s*)button/.test(sel) ? button : null)) };
		askInView(el as unknown as HTMLElement, motion(false));
		expect(el.scrollIntoView).toHaveBeenCalledWith({ block: 'center', behavior: 'smooth' });
		expect(button.focus).not.toHaveBeenCalled();
		const { el: withField, field } = prompt();
		askInView(withField as unknown as HTMLElement, motion(false));
		expect(field.focus).toHaveBeenCalledWith({ preventScroll: true });
	});

	it('skips the hidden user name box', () => {
		const { el } = prompt();
		askInView(el as unknown as HTMLElement, motion(false));
		expect(el.querySelector.mock.calls[0][0]).toMatch(/^input:not\(\[hidden\]/);
	});

	it('does it once the prompt is on the page, and not when told to stay still', () => {
		const frames: (() => void)[] = [];
		vi.stubGlobal('requestAnimationFrame', (f: () => void) => frames.push(f));
		vi.stubGlobal('cancelAnimationFrame', () => {});
		const a = prompt();
		inView(a.el as unknown as HTMLElement);
		expect(a.el.scrollIntoView).not.toHaveBeenCalled();
		frames.forEach((f) => f());
		expect(a.el.scrollIntoView).toHaveBeenCalledTimes(1);
		expect(a.field.focus).toHaveBeenCalledTimes(1);
		const b = prompt();
		frames.length = 0;
		inView(b.el as unknown as HTMLElement, false);
		frames.forEach((f) => f());
		expect(b.el.scrollIntoView).not.toHaveBeenCalled();
	});

	// Confirmed or cancelled, the cursor goes back to the button that opened the prompt, so a phone user is not left at the top of the page.
	it('hands the cursor back to the button that opened it when it closes', () => {
		const frames: (() => void)[] = [];
		vi.stubGlobal('requestAnimationFrame', (f: () => void) => frames.push(f));
		vi.stubGlobal('cancelAnimationFrame', () => {});
		const body = {};
		const button = { isConnected: true, focus: vi.fn() };
		const doc = { body, activeElement: button as unknown };
		vi.stubGlobal('document', doc);
		const a = prompt();
		(a.el as unknown as { contains: () => boolean }).contains = () => false;
		const act = inView(a.el as unknown as HTMLElement) as { destroy: () => void };
		frames.forEach((f) => f());
		frames.length = 0;
		doc.activeElement = body;
		act.destroy();
		frames.forEach((f) => f());
		expect(button.focus).toHaveBeenCalledTimes(1);
	});

	it('leaves the cursor alone when that button is gone or the cursor already moved on', () => {
		const frames: (() => void)[] = [];
		vi.stubGlobal('requestAnimationFrame', (f: () => void) => frames.push(f));
		vi.stubGlobal('cancelAnimationFrame', () => {});
		const body = {};
		for (const [connected, now] of [[false, body], [true, { elsewhere: true }]] as const) {
			const button = { isConnected: connected, focus: vi.fn() };
			const doc = { body, activeElement: button as unknown };
			vi.stubGlobal('document', doc);
			const a = prompt();
			(a.el as unknown as { contains: () => boolean }).contains = () => false;
			const act = inView(a.el as unknown as HTMLElement) as { destroy: () => void };
			frames.length = 0;
			doc.activeElement = now;
			act.destroy();
			frames.forEach((f) => f());
			expect(button.focus, `connected=${connected}`).not.toHaveBeenCalled();
		}
	});

	// Make key goes busy (disabled) while it asks the server, and the browser drops the cursor to the page before the prompt opens.
	it('hands the cursor back to a button that lost it by going busy', () => {
		const frames: (() => void)[] = [];
		vi.stubGlobal('requestAnimationFrame', (f: () => void) => frames.push(f));
		vi.stubGlobal('cancelAnimationFrame', () => {});
		const on: Record<string, (e: unknown) => void> = {};
		const body = {};
		const doc = { body, activeElement: body as unknown, addEventListener: (type: string, f: (e: unknown) => void) => (on[type] = f) };
		watchFocusDrops(doc as unknown as Document);
		const button = { isConnected: true, disabled: true, focus: vi.fn() };
		on.focusout({ target: button, relatedTarget: null });
		vi.stubGlobal('document', doc);
		const a = prompt();
		(a.el as unknown as { contains: () => boolean }).contains = () => false;
		const act = inView(a.el as unknown as HTMLElement) as { destroy: () => void };
		frames.length = 0;
		act.destroy();
		frames.forEach((f) => f());
		expect(button.focus).toHaveBeenCalledTimes(1);
		// A control that was only left (not made busy) is never what the cursor goes back to.
		const left = { isConnected: true, disabled: false, focus: vi.fn() };
		on.focusout({ target: left, relatedTarget: null });
		const b = prompt();
		(b.el as unknown as { contains: () => boolean }).contains = () => false;
		const act2 = inView(b.el as unknown as HTMLElement) as { destroy: () => void };
		frames.length = 0;
		act2.destroy();
		frames.forEach((f) => f());
		expect(left.focus).not.toHaveBeenCalled();
	});

	// Safari gives a tapped button no focus, so no focus event says which button opened the prompt; the tap itself does.
	it('hands the cursor back to a tapped button that never took focus (Safari)', () => {
		const frames: (() => void)[] = [];
		vi.stubGlobal('requestAnimationFrame', (f: () => void) => frames.push(f));
		vi.stubGlobal('cancelAnimationFrame', () => {});
		const on: Record<string, (e: unknown) => void> = {};
		const body = {};
		const doc = { body, activeElement: body as unknown, addEventListener: (type: string, f: (e: unknown) => void) => (on[type] = f) };
		watchFocusDrops(doc as unknown as Document);
		const button = { isConnected: true, disabled: false, focus: vi.fn() };
		on.pointerdown({ target: { closest: (sel: string) => (sel.includes('button') ? button : null) } });
		button.disabled = true;
		vi.stubGlobal('document', doc);
		const a = prompt();
		(a.el as unknown as { contains: () => boolean }).contains = () => false;
		const act = inView(a.el as unknown as HTMLElement) as { destroy: () => void };
		frames.length = 0;
		button.disabled = false;
		act.destroy();
		frames.forEach((f) => f());
		expect(button.focus).toHaveBeenCalledTimes(1);
	});

	// Safari: typing a key name, then tapping Make key. The tap gives the button no focus, and the name field then loses it to nothing.
	it('keeps the tapped button when the field typed in just before loses the cursor (Safari)', () => {
		const frames: (() => void)[] = [];
		vi.stubGlobal('requestAnimationFrame', (f: () => void) => frames.push(f));
		vi.stubGlobal('cancelAnimationFrame', () => {});
		const on: Record<string, (e: unknown) => void> = {};
		const body = {};
		const doc = { body, activeElement: body as unknown, addEventListener: (type: string, f: (e: unknown) => void) => (on[type] = f) };
		watchFocusDrops(doc as unknown as Document);
		const field = { isConnected: true, disabled: false, focus: vi.fn() };
		on.focusin({ target: field });
		const button = { isConnected: true, disabled: false, focus: vi.fn() };
		on.pointerdown({ target: { closest: () => button } });
		on.focusout({ target: field, relatedTarget: null });
		button.disabled = true;
		vi.stubGlobal('document', doc);
		const a = prompt();
		(a.el as unknown as { contains: () => boolean }).contains = () => false;
		const act = inView(a.el as unknown as HTMLElement) as { destroy: () => void };
		frames.length = 0;
		button.disabled = false;
		act.destroy();
		frames.forEach((f) => f());
		expect(button.focus).toHaveBeenCalledTimes(1);
		expect(field.focus).not.toHaveBeenCalled();
	});

	it('forgets a field that went busy long ago once something else is tapped', () => {
		const frames: (() => void)[] = [];
		vi.stubGlobal('requestAnimationFrame', (f: () => void) => frames.push(f));
		vi.stubGlobal('cancelAnimationFrame', () => {});
		const on: Record<string, (e: unknown) => void> = {};
		const body = {};
		const doc = { body, activeElement: body as unknown, addEventListener: (type: string, f: (e: unknown) => void) => (on[type] = f) };
		watchFocusDrops(doc as unknown as Document);
		// A text field disabled while its form saved: remembered.
		const field = { isConnected: true, disabled: true, focus: vi.fn() };
		on.focusout({ target: field, relatedTarget: null });
		// Later a tap on a button (Safari: no focus events), which opens the prompt.
		const button = { isConnected: true, disabled: false, focus: vi.fn() };
		on.pointerdown({ target: { closest: () => button } });
		vi.stubGlobal('document', doc);
		const a = prompt();
		(a.el as unknown as { contains: () => boolean }).contains = () => false;
		const act = inView(a.el as unknown as HTMLElement) as { destroy: () => void };
		frames.length = 0;
		act.destroy();
		frames.forEach((f) => f());
		expect(field.focus).not.toHaveBeenCalled();
		expect(button.focus).toHaveBeenCalledTimes(1);
		// A tap on nothing in particular forgets it too.
		on.focusout({ target: field, relatedTarget: null });
		on.pointerdown({ target: { closest: () => null } });
		const b = prompt();
		(b.el as unknown as { contains: () => boolean }).contains = () => false;
		const act2 = inView(b.el as unknown as HTMLElement) as { destroy: () => void };
		frames.length = 0;
		act2.destroy();
		frames.forEach((f) => f());
		expect(field.focus).not.toHaveBeenCalled();
	});

	it('tells screen readers what the password field is for', () => {
		const src = readFileSync(new URL('./Confirm.svelte', import.meta.url), 'utf8');
		const form = src.slice(src.indexOf('data-testid="confirm-sign-in"'), src.indexOf('</form>'));
		const hint = /<p class="hint" id="([^"]+)">\{t\('ui\.confirm\.confirmItSYou'\)\}<\/p>/.exec(form);
		expect(hint, 'hint has an id').not.toBeNull();
		const pw = /<input id="\{id\}-pw"[^>]*>/.exec(form)?.[0] ?? '';
		expect(pw).toContain(`aria-describedby="${hint![1]}"`);
	});

	it('is used by every prompt Confirm shows', () => {
		const src = readFileSync(new URL('./Confirm.svelte', import.meta.url), 'utf8');
		const markup = src.slice(src.indexOf('</script>'));
		for (const testid of ['confirm-local-visit', 'confirm-outside', 'confirm-sign-in']) {
			const tag = markup.slice(markup.lastIndexOf('<', markup.indexOf(`data-testid="${testid}"`)), markup.indexOf(`data-testid="${testid}"`));
			expect(tag, testid).toMatch(/use:inView=\{reveal\}/);
		}
	});
});

// Settings asks in a box above the page, often far from the button that needed it.
describe('Settings password-again box', () => {
	const src = readFileSync(new URL('../../routes/settings/+layout.svelte', import.meta.url), 'utf8');
	const markup = src.slice(src.indexOf('</script>'));

	it('comes into view in every form it takes, Set a password first included', () => {
		const start = markup.indexOf('{:else if reauth');
		const end = markup.indexOf('{/if}', markup.lastIndexOf('id="settings-reauth"'));
		const cards = markup.slice(start, end).split(/\{:else if reauth[^}]*\}/).filter((c) => c.includes('id="settings-reauth"'));
		expect(cards.length).toBe(3);
		// Confirm brings itself into view; a card without it does so on its own.
		for (const card of cards) expect(card, card.slice(0, 120)).toMatch(/<Confirm |use:inView/);
	});

	it('leaves every Jackett or Prowlarr call to the box above its Connect button, Test included', () => {
		const script = src.slice(0, src.indexOf('</script>'));
		expect(script).toContain("detail?.path?.startsWith('/bridges')");
		expect(script).not.toContain("=== '/bridges'");
	});
});
