<script lang="ts">
	import { lift } from '#shared/ui';
	import Avatar from './Avatar.svelte';

	let {
		usernames,
		avatars,
		max = 3
	}: {
		usernames: readonly string[];
		avatars?: Readonly<Record<string, string>>;
		max?: number;
	} = $props();

	const FACE =
		'peer relative transition-[translate,scale] duration-120 ease-out hover:z-1 hover:-translate-y-0.5 hover:scale-115 peer-hover:translate-x-1.5';

	const more = $derived(usernames.length - max);
	let tip: HTMLElement;
	let text = $state('');

	const show = (event: PointerEvent, name: string) => {
		if (event.pointerType !== 'mouse') return;
		const face = (event.currentTarget as Element).getBoundingClientRect();
		text = name;
		tip.style.left = `${face.left + face.width / 2}px`;
		tip.style.top = `${face.top}px`;
		tip.showPopover();
		lift(tip);
	};

	const hide = () => tip.hidePopover();
</script>

<span class="inline-flex items-center" aria-hidden="true" onpointerdown={hide} onwheel={hide}>
	{#each usernames.slice(0, max) as username, i (username)}
		<Avatar
			{username}
			src={avatars?.[username]}
			class={[FACE, 'ring-2 ring-surface', i > 0 && '-ml-1.5']}
			onpointerenter={(event) => show(event, username)}
			onpointerleave={hide}
		/>
	{/each}
	{#if more > 0}
		<span
			class={[
				FACE,
				'-ml-1.5 inline-grid size-6 flex-none place-items-center rounded-full bg-fill-3 text-[10px] leading-none font-semibold text-label-2 ring-2 ring-surface'
			]}
			aria-hidden="true"
			onpointerenter={(event) => show(event, usernames.slice(max).join(', '))}
			onpointerleave={hide}>+{more}</span
		>
	{/if}
	<span
		bind:this={tip}
		popover="manual"
		class="pointer-events-none inset-auto -translate-x-1/2 -translate-y-[calc(100%+0.5rem)] rounded-[8px] bg-label px-2.5 py-[5px] text-footnote font-semibold whitespace-nowrap text-canvas"
	>
		{text}
	</span>
</span>
