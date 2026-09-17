<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { Icon, StatusPage } from '@/shared/ui';

	const notFound = $derived(page.status === 404);
</script>

<svelte:head><title>{page.status} · Boreas</title></svelte:head>

<div class="flex flex-1 items-center justify-center px-4 py-16">
	<StatusPage
		code={notFound ? '404 · NOT FOUND' : `${page.status} · ERROR`}
		icon={notFound ? 'compass' : 'triangle-alert'}
		tone={notFound ? 'neutral' : 'negative'}
		title={notFound ? 'Nothing is served at this path' : 'Boreas could not serve this page'}
		description={notFound
			? 'The project or environment in this URL does not exist, or it was renamed or deleted. Routes disappear as soon as their task is removed.'
			: (page.error?.message ??
				'Something went wrong on the way to this environment. The environments themselves are unaffected.')}
		path={page.url.pathname}
	>
		<div class="flex flex-wrap justify-center gap-2">
			{#if !notFound}
				<button type="button" class="btn" onclick={() => location.reload()}>
					<Icon name="refresh-cw" /> Try again
				</button>
			{/if}
			<a class="btn" class:btn--ghost={!notFound} href={resolve('/')}>
				<Icon name="layers" /> Browse projects
			</a>
		</div>
	</StatusPage>
</div>
