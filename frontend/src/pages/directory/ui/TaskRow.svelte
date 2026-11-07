<script lang="ts">
	import ArrowUpRight from '@lucide/svelte/icons/arrow-up-right';
	import { ProjectTag } from '#entities/project';
	import { directory, PipeLabel, pipelineOf, StatusGlyph } from '#entities/task';
	import { appUrl, DEV_STATUS_LABEL, taskKey } from '#shared/api';
	import type { DirectoryTask } from '#shared/api';
	import { ago, stamp } from '#shared/lib';
	import { openTask, showProject, urlState } from '../model/url-state.ts';
	import ContainerFlag from './ContainerFlag.svelte';
	import Highlight from './Highlight.svelte';
	import { AREA, ROW } from './layout.ts';
	import TaskId from './TaskId.svelte';
	import TaskPeople from './TaskPeople.svelte';

	let {
		task,
		needle,
		projectColumn
	}: { task: DirectoryTask; needle: string; projectColumn: boolean } = $props();

	const key = $derived(taskKey(task));
	const open = $derived(urlState.task === key);
	const idle = $derived(task.status !== 'running');
</script>

<div
	class={[
		ROW,
		'transition-colors duration-[120ms] ease-snap hover:bg-fill-1 data-on:bg-accent-soft'
	]}
	data-on={open || undefined}
>
	<button
		type="button"
		class="absolute inset-0 cursor-pointer focus-visible:-outline-offset-2"
		aria-label="Details for {task.name} in {task.project.slug}"
		aria-current={open || undefined}
		onclick={() => openTask(key)}
	></button>
	<StatusGlyph
		status={task.devStatus}
		label={DEV_STATUS_LABEL[task.devStatus]}
		class={AREA.glyph}
	/>
	<TaskId {task} {needle} {idle} class={['@max-[720px]/main:text-[14px]', AREA.id]} />
	<span class={['flex min-w-0 items-center gap-2 @max-[720px]/main:items-start', AREA.desc]}>
		<ContainerFlag status={task.status} />
		<span
			class={[
				'min-w-0 truncate @max-[720px]/main:line-clamp-2 @max-[720px]/main:whitespace-normal',
				idle ? 'text-label-2' : task.description ? 'text-label' : 'text-label-3'
			]}
		>
			{#if task.description}<Highlight text={task.description} {needle} />{:else}No description{/if}
		</span>
	</span>
	{#if projectColumn}
		<span class={['flex min-w-0 items-center', AREA.proj]}>
			<ProjectTag
				slug={task.project.slug}
				icon
				title="Show every task in {task.project.name}"
				onclick={() => showProject(task.project.slug)}
			/>
		</span>
	{/if}
	<span class={['flex min-w-0 items-center', AREA.people]}><TaskPeople {task} /></span>
	<span class={['flex min-w-0 items-center', AREA.pipe]}><PipeLabel {...pipelineOf(task)} /></span>
	<span
		class={['text-footnote/5 whitespace-nowrap text-label-3 tabular-nums', AREA.time]}
		title={stamp(task.updatedAt)}>{ago(task.updatedAt, directory.now)}</span
	>
	<a
		class={['relative z-1 iconbtn', AREA.open]}
		href={appUrl(task)}
		target="_blank"
		rel="external noopener"
		aria-label="Open the {task.name} app"
		title="Open app"
	>
		<ArrowUpRight size={16} />
	</a>
</div>
