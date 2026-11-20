import type { Attachment } from 'svelte/attachments';

export const scrollHints: Attachment<HTMLElement> = (scroller) => {
	if (CSS.supports('animation-timeline', 'scroll()')) return;
	const edges = new IntersectionObserver(
		(entries) => {
			for (const { target, intersectionRatio } of entries) {
				const side = target === scroller.firstElementChild ? 'data-more-start' : 'data-more-end';
				scroller.toggleAttribute(side, intersectionRatio < 1);
			}
		},
		{ root: scroller, threshold: 1, rootMargin: '1px' }
	);
	const watch = () => {
		edges.disconnect();
		for (const edge of [scroller.firstElementChild, scroller.lastElementChild])
			if (edge) edges.observe(edge);
	};
	watch();
	const children = new MutationObserver(watch);
	children.observe(scroller, { childList: true });
	return () => {
		edges.disconnect();
		children.disconnect();
	};
};
