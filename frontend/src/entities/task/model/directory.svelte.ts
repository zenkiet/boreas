import { flattenTasks, listProjects } from '#shared/api';
import type { PublicProject } from '#shared/api';

const STREAM = '/api/v1/public/events/stream';
const GAP = 2000;
const REOPEN = 10_000;
const TICK = 30_000;

class Directory {
	projects = $state.raw<readonly PublicProject[]>([]);
	status = $state<'loading' | 'ready' | 'error'>('loading');
	error = $state<string>();
	now = $state(Date.now());
	tasks = $derived(flattenTasks(this.projects));

	#timer: ReturnType<typeof setTimeout> | undefined;
	#inflight = false;
	#again = false;
	#last = 0;

	refresh() {
		if (this.#inflight) this.#again = true;
		if (this.#inflight || this.#timer !== undefined) return;
		const wait = Math.max(0, this.#last + GAP - Date.now());
		this.#timer = setTimeout(() => {
			this.#timer = undefined;
			this.#load();
		}, wait);
	}

	#load() {
		this.#inflight = true;
		this.#last = Date.now();
		if (this.status === 'error') this.status = 'loading';
		listProjects()
			.then(
				(projects) => {
					this.projects = projects;
					this.status = 'ready';
					this.error = undefined;
					this.now = Date.now();
				},
				(error: Error) => {
					this.error = error.message;
					if (this.status === 'loading') this.status = 'error';
				}
			)
			.finally(() => {
				this.#inflight = false;
				if (!this.#again) return;
				this.#again = false;
				this.refresh();
			});
	}

	live(): () => void {
		let source: EventSource | undefined;
		let reopen: ReturnType<typeof setTimeout> | undefined;
		const open = () => {
			source = new EventSource(STREAM);
			source.onmessage = () => this.refresh();
			source.onerror = () => {
				if (source?.readyState === EventSource.CLOSED) reopen = setTimeout(open, REOPEN);
			};
		};
		open();
		this.refresh();
		const tick = setInterval(() => (this.now = Date.now()), TICK);
		return () => {
			source?.close();
			clearTimeout(reopen);
			clearInterval(tick);
			clearTimeout(this.#timer);
			this.#timer = undefined;
			this.#again = false;
		};
	}
}

export const directory = new Directory();
