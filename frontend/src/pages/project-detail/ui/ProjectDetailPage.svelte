<script lang="ts">
	/* eslint-disable svelte/no-navigation-without-resolve -- environment rows are served by the Go proxy. */
	import { resolve } from '$app/paths';
	import {
		TASK_GROUPS,
		TASK_GROUP_LABEL,
		bySeverity,
		groupCount,
		groupOf,
		type PublicProject,
		type PublicTask
	} from '@/shared/api';
	import { formatDateTime } from '@/shared/lib';
	import { EmptyState, Icon, InsetGroup, SearchField, SkeletonRows, StatusPage } from '@/shared/ui';
	import StatusFilter from './StatusFilter.svelte';

	let { slug, projects }: { slug: string; projects: Promise<readonly PublicProject[]> } = $props();

	let query = $state('');
	let filter = $state(0);

	const filterItems = (tasks: readonly PublicTask[]) => [
		{ label: 'All', count: tasks.length },
		...TASK_GROUPS.map((group) => ({
			label: TASK_GROUP_LABEL[group],
			count: groupCount(tasks, group)
		}))
	];

	const visibleTasks = (tasks: readonly PublicTask[]) => {
		const needle = query.trim().toLowerCase();
		return bySeverity(tasks).filter(
			(task) =>
				(filter === 0 || groupOf(task) === TASK_GROUPS[filter - 1]) &&
				`${task.name} ${task.description ?? ''}`.toLowerCase().includes(needle)
		);
	};

	const summary = (tasks: readonly PublicTask[]) =>
		TASK_GROUPS.filter((group) => groupCount(tasks, group))
			.map((group) => `${groupCount(tasks, group)} ${TASK_GROUP_LABEL[group].toLowerCase()}`)
			.join(' · ') || '0 environments';
</script>

<svelte:head>
	<title>{slug} · Boreas</title>
	<meta name="description" content="Preview environments served under /{slug}." />
</svelte:head>

{#await projects}
	<div
		class="mx-auto grid w-full max-w-160 grid-cols-[minmax(0,1fr)] content-start gap-4 px-4 py-6 sm:py-10"
	>
		<div class="grid gap-2">
			<span class="h-5 w-24 skeleton"></span>
			<span class="h-9 w-56 skeleton"></span>
		</div>
		<InsetGroup label="Environments">
			<SkeletonRows count={7} label="Loading environments" />
		</InsetGroup>
	</div>
{:then list}
	{@const found = list.find((project) => project.slug === slug)}
	{#if !found}
		<div class="flex flex-1 items-center justify-center px-4 py-16">
			<StatusPage
				code="404 · NOT FOUND"
				icon="compass"
				title="No project is served here"
				description="This server hosts no project under that prefix"
				path="/{slug}/"
			>
				{@const hints = list
					.map((project) => project.slug)
					.filter((other) => other.startsWith(slug.slice(0, 3)))
					.slice(0, 3)}
				<a class="btn" href={resolve('/')}><Icon name="layers" /> Browse projects</a>

				{#if hints.length}
					<div class="grid w-full gap-1.5 text-start">
						<span class="text-meta px-1 font-medium text-tertiary">Did you mean</span>
						<div class="overflow-hidden rounded-lg bg-base">
							{#each hints as hint (hint)}
								<a class="row row-divider relative" href="/{hint}/">
									<span class="row__id min-w-0 flex-1 font-mono">/{hint}</span>
									<Icon name="chevron-right" size={15} class="flex-none text-tertiary" />
								</a>
							{/each}
						</div>
					</div>
				{/if}
			</StatusPage>
		</div>
	{:else}
		{@const visible = visibleTasks(found.tasks)}
		<div
			class="mx-auto grid w-full max-w-160 grid-cols-[minmax(0,1fr)] content-start gap-4 px-4 py-6 sm:py-10"
		>
			<header class="grid gap-1.5">
				<a
					href={resolve('/')}
					class="text-row -ms-1 inline-flex items-center gap-0.5 font-medium text-secondary transition-colors hover:text-primary"
				>
					<Icon name="chevron-left" size={18} /> Projects
				</a>
				<h1
					class="mt-1 text-[clamp(1.75rem,5vw,2.125rem)] font-bold tracking-[-0.022em] text-primary"
				>
					{found.name}
				</h1>
				<p class="text-row-sub font-mono text-tertiary">/{found.slug}</p>
			</header>

			{#if found.tasks.length}
				<StatusFilter
					items={filterItems(found.tasks)}
					bind:active={filter}
					label="Filter environments by status"
				/>
				{#if found.tasks.length > 6}
					<SearchField
						bind:value={query}
						label="Search environments"
						placeholder="Search by task id or description"
					/>
				{/if}
			{/if}

			<InsetGroup label="Environments" trailing={summary(found.tasks)}>
				{#if found.tasks.length === 0}
					<EmptyState
						title="No environments yet"
						description="This project has no task environments running. Create one from the Boreas console and it appears here immediately."
					/>
				{:else if visible.length === 0}
					<EmptyState
						icon="search"
						title="No match"
						description="No environment in this project matches the current filter."
					/>
				{:else}
					{#each visible as task (task.name)}
						<a
							class="row row-divider relative"
							class:idle={task.status !== 'running'}
							href="/{found.slug}/{task.name}"
							data-sveltekit-reload
						>
							<i class="dot dot--{groupOf(task)}"></i>
							<span class="min-w-0 flex-1">
								<span class="row__id font-mono">
									{task.name}<span class="sr-only">, {task.status}</span>
								</span>
								<span class="row__sub font-mono">
									{task.description || '—'}{task.status !== 'running' ? ` · ${task.status}` : ''}
								</span>
							</span>
							<span class="row__meta tabular">{formatDateTime(task.updatedAt)}</span>
							<Icon name="chevron-right" size={15} class="flex-none text-tertiary" />
						</a>
					{/each}
				{/if}
			</InsetGroup>
		</div>
	{/if}
{/await}

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

	/* A route that cannot answer must not look as clickable as one that can. */
	.idle {
		opacity: 0.55;
	}

	.row__id,
	.row__sub {
		display: block;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.row__id {
		font-size: 1.0625rem;
		font-weight: 600;
		color: var(--tui-text-primary);
	}

	.row:hover .row__id {
		color: var(--tui-text-action);
	}

	.row__sub {
		font-size: 0.9375rem;
		line-height: 1.5;
		color: var(--tui-text-tertiary);
	}

	.row__meta {
		flex: none;
		font-size: 0.8125rem;
		color: var(--tui-text-tertiary);
		white-space: nowrap;
	}

	@media (max-width: 34rem) {
		.row__meta {
			display: none;
		}
	}
</style>
