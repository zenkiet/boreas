import type { Attachment } from 'svelte/attachments';
import { on } from 'svelte/events';

const STEP: Record<string, number | undefined> = { ArrowRight: 1, ArrowLeft: -1 };

export const roving: Attachment<HTMLElement> = (bar) => {
	if ('focusGroup' in HTMLElement.prototype) return;
	const items = () => [...bar.querySelectorAll('button')];
	let active: HTMLButtonElement | undefined;
	const settle = () => {
		const list = items();
		if (!active || !list.includes(active)) active = list[0];
		for (const item of list) item.tabIndex = item === active ? 0 : -1;
	};
	settle();
	const watcher = new MutationObserver(settle);
	watcher.observe(bar, { childList: true, subtree: true });
	const offs = [
		on(bar, 'focusin', ({ target }) => {
			if (!(target instanceof HTMLButtonElement)) return;
			active = target;
			settle();
		}),
		on(bar, 'keydown', (event) => {
			const list = items();
			const at = list.indexOf(event.target as HTMLButtonElement);
			const step = STEP[event.key];
			const flip = getComputedStyle(bar).direction === 'rtl' ? -1 : 1;
			const next =
				event.key === 'Home'
					? 0
					: event.key === 'End'
						? list.length - 1
						: step
							? (at + step * flip + list.length) % list.length
							: -1;
			if (at < 0 || next < 0) return;
			event.preventDefault();
			list[next].focus();
		})
	];
	return () => {
		watcher.disconnect();
		offs.forEach((off) => off());
	};
};
