import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

// iOS zooms in on any focused text field under 16px; the phone block must lift every field to 16px.
describe('phone text fields', () => {
	it('keeps every text field at 16px or more on phones', () => {
		const css = readFileSync(new URL('../../app.css', import.meta.url), 'utf8');
		const phone = css.split('@media (max-width: 767px) {').slice(1).join('\n');
		expect(phone).toMatch(/input:not\(\[type='checkbox'\]\):not\(\[type='radio'\]\):not\(\[type='range'\]\),\s*textarea,\s*select\s*\{\s*font-size:\s*max\(16px, 1em\) !important;/);
	});
});
