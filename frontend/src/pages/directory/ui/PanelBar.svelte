<script lang="ts">
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import ChevronUp from '@lucide/svelte/icons/chevron-up';
	import X from '@lucide/svelte/icons/x';
	import { taskKey } from '#shared/api';
	import type { DirectoryTask } from '#shared/api';
	import ProjectMark from './ProjectMark.svelte';
	import { hrefWith, switchTask } from '../model/url-state.ts';

	let { task, onclose }: { task: DirectoryTask; onclose: () => void } = $props();

	const step = (by: number) => {
		const keys = [
			...new Set(
				[...document.querySelectorAll<HTMLElement>('main [data-task]')].map(
					(row) => row.dataset.task!
				)
			)
		];
		const at = keys.indexOf(taskKey(task));
		const next = keys.at(at < 0 ? (by > 0 ? 0 : -1) : (at + by) % keys.length);
		if (next && next !== taskKey(task)) switchTask(next);
	};
</script>

<div class="flex-none">
	<span class="mx-auto mt-1.5 block h-[5px] w-9 rounded-full bg-fill-3 sm:hidden"></span>
	<div class="flex items-center gap-1 ps-4 pe-3 pt-3 max-sm:pt-1.5">
		<a
			class="inline-flex min-w-0 items-center gap-2 rounded-nest py-1 ps-1 pe-1.5 text-footnote font-medium text-label-2 no-underline transition-colors duration-200 ease-out hover:bg-fill-1 hover:text-label"
			href={hrefWith({ person: '' }, task.project.slug)}
			data-sveltekit-reset="false"
			title="Show every task in {task.project.name}"
		>
			<ProjectMark name={task.project.name} />
			<span class="truncate">{task.project.name}</span>
			<span class="font-mono text-caption font-medium text-label-3">/{task.project.slug}</span>
		</a>
		<div class="ms-auto flex gap-2">
			<button type="button" class="iconbtn" aria-label="Previous task" onclick={() => step(-1)}>
				<ChevronUp size={16} />
			</button>
			<button type="button" class="iconbtn" aria-label="Next task" onclick={() => step(1)}>
				<ChevronDown size={16} />
			</button>
			<!-- svelte-ignore a11y_autofocus -->
			<button
				type="button"
				autofocus
				class="iconbtn iconbtn-soft"
				aria-label="Close details"
				onclick={onclose}
			>
				<X size={16} />
			</button>
		</div>
	</div>
</div>
