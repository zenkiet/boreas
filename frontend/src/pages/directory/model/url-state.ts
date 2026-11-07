import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { page } from '$app/state';
import { DEV_STATUSES } from '#shared/api';
import type { DevStatus } from '#shared/api';
import type { GroupBy } from './groups.ts';

type Patch = Partial<Record<'person' | 'q' | 'status' | 'group' | 'view' | 'task', string>>;

const read = (key: keyof Patch) => page.url.searchParams.get(key) ?? '';

const oneOf = <T extends string>(value: string, options: readonly T[], fallback: T): T =>
	options.find((option) => option === value) ?? fallback;

export const urlState = {
	get project() {
		return page.params.project ?? '';
	},
	get person() {
		return read('person');
	},
	get q() {
		return read('q');
	},
	get status(): DevStatus | 'all' {
		return oneOf(read('status'), DEV_STATUSES, 'all');
	},
	get group(): GroupBy {
		return oneOf<GroupBy>(read('group'), ['project', 'person'], 'status');
	},
	get view() {
		return oneOf(read('view'), ['board'], 'list');
	},
	get task() {
		return read('task');
	}
};

const pathOf = (patch: Patch, project: string) => {
	const merged = { ...Object.fromEntries(page.url.searchParams), ...patch };
	const search = new URLSearchParams(
		Object.entries(merged).filter((entry): entry is [string, string] => !!entry[1])
	).toString();
	return `${encodeURIComponent(project)}${search ? `?${search}` : ''}`;
};

const go = (patch: Patch, project: string, options: Parameters<typeof goto>[1]) =>
	goto(resolve(pathOf(patch, project)), options);

export const hrefWith = (patch: Patch, project = urlState.project) =>
	resolve(pathOf(patch, project));

export const replaceWith = (patch: Patch) =>
	go(patch, urlState.project, { replace: true, reset: false });

export const showProject = (project: string, options?: { reset: boolean }) =>
	go({ person: '' }, project, options);

let pushed = '';

export const openTask = (key: string) => {
	go({ task: key }, urlState.project, { reset: false }).then(() => {
		pushed = page.url.href;
	});
};

export const closeTask = () => {
	if (pushed !== page.url.href) return replaceWith({ task: '' });
	pushed = '';
	history.back();
};

const VIEW = ['person', 'group', 'view'];

type Hop = { url: URL } | null;

export const switchesView = ({ from, to }: { from: Hop; to: Hop }) =>
	!!from &&
	!!to &&
	!to.url.searchParams.has('task') &&
	(from.url.pathname !== to.url.pathname ||
		VIEW.some((key) => from.url.searchParams.get(key) !== to.url.searchParams.get(key)));
