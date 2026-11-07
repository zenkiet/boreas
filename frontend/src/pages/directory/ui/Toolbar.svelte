<script lang="ts">
	import Kanban from '@lucide/svelte/icons/kanban';
	import List from '@lucide/svelte/icons/list';
	import { StatusGlyph } from '#entities/task';
	import { DEV_STATUSES, DEV_STATUS_LABEL, devCount } from '#shared/api';
	import type { DirectoryTask } from '#shared/api';
	import { Segmented } from '#shared/ui';
	import type { GroupBy } from '../model/groups.ts';
	import { replaceWith, urlState } from '../model/url-state.ts';

	let { tasks }: { tasks: DirectoryTask[] } = $props();

	const GROUPS: { value: GroupBy; label: string }[] = [
		{ value: 'status', label: 'Status' },
		{ value: 'project', label: 'Project' },
		{ value: 'person', label: 'People' }
	];
	const VIEWS = [
		{ value: 'list', label: 'List', icon: List },
		{ value: 'board', label: 'Board', icon: Kanban }
	] as const;
</script>

<div class="mb-4 flex flex-wrap items-center gap-x-3 gap-y-2.5 @max-[720px]/main:gap-2.5">
	<div
		class="flex [scrollbar-width:none] flex-wrap gap-1.5 @max-[720px]/main:-mx-4 @max-[720px]/main:flex-nowrap @max-[720px]/main:overflow-x-auto @max-[720px]/main:px-4 @max-[720px]/main:pb-0.5"
		role="group"
		aria-label="Filter by status"
	>
		<button
			type="button"
			class="chip"
			aria-pressed={urlState.status === 'all'}
			onclick={() => replaceWith({ status: '' })}
		>
			All<span data-count>{tasks.length}</span>
		</button>
		{#each DEV_STATUSES as status (status)}
			<button
				type="button"
				class="chip"
				aria-pressed={urlState.status === status}
				onclick={() => replaceWith({ status })}
			>
				<StatusGlyph {status} size={14} />
				{DEV_STATUS_LABEL[status]}<span data-count>{devCount(tasks, status)}</span>
			</button>
		{/each}
	</div>
	<div
		class="ms-auto flex items-center gap-2.5 @max-[720px]/main:ms-0 @max-[720px]/main:w-full @max-[720px]/main:justify-between"
	>
		{#if urlState.view === 'list'}
			<div class="inline-flex items-center gap-2">
				<span class="text-footnote font-medium text-label-3">Group</span>
				<Segmented
					options={GROUPS}
					label="Group by"
					bind:value={
						() => urlState.group, (group) => replaceWith({ group: group === 'status' ? '' : group })
					}
				/>
			</div>
		{/if}
		<Segmented
			options={VIEWS}
			label="Layout"
			iconOnly
			bind:value={
				() => urlState.view, (view) => replaceWith({ view: view === 'board' ? view : '' })
			}
		/>
	</div>
</div>
