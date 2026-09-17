/** Ordered worst first, so indexOf doubles as the severity sort key. */
const TASK_STATUSES = ['error', 'unknown', 'stopped', 'creating', 'starting', 'running'] as const;

export type TaskStatus = (typeof TASK_STATUSES)[number];

export const TASK_GROUPS = ['failed', 'stopped', 'running'] as const;

type TaskGroup = (typeof TASK_GROUPS)[number];

export const TASK_GROUP_LABEL: Record<TaskGroup, string> = {
	failed: 'Failed',
	stopped: 'Stopped',
	running: 'Running'
};

/** A visitor only cares whether the URL answers, so the six runtime states collapse to three. */
const GROUP: Record<TaskStatus, TaskGroup> = {
	error: 'failed',
	unknown: 'stopped',
	stopped: 'stopped',
	creating: 'stopped',
	starting: 'stopped',
	running: 'running'
};

/** Image, port, env and container ids stay in the authed API: this page is anonymous. */
export interface PublicTask {
	readonly name: string;
	readonly description?: string;
	readonly status: TaskStatus;
	readonly updatedAt: string;
}

export interface PublicProject {
	readonly slug: string;
	readonly name: string;
	readonly tasks: readonly PublicTask[];
}

export const groupOf = (task: PublicTask): TaskGroup => GROUP[task.status];

export const groupCount = (tasks: readonly PublicTask[], group: TaskGroup): number =>
	tasks.filter((task) => GROUP[task.status] === group).length;

export const bySeverity = (tasks: readonly PublicTask[]): readonly PublicTask[] =>
	[...tasks].sort((a, b) => TASK_STATUSES.indexOf(a.status) - TASK_STATUSES.indexOf(b.status));

interface TaskPayload {
	name: string;
	description?: string;
	status: TaskStatus;
	updated_at: string;
}

interface DirectoryPayload {
	projects: { slug: string; name: string; tasks: TaskPayload[] }[];
}

export const listProjects = (load: typeof fetch = fetch): Promise<readonly PublicProject[]> =>
	load('/api/v1/public/projects')
		.then((response) => {
			if (!response.ok) throw new Error(`The directory did not respond (${response.status}).`);
			return response.json() as Promise<DirectoryPayload>;
		})
		.then(({ projects }) =>
			projects.map((project) => ({
				slug: project.slug,
				name: project.name,
				tasks: project.tasks.map((task) => ({
					name: task.name,
					description: task.description,
					status: task.status,
					updatedAt: task.updated_at
				}))
			}))
		);
