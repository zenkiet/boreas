<script lang="ts">
	import ChevronLeft from '@lucide/svelte/icons/chevron-left';
	import { directory } from '#entities/task';
	import { Avatar, AvatarStack } from '#entities/user';
	import { plural } from '#shared/lib';
	import { SearchField } from '#shared/ui';
	import { people } from '../model/groups.ts';
	import type { Heading } from '../model/scope.ts';
	import { hrefWith, replaceWith, urlState } from '../model/url-state.ts';
	import { HEAD, HEAD_SIDE, HEAD_TEXT, TITLE } from './layout.ts';
	import ProjectMark from './ProjectMark.svelte';

	let { heading }: { heading: Heading } = $props();

	let query = $derived(urlState.q);

	const team = $derived(
		heading.kind === 'project'
			? people(directory.tasks.filter((task) => task.project.slug === heading.slug)).map(
					([name]) => name
				)
			: []
	);
</script>

<div class={HEAD}>
	<div class={HEAD_TEXT}>
		{#if heading.kind !== 'all'}
			<a
				class="mb-1.5 -ml-1 inline-flex items-center gap-0.5 rounded-label px-1 py-0.5 text-footnote/5 font-medium text-label-2 hover:bg-fill-1 hover:text-label"
				href={hrefWith({ person: '' }, '')}
			>
				<ChevronLeft size={14} />All tasks
			</a>
		{/if}
		<div class="flex min-w-0 items-center gap-3">
			{#if heading.kind === 'project'}
				<ProjectMark name={heading.title} large />
			{:else if heading.kind === 'person'}
				<Avatar username={heading.title} size={40} />
			{:else if heading.kind === 'none'}
				<Avatar size={40} />
			{/if}
			<h1 class={TITLE}>{heading.title}</h1>
		</div>
		{#if heading.kind === 'project'}
			<p class="mt-1.5 text-label-2">
				<span class="font-mono">/{heading.slug}</span> · {heading.sub}
			</p>
		{:else if heading.sub}
			<p class="mt-1.5 text-label-2">{heading.sub}</p>
		{/if}
		{#if team.length}
			<div class="mt-3 flex items-center gap-2 text-footnote text-label-2">
				<AvatarStack usernames={team} max={5} />
				<span>{plural(team.length, 'person', 'people')}</span>
			</div>
		{/if}
	</div>
	<div class={HEAD_SIDE}>
		<SearchField
			label="Search tasks, projects and people"
			placeholder="Search tasks, projects, people"
			bind:value={() => query, (q) => replaceWith({ q: (query = q) })}
		/>
	</div>
</div>
