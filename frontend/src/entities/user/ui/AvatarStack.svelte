<script lang="ts">
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
		'peer relative translate-0 scale-100 transition-[translate,scale] duration-120 ease-out hover:z-1 motion-safe:hover:-translate-y-0.5 motion-safe:hover:scale-115 motion-safe:peer-hover:translate-x-1.5';

	const uid = $props.id();
	const more = $derived(usernames.length - max);
	let tip: HTMLElement;
	let text = $state('');

	const show = (event: PointerEvent, name: string, anchor: string) => {
		if (event.pointerType !== 'mouse' || !('showPopover' in tip)) return;
		text = name;
		if (CSS.supports('position-area: block-start'))
			tip.style.setProperty('position-anchor', anchor);
		else {
			const face = (event.currentTarget as Element).getBoundingClientRect();
			tip.style.left = `${face.left + face.width / 2}px`;
			tip.style.top = `${face.top}px`;
		}
		tip.showPopover();
	};

	const hide = () => tip.hidePopover?.();
</script>

<span class="inline-flex items-center" aria-hidden="true" onpointerdown={hide} onwheel={hide}>
	{#each usernames.slice(0, max) as username, i (username)}
		<Avatar
			{username}
			src={avatars?.[username]}
			class={[FACE, 'ring-2 ring-surface', i > 0 && '-ms-1.5']}
			style="anchor-name: --{uid}-{i}"
			onpointerenter={(event) => show(event, username, `--${uid}-${i}`)}
			onpointerleave={hide}
		/>
	{/each}
	{#if more > 0}
		<span
			class={[
				FACE,
				'-ms-1.5 inline-grid size-6 flex-none place-items-center rounded-full bg-fill-3 text-[0.625rem] leading-none font-semibold text-label-2 ring-2 ring-surface'
			]}
			style="anchor-name: --{uid}-more"
			aria-hidden="true"
			onpointerenter={(event) => show(event, usernames.slice(max).join(', '), `--${uid}-more`)}
			onpointerleave={hide}
		>
			+{more}
		</span>
	{/if}
	<span
		bind:this={tip}
		popover="hint"
		class="tip pointer-events-none rounded-[8px] bg-label px-2.5 py-[5px] text-footnote font-semibold whitespace-nowrap text-canvas"
	>
		{text}
	</span>
</span>

<style>
	.tip {
		inset: auto;
		translate: -50% calc(-100% - 0.5rem);
		transition-property: opacity, transform;
		transition-duration: 120ms;
		transition-timing-function: var(--ease-out);

		@supports (position-area: block-start) {
			margin: 0 0 0.5rem;
			translate: none;
			position-area: block-start;
			position-try-fallbacks: flip-block;
		}
	}

	.tip:popover-open {
		@starting-style {
			opacity: 0;

			@media (prefers-reduced-motion: no-preference) {
				transform: translateY(0.25rem);
			}
		}
	}
</style>
