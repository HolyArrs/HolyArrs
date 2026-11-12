import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { checkLine, checkLines, covers, gatewayLike } from './local-lines.ts';

const card = readFileSync(new URL('./LocalSkipCard.svelte', import.meta.url), 'utf8');

describe('local network card', () => {
	it('saves the networks, person and changed proxies together', () => {
		expect(card).toContain('{ local_networks: lines(nets), local_person: person }');
		expect(card).toContain('body.trusted_proxies = lines(proxies)');
	});

	it('sends the typed networks and person when the skip is turned on', () => {
		expect(card).toContain('{ skip_login_local_network: true, local_networks: lines(nets), local_person: person }');
	});

	it('never resets what the owner types: the copy effect reads only the settings', () => {
		const copy = card.slice(card.indexOf('$effect.pre('), card.indexOf('});', card.indexOf('$effect.pre(')));
		expect(copy).toContain('savedText = savedOf(settings)');
		expect(copy).not.toMatch(/=\s*nets\s*\+/);
	});

	it('puts the fix bar on the reading-start side, so Arabic gets it on the right', () => {
		expect(card).not.toMatch(/border-left|padding-left/);
	});

	it('keeps addresses left-to-right in Arabic, so 192.168.l.0/24 is not shown out of order', () => {
		expect(card).toContain('<span class="mono xs" dir="ltr">{p.text}</span>');
		expect(card.match(/<textarea[^>]*dir="ltr"/g)?.length).toBe(2);
	});

	it('keeps Save off while a line needs a fix', () => {
		expect(card).toContain('disabled={saving || problems > 0 || !dirty}');
	});
});

describe('checkLine', () => {
	it('takes good networks and addresses', () => {
		for (const s of ['192.168.1.0/24', '10.0.0.0/8', '192.168.1.5', '172.16.0.0/12', 'fd00::/8', '100.64.0.0/10']) expect(checkLine(s, 'net')).toBeNull();
	});
	it('names each typo with a fix', () => {
		expect(checkLine('192.168.1.300', 'net')).toEqual({ key: 'fixPart' });
		expect(checkLine('10.0.0/8', 'net')).toEqual({ key: 'fixMissing', fix: '10.0.0.0/8' });
		expect(checkLine('192.168.l.0/24', 'net')).toEqual({ key: 'fixLetter', ch: 'l', fix: '192.168.1.0/24' });
		expect(checkLine('1.2.3.4.5', 'net')).toEqual({ key: 'fixExtra' });
		expect(checkLine('10.0.0.0/40', 'net')).toEqual({ key: 'fixBits' });
		expect(checkLine('0.0.0.0/0', 'net')).toEqual({ key: 'fixWide' });
		expect(checkLine('home', 'net')).toEqual({ key: 'fixNot' });
	});
	it("refuses Docker's bridge for home networks but not for proxies", () => {
		expect(checkLine('172.17.0.0/16', 'net')).toEqual({ key: 'fixBridge' });
		expect(checkLine('172.17.0.1', 'net')).toEqual({ key: 'fixBridge' });
		expect(checkLine('172.20.0.1', 'net', ['172.20.0.1'])).toEqual({ key: 'fixBridge' });
		expect(checkLine('172.17.0.2', 'proxy')).toBeNull();
	});
	it('numbers problems by line and skips blanks', () => {
		expect(checkLines('192.168.1.0/24\n\n192.168.1.300', 'net').map((p) => p.line)).toEqual([3]);
	});
});

describe('covers', () => {
	it('says whether this browser is inside the list', () => {
		expect(covers('192.168.1.40', ['192.168.1.0/24'])).toBe(true);
		expect(covers('10.1.1.1', ['192.168.1.0/24'])).toBe(false);
		expect(covers('fd00::1', ['fd00::/8'])).toBeNull();
	});
	it('spots gateway addresses', () => {
		expect(gatewayLike('172.17.0.1')).toBe(true);
		expect(gatewayLike('192.168.1.1')).toBe(false);
	});
});
