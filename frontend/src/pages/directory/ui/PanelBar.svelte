<script lang="ts">
	import X from '@lucide/svelte/icons/x';
	import type { Attachment } from 'svelte/attachments';
	import { on } from 'svelte/events';
	import { MediaQuery } from 'svelte/reactivity';
	import type { DirectoryTask } from '#shared/api';
	import ProjectMark from './ProjectMark.svelte';
	import { hrefWith } from '../model/url-state.ts';

	let { task, onclose }: { task: DirectoryTask; onclose: () => void } = $props();

	const phone = new MediaQuery('width < 45rem');

	const drag: Attachment<HTMLElement> = (handle) => {
		const sheet = handle.parentElement as HTMLElement;
		let offset = 0;
		let gesture: (() => void)[] = [];
		const release = () => gesture.splice(0).forEach((off) => off());
		const grab = (down: PointerEvent) => {
			offset = 0;
			release();
			if (!phone.current) return;
			const stop = (dismissed: boolean) => {
				release();
				sheet.style.transition = '';
				if (dismissed) onclose();
				else sheet.style.translate = '';
			};
			gesture = [
				on(window, 'pointermove', (event) => {
					offset = Math.max(0, event.clientY - down.clientY);
					if (offset < 4) return;
					sheet.style.transition = 'none';
					sheet.style.translate = `0 ${offset}px`;
				}),
				on(window, 'pointerup', () => stop(offset > 80)),
				on(window, 'pointercancel', () => stop(false))
			];
		};
		const swallow = (event: MouseEvent) => {
			if (offset < 4) return;
			event.preventDefault();
			event.stopPropagation();
		};
		const offs = [on(handle, 'pointerdown', grab), on(handle, 'click', swallow, { capture: true })];
		return () => {
			release();
			offs.forEach((off) => off());
		};
	};
</script>

<div class="flex-none max-sm:touch-none" {@attach drag}>
	<span class="mx-auto mt-1.5 block h-[5px] w-9 rounded-full bg-fill-3 sm:hidden"></span>
	<div class="flex items-center gap-1 pt-3 pr-3 pl-4 max-sm:pt-1.5">
		<a
			class="inline-flex min-w-0 items-center gap-2 rounded-nest py-1 pr-1.5 pl-1 text-footnote font-medium text-label-2 no-underline transition-colors duration-200 ease-out hover:bg-fill-1 hover:text-label"
			href={hrefWith({ person: '' }, task.project.slug)}
			data-sveltekit-reset="false"
			title="Show every task in {task.project.name}"
		>
			<ProjectMark name={task.project.name} />
			<span class="truncate">{task.project.name}</span>
			<span class="font-mono text-caption font-medium text-label-3">/{task.project.slug}</span>
		</a>
		<button
			type="button"
			class="ms-auto iconbtn iconbtn-soft"
			aria-label="Close details"
			onclick={onclose}
		>
			<X size={16} />
		</button>
	</div>
</div>
