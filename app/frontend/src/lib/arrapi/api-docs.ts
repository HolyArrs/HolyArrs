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

// Only these verbs are shown; other keys under a path are not operations.
const order = ['get', 'post', 'put', 'delete'];

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

export function filterOps(ops: ApiOp[], q: string): ApiOp[] {
	const w = q.trim().toLowerCase();
	if (!w) return ops;
	return ops.filter((o) => `${o.method} ${o.path} ${o.summary} ${o.tag}`.toLowerCase().includes(w));
}

export function groups(ops: ApiOp[]): { tag: string; ops: ApiOp[] }[] {
	const out: { tag: string; ops: ApiOp[] }[] = [];
	for (const o of ops) {
		const last = out[out.length - 1];
		if (last && last.tag === o.tag) last.ops.push(o);
		else out.push({ tag: o.tag, ops: [o] });
	}
	return out;
}

// The key goes in a header so it never ends up in the address.
export function curl(base: string, method: string, path: string, body?: string): string {
	const parts = ['curl'];
	if (method !== 'GET') parts.push(`-X ${method}`);
	parts.push(`-H "X-Api-Key: $HOLYARRS_KEY"`);
	if (body) parts.push(`-H "Content-Type: application/json" -d '${body}'`);
	parts.push(`"${base}${path}"`);
	return parts.join(' ');
}
