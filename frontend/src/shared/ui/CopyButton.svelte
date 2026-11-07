<script lang="ts">
	import Check from '@lucide/svelte/icons/check';
	import Link from '@lucide/svelte/icons/link';
	import type { ClassValue } from 'svelte/elements';
	import { copyText } from '#shared/lib';

	let { text, class: className }: { text: string; class?: ClassValue } = $props();

	let copied = $state(false);

	$effect(() => {
		if (!copied) return;
		const timer = setTimeout(() => (copied = false), 1800);
		return () => clearTimeout(timer);
	});
</script>

<button
	type="button"
	class={['btn btn-ghost', className]}
	onclick={() => copyText(text).then((done) => (copied = done))}
>
	{#if copied}<Check size={16} class="pop" />{:else}<Link size={16} class="pop" />{/if}
	<span aria-live="polite">{copied ? 'Copied' : 'Copy link'}</span>
</button>
