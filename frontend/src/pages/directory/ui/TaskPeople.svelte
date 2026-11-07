<script lang="ts">
	import Lock from '@lucide/svelte/icons/lock';
	import { directory } from '#entities/task';
	import { AvatarStack } from '#entities/user';
	import type { PublicTask } from '#shared/api';

	let { task }: { task: PublicTask } = $props();

	const spoken = $derived(
		task.locked
			? 'People hidden'
			: task.assignees.length
				? `Assigned to ${task.assignees.join(', ')}`
				: 'Unassigned'
	);
</script>

{#if task.locked}
	<span class="inline-flex items-center gap-[5px] text-footnote text-label-3" aria-hidden="true">
		<Lock size={12} />Private
	</span>
{:else if task.assignees.length}
	<AvatarStack usernames={task.assignees} avatars={directory.avatars} />
{:else}
	<span class="text-footnote text-label-3" aria-hidden="true">Unassigned</span>
{/if}
<span class="sr-only">{spoken}</span>
