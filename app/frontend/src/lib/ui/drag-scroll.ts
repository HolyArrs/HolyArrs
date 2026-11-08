
// Mouse drag-to-scroll with a short glide on release, for horizontal rows.

// Pixels of movement before a press counts as a drag, so plain clicks still work.
const THRESHOLD = 5;
// Share of speed kept each frame during the glide.
const FRICTION = 0.95;

// Attached as an action; touch keeps native scrolling, only the mouse is handled here.
export function dragScroll(el: HTMLElement) {
	let down = false;
	let moved = false;
	let startX = 0;
	let startLeft = 0;
	let lastX = 0;
	let lastT = 0;
	let velocity = 0;
	let frame = 0;
	let snap = '';
	const reduce = () => matchMedia('(prefers-reduced-motion: reduce)').matches;

	function stopGlide() {
		cancelAnimationFrame(frame);
		frame = 0;
	}
	function restoreSnap() {
		el.style.scrollSnapType = snap;
		el.style.scrollBehavior = '';
	}
	// Snap is off while gliding, otherwise it would fight the motion.
	function glide() {
		if (Math.abs(velocity) < 0.3) {
			restoreSnap();
			return;
		}
		el.scrollLeft -= velocity;
		velocity *= FRICTION;
		frame = requestAnimationFrame(glide);
	}

	function onDown(e: PointerEvent) {
		if (e.pointerType !== 'mouse' || e.button !== 0) return;
		if (el.scrollWidth <= el.clientWidth) return;
		stopGlide();
		down = true;
		moved = false;
		startX = lastX = e.clientX;
		lastT = performance.now();
		startLeft = el.scrollLeft;
		velocity = 0;
	}
	function onMove(e: PointerEvent) {
		if (!down) return;
		const dx = e.clientX - startX;
		if (!moved && Math.abs(dx) < THRESHOLD) return;
		if (!moved) {
			moved = true;
			snap = el.style.scrollSnapType;
			el.style.scrollSnapType = 'none';
			el.style.scrollBehavior = 'auto';
			el.setPointerCapture(e.pointerId);
			el.classList.add('dragging');
		}
		const now = performance.now();
		const dt = Math.max(1, now - lastT);
		// Scaled to px per 16ms frame so the glide matches the frame rate.
		velocity = ((e.clientX - lastX) / dt) * 16;
		lastX = e.clientX;
		lastT = now;
		el.scrollLeft = startLeft - dx;
		e.preventDefault();
	}
	function onUp(e: PointerEvent) {
		if (!down) return;
		down = false;
		el.classList.remove('dragging');
		if (el.hasPointerCapture(e.pointerId)) el.releasePointerCapture(e.pointerId);
		if (!moved) return;
		if (reduce()) restoreSnap();
		else frame = requestAnimationFrame(glide);
	}
	function onClick(e: MouseEvent) {
		// Swallow the click that ends a drag so cards do not open.
		if (moved) {
			e.preventDefault();
			e.stopPropagation();
			moved = false;
		}
	}
	function onDragStart(e: DragEvent) {
		e.preventDefault();
	}

	el.addEventListener('pointerdown', onDown);
	el.addEventListener('pointermove', onMove);
	el.addEventListener('pointerup', onUp);
	el.addEventListener('pointercancel', onUp);
	el.addEventListener('click', onClick, true);
	el.addEventListener('dragstart', onDragStart);
	return {
		destroy() {
			stopGlide();
			el.removeEventListener('pointerdown', onDown);
			el.removeEventListener('pointermove', onMove);
			el.removeEventListener('pointerup', onUp);
			el.removeEventListener('pointercancel', onUp);
			el.removeEventListener('click', onClick, true);
			el.removeEventListener('dragstart', onDragStart);
		}
	};
}
