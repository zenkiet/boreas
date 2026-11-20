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

let displayTransition: boolean | undefined;

const transitionsDisplay = (): boolean => {
	if (displayTransition !== undefined) return displayTransition;
	const probe = document.createElement('div');
	probe.style.cssText = 'transition: display 1s allow-discrete !important; display: block;';
	document.body.append(probe);
	getComputedStyle(probe).getPropertyValue('display');
	probe.style.display = 'none';
	displayTransition = getComputedStyle(probe).display === 'block';
	probe.remove();
	return displayTransition;
};

export const closeTopLayer = (dialog: HTMLDialogElement) => {
	if (CSS.supports('overlay', 'auto') && transitionsDisplay()) return dialog.close();
	dialog.setAttribute('data-closing', '');
	const settled = Promise.allSettled(
		dialog.getAnimations({ subtree: true }).map(({ finished }) => finished)
	);
	Promise.race([settled, new Promise((done) => setTimeout(done, 2000))]).then(() => {
		if (!dialog.hasAttribute('data-closing')) return;
		dialog.removeAttribute('data-closing');
		dialog.close();
	});
};

const typed = () => typeof ViewTransition !== 'undefined' && 'types' in ViewTransition.prototype;

export const viewTransition = (
	{ complete }: { complete: Promise<void> },
	type?: 'forward' | 'back'
) => {
	if (!document.startViewTransition || still()) return;
	crossfading = true;
	return new Promise<void>((resolve) => {
		const update = async () => {
			resolve();
			await complete;
		};
		document
			.startViewTransition(type && typed() ? { update, types: [type] } : update)
			.finished.finally(() => (crossfading = false));
	});
};
