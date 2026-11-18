import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const src = readFileSync(new URL('./ReportsList.svelte', import.meta.url), 'utf8');

describe('report window reporter', () => {
	it('shows the name through PersonText so deleted people get the deleted look', () => {
		const line = src.split('\n').find((l) => l.includes('class="rw-by"'));
		expect(line).toBeTruthy();
		expect(line).toContain('<PersonText text={who(win)} />');
	});

	it('shows who fixed it through PersonText', () => {
		const line = src.split('\n').find((l) => l.includes('requests.reports.fixedBy'));
		expect(line).toContain("<PersonText text={t('requests.reports.fixedBy', { by: fixedBy(win) })} />");
	});

	it('gives the avatar the bare name so the first letter shows', () => {
		expect(src).toContain('<Avatar name={bareNames(face(win))}');
	});
});
