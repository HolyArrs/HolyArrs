import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import vm from 'node:vm';
import { describe, expect, it } from 'vitest';

// The first-frame script in app.html, run against a stand-in page: where does it ask for the first sign-in picture?
const html = readFileSync(join(dirname(fileURLToPath(import.meta.url)), '../../app.html'), 'utf8');
const script = html
	.split('<script>')
	.map((part) => part.split('</script>')[0])
	.find((part) => part.includes('holyarrs-signin-inline')) as string;

const seed = JSON.stringify({
	shots: [{ backdrop: { url: '/api/v1/public/signin-pic/t/p/w1280/a.jpg', w: 3840, h: 2160 }, logo: '' }],
	seed: { backdrop: { t24: 'data:image/jpeg;base64,AA==', t64: 'data:image/jpeg;base64,AA==' } }
});

function run(pathname: string) {
	const links: { href?: string }[] = [];
	const sandbox: Record<string, unknown> = {
		location: { pathname, search: '' },
		localStorage: { getItem: () => null, setItem() {}, removeItem() {} },
		navigator: { language: 'en' },
		matchMedia: () => ({ matches: false }),
		innerWidth: 1280,
		innerHeight: 800,
		devicePixelRatio: 1,
		URLSearchParams,
		JSON,
		Math,
		document: {
			documentElement: { dataset: {}, classList: { add() {} } },
			getElementById: () => ({ textContent: seed }),
			createElement: () => {
				const el: { href?: string } = {};
				links.push(el);
				return el;
			},
			head: { appendChild() {} }
		}
	};
	sandbox.window = sandbox;
	vm.runInNewContext(script, sandbox);
	return { links, inline: sandbox.__holyarrsSigninInline as { thumb: { url: string } } | undefined };
}

describe('the sign-in first frame', () => {
	it('asks for its picture under the URL base', () => {
		const { links, inline } = run('/holyarrs/login');
		expect(links[0]?.href).toBe('/holyarrs/api/v1/public/signin-pic/t/p/original/a.jpg');
		expect(inline?.thumb.url).toBe('/holyarrs/api/v1/public/signin-pic/t/p/original/a.jpg');
	});
	it('keeps the plain address without a base', () => {
		expect(run('/login').links[0]?.href).toBe('/api/v1/public/signin-pic/t/p/original/a.jpg');
		expect(run('/login/').links[0]?.href).toBe('/api/v1/public/signin-pic/t/p/original/a.jpg');
	});
});
