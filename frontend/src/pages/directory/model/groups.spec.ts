import assert from 'node:assert/strict';
import { test } from 'node:test';
import type { DevStatus, DirectoryTask } from '#shared/api';
import { breakdown, columns, groupTasks, people } from './groups.ts';

const PROJECTS = [
	{ slug: 'blogic-view', name: 'Blogic View' },
	{ slug: 'kiosk', name: 'Kiosk' },
	{ slug: 'portal', name: 'Pos Portal' }
];

const task = (
	name: string,
	devStatus: DevStatus,
	minutesAgo: number,
	assignees: string[] = [],
	more: Partial<DirectoryTask> = {}
): DirectoryTask => ({
	name,
	status: 'running',
	devStatus,
	updatedAt: new Date(Date.UTC(2026, 10, 7, 14, 30) - minutesAgo * 60_000).toISOString(),
	assignees,
	formFactor: 'web',
	locked: false,
	project: PROJECTS[0],
	...more
});

const TASKS = [
	task('wo-718', 'ready', 1490, ['qui.nguyen']),
	task('kiosk-403', 'blocked', 190, ['tuan.pham', 'hoang.vu'], { project: PROJECTS[1] }),
	task('bview-21', 'in_progress', 6, ['ty.go', 'hoang.vu']),
	task('myl-1976', 'ready', 290, ['ty.go']),
	task('myl-2101', 'in_progress', 3, ['hoang.vu']),
	task('staging', 'ready', 185),
	task('bdm-999', 'ready', 8690, [], { locked: true, project: PROJECTS[2] })
];

const names = (tasks: DirectoryTask[]) => tasks.map((each) => each.name);

test('people rank by task count, then by the most recent activity', () => {
	assert.deepEqual(
		people(TASKS).map(([name, own]) => [name, own.length]),
		[
			['hoang.vu', 3],
			['ty.go', 2],
			['tuan.pham', 1],
			['qui.nguyen', 1]
		]
	);
});

test('a task with several people appears in each person group, then Unassigned and Private', () => {
	assert.deepEqual(
		groupTasks(TASKS, 'person', PROJECTS).map((group) => [
			group.kind,
			group.key,
			names(group.tasks)
		]),
		[
			['person', 'hoang.vu', ['kiosk-403', 'bview-21', 'myl-2101']],
			['person', 'ty.go', ['bview-21', 'myl-1976']],
			['person', 'tuan.pham', ['kiosk-403']],
			['person', 'qui.nguyen', ['wo-718']],
			['none', 'none', ['staging']],
			['private', 'private', ['bdm-999']]
		]
	);
});

test('a locked task lands in Private, never in Unassigned', () => {
	const locked = task('bdm-1', 'ready', 1, [], { locked: true });
	const keys = groupTasks([locked], 'person', PROJECTS).map((group) => group.key);
	assert.deepEqual(keys, ['private']);
});

test('status and project groups keep their order and drop empty groups', () => {
	assert.deepEqual(
		groupTasks(TASKS.slice(2, 4), 'status', PROJECTS).map((group) => group.label),
		['In progress', 'Ready']
	);
	assert.deepEqual(
		groupTasks(TASKS, 'project', PROJECTS).map((group) => [group.label, group.tasks.length]),
		[
			['Blogic View', 5],
			['Kiosk', 1],
			['Pos Portal', 1]
		]
	);
});

test('breakdown and board columns follow severity order', () => {
	assert.equal(breakdown(TASKS), '1 blocked · 2 in progress · 4 ready');
	assert.equal(breakdown([TASKS[0], TASKS[3]]), '2 ready');
	assert.deepEqual(
		columns(TASKS, 'all').map((column) => [column.status, column.tasks.length]),
		[
			['blocked', 1],
			['in_progress', 2],
			['ready', 4]
		]
	);
	assert.deepEqual(
		columns(TASKS, 'blocked').map((column) => column.status),
		['blocked']
	);
});
