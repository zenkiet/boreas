<script lang="ts">
	import type { DirectoryTask } from '#shared/api';
	import { ProjectTag } from '#entities/project';
	import { directory, PipelineRows, StatusBadge } from '#entities/task';
	import { AvatarStack } from '#entities/user';

	let { task }: { task: DirectoryTask } = $props();
</script>

<section
	aria-label="About this environment"
	class="mt-2 w-full rounded-box bg-surface p-4 text-left shadow-1"
>
	<div class="flex min-w-0 items-center gap-2">
		<ProjectTag slug={task.project.slug} link icon title="Every task in {task.project.name}" />
		<span class="min-w-0 flex-1 truncate font-mono text-subhead font-semibold">{task.name}</span>
		<StatusBadge status={task.devStatus} />
	</div>
	<p class={['mt-2 mb-3 text-subhead/[21px]', task.description ? 'text-label' : 'text-label-3']}>
		{task.description || 'No description'}
	</p>
	<ul class="ilist">
		{#if task.assignees.length}
			<li class="ilist-row">
				<span class="w-16 flex-none text-footnote text-label-2">People</span>
				<AvatarStack usernames={task.assignees} />
				<span class="min-w-0 flex-1 truncate">{task.assignees.join(', ')}</span>
			</li>
		{/if}
		<PipelineRows {task} now={directory.now} optionalRevision />
	</ul>
</section>
