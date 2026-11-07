import type { DirectoryTask, PublicProject, PublicTask } from '#shared/api';
import { plural } from '#shared/lib';

export const NONE = 'none';

export interface ScopeFilter {
	readonly project: string;
	readonly person: string;
}

export type Heading =
	| { kind: 'all' | 'none' | 'person'; title: string; sub: string }
	| { kind: 'project'; title: string; slug: string; sub: string };

export const isUnassigned = (task: PublicTask): boolean => !task.locked && !task.assignees.length;

export const inScope = (task: DirectoryTask, { project, person }: ScopeFilter): boolean =>
	(!project || task.project.slug === project) &&
	(!person || (person === NONE ? isUnassigned(task) : task.assignees.includes(person)));

export const headingOf = (
	tasks: readonly DirectoryTask[],
	projects: readonly PublicProject[],
	{ project, person }: ScopeFilter
): Heading => {
	const found = projects.find(({ slug }) => slug === project);
	if (found) {
		const own = tasks.filter((task) => task.project.slug === found.slug);
		const running = own.filter((task) => task.status === 'running').length;
		const sub = `${plural(own.length, 'task')} · ${running} running`;
		return { kind: 'project', title: found.name, slug: found.slug, sub };
	}
	if (person === NONE) {
		const sub = plural(tasks.filter(isUnassigned).length, 'task');
		return { kind: 'none', title: 'Unassigned', sub };
	}
	if (person) {
		const own = tasks.filter((task) => inScope(task, { project: '', person }));
		const count = new Set(own.map((task) => task.project.slug)).size;
		const sub = `${plural(own.length, 'task')} in ${plural(count, 'project')}`;
		return { kind: 'person', title: person, sub };
	}
	const sub = tasks.length
		? `${plural(tasks.length, 'task')} · ${plural(projects.length, 'project')}`
		: '';
	return { kind: 'all', title: 'All tasks', sub };
};

export const suggest = (slug: string, projects: readonly PublicProject[]): PublicProject[] =>
	projects
		.filter(
			(project) =>
				project.slug.slice(0, 3) === slug.slice(0, 3) || project.slug.includes(slug.slice(0, 4))
		)
		.slice(0, 3);
