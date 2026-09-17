import { listProjects } from '@/shared/api';
import type { PageLoad } from './$types';

export const load: PageLoad = ({ fetch }) => ({ projects: listProjects(fetch) });
