<script lang="ts">
	import GitCommitHorizontal from '@lucide/svelte/icons/git-commit-horizontal';
	import type { PublicTask } from '#shared/api';
	import { buildLine, deployLine } from '../model/pipeline.ts';
	import PipeLabel from './PipeLabel.svelte';

	let {
		task,
		now,
		optionalRevision = false
	}: { task: PublicTask; now: number; optionalRevision?: boolean } = $props();

	const lines = $derived([
		{ key: 'Build', line: buildLine(task, now) },
		{ key: 'Deploy', line: deployLine(task, now) }
	]);
</script>

{#snippet title(key: string)}
	<span class="w-16 flex-none text-footnote text-label-2">{key}</span>
{/snippet}

{#snippet aside(text: string)}
	<span class="ms-auto ps-2.5 text-footnote whitespace-nowrap text-label-3">{text}</span>
{/snippet}

{#each lines as { key, line } (key)}
	<li class="ilist-row">
		{@render title(key)}
		<PipeLabel kind={line.kind} text={line.text} progress={line.progress} />
		{@render aside(line.when)}
	</li>
{/each}
{#if task.revision}
	<li class="ilist-row">
		{@render title('Revision')}
		<span
			class="inline-flex h-[22px] items-center gap-[5px] rounded-label bg-fill-1 px-[7px] font-mono text-caption/none font-medium text-label"
			title={task.revision.commit}
		>
			<GitCommitHorizontal size={12} />{task.revision.commit.slice(0, 7)}
		</span>
		{#if task.revision.previous}
			<span class="ms-auto ps-2.5 text-footnote whitespace-nowrap text-label-3">
				from <span class="font-mono" title={task.revision.previous}
					>{task.revision.previous.slice(0, 7)}</span
				>
			</span>
		{/if}
	</li>
{:else if !optionalRevision}
	<li class="ilist-row">
		{@render title('Revision')}
		<span class="text-footnote text-label-3">Not reported</span>
	</li>
{/if}
