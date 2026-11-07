<script lang="ts">
	import type { TaskStatus } from '#shared/api';

	let { status, large = false }: { status: TaskStatus; large?: boolean } = $props();

	const STARTING = { label: 'Starting', tone: 'bg-progress-soft text-progress-text' };
	const STOPPED = { label: 'Stopped', tone: 'bg-neutral-soft text-label-2' };

	const FLAG: Record<TaskStatus, { label: string; tone: string }> = {
		running: { label: 'Running', tone: 'bg-ready-soft text-ready' },
		creating: STARTING,
		starting: STARTING,
		stopped: STOPPED,
		unknown: STOPPED,
		error: { label: 'Error', tone: 'bg-blocked-soft text-blocked' }
	};

	const flag = $derived(FLAG[status]);
</script>

{#if large || status !== 'running'}
	<span class={['flag', large && 'flag-lg', flag.tone]}>
		{#if large}<span class="size-1.5 flex-none rounded-full bg-current"></span>{/if}
		{flag.label}
	</span>
{/if}
