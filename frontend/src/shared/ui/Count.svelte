<script lang="ts">
	import { cubicOut } from 'svelte/easing';
	import { prefersReducedMotion, Tween } from 'svelte/motion';
	import { T } from './motion.ts';

	let { value }: { value: number } = $props();

	const shown = Tween.of(() => value, {
		duration: () => (prefersReducedMotion.current ? 0 : T.layout),
		easing: cubicOut
	});
</script>

{Math.round(shown.current)}
