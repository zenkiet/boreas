<script lang="ts" module>
	import type { TaskStatus } from '@/shared/api';
	import type { IconName } from '@/shared/ui/Icon.svelte';

	interface Copy {
		code: string;
		icon: IconName;
		tone: 'neutral' | 'warning' | 'negative';
		title: (task: string) => string;
		body: string;
	}

	// Running yet unreachable means the route is not registered yet, so keep retrying.
	const RUNTIME: Record<TaskStatus, 'starting' | 'stopped' | 'error'> = {
		creating: 'starting',
		starting: 'starting',
		running: 'starting',
		stopped: 'stopped',
		unknown: 'stopped',
		error: 'error'
	};

	const COPY: Record<'starting' | 'stopped' | 'error', Copy> = {
		starting: {
			code: '503 · STARTING',
			icon: 'clock',
			tone: 'warning',
			title: (task) => `${task} is still starting`,
			body: 'The container was pulled and is booting. This page retries on its own — the environment usually answers within a minute.'
		},
		stopped: {
			code: '503 · STOPPED',
			icon: 'power',
			tone: 'neutral',
			title: (task) => `${task} is stopped`,
			body: 'This environment exists but its container is not running. A project member can start it again from the Boreas console.'
		},
		error: {
			code: '502 · FAILED',
			icon: 'triangle-alert',
			tone: 'negative',
			title: (task) => `${task} failed to start`,
			body: 'The container exited before it could serve traffic. The project owner has the logs and can redeploy it.'
		}
	};

	const RETRY_SECONDS = 10;
</script>

<script lang="ts">
	import { resolve } from '$app/paths';
	import { Icon, StatusPage } from '@/shared/ui';

	let { slug, task, status }: { slug: string; task: string; status: TaskStatus | null } = $props();

	const copy = $derived(status ? COPY[RUNTIME[status]] : null);
	const retrying = $derived(copy?.tone === 'warning');

	let seconds = $state(RETRY_SECONDS);

	$effect(() => {
		if (!retrying) return;
		const timer = setInterval(() => {
			seconds -= 1;
			if (seconds <= 0) location.reload();
		}, 1000);
		return () => clearInterval(timer);
	});
</script>

<svelte:head><title>{task} · {slug} · Boreas</title></svelte:head>

<div class="flex flex-1 items-center justify-center px-4 py-16">
	{#if !copy}
		<StatusPage
			code="404 · NOT FOUND"
			icon="compass"
			title="No environment is served here"
			description="This project hosts no environment with that name. It may have been renamed or deleted — routes disappear as soon as their task is removed."
			path="/{slug}/{task}"
		>
			<a class="btn" href={resolve('/[project]', { project: slug })}>
				<Icon name="layers" /> All environments
			</a>
		</StatusPage>
	{:else}
		<StatusPage
			code={copy.code}
			icon={copy.icon}
			tone={copy.tone}
			title={copy.title(task)}
			description={copy.body}
			path="/{slug}/{task}"
		>
			<div class="flex flex-wrap justify-center gap-2">
				<button type="button" class="btn" onclick={() => location.reload()}>
					<Icon name="refresh-cw" />
					{retrying ? `Retry now · ${seconds}s` : 'Retry'}
				</button>
				<a class="btn btn--ghost" href={resolve('/[project]', { project: slug })}>
					<Icon name="layers" /> All environments
				</a>
			</div>
		</StatusPage>
	{/if}
</div>
