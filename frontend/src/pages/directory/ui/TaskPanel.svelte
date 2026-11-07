<script lang="ts">
	import type { Attachment } from 'svelte/attachments';
	import { on } from 'svelte/events';
	import type { DirectoryTask } from '#shared/api';
	import PanelBar from './PanelBar.svelte';
	import PanelDetails from './PanelDetails.svelte';
	import PanelHero from './PanelHero.svelte';

	let { task, onclose }: { task: DirectoryTask | undefined; onclose: () => void } = $props();

	let last: DirectoryTask | undefined;
	const shown = $derived.by(() => {
		if (task) last = task;
		return last;
	});

	const sync: Attachment<HTMLDialogElement> = (dialog) => {
		if (!task) dialog.close();
		else if (!dialog.open) {
			dialog.style.translate = '';
			dialog.showModal();
			dialog.focus();
		}
	};

	const dismiss: Attachment<HTMLDialogElement> = (dialog) => {
		let pressedOutside = false;
		const offs = [
			on(dialog, 'close', () => {
				if (task && !dialog.open) onclose();
			}),
			on(dialog, 'pointerdown', (event) => (pressedOutside = event.target === dialog)),
			on(dialog, 'click', (event) => {
				if (pressedOutside && event.target === dialog) onclose();
			})
		];
		return () => offs.forEach((off) => off());
	};
</script>

<dialog
	{@attach sync}
	{@attach dismiss}
	tabindex="-1"
	aria-label={shown && `${shown.name} in ${shown.project.slug}`}
	class="fixed inset-y-3 right-3 left-auto h-auto max-h-none w-110 max-w-[calc(100%-24px)] flex-col overflow-hidden rounded-box bg-surface text-label shadow-2 outline-none open:flex max-sm:inset-x-0 max-sm:top-auto max-sm:bottom-0 max-sm:max-h-[90dvh] max-sm:w-auto max-sm:max-w-none max-sm:rounded-b-none"
>
	{#if shown}
		<PanelBar task={shown} {onclose} />
		<div class="flex-auto overflow-y-auto overscroll-contain px-5 pt-3 pb-6">
			<PanelHero task={shown} />
			<PanelDetails task={shown} />
		</div>
	{/if}
</dialog>

<style>
	dialog {
		--from: translateX(calc(100% + 12px));
		transform: var(--from);
		transition:
			transform 400ms var(--ease-layout),
			translate 400ms var(--ease-layout),
			display 400ms allow-discrete,
			overlay 400ms allow-discrete;

		@media (width < 45rem) {
			--from: translateY(100%);
		}

		&::backdrop {
			background-color: transparent;
			transition:
				background-color 200ms var(--ease-out),
				display 200ms allow-discrete,
				overlay 200ms allow-discrete;
		}
	}

	dialog[open] {
		transform: none;

		&::backdrop {
			background-color: var(--app-scrim);
		}

		@starting-style {
			transform: var(--from);

			&::backdrop {
				background-color: transparent;
			}
		}
	}
</style>
