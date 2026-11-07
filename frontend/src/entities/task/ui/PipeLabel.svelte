<script lang="ts">
	import type { PipeKind } from '../model/pipeline.ts';

	let { kind, text, progress }: { kind: PipeKind; text: string; progress?: number } = $props();

	const ARC = 2 * Math.PI * 7.5;

	const TONE: Record<PipeKind, string> = {
		run: 'text-label',
		ok: 'text-label-2',
		bad: 'font-semibold text-blocked',
		cancel: 'text-label-3',
		none: 'text-label-3',
		wait: 'text-label-3'
	};

	const MARK = {
		ok: 'm6.9 10.2 2.1 2.1 4.2-4.5',
		bad: 'm7.6 7.6 4.8 4.8m0-4.8-4.8 4.8',
		cancel: 'm4.7 4.7 10.6 10.6'
	};

	const dash = $derived(`${((ARC * (progress || 25)) / 100).toFixed(1)} ${ARC.toFixed(1)}`);
</script>

<span
	class={['inline-flex min-w-0 items-center gap-1.5 text-footnote whitespace-nowrap', TONE[kind]]}
>
	<svg
		viewBox="0 0 20 20"
		class={['size-4 flex-none', kind === 'ok' && 'text-ready']}
		aria-hidden="true"
	>
		{#if kind === 'run'}
			<circle class="stroke-fill-3" cx="10" cy="10" r="7.5" fill="none" stroke-width="2.6" />
			<g class={{ spin: !progress }}>
				<circle
					class="stroke-accent"
					cx="10"
					cy="10"
					r="7.5"
					fill="none"
					stroke-width="2.6"
					stroke-linecap="round"
					stroke-dasharray={dash}
					transform="rotate(-90 10 10)"
				/>
			</g>
		{:else if kind === 'none' || kind === 'wait'}
			<circle
				cx="10"
				cy="10"
				r="7.6"
				fill="none"
				stroke="currentColor"
				stroke-width="1.8"
				stroke-dasharray="2.4 2.6"
			/>
		{:else}
			<g
				fill="none"
				stroke="currentColor"
				stroke-width="1.8"
				stroke-linecap="round"
				stroke-linejoin="round"
			>
				<circle cx="10" cy="10" r="7.6" />
				<path d={MARK[kind]} />
			</g>
		{/if}
	</svg>
	<span class="truncate">{text}</span>
</span>

<style>
	.spin {
		animation: spin 1.1s linear 4;
		transform-origin: 10px 10px;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
</style>
