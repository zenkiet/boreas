<script lang="ts">
	import Clock from '@lucide/svelte/icons/clock';
	import Compass from '@lucide/svelte/icons/compass';
	import List from '@lucide/svelte/icons/list';
	import Power from '@lucide/svelte/icons/power';
	import RefreshCw from '@lucide/svelte/icons/refresh-cw';
	import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import type { TaskStatus } from '#shared/api';
	import { PageTop, StatusPage } from '#shared/ui';
	import { directory } from '#entities/task';
	import { closestMatches, copyOf, findTask, kindOf } from '../model/kind.ts';
	import ClosestMatches from './ClosestMatches.svelte';
	import RetryRing from './RetryRing.svelte';
	import TaskContext from './TaskContext.svelte';

	const LOOK = {
		starting: { icon: Clock, tone: 'warn' },
		stopped: { icon: Power, tone: 'neutral' },
		failed: { icon: TriangleAlert, tone: 'bad' },
		missing: { icon: Compass, tone: 'neutral' },
		notfound: { icon: Compass, tone: 'neutral' }
	} as const;

	$effect(() => directory.live());

	const slug = $derived(page.params.project ?? '');
	const name = $derived(page.params.task ?? '');
	const task = $derived(findTask(directory.tasks, slug, name));
	const kind = $derived(kindOf(directory.projects, slug, task));
	const copy = $derived(copyOf(kind, slug, task?.name ?? name, task?.assignees));
	const outage = $derived(copyOf('error', slug, name));
	const retries = $derived(kind !== 'missing' && kind !== 'notfound');
	const scoped = $derived(kind !== 'notfound');

	let checking = $state(false);
	let first: TaskStatus | undefined;

	const check = () => {
		checking = true;
		location.reload();
	};

	$effect(() => {
		if (!task) return;
		first ??= task.status;
		if (task.status !== 'running' || first === 'running') return;
		first = 'running';
		check();
	});
</script>

<svelte:head>
	<title>
		{directory.status === 'loading'
			? 'Boreas'
			: `${(directory.status === 'error' ? outage : copy).title} · Boreas`}
	</title>
</svelte:head>

{#snippet checkingLine()}
	<p role="status" class={['text-footnote text-label-3', !checking && 'sr-only']}>
		{checking ? 'Checking…' : ''}
	</p>
{/snippet}

{#snippet serverError()}
	<StatusPage icon={TriangleAlert} tone="bad" {...outage} path={page.url.pathname}>
		{#snippet actions()}
			<button type="button" class="btn" onclick={check}><RefreshCw size={16} />Try again</button>
			<a class="btn btn-ghost" href={resolve('/(directory)')}><List size={16} />Browse all tasks</a>
		{/snippet}
		{#snippet extra()}{@render checkingLine()}{/snippet}
	</StatusPage>
{/snippet}

<svelte:boundary failed={serverError}>
	{#if directory.status === 'loading'}
		<PageTop />
	{:else if directory.status === 'error'}
		{@render serverError()}
	{:else}
		<StatusPage
			icon={LOOK[kind].icon}
			tone={LOOK[kind].tone}
			{...copy}
			path={scoped ? `/${slug}/${task?.name ?? name}/` : page.url.pathname}
		>
			{#snippet actions()}
				{#if kind === 'starting'}
					<RetryRing onretry={check} />
				{:else if retries}
					<button type="button" class="btn" onclick={check}><RefreshCw size={16} />Retry</button>
				{/if}
				<a
					class={['btn', retries && 'btn-ghost']}
					href={scoped
						? resolve('/(directory)/[project]', { project: slug })
						: resolve('/(directory)')}
				>
					<List size={16} />{scoped ? `All tasks in ${slug}` : 'Browse all tasks'}
				</a>
			{/snippet}
			{#snippet extra()}
				{#if retries}{@render checkingLine()}{/if}
				{#if task}
					<TaskContext {task} />
				{:else if kind === 'missing'}
					{@const matches = closestMatches(directory.tasks, slug, name)}
					{#if matches.length}<ClosestMatches tasks={matches} />{/if}
				{/if}
			{/snippet}
		</StatusPage>
	{/if}
</svelte:boundary>
