<script lang="ts">
	/* eslint-disable svelte/no-navigation-without-resolve -- environment rows are served by the Go proxy. */
	import { resolve } from '$app/paths';
	import {
		DEV_STATUSES,
		DEV_STATUS_LABEL,
		bySeverity,
		devCount,
		type PublicProject,
		type PublicTask
	} from '@/shared/api';
	import { formatDateTime, renderNote } from '@/shared/lib';
	import { EmptyState, Icon, InsetGroup, SearchField, SkeletonRows, StatusPage } from '@/shared/ui';
	import StatusFilter from './StatusFilter.svelte';

	let { slug, projects }: { slug: string; projects: Promise<readonly PublicProject[]> } = $props();

	let query = $state('');
	let filter = $state(0);
	let openNote = $state('');

	const filterItems = (tasks: readonly PublicTask[]) => [
		{ label: 'All', count: tasks.length },
		...DEV_STATUSES.map((status) => ({
			label: DEV_STATUS_LABEL[status],
			count: devCount(tasks, status)
		}))
	];

	const visibleTasks = (tasks: readonly PublicTask[]) => {
		const needle = query.trim().toLowerCase();
		return bySeverity(tasks).filter(
			(task) =>
				(filter === 0 || task.devStatus === DEV_STATUSES[filter - 1]) &&
				`${task.name} ${task.description ?? ''}`.toLowerCase().includes(needle)
		);
	};

	const summary = (tasks: readonly PublicTask[]) =>
		DEV_STATUSES.filter((status) => devCount(tasks, status))
			.map((status) => `${devCount(tasks, status)} ${DEV_STATUS_LABEL[status].toLowerCase()}`)
			.join(' · ') || '0 environments';

	const copyCode = async (event: MouseEvent) => {
		const button = (event.target as Element).closest<HTMLButtonElement>('[data-copy-code]');
		const code = button?.parentElement?.querySelector('code');
		if (!button || !code) return;

		let copied = false;
		try {
			if (navigator.clipboard) {
				await navigator.clipboard.writeText(code.textContent ?? '');
				copied = true;
			}
		} catch {
			copied = false;
		}
		if (!copied) {
			const input = document.createElement('textarea');
			input.value = code.textContent ?? '';
			input.style.position = 'fixed';
			input.style.opacity = '0';
			document.body.append(input);
			input.select();
			try {
				copied = document.execCommand('copy');
			} catch {
				copied = false;
			} finally {
				input.remove();
				button.focus();
			}
		}
		button.title = copied ? 'Copied' : 'Copy failed';
		button.setAttribute('aria-label', button.title);
		button.dataset.copyState = copied ? 'copied' : 'failed';
		setTimeout(() => {
			button.title = 'Copy code';
			button.setAttribute('aria-label', button.title);
			delete button.dataset.copyState;
		}, 2000);
	};
</script>

<svelte:head>
	<title>{slug} · Boreas</title>
	<meta name="description" content="Preview environments served under /{slug}." />
</svelte:head>

