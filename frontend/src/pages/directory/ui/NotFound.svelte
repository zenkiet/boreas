<script lang="ts">
	import Compass from '@lucide/svelte/icons/compass';
	import List from '@lucide/svelte/icons/list';
	import { directory } from '#entities/task';
	import { plural } from '#shared/lib';
	import { EmptyState } from '#shared/ui';
	import { suggest } from '../model/scope.ts';
	import { hrefWith } from '../model/url-state.ts';
	import { CARD } from './layout.ts';
	import ProjectMark from './ProjectMark.svelte';

	let { slug }: { slug: string } = $props();

	const hints = $derived(suggest(slug, directory.projects));
</script>

<div class={CARD}>
	<h1 class="sr-only">Not found</h1>
	<EmptyState
		icon={Compass}
		code="404 · NOT FOUND"
		title="No project at /{slug}"
		description="It may have been renamed or deleted."
	>
		{#if hints.length}
			<div class="mb-2 w-full max-w-105 text-left">
				<h3 class="text-headline text-label-3">Did you mean</h3>
				<ul class="ilist">
					{#each hints as hint (hint.slug)}
						<li>
							<a class="ilist-row" href={hrefWith({ person: '' }, hint.slug)}>
								<ProjectMark name={hint.name} />
								<span class="min-w-0 flex-1 truncate">
									{hint.name} <span class="font-mono text-label-3">/{hint.slug}</span>
								</span>
								<span class="text-footnote whitespace-nowrap text-label-3">
									{plural(hint.tasks.length, 'task')}
								</span>
							</a>
						</li>
					{/each}
				</ul>
			</div>
		{/if}
		<a class="btn" href={hrefWith({ person: '' }, '')}><List size={16} />Browse all tasks</a>
	</EmptyState>
</div>
