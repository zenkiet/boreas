<script lang="ts">
	import Layers2 from '@lucide/svelte/icons/layers-2';
	import RefreshCw from '@lucide/svelte/icons/refresh-cw';
	import Search from '@lucide/svelte/icons/search';
	import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
	import { directory } from '#entities/task';
	import { EmptyState } from '#shared/ui';
	import { NONE } from '../model/scope.ts';
	import { hrefWith, urlState } from '../model/url-state.ts';
	import { CARD, HEAD, HEAD_TEXT, TITLE } from './layout.ts';

	let { kind }: { kind: 'error' | 'empty' | 'nomatch' } = $props();

	const query = $derived(urlState.q.trim());
	const place = $derived(
		urlState.project || (urlState.person === NONE ? 'unassigned tasks' : urlState.person)
	);
</script>

{#if kind === 'error'}
	<div class={HEAD}>
		<div class={HEAD_TEXT}>
			<h1 class={TITLE}>All tasks</h1>
		</div>
	</div>
{/if}
<div class={CARD}>
	{#if kind === 'error'}
		<EmptyState
			icon={TriangleAlert}
			tone="bad"
			title="Couldn’t load the directory"
			description="{directory.error} Environments keep running."
		>
			<button type="button" class="btn" onclick={() => directory.refresh()}>
				<RefreshCw size={16} />Try again
			</button>
		</EmptyState>
	{:else if kind === 'empty'}
		<EmptyState
			icon={Layers2}
			title="No tasks yet"
			description="Tasks show up here once created or deployed."
		/>
	{:else}
		<EmptyState
			icon={Search}
			title="No tasks match"
			description={query
				? `Nothing matches “${query}”${place ? ` in ${place}` : ''}.`
				: 'None with this status.'}
		>
			<a class="btn btn-ghost" href={hrefWith({ q: '', status: '', person: '' }, '')}
				>Clear filters</a
			>
		</EmptyState>
	{/if}
</div>
