<script lang="ts">
	import ChevronRight from '@lucide/svelte/icons/chevron-right';
	import Lock from '@lucide/svelte/icons/lock';
	import { StatusGlyph } from '#entities/task';
	import { Avatar } from '#entities/user';
	import { breakdown } from '../model/groups.ts';
	import type { TaskGroup } from '../model/groups.ts';
	import { hrefWith, urlState } from '../model/url-state.ts';
	import ProjectMark from './ProjectMark.svelte';

	let { group }: { group: TaskGroup } = $props();

	const pick = $derived.by(() => {
		if (group.kind === 'project' && !urlState.project) {
			return { label: 'Open project', href: hrefWith({ person: '' }, group.key), replace: false };
		}
		if ((group.kind === 'person' || group.kind === 'none') && urlState.person !== group.key) {
			return { label: 'Only these', href: hrefWith({ person: group.key }, ''), replace: true };
		}
	});
</script>

<div class="flex min-h-7 items-center gap-2 px-1 pb-2 text-footnote/5 text-label-2">
	{#if group.status}
		<StatusGlyph status={group.status} />
	{:else if group.kind === 'project'}
		<ProjectMark name={group.label} />
	{:else if group.kind === 'person'}
		<Avatar username={group.key} />
	{:else if group.kind === 'none'}
		<Avatar />
	{:else}
		<Lock size={16} class="flex-none" />
	{/if}
	<h2 class="text-footnote font-semibold text-label">{group.label}</h2>
	{#if group.kind === 'project'}
		<span class="font-mono text-caption font-normal text-label-3">/{group.key}</span>
	{/if}
	<span class="font-medium text-label-3 tabular-nums">{group.tasks.length}</span>
	{#if !group.status}<span class="text-label-3">· {breakdown(group.tasks)}</span>{/if}
	{#if pick}
		<a
			class="ms-auto inline-flex items-center gap-1 font-semibold text-accent-text hover:underline hover:underline-offset-2"
			href={pick.href}
			data-sveltekit-replacestate={pick.replace || undefined}
		>
			{pick.label}<ChevronRight size={12} />
		</a>
	{/if}
</div>
