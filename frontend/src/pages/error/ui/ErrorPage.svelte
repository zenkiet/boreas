<script lang="ts">
	import Compass from '@lucide/svelte/icons/compass';
	import List from '@lucide/svelte/icons/list';
	import RefreshCw from '@lucide/svelte/icons/refresh-cw';
	import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { StatusPage } from '#shared/ui';

	const title = $derived(
		page.status === 404 ? 'This page doesn’t exist' : 'Boreas couldn’t load this page'
	);
</script>

<svelte:head><title>{title} · Boreas</title></svelte:head>

{#if page.status === 404}
	<StatusPage
		icon={Compass}
		code="404 · NOT FOUND"
		{title}
		body="Check the address."
		path={page.url.pathname}
	>
		{#snippet actions()}
			<a class="btn" href={resolve('/(directory)')}><List size={16} />Browse all tasks</a>
		{/snippet}
	</StatusPage>
{:else}
	<StatusPage
		icon={TriangleAlert}
		tone="bad"
		code="{page.status} · SERVER ERROR"
		{title}
		body="Environments keep running."
		path={page.url.pathname}
	>
		{#snippet actions()}
			<button type="button" class="btn" onclick={() => location.reload()}>
				<RefreshCw size={16} />Try again
			</button>
			<a class="btn btn-ghost" href={resolve('/(directory)')}><List size={16} />Browse all tasks</a>
		{/snippet}
	</StatusPage>
{/if}
