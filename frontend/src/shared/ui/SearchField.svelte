<script lang="ts">
	import Search from '@lucide/svelte/icons/search';
	import X from '@lucide/svelte/icons/x';
	import type { Attachment } from 'svelte/attachments';
	import { on } from 'svelte/events';

	let {
		value = $bindable(''),
		label,
		placeholder
	}: { value?: string; label: string; placeholder: string } = $props();

	let input: HTMLInputElement;

	const slashFocuses: Attachment<HTMLInputElement> = (field) =>
		on(window, 'keydown', (event) => {
			const target = event.target as HTMLElement;
			if (event.key !== '/' || event.metaKey || event.ctrlKey || event.altKey) return;
			if (target.isContentEditable || target.matches('input, textarea, select')) return;
			event.preventDefault();
			field.focus();
		});

	const clear = () => {
		value = '';
		input.focus();
	};
</script>

<label
	class="flex h-9 cursor-text items-center gap-2 rounded-full bg-surface pr-2 pl-3 text-label-3 inset-ring inset-ring-sep transition-shadow duration-200 ease-snap focus-within:inset-ring-2 focus-within:inset-ring-accent"
>
	<Search size={16} />
	<input
		bind:this={input}
		bind:value
		{@attach slashFocuses}
		type="search"
		aria-label={label}
		{placeholder}
		autocomplete="off"
		spellcheck="false"
		class="h-full min-w-0 flex-1 bg-transparent text-subhead text-label outline-none placeholder:text-label-3 [&::-webkit-search-cancel-button]:appearance-none"
	/>
	{#if value}
		<button
			type="button"
			class="relative grid size-5 flex-none cursor-pointer place-items-center rounded-full bg-fill-3 text-label-2 after:absolute after:-inset-3"
			aria-label="Clear search"
			onclick={clear}
		>
			<X size={12} />
		</button>
	{:else}
		<kbd
			class="inline-grid h-5 min-w-5 place-items-center rounded-label bg-fill-1 px-[5px] font-mono text-caption2/none font-medium text-label-3 inset-ring inset-ring-sep pointer-coarse:hidden"
			title="Press / to search">/</kbd
		>
	{/if}
</label>
