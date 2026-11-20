<script lang="ts">
	import X from '@lucide/svelte/icons/x';
	import type { DirectoryTask } from '#shared/api';
	import ProjectMark from './ProjectMark.svelte';
	import { hrefWith } from '../model/url-state.ts';

	let { task, onclose }: { task: DirectoryTask; onclose: () => void } = $props();
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
		<!-- svelte-ignore a11y_autofocus -->
		<button
			type="button"
			autofocus
			class="ms-auto iconbtn iconbtn-soft"
			aria-label="Close details"
			onclick={onclose}
		>
			<X size={16} />
		</button>
	</div>
</div>
