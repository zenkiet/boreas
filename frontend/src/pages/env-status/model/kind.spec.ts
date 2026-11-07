import assert from 'node:assert/strict';
import { test } from 'node:test';
import type { DirectoryTask, PublicProject, TaskStatus } from '#shared/api';
import { closestMatches, copyOf, findTask, kindOf } from './kind.ts';

const task = (slug: string, name: string, status: TaskStatus = 'running'): DirectoryTask => ({
	name,
	status,
	devStatus: 'ready',
	updatedAt: '2026-11-07T14:00:00Z',
	assignees: [],
	formFactor: 'web',
	locked: false,
	project: { slug, name: slug }
});

const TASKS = [
	task('blogic-view', 'staging'),
	task('blogic-view', 'wo-718'),
	task('qrcode-dinein', 'staging'),
	task('qrcode-dinein', 'wo-718'),
	task('qrcode-dinein', 'wo-715'),
	task('qrcode-dinein', 'bweb-1'),
	task('online-ordering', 'wo-704'),
	task('online-ordering', 'wo-714')
];

const PROJECTS: PublicProject[] = ['blogic-view', 'qrcode-dinein', 'online-ordering'].map(
	(slug) => ({ slug, name: slug, tasks: [] })
);

test('a task status picks the page, and an unknown address is missing or not found', () => {
	const kind = (status: TaskStatus) => kindOf(PROJECTS, 'portal', task('portal', 'x', status));
	assert.deepEqual(
		(['creating', 'starting', 'running', 'stopped', 'unknown', 'error'] as const).map(kind),
		['starting', 'starting', 'starting', 'stopped', 'stopped', 'failed']
	);
	assert.equal(kindOf(PROJECTS, 'qrcode-dinein'), 'missing');
	assert.equal(kindOf(PROJECTS, 'legal'), 'notfound');
});

test('task names match whatever their case', () => {
	assert.equal(findTask(TASKS, 'qrcode-dinein', 'WO-715')?.name, 'wo-715');
	assert.equal(findTask(TASKS, 'qrcode-dinein', 'wo-716'), undefined);
});

test('the body names the assignees, or falls back to a role', () => {
	assert.equal(
		copyOf('stopped', 'portal', 'bdm-900', ['christie']).body,
		'Ask christie to start it from the Boreas console.'
	);
	assert.match(copyOf('stopped', 'portal', 'bdm-900').body, /^Ask a project member to start/);
	assert.equal(
		copyOf('failed', 'license', 'bld-10', ['a', 'b', 'c']).body,
		'Ask a, b and c to check the logs.'
	);
	assert.match(copyOf('failed', 'license', 'bld-10').body, /^Ask the project owner to check/);
	assert.equal(copyOf('missing', 'qrcode-dinein', 'wo-719').code, '404 · NO SUCH ENVIRONMENT');
});

test('closest matches put the same project first, then the nearest names elsewhere', () => {
	const keys = closestMatches(TASKS, 'qrcode-dinein', 'wo-719').map(
		(match) => `${match.project.slug}/${match.name}`
	);
	assert.deepEqual(keys, [
		'qrcode-dinein/wo-718',
		'qrcode-dinein/wo-715',
		'blogic-view/wo-718',
		'online-ordering/wo-714'
	]);
	assert.deepEqual(
		closestMatches(TASKS, 'kiosk', 'staging').map((match) => match.project.slug),
		['blogic-view', 'qrcode-dinein']
	);
	assert.deepEqual(closestMatches(TASKS, 'qrcode-dinein', 'release-candidate'), []);
});
