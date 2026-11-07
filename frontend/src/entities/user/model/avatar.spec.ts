import assert from 'node:assert/strict';
import { test } from 'node:test';
import { initials, toneOf } from './avatar.ts';

test('initials take the first letters of the first and last parts', () => {
	assert.equal(initials('qui.nguyen'), 'QN');
	assert.equal(initials('christie'), 'C');
	assert.equal(initials('kiet.le'), 'KL');
	assert.equal(initials('anh-duc_nguyen'), 'AN');
});

test('toneOf is stable and stays within the seven tones', () => {
	const names = [
		'qui.nguyen',
		'phuong.nam',
		'ty.go',
		'hoang.vu',
		'tuan.pham',
		'christie',
		'kiet.le'
	];
	for (const name of names) {
		assert.equal(toneOf(name), toneOf(name));
		assert.ok(toneOf(name) >= 0 && toneOf(name) <= 6);
	}
	assert.ok(new Set(names.map(toneOf)).size > 1);
});
