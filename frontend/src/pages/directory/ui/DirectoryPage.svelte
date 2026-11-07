<script lang="ts">
	import { directory } from '#entities/task';
	import { bySeverity } from '#shared/api';
	import { rise } from '#shared/ui';
	import { headingOf, inScope } from '../model/scope.ts';
	import { matches, needleOf } from '../model/search.ts';
	import { urlState } from '../model/url-state.ts';
	import DirectoryStates from './DirectoryStates.svelte';
	import LoadingState from './LoadingState.svelte';
	import NotFound from './NotFound.svelte';
	import ProjectChips from './ProjectChips.svelte';
	import ScopeHeader from './ScopeHeader.svelte';
	import TaskBoard from './TaskBoard.svelte';
	import TaskList from './TaskList.svelte';
	import Toolbar from './Toolbar.svelte';

	const scope = $derived({ project: urlState.project, person: urlState.person });
	const needle = $derived(needleOf(urlState.q));
	const missing = $derived(
		directory.status === 'ready' &&
			!!scope.project &&
			!directory.projects.some((project) => project.slug === scope.project)
	);
	const heading = $derived(headingOf(directory.tasks, directory.projects, scope));
	const scoped = $derived(
		directory.tasks.filter((task) => inScope(task, scope) && matches(task, needle))
	);
	const visible = $derived(
		bySeverity(
			urlState.status === 'all'
				? scoped
				: scoped.filter((task) => task.devStatus === urlState.status)
		)
	);
</script>

<svelte:head><title>{missing ? 'Not found' : heading.title} · Boreas</title></svelte:head>

{#if directory.status === 'loading'}
	<LoadingState />
{:else if directory.status === 'error'}
	<DirectoryStates kind="error" />
{:else if missing}
	<NotFound slug={scope.project} />
{:else}
	<div in:rise>
		<ScopeHeader {heading} />
		<ProjectChips />
		<Toolbar tasks={scoped} />
		{#if !directory.tasks.length}
			<DirectoryStates kind="empty" />
		{:else if !visible.length}
			<DirectoryStates kind="nomatch" />
		{:else if urlState.view === 'board'}
			<TaskBoard tasks={visible} />
		{:else}
			<TaskList tasks={visible} {needle} />
		{/if}
	</div>
{/if}
