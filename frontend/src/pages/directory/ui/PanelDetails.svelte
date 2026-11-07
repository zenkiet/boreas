<script lang="ts">
	import Lock from '@lucide/svelte/icons/lock';
	import type { DirectoryTask } from '#shared/api';
	import { plural, stamp } from '#shared/lib';
	import { directory, PipelineRows } from '#entities/task';
	import { Avatar } from '#entities/user';

	let { task }: { task: DirectoryTask } = $props();

	const load = (username: string) =>
		directory.tasks.filter((other) => other.assignees.includes(username)).length;
</script>

{#snippet label(text: string)}
	<h3 class="mt-6 mb-2 text-caption text-label-3">{text}</h3>
{/snippet}

{#snippet muted(text: string, locked = false)}
	<p class="flex items-center gap-1.5 rounded-nest bg-fill-1 p-3 text-footnote text-label-2">
		{#if locked}<Lock size={14} class="flex-none" />{/if}{text}
	</p>
{/snippet}

<section>
	{@render label('People')}
	{#if task.locked}
		{@render muted('Private', true)}
	{:else if task.assignees.length}
		<ul class="ilist">
			{#each task.assignees as username (username)}
				<li class="ilist-row">
					<Avatar {username} size={28} />
					<span class="min-w-0 flex-1 truncate">{username}</span>
					<span class="text-footnote whitespace-nowrap text-label-3">
						{plural(load(username), 'task')}
					</span>
				</li>
			{/each}
		</ul>
	{:else}
		{@render muted('Unassigned')}
	{/if}
</section>

<section>
	{@render label('Pipeline')}
	<ul class="ilist"><PipelineRows {task} now={directory.now} /></ul>
</section>

<section>
	{@render label('Note')}
	{#if task.locked}
		{@render muted('Private', true)}
	{:else if task.note}
		{#await import('./NoteMarkdown.svelte') then { default: NoteMarkdown }}
			<NoteMarkdown source={task.note} />
		{:catch}
			<p class="text-footnote whitespace-pre-wrap text-label-2">{task.note}</p>
		{/await}
	{:else}
		{@render muted('No note')}
	{/if}
</section>

<p class="mt-6 text-caption/5 font-normal text-label-3">Updated {stamp(task.updatedAt)}</p>
