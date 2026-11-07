<script lang="ts">
	import { Skeleton } from '#shared/ui';
	import { AREA, CARD, HEAD, HEAD_SIDE, HEAD_TEXT, ROW, ROWS } from './layout.ts';

	const CHIPS = [58, 96, 118, 82];
	const LINES = [52, 70, 64, 45, 78, 58, 66, 49, 72].map((desc, i) => ({
		id: 64 + ((i * 17) % 40),
		desc,
		proj: 84 + ((i * 23) % 50),
		pipe: 70 + ((i * 29) % 60)
	}));
</script>

<div role="status" aria-label="Loading the directory">
	<div class={HEAD}>
		<div class={HEAD_TEXT}>
			<Skeleton width="180px" height="30px" />
			<Skeleton class="mt-3" width="240px" height="14px" />
		</div>
		<div class={HEAD_SIDE}><Skeleton class="rounded-full" height="36px" /></div>
	</div>
	<div class="mb-4 flex flex-wrap items-center gap-x-3 gap-y-2.5 @max-[720px]/main:gap-2.5">
		{#each CHIPS as width (width)}
			<Skeleton class="rounded-full" width="{width}px" height="32px" />
		{/each}
	</div>
	<div class="flex min-h-7 items-center px-1 pb-2"><Skeleton width="110px" height="12px" /></div>
	<div class={[CARD, ROWS.project]}>
		{#each LINES as line, i (i)}
			<div class={ROW}>
				<Skeleton class={['size-4 rounded-full', AREA.glyph]} />
				<Skeleton class={AREA.id} width="{line.id}px" height="12px" />
				<Skeleton class={AREA.desc} width="{line.desc}%" height="12px" />
				<Skeleton class={AREA.proj} width="{line.proj}px" height="20px" />
				<Skeleton class={['rounded-full', AREA.people]} width="44px" height="24px" />
				<Skeleton class={AREA.pipe} width="{line.pipe}px" height="12px" />
				<Skeleton class={['justify-self-end', AREA.time]} width="24px" height="12px" />
				<span class={AREA.open}></span>
			</div>
		{/each}
	</div>
</div>
