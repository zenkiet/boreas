import assert from 'node:assert/strict';
import { test } from 'node:test';
import { monogram } from './monogram.ts';

test('monogram joins the first letters of the first and last words', () => {
	const names = [
		['Blogic View', 'BV'],
		['QRCode DineIn', 'QD'],
		['Online Ordering', 'OO'],
		['Kiosk', 'K'],
		['License', 'L'],
		['Pos Portal', 'PP'],
		['Wait And Order', 'WO'],
		['', '']
	];
	for (const [name, expected] of names) assert.equal(monogram(name), expected);
});
