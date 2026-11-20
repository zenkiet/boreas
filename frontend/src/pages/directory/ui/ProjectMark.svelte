<script lang="ts">
	import { monogram } from '../model/monogram.ts';

	let { name, large = false }: { name: string; large?: boolean } = $props();
</script>

<span
	class={[
		'inline-grid flex-none place-items-center bg-fill-2 leading-none font-bold tracking-[0.02em] text-label-2',
		large ? 'size-10 rounded-nest text-[0.9375rem]' : 'size-5 rounded-label text-[0.59375rem]'
	]}
	aria-hidden="true"
>
	{monogram(name)}
</span>
