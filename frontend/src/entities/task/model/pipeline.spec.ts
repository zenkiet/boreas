import assert from 'node:assert/strict';
import { test } from 'node:test';
import type { Build, Deploy, PublicTask } from '#shared/api';
import { buildLine, deployLine, pipelineOf } from './pipeline.ts';

const NOW = Date.parse('2026-11-07T14:30:00Z');
const at = (minutes: number) => new Date(NOW - minutes * 60_000).toISOString();

const task = (build?: Build, lastDeploy?: Deploy): PublicTask => ({
	name: 'wo-715',
	status: 'running',
	devStatus: 'in_progress',
	updatedAt: at(2),
	assignees: [],
	build,
	lastDeploy,
	formFactor: 'web',
	locked: false
});

const cell = (build?: Build, lastDeploy?: Deploy) => {
	const { kind, text } = pipelineOf(task(build, lastDeploy));
	return `${kind}: ${text}`;
};

const line = ({ kind, text, when }: ReturnType<typeof buildLine>) => `${kind}: ${text} | ${when}`;

test('the row cell shows running, then failed, then canceled, then deployed', () => {
	const deployed: Deploy = { status: 'success', at: at(1440) };
	assert.equal(
		cell({ state: 'running', stage: 'Test', progress: 62, at: at(6) }, deployed),
		'run: Test · 62%'
	);
	assert.equal(
		cell({ state: 'running', at: at(3) }, { status: 'failure', at: at(1) }),
		'run: Building'
	);
	assert.equal(
		cell({ state: 'success', at: at(48) }, { status: 'failure', at: at(40) }),
		'bad: Deploy failed'
	);
	assert.equal(
		cell({ state: 'failure', stage: 'Test', at: at(10) }, { status: 'failure', at: at(60) }),
		'bad: Build failed · Test'
	);
	assert.equal(
		cell({ state: 'failure', stage: 'Build', at: at(190) }, deployed),
		'bad: Build failed · Build'
	);
	assert.equal(
		cell({ state: 'canceled', stage: 'Test', at: at(310) }, deployed),
		'cancel: Canceled at Test'
	);
	assert.equal(cell({ state: 'success', at: at(130) }, deployed), 'ok: Deployed');
	assert.equal(cell(undefined, deployed), 'ok: Deployed');
	assert.equal(cell({ state: 'success', at: at(130) }), 'ok: Passed');
	assert.equal(cell(), 'none: No pipeline');
	assert.equal(pipelineOf(task({ state: 'running', progress: 62, at: at(6) })).progress, 62);
});

test('the build line reads the build with its time', () => {
	assert.equal(
		line(buildLine(task({ state: 'running', stage: 'Lint', progress: 20, at: at(2) }), NOW)),
		'run: Lint · 20% | started 2 min ago'
	);
	assert.equal(
		line(buildLine(task({ state: 'running', at: at(3) }), NOW)),
		'run: Running | started 3 min ago'
	);
	assert.equal(
		line(buildLine(task({ state: 'success', at: at(48) }), NOW)),
		'ok: Passed | 48 min ago'
	);
	assert.equal(
		line(buildLine(task({ state: 'failure', stage: 'Test', at: at(65) }), NOW)),
		'bad: Failed at Test | 1 hour ago'
	);
	assert.equal(
		line(buildLine(task({ state: 'canceled', stage: 'Test', at: at(310) }), NOW)),
		'cancel: Canceled at Test | 5 hours ago'
	);
	assert.equal(line(buildLine(task(), NOW)), 'none: No CI report yet | ');
});

test('the deploy line waits for a running build', () => {
	const running: Build = { state: 'running', stage: 'Lint', progress: 20, at: at(2) };
	assert.equal(
		line(deployLine(task(running, { status: 'success', at: at(720) }), NOW)),
		'wait: Runs after the build | last 12 hours ago'
	);
	assert.equal(line(deployLine(task(running), NOW)), 'wait: Runs after the build | ');
	assert.equal(line(deployLine(task(), NOW)), 'none: Not deployed yet | ');
	assert.equal(
		line(deployLine(task(undefined, { status: 'success', at: at(14400) }), NOW)),
		'ok: Succeeded | 10 days ago'
	);
	assert.equal(
		line(deployLine(task(undefined, { status: 'failure', at: at(40) }), NOW)),
		'bad: Failed | 40 min ago'
	);
});
