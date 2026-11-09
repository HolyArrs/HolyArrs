// Search's recent searches: kept per person on this device.
import { addRecent, readRecent, recentKey } from './search-address.ts';
import { focusField, searchField } from '#lib/core/search-field.svelte.ts';
import { session } from '#lib/core/session.svelte.ts';

export function createSearchRecent(ctx: { run: () => unknown }) {
	let recent = $state<string[]>([]);
	const key = () => recentKey(session.s.name);
	function loadRecent() {
		try {
			recent = readRecent(localStorage.getItem(key()));
		} catch {
			recent = [];
		}
	}
	function saveRecent() {
		try {
			localStorage.setItem(key(), JSON.stringify(recent));
		} catch {
			/* A private window: recents just are not kept. */
		}
	}
	function remember(q: string) {
		recent = addRecent(recent, q);
		saveRecent();
	}
	function clearRecent() {
		recent = [];
		saveRecent();
		focusField();
	}
	function pickRecent(q: string) {
		searchField.term = q;
		void ctx.run();
	}

	return {
		key,
		remember,
		loadRecent,
		get recent() {
			return recent;
		},
		set recent(v) {
			recent = v;
		},
		clearRecent,
		pickRecent,
	};
}
