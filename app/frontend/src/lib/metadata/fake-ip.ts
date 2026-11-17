// The host the server names when a lookup went to a fake address
// (198.18.x.x from Clash or Surge); "" for anything that is not a host name.
export function fakeIpHost(r: { fakeIP?: unknown } | null | undefined): string {
	const h = r?.fakeIP;
	return typeof h === 'string' && h.length <= 253 && /^[A-Za-z0-9.:-]+$/.test(h) ? h : '';
}
