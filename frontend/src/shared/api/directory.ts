/** Ordered worst first, so indexOf doubles as the severity sort key. */
export const DEV_STATUSES = ['blocked', 'in_progress', 'ready'] as const;

export type DevStatus = (typeof DEV_STATUSES)[number];

export const DEV_STATUS_LABEL: Record<DevStatus, string> = {
	blocked: 'Blocked',
	in_progress: 'In progress',
	ready: 'Ready'
};

export type TaskStatus = 'creating' | 'starting' | 'running' | 'stopped' | 'error' | 'unknown';

/** Image, port, env and container ids stay in the authed API: this page is anonymous. */
export interface PublicTask {
	readonly name: string;
	readonly description?: string;
	readonly status: TaskStatus;
	readonly devStatus: DevStatus;
	readonly updatedAt: string;
}

export interface PublicProject {
	readonly slug: string;
	readonly name: string;
	readonly tasks: readonly PublicTask[];
}

export const bySeverity = (tasks: readonly PublicTask[]): readonly PublicTask[] =>
	[...tasks].sort((a, b) => DEV_STATUSES.indexOf(a.devStatus) - DEV_STATUSES.indexOf(b.devStatus));

export const devCount = (tasks: readonly PublicTask[], status: DevStatus): number =>
	tasks.filter((task) => task.devStatus === status).length;

export const runningCount = (tasks: readonly PublicTask[]): number =>
	tasks.filter((task) => task.status === 'running').length;

interface TaskPayload {
	name: string;
	description?: string;
	status: TaskStatus;
	dev_status: DevStatus;
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
					devStatus: task.dev_status,
					updatedAt: task.updated_at
				}))
			}))
		);
