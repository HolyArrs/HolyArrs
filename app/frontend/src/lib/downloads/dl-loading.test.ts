import { describe, expect, it, vi } from 'vitest';
import { render } from 'svelte/server';

// A server render has no router: the tabs read show= from this address.
vi.mock('$app/state', () => ({ page: { url: new URL('http://localhost/downloads') } }));
import DlSections from './DlSections.svelte';
import Page from '../../routes/downloads/+page.svelte';

const base = { rows: [], importOf: new Map(), titles: new Map(), busy: 0, decisionsOf: () => [], onfail: () => {}, onremove: () => {}, ondone: () => {} };

describe('Downloads while the first read is on its way', () => {
	it('keeps the status tabs and shows the loader in the list', () => {
		const { body } = render(DlSections, { props: { ...base, loading: true } });
		expect(body).toContain('role="tablist"');
		expect(body).toContain('data-testid="page-loader"');
		expect(body).not.toContain('data-testid="dl-empty"');
	});
	it('drops the loader once the rows are read', () => {
		const { body } = render(DlSections, { props: { ...base } });
		expect(body).toContain('role="tablist"');
		expect(body).not.toContain('data-testid="page-loader"');
	});
	it('the page draws its heading and tabs before the first answer', () => {
		const { body } = render(Page);
		expect(body).toContain('<h1');
		expect(body).toContain('role="tablist"');
		expect(body).toContain('data-testid="page-loader"');
	});
});
