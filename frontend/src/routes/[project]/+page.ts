import { listProjects } from '@/shared/api';
import type { PageLoad } from './$types';

export const prerender = false;

export const load: PageLoad = ({ params, fetch }) => ({
	slug: params.project,
	projects: listProjects(fetch)
});
