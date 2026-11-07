<script lang="ts">
	import { directory } from '#entities/task';
	import { taskKey } from '#shared/api';
	import type { DirectoryTask } from '#shared/api';
	import { plural } from '#shared/lib';
	import { flash, move, rise } from '#shared/ui';
	import { groupTasks } from '../model/groups.ts';
	import { urlState } from '../model/url-state.ts';
	import GroupHead from './GroupHead.svelte';
	import { AREA, CARD, COLHEAD, ROW, ROWS } from './layout.ts';
	import TaskRow from './TaskRow.svelte';

	let { tasks, needle }: { tasks: DirectoryTask[]; needle: string } = $props();

	const projectColumn = $derived(urlState.group !== 'project' && !urlState.project);
	const groups = $derived(groupTasks(tasks, urlState.group, directory.projects));
</script>

<div class={projectColumn ? ROWS.project : ROWS.plain}>
	<div class={COLHEAD} aria-hidden="true">
		<span></span><span>Task</span><span>Description</span>
		{#if projectColumn}<span>Project</span>{/if}
		<span>People</span><span>Pipeline</span>
		<span class={AREA.time}>Updated</span><span class={AREA.open}></span>
	</div>
	{#each groups as group (`${group.kind}/${group.key}`)}
		<section
			class="mb-6 last:mb-0"
			aria-label="{group.label}, {plural(group.tasks.length, 'task')}"
			animate:move
			transition:rise
		>
			<GroupHead {group} />
			<div class={CARD}>
				{#each group.tasks as task (taskKey(task))}
					<div
						class={[
							ROW,
							'transition-colors duration-[120ms] ease-out hover:bg-fill-1 has-[[aria-current]]:bg-accent-soft'
						]}
						animate:move
						transition:rise
						{@attach flash(task)}
					>
						<TaskRow {task} {needle} {projectColumn} />
					</div>
				{/each}
			</div>
		</section>
	{/each}
</div>
