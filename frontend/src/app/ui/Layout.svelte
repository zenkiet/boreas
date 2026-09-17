<script lang="ts">
	import type { Snippet } from 'svelte';
	import { resolve } from '$app/paths';
	import { Icon } from '@/shared/ui';
	import favicon from '../favicon.svg';
	import '../styles/app.css';
	import { theme } from '../model/theme.svelte';

	let { children }: { children: Snippet } = $props();

	const ICON = { system: 'monitor', light: 'sun', dark: 'moon' } as const;

	$effect(() => {
		const query = matchMedia('(prefers-color-scheme: dark)');
		theme.prefersDark = query.matches;
		const onChange = (event: MediaQueryListEvent) => (theme.prefersDark = event.matches);
		query.addEventListener('change', onChange);
		return () => query.removeEventListener('change', onChange);
	});

	$effect(() => {
		document.documentElement.dataset.theme = theme.resolved;
	});
</script>

<svelte:head><link rel="icon" href={favicon} /></svelte:head>

<div class="flex min-h-dvh flex-col bg-canvas">
	<header class="bar">
		<div class="mx-auto flex h-13 max-w-7xl items-center gap-3 px-4 sm:px-6">
			<a
				href={resolve('/')}
				class="inline-flex items-center gap-2 text-primary no-underline"
				aria-label="Boreas"
			>
				<img src={favicon} alt="" class="size-7" />
				<span class="text-lg font-semibold">Boreas</span>
			</a>
			<button
				type="button"
				class="toggle ms-auto"
				title="Theme: {theme.mode}"
				aria-label="Theme: {theme.mode}"
				onclick={() => theme.cycle()}
			>
				<Icon name={ICON[theme.mode]} size={17} />
			</button>
		</div>
	</header>

	<main class="flex flex-1 flex-col">{@render children()}</main>

	<footer
		class="mx-auto flex w-full max-w-160 flex-wrap items-center justify-center gap-x-4 gap-y-1 px-5 pt-2 pb-6 text-sm text-tertiary"
	>
		<span>
			© 2026 <a
				href="https://github.com/zenkiet"
				class="text-secondary hover:text-primary"
				target="_blank">ZenSoftware</a
			> All rights reserved.</span
		>
	</footer>
</div>

<style>
	.bar {
		position: sticky;
		z-index: 9;
		inset-block-start: 0;
		border-block-end: 1px solid var(--tui-border-normal);
		background: color-mix(in srgb, var(--tui-background-base) 82%, transparent);
		-webkit-backdrop-filter: blur(12px) saturate(180%);
		backdrop-filter: blur(12px) saturate(180%);
	}

	.toggle {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		inline-size: 2rem;
		block-size: 2rem;
		border: 0;
		border-radius: 999px;
		padding: 0;
		background: none;
		color: var(--tui-text-secondary);
		cursor: pointer;
		transition:
			background-color var(--tui-duration),
			color var(--tui-duration);
	}

	.toggle:hover {
		background: var(--tui-background-neutral-1);
		color: var(--tui-text-primary);
	}
</style>