{#await projects}
	<div
		class="mx-auto grid w-full max-w-160 grid-cols-[minmax(0,1fr)] content-start gap-4 px-4 py-6 sm:py-10"
	>
		<div class="grid gap-2">
			<span class="h-5 w-24 skeleton"></span>
			<span class="h-9 w-56 skeleton"></span>
		</div>
		<InsetGroup label="Environments">
			<SkeletonRows count={7} label="Loading environments" />
		</InsetGroup>
	</div>
{:then list}
	{@const found = list.find((project) => project.slug === slug)}
	{#if !found}
		<div class="flex flex-1 items-center justify-center px-4 py-16">
			<StatusPage
				code="404 · NOT FOUND"
				icon="compass"
				title="No project is served here"
				description="This server hosts no project under that prefix"
				path="/{slug}/"
			>
				{@const hints = list
					.map((project) => project.slug)
					.filter((other) => other.startsWith(slug.slice(0, 3)))
					.slice(0, 3)}
				<a class="btn" href={resolve('/')}><Icon name="layers" /> Browse projects</a>

				{#if hints.length}
					<div class="grid w-full gap-1.5 text-start">
						<span class="text-meta px-1 font-medium text-tertiary">Did you mean</span>
						<div class="overflow-hidden rounded-lg bg-base">
							{#each hints as hint (hint)}
								<a class="row row-divider relative" href="/{hint}/">
									<span class="row__id min-w-0 flex-1 font-mono">/{hint}</span>
									<Icon name="chevron-right" size={15} class="flex-none text-tertiary" />
								</a>
							{/each}
						</div>
					</div>
				{/if}
			</StatusPage>
		</div>
	{:else}
		{@const visible = visibleTasks(found.tasks)}
		<div
			class="mx-auto grid w-full max-w-160 grid-cols-[minmax(0,1fr)] content-start gap-4 px-4 py-6 sm:py-10"
		>
			<header class="grid gap-1.5">
				<a
					href={resolve('/')}
					class="text-row -ms-1 inline-flex items-center gap-0.5 font-medium text-secondary transition-colors hover:text-primary"
				>
					<Icon name="chevron-left" size={18} /> Projects
				</a>
				<h1
					class="mt-1 text-[clamp(1.75rem,5vw,2.125rem)] font-bold tracking-[-0.022em] text-primary"
				>
					{found.name}
				</h1>
				<p class="text-row-sub font-mono text-tertiary">/{found.slug}</p>
			</header>

			{#if found.tasks.length}
				<StatusFilter
					items={filterItems(found.tasks)}
					bind:active={filter}
					label="Filter environments by status"
				/>
				{#if found.tasks.length > 6}
					<SearchField
						bind:value={query}
						label="Search environments"
						placeholder="Search by task id or description"
					/>
				{/if}
			{/if}

			<InsetGroup label="Environments" trailing={summary(found.tasks)}>
				{#if found.tasks.length === 0}
					<EmptyState
						title="No environments yet"
						description="This project has no task environments running. Create one from the Boreas console and it appears here immediately."
					/>
				{:else if visible.length === 0}
					<EmptyState
						icon="search"
						title="No match"
						description="No environment in this project matches the current filter."
					/>
				{:else}
					{#each visible as task (task.name)}
						{@const open = openNote === task.name}
						<div class="row row-divider relative" class:idle={task.status !== 'running'}>
							<!-- Stretched link: a note button cannot nest inside the row's anchor. -->
							<a
								class="stretched"
								href="/{found.slug}/{task.name}"
								data-sveltekit-reload
								aria-label="Open {task.name}"
							></a>
							<i class="dot dot--{task.devStatus}"></i>
							<span class="min-w-0 flex-1">
								<span class="row__id font-mono">
									{task.name}<span class="sr-only">, {task.status}</span>
								</span>
								<span class="row__sub font-mono">
									{task.description || '—'}{task.status !== 'running' ? ` · ${task.status}` : ''}
								</span>
							</span>
							{#if task.note}
								<button
									type="button"
									class="notebtn"
									class:notebtn--on={open}
									aria-expanded={open}
									onclick={() => (openNote = open ? '' : task.name)}
								>
									<Icon name="note" size={13} />
									Note
									<Icon
										name="chevron-right"
										size={12}
										class="transition-transform {open ? 'rotate-90' : ''}"
									/>
								</button>
							{/if}
							<span class="row__meta tabular">{formatDateTime(task.updatedAt)}</span>
							<Icon name="chevron-right" size={15} class="flex-none text-tertiary" />
						</div>
						{#if open && task.note}
							<div class="note-panel dot--{task.devStatus}">
								<div class="md" onclick={copyCode} role="presentation">
									<!-- eslint-disable-next-line svelte/no-at-html-tags -->
									{@html renderNote(task.note)}
								</div>
							</div>
						{/if}
					{/each}
				{/if}
			</InsetGroup>
		</div>
	{/if}
{/await}

<style>
	.row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		min-block-size: 3.75rem;
		padding: 0.6875rem 1rem;
		text-decoration: none;
		transition: background-color var(--tui-duration);
	}

	.row:hover {
		background: var(--tui-background-neutral-1);
	}

	/* A route that cannot answer must not look as clickable as one that can. */
	.idle {
		opacity: 0.55;
	}

	.row__id,
	.row__sub {
		display: block;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.row__id {
		font-size: 1.0625rem;
		font-weight: 600;
		color: var(--tui-text-primary);
	}

	.row:hover .row__id {
		color: var(--tui-text-action);
	}

	.row__sub {
		font-size: 0.9375rem;
		line-height: 1.5;
		color: var(--tui-text-tertiary);
	}

	.row__meta {
		flex: none;
		font-size: 0.8125rem;
		color: var(--tui-text-tertiary);
		white-space: nowrap;
	}

	@media (max-width: 34rem) {
		.row__meta {
			display: none;
		}
	}

	.stretched {
		position: absolute;
		inset: 0;
	}

	.notebtn {
		position: relative;
		display: inline-flex;
		flex: none;
		align-items: center;
		gap: 0.3125rem;
		block-size: 1.625rem;
		padding-inline: 0.5625rem;
		border: 0;
		border-radius: 999px;
		background: var(--tui-background-neutral-1);
		color: var(--tui-text-secondary);
		font: inherit;
		font-size: 0.75rem;
		font-weight: 500;
		cursor: pointer;
		transition:
			background-color var(--tui-duration),
			color var(--tui-duration);
	}

	.notebtn:hover {
		background: var(--tui-background-neutral-2);
		color: var(--tui-text-primary);
	}

	.notebtn--on,
	.notebtn--on:hover {
		background: var(--tui-background-accent-1);
		color: var(--tui-text-on-accent);
	}

	.note-panel {
		position: relative;
		padding: 0.125rem 1rem 1rem 2rem;
	}

	.note-panel::after {
		content: '';
		position: absolute;
		inset-block: 0.25rem 1rem;
		inset-inline-start: 1.125rem;
		inline-size: 2px;
		border-radius: 999px;
		background: var(--dot-color, var(--tui-status-neutral));
		opacity: 0.55;
	}

	.md {
		font-size: 0.875rem;
		line-height: 1.6;
		color: var(--tui-text-secondary);

		:global {
			> :first-child {
				margin-block-start: 0;
			}

			> :last-child {
				margin-block-end: 0;
			}

			h1,
			h2,
			h3,
			h4,
			h5,
			h6 {
				margin: 0.875rem 0 0.375rem;
				font-size: 0.9375rem;
				font-weight: 600;
				letter-spacing: -0.01em;
				color: var(--tui-text-primary);
			}

			p {
				margin: 0 0 0.5rem;
			}

			strong {
				font-weight: 600;
				color: var(--tui-text-primary);
			}

			a {
				color: var(--tui-text-action);
				text-decoration: underline;
				text-underline-offset: 2px;
			}

			code {
				padding: 1px 5px;
				border-radius: 5px;
				background: var(--tui-background-neutral-2);
				font-family: var(--app-font-mono);
				font-size: 0.75rem;
				color: var(--tui-text-primary);
			}

			.code-block {
				position: relative;
				margin: 0.75rem 0;
			}

			.code-block pre {
				overflow-x: auto;
				white-space: pre-wrap;
				overflow-wrap: anywhere;
				margin: 0;
				padding: 1rem 3rem 1rem 1rem;
				border-radius: 0.5rem;
				background: var(--tui-background-neutral-1);
			}

			.code-block code {
				padding: 0;
				background: none;
				font-size: 0.8125rem;
				line-height: 1.5;
			}

			.code-block button {
				position: absolute;
				inset-block-start: 0.5rem;
				inset-inline-end: 0.5rem;
				display: grid;
				place-items: center;
				width: 1.75rem;
				height: 1.75rem;
				padding: 0;
				border: 1px solid var(--tui-border-normal);
				border-radius: 0.375rem;
				background: var(--tui-background-base);
				color: var(--tui-text-secondary);
				cursor: pointer;
				transition:
					background-color var(--tui-duration),
					color var(--tui-duration);
			}

			.code-block button:hover,
			.code-block button:focus-visible {
				background: var(--tui-background-neutral-2);
				color: var(--tui-text-primary);
			}

			.code-block button:focus-visible {
				outline: 2px solid var(--tui-text-action);
				outline-offset: 2px;
			}

			.code-block button [data-copy-check],
			.code-block button[data-copy-state='copied'] [data-copy-icon] {
				display: none;
			}

			.code-block button[data-copy-state='copied'] [data-copy-check] {
				display: block;
			}

			.code-block button[data-copy-state='copied'] {
				color: var(--tui-status-positive);
			}

			.code-block button[data-copy-state='failed'] {
				color: var(--tui-status-negative);
			}

			.hljs-keyword,
			.hljs-literal {
				color: var(--tui-text-action);
			}

			.hljs-string {
				color: var(--tui-status-positive);
			}

			.hljs-number,
			.hljs-built_in {
				color: var(--tui-status-warning);
			}

			.hljs-comment {
				color: var(--tui-text-secondary);
			}

			/* Tailwind's preflight strips list markers, and a numbered test step needs its number. */
			ul,
			ol {
				display: grid;
				gap: 0.1875rem;
				margin: 0 0 0.5rem;
				padding-inline-start: 1.375rem;
				list-style-position: outside;
			}

			ul {
				list-style-type: disc;
			}

			ol {
				list-style-type: decimal;
			}

			li::marker {
				color: var(--tui-text-tertiary);
			}

			blockquote {
				margin: 0.625rem 0;
				padding: 0.4375rem 0.75rem;
				border-radius: 0.5rem;
				background: var(--tui-border-normal);
				color: var(--tui-text-primary);
			}

			blockquote p {
				margin: 0;
			}

			hr {
				margin: 0.75rem 0;
				border: 0;
				border-block-start: 1px solid var(--tui-border-normal);
			}
		}
	}
</style>
