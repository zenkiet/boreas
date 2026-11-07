import { flip } from 'svelte/animate';
import type { Attachment } from 'svelte/attachments';
import { cubicOut } from 'svelte/easing';
import { prefersReducedMotion } from 'svelte/motion';
import { fly } from 'svelte/transition';

export const T = { quick: 120, base: 200, layout: 400 } as const;
const EASE_OUT = 'cubic-bezier(0.32, 0.72, 0, 1)';

let crossfading = false;
const still = () => crossfading || prefersReducedMotion.current;

export const move = (node: Element, rects: { from: DOMRect; to: DOMRect }) =>
	flip(node, rects, { duration: still() ? 0 : T.layout, easing: cubicOut });

export const rise = (node: Element) =>
	fly(node, { y: 4, duration: still() ? 0 : T.base, easing: cubicOut });

const seen = new WeakMap<Element, string>();

export const flash =
	(value: unknown): Attachment<HTMLElement> =>
	(node) => {
		const next = JSON.stringify(value);
		if (seen.has(node) && seen.get(node) !== next) {
			node.animate([{ backgroundColor: 'var(--app-accent-soft)' }], {
				duration: 1000,
				easing: EASE_OUT
			});
		}
		seen.set(node, next);
	};

export const lift = (node: Element) =>
	node.animate(
		still() ? { opacity: [0, 1] } : { opacity: [0, 1], transform: ['translateY(0.25rem)', 'none'] },
		{ duration: T.quick, easing: EASE_OUT }
	);

export const viewTransition = ({ complete }: { complete: Promise<void> }) => {
	if (!document.startViewTransition || still()) return;
	crossfading = true;
	return new Promise<void>((resolve) => {
		document
			.startViewTransition(async () => {
				resolve();
				await complete;
			})
			.finished.finally(() => (crossfading = false));
	});
};
