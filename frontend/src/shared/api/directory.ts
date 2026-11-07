export const DEV_STATUSES = ['blocked', 'in_progress', 'ready'] as const;

export type DevStatus = (typeof DEV_STATUSES)[number];

export const DEV_STATUS_LABEL: Record<DevStatus, string> = {
	blocked: 'Blocked',
	in_progress: 'In progress',
	ready: 'Ready'
};

export type TaskStatus = 'creating' | 'starting' | 'running' | 'stopped' | 'error' | 'unknown';

export type BuildState = 'running' | 'success' | 'failure' | 'canceled';

export interface Build {
	readonly state: BuildState;
	readonly stage?: string;
	readonly progress?: number;
	readonly at: string;
}

export interface Deploy {
	readonly status: 'success' | 'failure';
	readonly at: string;
}

export interface Revision {
	readonly commit: string;
	readonly previous?: string;
}

export interface PublicTask {
	readonly name: string;
	readonly description?: string;
	readonly note?: string;
	readonly status: TaskStatus;
	readonly devStatus: DevStatus;
	readonly updatedAt: string;
	readonly assignees: readonly string[];
	readonly build?: Build;
	readonly lastDeploy?: Deploy;
	readonly revision?: Revision;
	readonly formFactor: 'web' | 'mobile';
	readonly locked: boolean;
}

export interface PublicProject {
	readonly slug: string;
	readonly name: string;
	readonly tasks: readonly PublicTask[];
}

export type DirectoryTask = PublicTask & { readonly project: { slug: string; name: string } };

interface TaskPayload {
	name: string;
	description?: string;
	note?: string;
	status: TaskStatus;
	dev_status: DevStatus;
	updated_at: string;
	assignees?: string[];
	build?: Build;
	last_deploy?: { status: string; at: string };
	revision?: Revision;
	form_factor: 'web' | 'mobile';
	has_password: boolean;
}

interface DirectoryPayload {
	projects: { slug: string; name: string; tasks: TaskPayload[] }[];
}

const deployOf = (deploy: TaskPayload['last_deploy']): Deploy | undefined =>
	deploy?.status === 'success' || deploy?.status === 'failure'
		? { status: deploy.status, at: deploy.at }
		: undefined;

const toTask = (task: TaskPayload): PublicTask => ({
	name: task.name,
	description: task.description,
	note: task.has_password ? undefined : task.note,
	status: task.status,
	devStatus: task.dev_status,
	updatedAt: task.updated_at,
	assignees: task.has_password ? [] : (task.assignees ?? []),
	build: task.build,
	lastDeploy: deployOf(task.last_deploy),
	revision: task.revision,
	formFactor: task.form_factor,
	locked: task.has_password
});

export const listProjects = (load: typeof fetch = fetch): Promise<readonly PublicProject[]> =>
	load('/api/v1/public/projects')
		.catch(() => {
			throw new Error('The Boreas API didn’t answer.');
		})
		.then((response) => {
			if (!response.ok) throw new Error(`The Boreas API didn’t answer (${response.status}).`);
			return response.json() as Promise<DirectoryPayload>;
		})
		.then(({ projects }) =>
			projects.map((project) => ({
				slug: project.slug,
				name: project.name,
				tasks: project.tasks.map(toTask)
			}))
		);

export const flattenTasks = (projects: readonly PublicProject[]): DirectoryTask[] =>
	projects.flatMap(({ slug, name, tasks }) =>
		tasks.map((task) => ({ ...task, project: { slug, name } }))
	);

export const bySeverity = <T extends PublicTask>(tasks: readonly T[]): T[] =>
	[...tasks].sort(
		(a, b) =>
			DEV_STATUSES.indexOf(a.devStatus) - DEV_STATUSES.indexOf(b.devStatus) ||
			Date.parse(b.updatedAt) - Date.parse(a.updatedAt)
	);

export const devCount = (tasks: readonly PublicTask[], status: DevStatus): number =>
	tasks.filter((task) => task.devStatus === status).length;

export const taskKey = (task: DirectoryTask): string => `${task.project.slug}/${task.name}`;

export const appUrl = (task: DirectoryTask): string => `${location.origin}/${taskKey(task)}/`;
