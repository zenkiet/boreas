<script lang="ts">
	import { DEV_STATUS_LABEL } from '#shared/api';
	import type { DevStatus } from '#shared/api';
	import StatusGlyph from './StatusGlyph.svelte';

	let { status }: { status: DevStatus } = $props();

	const TONE = {
		blocked: 'bg-blocked-soft text-blocked',
		in_progress: 'bg-progress-soft text-progress-text',
		ready: 'bg-ready-soft text-ready'
	};
</script>

<span
	class={[
		'inline-flex h-6 items-center gap-1.5 rounded-label pr-[9px] pl-1.5 text-footnote font-semibold whitespace-nowrap',
		TONE[status]
	]}
>
	<StatusGlyph {status} size={14} />{DEV_STATUS_LABEL[status]}
</span>
