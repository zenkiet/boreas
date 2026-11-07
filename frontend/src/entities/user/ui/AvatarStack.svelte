<script lang="ts">
	import Avatar from './Avatar.svelte';

	let { usernames, max = 3 }: { usernames: readonly string[]; max?: number } = $props();

	const more = $derived(usernames.length - max);
</script>

<span class="inline-flex items-center" title={usernames.join(', ')} aria-hidden="true">
	{#each usernames.slice(0, max) as username, i (username)}
		<Avatar {username} class={['ring-2 ring-surface', i > 0 && '-ml-1.5']} />
	{/each}
	{#if more > 0}
		<span
			class="-ml-1.5 inline-grid size-6 flex-none place-items-center rounded-full bg-fill-3 text-[10px] leading-none font-semibold text-label-2 ring-2 ring-surface"
			>+{more}</span
		>
	{/if}
</span>
