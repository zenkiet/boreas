<script lang="ts">
	import { appUrl, DEV_STATUS_LABEL, taskKey } from '#shared/api';
	import type { DirectoryTask } from '#shared/api';
	import { ProjectTag } from '#entities/project';
	import { StatusGlyph } from '#entities/task';

	let { tasks }: { tasks: readonly DirectoryTask[] } = $props();
</script>

<section class="mt-2 w-full text-left">
	<h2 class="mb-2 text-caption text-label-3">Did you mean</h2>
	<ul class="ilist rounded-box bg-surface shadow-1">
		{#each tasks as task (taskKey(task))}
			<li>
				<a
					class="ilist-row text-label no-underline hover:bg-fill-1 focus-visible:-outline-offset-2"
					href={appUrl(task)}
					target="_blank"
					rel="external noopener"
				>
					<StatusGlyph status={task.devStatus} label={DEV_STATUS_LABEL[task.devStatus]} />
					<span class="flex-none font-mono text-footnote font-semibold">{task.name}</span>
					<span
						class={[
							'min-w-0 flex-1 truncate text-footnote',
							task.description ? 'text-label-2' : 'text-label-3'
						]}
					>
						{task.description || 'No description'}
					</span>
					<ProjectTag slug={task.project.slug} />
				</a>
			</li>
		{/each}
	</ul>
</section>
