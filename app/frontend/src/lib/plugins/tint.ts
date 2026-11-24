// The main colour of a plugin's icon, for the soft colour wash behind its card and dialog header.
// The icon is drawn once onto an 8 x 8 canvas and its visible pixels averaged; the answer is kept per icon.
const seen = new Map<string, Promise<string>>();

export function iconTint(src: string | undefined): Promise<string> {
	if (!src || typeof document === 'undefined') return Promise.resolve('');
	let hit = seen.get(src);
	if (!hit) {
		hit = new Promise<string>((done) => {
			const img = new Image();
			img.onload = () => {
				try {
					const c = document.createElement('canvas');
					c.width = c.height = 8;
					const g = c.getContext('2d', { willReadFrequently: true });
					if (!g) return done('');
					g.drawImage(img, 0, 0, 8, 8);
					const px = g.getImageData(0, 0, 8, 8).data;
					let r = 0, gr = 0, b = 0, n = 0;
					for (let i = 0; i < px.length; i += 4) {
						if (px[i + 3] < 128) continue;
						r += px[i]; gr += px[i + 1]; b += px[i + 2]; n++;
					}
					done(n ? `rgb(${Math.round(r / n)} ${Math.round(gr / n)} ${Math.round(b / n)})` : '');
				} catch {
					done('');
				}
			};
			img.onerror = () => done('');
			img.src = src;
		});
		seen.set(src, hit);
	}
	return hit;
}

/** Svelte action: sets --tint on the element from the icon's colour. */
export function tint(node: HTMLElement, src: string | undefined) {
	let cur = src;
	const apply = (s: string | undefined) => {
		cur = s;
		iconTint(s).then((c) => {
			if (cur !== s) return;
			if (c) node.style.setProperty('--tint', c);
			else node.style.removeProperty('--tint');
		});
	};
	apply(src);
	return { update: apply };
}
