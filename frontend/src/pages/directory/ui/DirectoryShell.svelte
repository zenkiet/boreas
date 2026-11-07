<script lang="ts">
	import RefreshCw from '@lucide/svelte/icons/refresh-cw';
	import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
	import type { Snippet } from 'svelte';
	import { onNavigate } from '$app/navigation';
	import { directory } from '#entities/task';
	import { taskKey } from '#shared/api';
	import { ThemeToggle } from '#shared/theme';
	import { Brand, EmptyState, viewTransition } from '#shared/ui';
	import { closeTask, switchesView, urlState } from '../model/url-state.ts';
	import { CARD } from './layout.ts';
	import Sidebar from './Sidebar.svelte';
	import TaskPanel from './TaskPanel.svelte';

	let { children }: { children: Snippet } = $props();

	const opened = $derived(directory.tasks.find((task) => taskKey(task) === urlState.task));

	const reason = (error: unknown) =>
		(error as Partial<App.Error> | undefined)?.message?.replace(/\.$/, '') ||
		'Something went wrong while showing it';

	$effect(() => directory.live());

	onNavigate((navigation) => (switchesView(navigation) ? viewTransition(navigation) : undefined));
</script>

<div class="flex min-h-dvh max-lg:flex-col">
	<Sidebar />
	<header
		class="sticky top-0 z-5 flex h-13 items-center gap-1 border-b border-sep bg-surface/86 pr-2 pl-4 backdrop-blur-[14px] backdrop-saturate-180 lg:hidden"
	>
		<Brand />
		<span class="flex-auto"></span>
		<ThemeToggle />
	</header>
	<main
		class="@container/main min-w-0 flex-auto px-8 pt-7 pb-14 max-lg:pt-5 max-sm:px-4 max-sm:pt-3 max-sm:pb-10"
	>
		<div class="mx-auto max-w-300">
			<svelte:boundary>
				{@render children()}
				{#snippet failed(error, reset)}
					<div class={CARD}>
						<EmptyState
							icon={TriangleAlert}
							tone="bad"
							title="Couldn’t load the directory"
							description={`${reason(error)}. Environments keep running.`}
						>
							<button type="button" class="btn" onclick={reset}>
								<RefreshCw size={16} />Try again
							</button>
						</EmptyState>
					</div>
				{/snippet}
			</svelte:boundary>
		</div>
	</main>
</div>

<TaskPanel task={opened} onclose={closeTask} />
