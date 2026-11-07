import type { DirectoryTask, PublicProject, TaskStatus } from '#shared/api';

type EnvKind = 'starting' | 'stopped' | 'failed' | 'missing' | 'notfound';

interface EnvCopy {
	readonly code: string;
	readonly title: string;
	readonly body: string;
}

const BY_STATUS: Record<TaskStatus, EnvKind> = {
	creating: 'starting',
	starting: 'starting',
	running: 'starting',
	stopped: 'stopped',
	unknown: 'stopped',
	error: 'failed'
};

const people = new Intl.ListFormat('en-GB');

export const findTask = (
	tasks: readonly DirectoryTask[],
	slug: string,
	name: string
): DirectoryTask | undefined =>
	tasks.find(
		(task) => task.project.slug === slug && task.name.toLowerCase() === name.toLowerCase()
	);

export const kindOf = (
	projects: readonly PublicProject[],
	slug: string,
	task?: DirectoryTask
): EnvKind => {
	if (task) return BY_STATUS[task.status];
	return projects.some((project) => project.slug === slug) ? 'missing' : 'notfound';
};

export const copyOf = (
	kind: EnvKind | 'error',
	slug: string,
	name: string,
	assignees: readonly string[] = []
): EnvCopy => {
	const who = people.format(assignees);
	switch (kind) {
		case 'starting':
			return {
				code: '503 · STARTING',
				title: `${name} is starting`,
				body: 'Booting after a deploy. This page retries on its own.'
			};
		case 'stopped':
			return {
				code: '503 · STOPPED',
				title: `${name} is stopped`,
				body: `Ask ${who || 'a project member'} to start it from the Boreas console.`
			};
		case 'failed':
			return {
				code: '502 · FAILED TO START',
				title: `${name} failed to start`,
				body: `Ask ${who || 'the project owner'} to check the logs.`
			};
		case 'missing':
			return {
				code: '404 · NO SUCH ENVIRONMENT',
				title: 'Nothing is served at this address',
				body: `${slug} has no task called ${name}.`
			};
		case 'notfound':
			return {
				code: '404 · NOT FOUND',
				title: 'This page doesn’t exist',
				body: 'Check the address.'
			};
		default:
			return {
				code: '500 · SERVER ERROR',
				title: 'Boreas couldn’t load this page',
				body: 'Environments keep running.'
			};
	}
};

const distance = (a: string, b: string): number => {
	let previous = Array.from({ length: b.length + 1 }, (_, j) => j);
	for (let i = 1; i <= a.length; i++) {
		const row = [i];
		for (let j = 1; j <= b.length; j++) {
			const swap = previous[j - 1] + Number(a[i - 1] !== b[j - 1]);
			row[j] = Math.min(previous[j] + 1, row[j - 1] + 1, swap);
		}
		previous = row;
	}
	return previous[b.length];
};

export const closestMatches = (
	tasks: readonly DirectoryTask[],
	slug: string,
	name: string,
	limit = 4
): DirectoryTask[] =>
	tasks
		.map((task) => ({
			task,
			elsewhere: Number(task.project.slug !== slug),
			far: distance(name.toLowerCase(), task.name.toLowerCase())
		}))
		.filter(({ far }) => far <= 2)
		.sort((a, b) => a.elsewhere - b.elsewhere || a.far - b.far)
		.slice(0, limit)
		.map(({ task }) => task);
