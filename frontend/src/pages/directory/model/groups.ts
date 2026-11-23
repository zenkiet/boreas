import { DEV_STATUSES, DEV_STATUS_LABEL } from '#shared/api';
import type { DevStatus, DirectoryTask, PublicTask } from '#shared/api';
import { byName } from '#shared/lib';
import { isUnassigned, NONE } from './scope.ts';

export type GroupBy = 'status' | 'project' | 'person';

export interface TaskGroup {
	readonly kind: 'status' | 'project' | 'person' | 'none' | 'private';
	readonly key: string;
	readonly label: string;
	readonly tasks: DirectoryTask[];
	readonly status?: DevStatus;
}

export const people = <T extends PublicTask>(tasks: readonly T[]): [string, T[]][] =>
	[...new Set(tasks.flatMap((task) => task.assignees))]
		.sort(byName)
		.map((name): [string, T[]] => [name, tasks.filter((task) => task.assignees.includes(name))]);

const byStatus = (tasks: readonly DirectoryTask[], status: DevStatus) =>
	tasks.filter((task) => task.devStatus === status);

const grouping: Record<
	GroupBy,
	(
		tasks: readonly DirectoryTask[],
		projects: readonly { slug: string; name: string }[]
	) => TaskGroup[]
> = {
	status: (tasks) =>
		DEV_STATUSES.map((status) => ({
			kind: 'status',
			key: status,
			label: DEV_STATUS_LABEL[status],
			tasks: byStatus(tasks, status),
			status
		})),
	project: (tasks, projects) =>
		projects.map(({ slug, name }) => ({
			kind: 'project',
			key: slug,
			label: name,
			tasks: tasks.filter((task) => task.project.slug === slug)
		})),
	person: (tasks) => [
		...people(tasks).map(([name, own]): TaskGroup => ({
			kind: 'person',
			key: name,
			label: name,
			tasks: own
		})),
		{ kind: 'none', key: NONE, label: 'Unassigned', tasks: tasks.filter(isUnassigned) },
		{
			kind: 'private',
			key: 'private',
			label: 'Private',
			tasks: tasks.filter((task) => task.locked)
		}
	]
};

export const groupTasks = (
	tasks: readonly DirectoryTask[],
	by: GroupBy,
	projects: readonly { slug: string; name: string }[]
): TaskGroup[] => grouping[by](tasks, projects).filter((group) => group.tasks.length);

export const columns = (tasks: readonly DirectoryTask[], status: DevStatus | 'all') =>
	DEV_STATUSES.filter((each) => status === 'all' || each === status).map((each) => ({
		status: each,
		tasks: byStatus(tasks, each)
	}));
