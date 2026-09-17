import { listProjects } from '@/shared/api';
import type { PageLoad } from './$types';

export const prerender = false;

export const load: PageLoad = ({ params, fetch }) =>
	listProjects(fetch).then((projects) => ({
		slug: params.project,
		task: params.task,
		status:
			projects
				.find((project) => project.slug === params.project)
				?.tasks.find((task) => task.name === params.task)?.status ?? null
	}));
