<script lang="ts">
	import type { Attachment } from 'svelte/attachments';
	import { on } from 'svelte/events';
	import { MediaQuery } from 'svelte/reactivity';
	import type { DirectoryTask } from '#shared/api';
	import { closeTopLayer } from '#shared/ui';
	import PanelBar from './PanelBar.svelte';
	import PanelDetails from './PanelDetails.svelte';
	import PanelHero from './PanelHero.svelte';

	let { task, onclose }: { task: DirectoryTask | undefined; onclose: () => void } = $props();

	const phone = new MediaQuery('width < 45rem');
	const heading = $props.id();

	let last: DirectoryTask | undefined;
	const shown = $derived.by(() => {
		if (task) last = task;
		return last;
	});

	const parts = (dialog: HTMLDialogElement) => {
		const scroller = dialog.firstElementChild as HTMLElement;
		return { scroller, sheet: scroller.lastElementChild as HTMLElement };
	};

	const sync: Attachment<HTMLDialogElement> = (dialog) => {
		const { scroller } = parts(dialog);
		if (!task) {
			if (!dialog.open) return;
			if (phone.current) scroller.scrollTo({ top: 0 });
			else closeTopLayer(dialog);
		} else if (dialog.open) {
			dialog.removeAttribute('data-closing');
			if (phone.current) scroller.scrollTo({ top: scroller.scrollHeight });
		} else {
			dialog.showModal();
			if (!phone.current) return;
			scroller.scrollTo({ top: 0, behavior: 'instant' });
			requestAnimationFrame(() =>
				requestAnimationFrame(() => scroller.scrollTo({ top: scroller.scrollHeight }))
			);
		}
	};

	const outside = (dialog: HTMLDialogElement, { clientX: x, clientY: y }: PointerEvent) => {
		const box = dialog.getBoundingClientRect();
		return x < box.left || x > box.right || y < box.top || y > box.bottom;
	};

	const wire: Attachment<HTMLDialogElement> = (dialog) => {
		const { scroller, sheet } = parts(dialog);
		const seen = 1 / innerHeight;
		let opened = false;
		let pressedOutside = false;
		const watcher = new IntersectionObserver(
			(entries) => {
				if (entries.at(-1)!.intersectionRatio >= seen) opened = true;
				else if (opened && dialog.open) dialog.close();
			},
			{ root: dialog, threshold: [seen, 1] }
		);
		watcher.observe(sheet);
		const offs = [
			on(dialog, 'close', () => {
				opened = false;
				if (task) onclose();
			}),
			on(dialog, 'cancel', (event) => {
				if (!event.cancelable) return;
				event.preventDefault();
				onclose();
			}),
			on(dialog, 'click', (event) => {
				if (phone.current && !sheet.contains(event.target as Node)) onclose();
			})
		];
		if (!('closedBy' in HTMLDialogElement.prototype))
			offs.push(
				on(dialog, 'pointerdown', (event) => (pressedOutside = outside(dialog, event))),
				on(dialog, 'pointerup', (event) => {
					if (pressedOutside && outside(dialog, event)) onclose();
				})
			);
		if (!CSS.supports('animation-timeline: scroll()'))
			offs.push(
				on(scroller, 'scroll', () =>
					dialog.style.setProperty(
						'--drawer-backdrop',
						String(scroller.scrollTop / sheet.offsetHeight)
					)
				)
			);
		return () => {
			watcher.disconnect();
			offs.forEach((off) => off());
		};
	};
</script>

<dialog
	{@attach sync}
	{@attach wire}
	closedby={phone.current ? 'closerequest' : 'any'}
	aria-labelledby={heading}
	class="fixed inset-y-3 start-auto end-3 h-auto max-h-none w-110 max-w-[calc(100%-24px)] flex-col overflow-hidden rounded-box bg-surface text-label shadow-2 open:flex max-sm:inset-0 max-sm:size-full max-sm:max-w-none max-sm:overflow-visible max-sm:rounded-none max-sm:bg-transparent max-sm:shadow-none"
>
	<div
		class="scroller max-sm:grid max-sm:size-full max-sm:snap-y max-sm:snap-mandatory max-sm:[scrollbar-width:none] max-sm:grid-rows-[100%_max-content] max-sm:overflow-y-scroll max-sm:overscroll-none sm:contents"
	>
		<span class="max-sm:snap-start sm:hidden"></span>
		<div
			class="flex min-h-0 min-w-0 flex-col max-sm:max-h-[90svh] max-sm:snap-end max-sm:rounded-t-box max-sm:bg-surface max-sm:shadow-2 sm:flex-auto"
		>
			{#if shown}
				<PanelBar task={shown} {onclose} />
				<div class="flex-auto overflow-y-auto px-5 pt-3 pb-6">
					<PanelHero task={shown} {heading} />
					<PanelDetails task={shown} />
				</div>
			{/if}
		</div>
	</div>
</dialog>

<style>
	dialog {
		@media (width >= 45rem) {
			--from: calc(100% + 12px) 0;
			--fade: 1;

			translate: var(--from);
			opacity: var(--fade);
			transition-property: translate, opacity, display, overlay;
			transition-duration: 400ms;
			transition-timing-function: var(--ease-layout);
			transition-behavior: allow-discrete;

			&:dir(rtl) {
				--from: calc(-100% - 12px) 0;
			}

			@media (prefers-reduced-motion: reduce) {
				--from: 0;
				--fade: 0;
				transition-duration: 150ms;
			}

			&::backdrop {
				background-color: transparent;
				transition-property: background-color, display, overlay;
				transition-duration: 200ms;
				transition-timing-function: var(--ease-out);
				transition-behavior: allow-discrete;
			}
		}

		@media (width < 45rem) {
			&::backdrop {
				background-color: var(--app-scrim);
				opacity: var(--drawer-backdrop, 0);
			}

			@supports (animation-timeline: scroll()) {
				timeline-scope: --sheet;
				animation: backdrop linear both;
				animation-timeline: --sheet;

				.scroller {
					scroll-timeline: --sheet y;
				}
			}

			@media (prefers-reduced-motion: no-preference) {
				.scroller {
					scroll-behavior: smooth;
				}
			}
		}
	}

	dialog:modal {
		overscroll-behavior: contain;

		&::backdrop {
			overflow: hidden;
			overscroll-behavior: contain;
		}
	}

	dialog[open]:where(:not([data-closing])) {
		@media (width >= 45rem) {
			translate: none;
			opacity: 1;

			&::backdrop {
				background-color: var(--app-scrim);
			}

			@starting-style {
				translate: var(--from);
				opacity: var(--fade);

				&::backdrop {
					background-color: transparent;
				}
			}
		}
	}

	@keyframes backdrop {
		from {
			--drawer-backdrop: 0;
		}

		to {
			--drawer-backdrop: 1;
		}
	}
</style>
