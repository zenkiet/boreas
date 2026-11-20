<script lang="ts">
	import type { PipeKind } from '../model/pipeline.ts';

	let { kind, text, progress }: { kind: PipeKind; text: string; progress?: number } = $props();

	const label = $props.id();

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
</script>

<span
	class={['inline-flex min-w-0 items-center gap-1.5 text-footnote whitespace-nowrap', TONE[kind]]}
>
	{#if kind === 'run'}
		<progress
			class="m-px size-3.5 flex-none"
			value={progress || undefined}
			max="100"
			aria-labelledby={label}
			style:--value={progress || undefined}
		></progress>
	{:else}
		<svg
			viewBox="0 0 20 20"
			class={['size-4 flex-none', kind === 'ok' && 'text-ready']}
			aria-hidden="true"
		>
			{#if kind === 'none' || kind === 'wait'}
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
	{/if}
	<span class="truncate">
		<span id={label}>{text}</span>{#if progress}<span aria-hidden="true">
				{` · ${progress}%`}
			</span>{/if}
	</span>
</span>

<style>
	progress {
		--fill: var(--app-accent);
		--track: var(--app-fill-3);

		appearance: none;
		border: 2px solid transparent;
		border-radius: 50%;
		background: conic-gradient(var(--fill) calc(var(--value) * 1%), var(--track) 0) border-box;
		background-clip: border-area;

		&::-webkit-progress-bar {
			background: none;
		}

		&::-webkit-progress-value {
			background: none;
		}

		&::-moz-progress-bar {
			background: none;
		}

		@supports not (background-clip: border-area) {
			border: 0;
			mask-image: radial-gradient(transparent calc(50% - 2px), black calc(50% - 1.5px));
		}

		@media (forced-colors: active) {
			forced-color-adjust: none;
			--fill: CanvasText;
			--track: Canvas;
		}
	}

	progress:indeterminate {
		--value: 25;

		animation: spin 1.1s linear 4;

		@media (prefers-reduced-motion: reduce) {
			animation-duration: 4.4s;
			animation-iteration-count: 1;
		}
	}

	@keyframes spin {
		to {
			rotate: 1turn;
		}
	}
</style>
