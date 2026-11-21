import { afterEach, describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import DeletePersonAsk from './DeletePersonAsk.svelte';
import { session } from '#lib/core/session.svelte.ts';

describe('Delete a person asks in the row first', () => {
	it('names the person and offers Delete person and Keep', () => {
		const { body } = render(DeletePersonAsk, { props: { name: 'pat', busy: false, onyes: () => {}, onno: () => {} } });
		expect(body).toContain('data-testid="user-delete-ask"');
		expect(body).toContain('Delete pat? They');
		expect(body).toContain('data-testid="user-delete-yes"');
		expect(body).toContain('Delete person');
		expect(body).toContain('Keep');
		expect(body).toContain('Any Seerr keys they made stop working for everyone');
		// Announced as a small dialog, named and described by its own words.
		expect(body).toContain('role="alertdialog"');
		expect(body).toMatch(/aria-labelledby="[^"]+"/);
		expect(body).toMatch(/aria-describedby="[^"]+"/);
		expect(body).toContain('data-testid="user-delete-keep"');
	});
	it('holds the Delete button while it works', () => {
		const { body } = render(DeletePersonAsk, { props: { name: 'pat', busy: true, onyes: () => {}, onno: () => {} } });
		expect(body).toMatch(/data-testid="user-delete-yes"[^>]*disabled|disabled[^>]*data-testid="user-delete-yes"/);
	});
	describe('their YouTube library', () => {
		afterEach(() => {
			session.s.held = [];
		});
		const ask = () => render(DeletePersonAsk, { props: { name: 'pat', busy: false, onyes: () => {}, onno: () => {} } }).body;
		it('says their videos and follows go too while YouTube is on', () => {
			session.s.held = [];
			expect(ask()).toContain('Their YouTube videos and follows are deleted too.');
		});
		it('says nothing of YouTube while it is held back', () => {
			session.s.held = ['youtube'];
			expect(ask()).not.toContain('YouTube');
		});
	});
});
