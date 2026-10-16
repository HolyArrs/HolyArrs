// The API page: the OpenAPI file turned into a list a person can read,
// and example calls.

export interface ApiOp {
	method: string;
	path: string;
	summary: string;
	description: string;
	scope: string;
	permission: string;
	tag: string;
}

interface SpecOp {
	summary?: string;
	description?: string;
	tags?: string[];
	'x-holyarrs-scope'?: string;
	'x-holyarrs-permission'?: string;
}

export interface Spec {
	info?: { title?: string; version?: string };
	paths?: Record<string, Record<string, SpecOp>>;
	'x-holyarrs-scopes'?: Record<string, string>;
}

const order = ['get', 'post', 'put', 'delete'];

/** operations lists every operation, by tag, then path, then method. */
export function operations(spec: Spec): ApiOp[] {
	const out: ApiOp[] = [];
	for (const [path, ops] of Object.entries(spec.paths ?? {})) {
		for (const [m, op] of Object.entries(ops)) {
			if (!order.includes(m)) continue;
			out.push({
				method: m.toUpperCase(),
				path,
				summary: op.summary ?? '',
				description: op.description ?? '',
				scope: op['x-holyarrs-scope'] ?? '',
				permission: op['x-holyarrs-permission'] ?? '',
				tag: op.tags?.[0] ?? 'Other'
			});
		}
	}
	return out.sort(
		(a, b) => a.tag.localeCompare(b.tag) || a.path.localeCompare(b.path) || order.indexOf(a.method.toLowerCase()) - order.indexOf(b.method.toLowerCase())
	);
}

/** filterOps keeps operations whose path, summary or tag hold the words. */
export function filterOps(ops: ApiOp[], q: string): ApiOp[] {
	const w = q.trim().toLowerCase();
	if (!w) return ops;
	return ops.filter((o) => `${o.method} ${o.path} ${o.summary} ${o.tag}`.toLowerCase().includes(w));
}

/** groups splits operations by tag, keeping their order. */
export function groups(ops: ApiOp[]): { tag: string; ops: ApiOp[] }[] {
	const out: { tag: string; ops: ApiOp[] }[] = [];
	for (const o of ops) {
		const last = out[out.length - 1];
		if (last && last.tag === o.tag) last.ops.push(o);
		else out.push({ tag: o.tag, ops: [o] });
	}
	return out;
}

/** curl is an example call with the key in its header (never the address). */
export function curl(base: string, method: string, path: string, body?: string): string {
	const parts = ['curl'];
	if (method !== 'GET') parts.push(`-X ${method}`);
	parts.push(`-H "X-Api-Key: $HOLYARRS_KEY"`);
	if (body) parts.push(`-H "Content-Type: application/json" -d '${body}'`);
	parts.push(`"${base}${path}"`);
	return parts.join(' ');
}
