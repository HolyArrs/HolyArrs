// Small helpers every fake server uses.

/** @param {import('node:http').IncomingMessage} req @returns {Promise<string>} */
export function body(req) {
	return new Promise((resolve) => {
		let b = '';
		req.on('data', (/** @type {Buffer} */ c) => (b += c));
		req.on('end', () => resolve(b));
	});
}

/** @param {import('node:http').ServerResponse} res @param {number} code @param {unknown} obj */
export function send(res, code, obj, type = 'application/json') {
	res.writeHead(code, { 'Content-Type': type });
	res.end(typeof obj === 'string' ? obj : JSON.stringify(obj));
}

export const squash = (/** @type {string} */ s) => s.toLowerCase().replace(/[^a-z0-9]+/g, '');
