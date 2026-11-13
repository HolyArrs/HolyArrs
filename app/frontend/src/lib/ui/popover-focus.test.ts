import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';

// Safari does not focus a button when it is clicked, so focus falls to the nearest focusable parent.
// The panel must be that parent, or the leave-on-focus-out check sees focus going outside and closes the panel before the click lands.
describe('Popover panel', () => {
	const src = readFileSync(new URL('./Popover.svelte', import.meta.url), 'utf8');
	const panel = src.slice(src.indexOf('bind:this={panel}'), src.indexOf('{@render children'));

	it('can take focus itself, so a click on a button inside keeps focus inside', () => {
		expect(panel).toMatch(/tabindex="-1"/);
	});
});
