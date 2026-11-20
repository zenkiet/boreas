<script module lang="ts">
	let last = '';

	const fresh = (message: string) => (last = message === last ? `${message}\u00A0` : message);
</script>

<script lang="ts">
	import Check from '@lucide/svelte/icons/check';
	import Copy from '@lucide/svelte/icons/copy';
	import Link from '@lucide/svelte/icons/link';
	import type { ClassValue } from 'svelte/elements';
	import { copyText } from '#shared/lib';

	let {
		text,
		label = 'Copy link',
		compact = false,
		class: className
	}: { text: string; label?: string; compact?: boolean; class?: ClassValue } = $props();

	let outcome = $state<'copied' | 'failed'>();
	let message = $state('');
	let timer: ReturnType<typeof setTimeout> | undefined;

	const copy = () =>
		copyText(text).then((done) => {
			clearTimeout(timer);
			outcome = done ? 'copied' : 'failed';
			message = '';
			requestAnimationFrame(() => (message = fresh(done ? 'Copied' : 'Copy failed')));
			timer = setTimeout(() => {
				outcome = undefined;
				message = '';
			}, 2000);
		});
</script>

<span class={['relative inline-flex', className]}>
	<button
		type="button"
		class={[
			compact
				? 'grid min-h-7 min-w-7 cursor-pointer place-items-center rounded-label border border-sep bg-surface text-label-2 transition-colors duration-200 ease-out hover:bg-fill-2 hover:text-label max-sm:min-h-11 max-sm:min-w-11'
				: 'btn flex-1 btn-ghost',
			compact && outcome === 'copied' && 'text-ready',
			compact && outcome === 'failed' && 'text-blocked'
		]}
		aria-label={compact ? label : undefined}
		onclick={copy}
	>
		{#if outcome === 'copied'}
			<Check size={16} class="pop" />
		{:else if compact}
			<Copy size={16} class="pop" />
		{:else}
			<Link size={16} class="pop" />
		{/if}
		{#if !compact}{label}{/if}
	</button>
	<span
		role="status"
		class={[
			'pointer-events-none absolute bottom-full left-1/2 mb-1.5 -translate-x-1/2 rounded-[8px] bg-label px-2 py-1 text-caption whitespace-nowrap text-canvas transition-opacity duration-200 ease-out',
			!message && 'opacity-0'
		]}
	>
		{message}
	</span>
</span>
