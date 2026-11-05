// Small checks for pictures shown over other pictures (Home's tiles).

/**
 * lightOnDark marks a title logo that is dark (black letters made for light
 * posters): it gets the class "dark", which draws it white, so it reads on a
 * dark picture. The same test as the title page hero. A CORS copy is read, so
 * logos from other sites can be checked; when that fails the logo stays as it is.
 */
export function lightOnDark(img: HTMLImageElement) {
	const probe = new Image();
	probe.crossOrigin = 'anonymous';
	probe.onload = () => img.classList.toggle('dark', isDark(probe));
	probe.src = img.currentSrc || img.src;
	return { destroy: () => (probe.onload = null) };
}

function isDark(img: HTMLImageElement): boolean {
	try {
		const c = document.createElement('canvas');
		c.width = 48;
		c.height = 24;
		const g = c.getContext('2d', { willReadFrequently: true });
		if (!g) return false;
		g.drawImage(img, 0, 0, 48, 24);
		const d = g.getImageData(0, 0, 48, 24).data;
		let n = 0;
		let l = 0;
		for (let k = 0; k < d.length; k += 4) {
			if (d[k + 3] < 96) continue;
			n++;
			l += (0.2126 * d[k] + 0.7152 * d[k + 1] + 0.0722 * d[k + 2]) / 255;
		}
		return n > 20 && l / n < 0.3;
	} catch {
		return false;
	}
}

/**
 * artGone calls back when a picture cannot be shown: on its error, or at once
 * when it already failed before the page was ready to listen.
 */
export function artGone(img: HTMLImageElement, gone: () => void) {
	let cb = gone;
	const fail = () => cb();
	if (img.complete && img.naturalWidth === 0 && img.src) queueMicrotask(fail);
	img.addEventListener('error', fail);
	return {
		update: (next: () => void) => (cb = next),
		destroy: () => img.removeEventListener('error', fail)
	};
}
