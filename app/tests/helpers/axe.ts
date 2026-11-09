import AxeBuilder from '@axe-core/playwright';
import { expect, type Page } from '@playwright/test';

export interface AxeOptions {
	/** Only these axe rules. */
	only?: string[];
	/** Only this part of the page (a CSS selector). */
	include?: string;
	/** Wait for every animation, endless ones too (by default only the ones that finish). */
	waitAll?: boolean;
}

/**
 * axe scans a settled page against WCAG 2.2 AA and fails on serious and
 * critical problems, logging each with up to three of its elements. Endless
 * animations (a pulsing state dot) never finish, so by default it waits only
 * for the ones that do.
 */
export async function axe(p: Page, where: string, o: AxeOptions = {}) {
	await p.evaluate((all) => Promise.all(document.getAnimations().filter((a) => all || a.effect?.getComputedTiming().iterations !== Infinity).map((a) => a.finished.catch(() => undefined))), o.waitAll ?? false);
	let b = new AxeBuilder({ page: p }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']);
	if (o.only) b = b.withRules(o.only);
	if (o.include) b = b.include(o.include);
	const r = await b.analyze();
	const bad = r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
	if (bad.length) console.log('AXE', where, bad.map((v) => v.id + ' ' + v.nodes.slice(0, 3).map((n) => n.target.join(' ') + ' [' + (n.any?.[0]?.message ?? '') + ']').join(' | ')).join(' ;; '));
	expect(bad.map((v) => v.id), where).toEqual([]);
}
