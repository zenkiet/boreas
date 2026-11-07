import assert from 'node:assert/strict';
import { test } from 'node:test';
import type { DirectoryTask } from '#shared/api';
import { highlight, matches, needleOf } from './search.ts';

const task = (more: Partial<DirectoryTask> = {}): DirectoryTask => ({
	name: 'wo-715',
	description: 'Improve Menu Scheduler logic',
	status: 'running',
	devStatus: 'in_progress',
	updatedAt: '2026-11-07T14:28:00Z',
	assignees: ['ty.go', 'phuong.nam'],
	formFactor: 'web',
	locked: false,
	project: { slug: 'qrcode-dinein', name: 'QRCode DineIn' },
	...more
});

test('matches the id, description, slug, project name and people', () => {
	for (const query of ['WO-71', 'scheduler', 'qrcode-di', 'dinein', 'phuong']) {
		assert.ok(matches(task(), needleOf(query)), query);
	}
	assert.ok(matches(task(), ''));
	assert.ok(!matches(task(), needleOf('refund')));
});

test('highlight splits every case-insensitive hit', () => {
	assert.deepEqual(highlight('Improve Happy Hours', 'happy'), [
		{ text: 'Improve ', hit: false },
		{ text: 'Happy', hit: true },
		{ text: ' Hours', hit: false }
	]);
	assert.deepEqual(highlight('wo-718 wo-71', 'wo-71'), [
		{ text: 'wo-71', hit: true },
		{ text: '8 ', hit: false },
		{ text: 'wo-71', hit: true }
	]);
	assert.deepEqual(highlight('staging', ''), [{ text: 'staging', hit: false }]);
	assert.deepEqual(highlight('staging', 'prod'), [{ text: 'staging', hit: false }]);
});
