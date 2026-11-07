<script lang="ts">
	import List from '@lucide/svelte/icons/list';
	import { directory, StatusGlyph } from '#entities/task';
	import { Avatar } from '#entities/user';
	import { devCount } from '#shared/api';
	import { plural } from '#shared/lib';
	import { ThemeToggle } from '#shared/theme';
	import { Brand, Skeleton } from '#shared/ui';
	import { people } from '../model/groups.ts';
	import { isUnassigned, NONE } from '../model/scope.ts';
	import { hrefWith, urlState } from '../model/url-state.ts';
	import NavItem from './NavItem.svelte';
	import ProjectMark from './ProjectMark.svelte';

	const loading = $derived(directory.status === 'loading');
	const team = $derived(people(directory.tasks));
	const unassigned = $derived(directory.tasks.filter(isUnassigned).length);

	const NAV = 'flex flex-col gap-px';
	const LABEL = 'flex justify-between px-2.5 pb-1.5 text-caption/5 font-semibold text-label-3';
	const NAME = 'min-w-0 flex-1 truncate';
	const SKELETON = [70, 55, 80, 45, 62];
</script>

{#snippet skeletons(round: boolean)}
	{#each SKELETON as width (width)}
		<div class="flex h-[34px] items-center gap-2.5 px-2.5">
			<Skeleton class={[round && 'rounded-full']} width="20px" height="20px" />
			<Skeleton width="{width}%" height="12px" />
		</div>
	{/each}
{/snippet}

<aside
	class="flex w-62 flex-none flex-col gap-[22px] border-r border-sep px-3 pt-4 pb-5 max-lg:hidden"
	aria-label="Directory"
>
	<div class="flex items-center justify-between ps-2.5"><Brand /><ThemeToggle /></div>

	<nav class={NAV} aria-label="Views">
		<NavItem
			href={hrefWith({ person: '' }, '')}
			current={!urlState.project && !urlState.person}
			count={loading ? undefined : directory.tasks.length}
		>
			<List size={16} class="text-label-2 group-aria-[current=page]:text-inherit" />
			<span class={NAME}>All tasks</span>
		</NavItem>
	</nav>

	<nav class={NAV} aria-label="Projects">
		<div class={LABEL}>
			<span>Projects</span><span class="tabular-nums"
				>{loading ? '' : directory.projects.length}</span
			>
		</div>
		{#if loading}
			{@render skeletons(false)}
		{/if}
		{#each directory.projects as project (project.slug)}
			{@const blocked = devCount(project.tasks, 'blocked')}
			<NavItem
				href={hrefWith({ person: '' }, project.slug)}
				current={urlState.project === project.slug}
				count={project.tasks.length}
			>
				<ProjectMark name={project.name} />
				<span class={NAME}>{project.name}</span>
				{#if blocked}
					<StatusGlyph status="blocked" size={12} label={plural(blocked, 'blocked task')} />
				{/if}
			</NavItem>
		{:else}
			{#if !loading}<p class="px-2.5 py-1 text-footnote/5 text-label-3">No projects yet</p>{/if}
		{/each}
	</nav>

	<nav class={NAV} aria-label="People">
		<div class={LABEL}>
			<span>People</span><span class="tabular-nums">{loading ? '' : team.length}</span>
		</div>
		{#if loading}
			{@render skeletons(true)}
		{/if}
		{#each team as [name, own] (name)}
			<NavItem
				href={hrefWith({ person: name }, '')}
				current={urlState.person === name}
				count={own.length}
				replace
			>
				<Avatar username={name} size={20} />
				<span class={NAME}>{name}</span>
			</NavItem>
		{/each}
		{#if unassigned}
			<NavItem
				href={hrefWith({ person: NONE }, '')}
				current={urlState.person === NONE}
				count={unassigned}
				replace
			>
				<Avatar size={20} />
				<span class={NAME}>Unassigned</span>
			</NavItem>
		{/if}
	</nav>
</aside>
