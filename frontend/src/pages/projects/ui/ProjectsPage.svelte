<script lang="ts">
	import { resolve } from '$app/paths';
	import { bySeverity, devCount, runningCount, type PublicProject } from '@/shared/api';
	import { EmptyState, Icon, InsetGroup, SearchField, SkeletonRows } from '@/shared/ui';

	let { projects }: { projects: Promise<readonly PublicProject[]> } = $props();

	let query = $state('');

	const MAX_DOTS = 6;

	const matches = (project: PublicProject, term: string) =>
		`${project.name} ${project.slug}`.toLowerCase().includes(term.trim().toLowerCase());

	const taskLabel = (project: PublicProject) =>
		project.tasks.length
			? `${runningCount(project.tasks)}/${project.tasks.length} running`
			: 'empty';
</script>

<svelte:head>
	<title>Projects · Boreas</title>
	<meta name="description" content="Preview environments hosted on this Boreas server." />
</svelte:head>

<div
	class="mx-auto grid w-full max-w-160 grid-cols-[minmax(0,1fr)] content-start gap-4 px-4 py-6 sm:py-10"
>
	<header class="grid gap-1.5">
		<h1 class="text-[clamp(1.75rem,5vw,2.125rem)] font-bold tracking-[-0.022em] text-primary">
			Projects
		</h1>
		<p class="text-row-sub leading-relaxed text-secondary">
			Every project below is served under its own path prefix.
		</p>
	</header>

	{#await projects}
		<div class="grid grid-cols-2 gap-2 sm:grid-cols-4">
			{#each { length: 4 }, i (i)}
				<div class="flex flex-col gap-2 rounded-lg bg-base px-4 py-3">
					<span class="h-3.25 w-16 skeleton"></span>
					<span class="h-5.5 w-10 skeleton"></span>
				</div>
			{/each}
		</div>
		<InsetGroup label="Projects"><SkeletonRows count={4} label="Loading projects" /></InsetGroup>
	{:then list}
		{@const tasks = list.flatMap((project) => project.tasks)}
		{@const visible = list.filter((project) => matches(project, query))}

		<div class="grid grid-cols-2 gap-2 sm:grid-cols-4">
			{#each [['Projects', list.length, ''], ['In progress', devCount(tasks, 'in_progress'), 'var(--tui-status-warning)'], ['Ready', devCount(tasks, 'ready'), 'var(--tui-status-positive)'], ['Blocked', devCount(tasks, 'blocked'), 'var(--tui-status-negative)']] as [label, value, color] (label)}
				<div class="flex flex-col gap-1.5 rounded-lg bg-base px-4 py-3">
					<span class="text-meta text-secondary">{label}</span>
					<span class="text-[1.375rem] leading-none font-semibold tabular" style:color>{value}</span
					>
				</div>
			{/each}
		</div>

		{#if list.length > 5}
			<SearchField bind:value={query} label="Search projects" placeholder="Search projects" />
		{/if}

		<InsetGroup
			label="Projects"
			trailing="{visible.length} project{visible.length === 1 ? '' : 's'}"
		>
			{#if list.length === 0}
				<EmptyState
					icon="layers"
					title="No projects yet"
					description="A project groups related preview environments under one URL prefix. Create the first one from the Boreas console."
				/>
			{:else if visible.length === 0}
				<EmptyState
					icon="search"
					title="No match"
					description="No project name or prefix matches “{query}”."
				/>
			{:else}
				{#each visible as project (project.slug)}
					<a
						class="row row-divider relative"
						href={resolve('/[project]', { project: project.slug })}
					>
						<span class="min-w-0 flex-1">
							<span class="row__name">{project.name}</span>
							<span class="row__sub font-mono tabular">/{project.slug} · {taskLabel(project)}</span>
						</span>
						<span class="flex flex-none items-center gap-1" aria-hidden="true">
							{#each bySeverity(project.tasks).slice(0, MAX_DOTS) as task (task.name)}
								<i class="dot dot--{task.devStatus} size-1.5"></i>
							{/each}
							{#if project.tasks.length > MAX_DOTS}
								<span class="text-xs text-tertiary tabular">
									+{project.tasks.length - MAX_DOTS}
								</span>
							{/if}
						</span>
						<Icon name="chevron-right" size={15} class="flex-none text-tertiary" />
					</a>
				{/each}
			{/if}
		</InsetGroup>
	{:catch error}
		<InsetGroup label="Projects">
			<EmptyState
				icon="triangle-alert"
				tone="negative"
				title="Unable to load the directory"
				description={error.message ??
					'The Boreas API did not respond. The environments themselves are unaffected.'}
			>
				<button type="button" class="btn btn--ghost" onclick={() => location.reload()}>
					<Icon name="refresh-cw" /> Try again
				</button>
			</EmptyState>
		</InsetGroup>
	{/await}
</div>

<style>
	.row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		min-block-size: 3.75rem;
		padding: 0.6875rem 1rem;
		text-decoration: none;
		transition: background-color var(--tui-duration);
	}

	.row:hover {
		background: var(--tui-background-neutral-1);
	}

	.row__name,
	.row__sub {
		display: block;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.row__name {
		font-size: 1.0625rem;
		font-weight: 600;
		color: var(--tui-text-primary);
	}

	.row__sub {
		font-size: 0.9375rem;
		line-height: 1.5;
		color: var(--tui-text-tertiary);
	}
</style>
