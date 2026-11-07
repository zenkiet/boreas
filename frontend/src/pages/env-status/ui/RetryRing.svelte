<script lang="ts">
	let { onretry }: { onretry: () => void } = $props();

	const RING = 2 * Math.PI * 7;

	let seconds = $state(10);

	$effect(() => {
		const timer = setInterval(() => {
			seconds = seconds > 1 ? seconds - 1 : 10;
			if (seconds === 10) onretry();
		}, 1000);
		return () => clearInterval(timer);
	});
</script>

<button type="button" class="btn" onclick={onretry}>
	<svg viewBox="0 0 20 20" class="size-[18px] flex-none" aria-hidden="true">
		<circle class="stroke-current/30" cx="10" cy="10" r="7" fill="none" stroke-width="2.4" />
		<circle
			class="stroke-current"
			cx="10"
			cy="10"
			r="7"
			fill="none"
			stroke-width="2.4"
			stroke-linecap="round"
			stroke-dasharray="{((RING * seconds) / 10).toFixed(1)} {RING.toFixed(1)}"
			transform="rotate(-90 10 10)"
		/>
	</svg>
	Retry now · {seconds}s
</button>
