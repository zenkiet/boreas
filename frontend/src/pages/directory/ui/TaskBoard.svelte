<script lang="ts">
	import { StatusGlyph } from '#entities/task';
	import { DEV_STATUS_LABEL, taskKey } from '#shared/api';
	import type { DirectoryTask } from '#shared/api';
	import { plural } from '#shared/lib';
	import { columns } from '../model/groups.ts';
	import { urlState } from '../model/url-state.ts';
	import BoardCard from './BoardCard.svelte';

	let { tasks }: { tasks: DirectoryTask[] } = $props();
</script>

<div
	class="grid grid-cols-3 items-start gap-4 @max-[720px]/main:-mx-4 @max-[720px]/main:snap-x @max-[720px]/main:snap-mandatory @max-[720px]/main:grid-cols-[repeat(3,86%)] @max-[720px]/main:overflow-x-auto @max-[720px]/main:px-4 @max-[720px]/main:pb-1"
>
	{#each columns(tasks, urlState.status) as column (column.status)}
		{@const label = DEV_STATUS_LABEL[column.status]}
		<section
			class="flex snap-start flex-col gap-2 rounded-box bg-fill-1 p-2"
			aria-label="{label}, {plural(column.tasks.length, 'task')}"
		>
			<div class="flex items-center gap-2 px-2 pt-1.5 pb-1">
				<StatusGlyph status={column.status} />
				<h2 class="text-footnote font-semibold">{label}</h2>
				<span class="font-medium text-label-3 tabular-nums">{column.tasks.length}</span>
			</div>
			{#each column.tasks as task (taskKey(task))}
				<BoardCard {task} />
			{:else}
				<p class="px-2 py-5 text-center text-footnote text-label-3">Nothing here right now</p>
			{/each}
		</section>
	{/each}
</div>
