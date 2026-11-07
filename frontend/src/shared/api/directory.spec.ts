import assert from 'node:assert/strict';
import { test } from 'node:test';
import { bySeverity, flattenTasks, listProjects, taskKey } from './directory.ts';

const payload = {
	projects: [
		{
			slug: 'portal',
			name: 'Pos Portal',
			tasks: [
				{
					name: 'bdm-999',
					note: 'leaked',
					assignees: ['tuan.pham'],
					status: 'running',
					dev_status: 'ready',
					updated_at: '2026-11-01T09:00:00Z',
					last_deploy: { status: 'success', at: '2026-11-01T09:00:00Z' },
					form_factor: 'mobile',
					has_password: true
				},
				{
					name: 'bdm-900',
					status: 'stopped',
					dev_status: 'ready',
					updated_at: '2026-11-07T14:00:00Z',
					last_deploy: { status: 'info', at: '2026-11-07T14:00:00Z' },
					form_factor: 'web',
					has_password: false
				},
				{
					name: 'bdm-1',
					status: 'running',
					dev_status: 'blocked',
					updated_at: '2026-10-01T09:00:00Z',
					assignees: ['christie'],
					form_factor: 'web',
					has_password: false
				}
			]
		}
	],
	total: 3
};

const respond = (status: number, body: unknown) => () =>
	Promise.resolve(new Response(JSON.stringify(body), { status }));

test('listProjects maps the wire to camelCase and drops non-deploys', async () => {
	const [project] = await listProjects(respond(200, payload));
	const [locked, stopped] = project.tasks;
	assert.equal(locked.locked, true);
	assert.equal(locked.formFactor, 'mobile');
	assert.deepEqual(locked.assignees, []);
	assert.equal(locked.note, undefined);
	assert.deepEqual(locked.lastDeploy, { status: 'success', at: '2026-11-01T09:00:00Z' });
	assert.equal(stopped.lastDeploy, undefined);
	assert.equal(stopped.devStatus, 'ready');
});

test('listProjects explains a failed answer', async () => {
	await assert.rejects(listProjects(respond(502, {})), /didn’t answer \(502\)/);
	await assert.rejects(
		listProjects(() => Promise.reject(new TypeError('offline'))),
		/didn’t answer\./
	);
});

test('tasks sort worst first, then newest', async () => {
	const tasks = flattenTasks(await listProjects(respond(200, payload)));
	assert.deepEqual(bySeverity(tasks).map(taskKey), [
		'portal/bdm-1',
		'portal/bdm-900',
		'portal/bdm-999'
	]);
});
