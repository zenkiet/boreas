<script lang="ts">
	import { ProjectTag } from '#entities/project';
	import { directory, PipeLabel, pipelineOf } from '#entities/task';
	import { taskKey } from '#shared/api';
	import type { DirectoryTask } from '#shared/api';
	import { ago, stamp } from '#shared/lib';
	import { openTask, showProject, urlState } from '../model/url-state.ts';
	import ContainerFlag from './ContainerFlag.svelte';
	import TaskId from './TaskId.svelte';
	import TaskPeople from './TaskPeople.svelte';

	let { task }: { task: DirectoryTask } = $props();

	const key = $derived(taskKey(task));
	const open = $derived(urlState.task === key);
</script>

<article
	class="relative flex flex-col gap-2 rounded-nest bg-surface p-3 shadow-1 transition-shadow duration-200 ease-snap hover:ring-1 hover:ring-line data-on:ring-2 data-on:ring-accent"
	data-on={open || undefined}
>
	<button
		type="button"
		class="absolute inset-0 cursor-pointer rounded-nest"
		aria-label="Details for {task.name} in {task.project.slug}"
		aria-current={open || undefined}
		onclick={() => openTask(key)}
	></button>
	<div class="flex min-w-0 items-center gap-1.5">
		<ProjectTag
			slug={task.project.slug}
			title="Show every task in {task.project.name}"
			onclick={() => showProject(task.project.slug)}
		/>
		<span class="flex-auto"></span>
		<span class="text-caption font-normal text-label-3 tabular-nums" title={stamp(task.updatedAt)}
			>{ago(task.updatedAt, directory.now)}</span
		>
	</div>
	<div class="flex min-w-0 items-center gap-1.5">
		<TaskId {task} />
		<ContainerFlag status={task.status} />
	</div>
	<p class={['line-clamp-2 text-[14px]/[19px]', task.description ? 'text-label' : 'text-label-3']}>
		{task.description || 'No description'}
	</p>
	<div class="flex min-w-0 items-center justify-between gap-2">
		<TaskPeople {task} />
		<PipeLabel {...pipelineOf(task)} />
	</div>
</article>
