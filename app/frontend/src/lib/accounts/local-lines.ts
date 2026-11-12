// Checks the home networks and trusted proxies typed on the Sign-in page, line by line,
// so each typo gets a plain fix before Save. The server checks again on save.

export type LineFix =
	| { key: 'fixPart' }
	| { key: 'fixMissing'; fix: string }
	| { key: 'fixLetter'; ch: string; fix: string }
	| { key: 'fixExtra' }
	| { key: 'fixBits' }
	| { key: 'fixWide' }
	| { key: 'fixBridge' }
	| { key: 'fixNot' };

export interface LineProblem {
	line: number;
	text: string;
	fix: LineFix;
}

const v4 = (s: string) => s.split('.');

/** IPv4 as a number, or null. */
export function ip4(s: string): number | null {
	const p = v4(s);
	if (p.length !== 4 || p.some((x) => !/^\d{1,3}$/.test(x) || Number(x) > 255)) return null;
	return p.reduce((n, x) => n * 256 + Number(x), 0);
}

function inNet(ip: number, base: number, bits: number): boolean {
	const size = 2 ** (32 - bits);
	return Math.floor(ip / size) === Math.floor(base / size);
}

/** Whether an IPv4 network (a.b.c.d/bits) sits inside Docker's default bridge or a listed gateway entry. */
function bridge(base: number, bits: number, text: string, gateways: string[]): boolean {
	if (gateways.includes(text)) return true;
	return bits >= 16 && inNet(base, ip4('172.17.0.0')!, 16);
}

/** Checks one line. kind 'net' also refuses Docker's bridge, since every visit through it would count as home. */
export function checkLine(raw: string, kind: 'net' | 'proxy', gateways: string[] = []): LineFix | null {
	const text = raw.trim();
	const [addr, bitsText, ...more] = text.split('/');
	if (more.length) return { key: 'fixNot' };
	if (addr.includes(':')) {
		const bits = bitsText === undefined ? 128 : Number(bitsText);
		if (!/^[0-9a-f:.]+$/i.test(addr) || !/^\d+$/.test(bitsText ?? '128') || bits > 128) return { key: 'fixNot' };
		return bits < 7 ? { key: 'fixWide' } : null;
	}
	const tail = bitsText === undefined ? '' : '/' + bitsText;
	// Letters that look like digits.
	const swapped = addr.replace(/[lI|]/g, '1').replace(/[oO]/g, '0');
	if (swapped !== addr && /^[\d.]+$/.test(swapped)) {
		const ch = addr.match(/[lI|oO]/)![0];
		return { key: 'fixLetter', ch, fix: swapped + tail };
	}
	if (!/^[\d.]+$/.test(addr)) return { key: 'fixNot' };
	const parts = v4(addr);
	if (parts.some((x) => x === '') && parts.length > 4) return { key: 'fixNot' };
	if (parts.length > 4) return { key: 'fixExtra' };
	if (parts.some((x) => x !== '' && Number(x) > 255)) return { key: 'fixPart' };
	if (parts.length < 4 || parts.some((x) => x === '')) {
		const full = parts.filter((x) => x !== '');
		while (full.length < 4) full.push('0');
		return { key: 'fixMissing', fix: full.join('.') + tail };
	}
	if (bitsText !== undefined && (!/^\d{1,2}$/.test(bitsText) || Number(bitsText) > 32)) return { key: 'fixBits' };
	const bits = bitsText === undefined ? 32 : Number(bitsText);
	if (bits < 8) return { key: 'fixWide' };
	if (kind === 'net' && bridge(ip4(addr)!, bits, text, gateways)) return { key: 'fixBridge' };
	return null;
}

/** Every line that needs a fix, numbered from 1 as the person sees them. */
export function checkLines(text: string, kind: 'net' | 'proxy', gateways: string[] = []): LineProblem[] {
	const out: LineProblem[] = [];
	text.split('\n').forEach((t, i) => {
		if (!t.trim()) return;
		const fix = checkLine(t, kind, gateways);
		if (fix) out.push({ line: i + 1, text: t.trim(), fix });
	});
	return out;
}

/** The lines, cleaned, without blanks. */
export const lines = (text: string) => text.split(/[\s,]+/).filter(Boolean);

/** Whether an IPv4 address is inside one of the listed lines (IPv6 is not checked here: null). */
export function covers(ip: string, list: string[]): boolean | null {
	const n = ip4(ip);
	if (n === null) return null;
	return list.some((e) => {
		const [a, b] = e.split('/');
		const base = ip4(a);
		const bits = b === undefined ? 32 : Number(b);
		return base !== null && bits >= 0 && bits <= 32 && inNet(n, base, bits);
	});
}

/** Whether this address looks like a Docker or Podman gateway (172.16-31.x.1, 10.88.0.1). */
export function gatewayLike(ip: string): boolean {
	return /^172\.(1[6-9]|2\d|3[01])\.\d{1,3}\.1$/.test(ip) || ip === '10.88.0.1';
}
