// The proxy makes this the first script of every page of a mobile task (internal/infra/proxy/registry.go).
// On a desktop it swaps the page for the phone frame; inside that frame it makes the app believe it runs on an iPhone.
(() => {
	if (window.top === window) {
		if (navigator.webdriver || !matchMedia('(hover: hover) and (pointer: fine)').matches) return;
		// The app's scripts come after this one in the page, so stopping now means they never run up here.
		window.stop();
		const html = document.documentElement;
		for (const { name } of [...html.attributes]) html.removeAttribute(name);
		html.innerHTML =
			'<head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">' +
			'<link rel="stylesheet" href="/_device/frame.css"></head><body></body>';
		const frame = document.createElement('script');
		frame.type = 'module';
		frame.src = '/_device/frame.js';
		document.head.append(frame);
		return;
	}

	let device;
	try {
		device = window.parent.__boreasDevice;
	} catch {
		return; // embedded by another origin
	}
	if (!device) return;

	// Safari's agent: iOS 26 and later freeze the OS version in it.
	const ua =
		'Mozilla/5.0 (iPhone; CPU iPhone OS 18_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/27.0 Mobile/15E148 Safari/604.1';
	const iphone = {
		userAgent: ua,
		appVersion: ua.slice('Mozilla/'.length),
		platform: 'iPhone',
		vendor: 'Apple Computer, Inc.',
		maxTouchPoints: 5,
		userAgentData: undefined,
		standalone: false
	};
	for (const [name, value] of Object.entries(iphone)) {
		Object.defineProperty(Navigator.prototype, name, { get: () => value, configurable: true });
	}
	// Scripts and stylesheets asking for a touch screen get the phone's answer by asking the desktop the opposite.
	const flipped = { fine: 'coarse', coarse: 'fine', hover: 'none', none: 'hover' };
	const touch = (query) =>
		String(query).replace(/(pointer|hover)\s*:\s*(fine|coarse|hover|none)/g, (_, feature, value) => `${feature}: ${flipped[value]}`);
	const desktopMedia = window.matchMedia.bind(window);
	window.matchMedia = (query) => desktopMedia(touch(query));

	// Nothing is under a finger until it presses, so :hover becomes :active.
	const done = new WeakSet(); // a second flip would undo the first
	const restyle = (rules) => {
		for (const rule of rules) {
			if (done.has(rule)) continue;
			done.add(rule);
			if (rule.media && touch(rule.media.mediaText) !== rule.media.mediaText) rule.media.mediaText = touch(rule.media.mediaText);
			if (rule.selectorText?.includes(':hover')) rule.selectorText = rule.selectorText.replaceAll(':hover', ':active');
			if (rule.cssRules) restyle(rule.cssRules);
		}
	};
	const sheet = (target) => {
		try {
			restyle(target?.cssRules ?? []);
		} catch {
			// another origin's stylesheet cannot be read
		}
	};
	// ponytail: <style>, <link> and replaceSync() sheets only; one grown by insertRule() or rewritten in place keeps its hover.
	new MutationObserver((records) => {
		for (const { addedNodes } of records)
			for (const node of addedNodes) {
				if (node.localName === 'style') sheet(node.sheet);
				if (node.localName === 'link') node.addEventListener('load', () => sheet(node.sheet));
			}
	}).observe(document, { childList: true, subtree: true });
	const { replaceSync } = CSSStyleSheet.prototype;
	CSSStyleSheet.prototype.replaceSync = function (text) {
		replaceSync.call(this, text);
		sheet(this);
	};

	// env(safe-area-inset-*) is 0 inside an iframe; Ionic reads these variables first.
	const style = document.createElement('style');
	const finger =
		"url(\"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='28' height='28'%3E%3Ccircle cx='14' cy='14' r='11' fill='%2300000033' stroke='%23ffffffd9' stroke-width='2'/%3E%3C/svg%3E\") 14 14";
	const touchCSS = `*{scrollbar-width:none!important;cursor:${finger},auto!important}::-webkit-scrollbar{display:none!important}`;
	const insets = (top, bottom) => {
		style.textContent =
			`html{--safe-area-inset-top:${top}px!important;--safe-area-inset-bottom:${bottom}px!important;` +
			`--safe-area-inset-left:0px!important;--safe-area-inset-right:0px!important}${touchCSS}`;
	};
	insets(device.insets.top, device.insets.bottom);
	document.head.append(style);
	// Shadow roots too, or Ionic's controls would show the hand cursor and their own scrollbars.
	const shadowCSS = new CSSStyleSheet();
	shadowCSS.replaceSync(touchCSS);
	const { attachShadow } = Element.prototype;
	Element.prototype.attachShadow = function (init) {
		const root = attachShadow.call(this, init);
		root.adoptedStyleSheets = [shadowCSS];
		return root;
	};

	// A drag moves the scroller under it like a finger: it glides on after release, and the press clicks nothing.
	const axes = {
		x: { pans: /auto|manipulation|pan-(x|left|right)/, offset: 'scrollLeft', size: 'scrollWidth', view: 'clientWidth', overflow: 'overflowX', at: 'clientX', side: 'left' },
		y: { pans: /auto|manipulation|pan-(y|up|down)/, offset: 'scrollTop', size: 'scrollHeight', view: 'clientHeight', overflow: 'overflowY', at: 'clientY', side: 'top' }
	};
	const movable = (element, axis, delta) => {
		const { offset, size, view, overflow } = axes[axis];
		const page = element === document.scrollingElement;
		const open = page
			? [document.documentElement, document.body].every((e) => !/hidden|clip/.test(getComputedStyle(e)[overflow]))
			: /auto|scroll|overlay/.test(getComputedStyle(element)[overflow]);
		return open && (delta < 0 ? element[offset] < element[size] - element[view] - 1 : element[offset] > 0);
	};
	// The first scroller under the press that can move that way, unless touch-action leaves the drag to the app.
	const scroller = (path, axis, delta) => {
		for (const element of path) {
			if (!(element instanceof Element)) continue;
			if (!axes[axis].pans.test(getComputedStyle(element).touchAction)) return null;
			if (element !== document.documentElement && element !== document.body && movable(element, axis, delta)) return element;
		}
		return movable(document.scrollingElement, axis, delta) ? document.scrollingElement : null;
	};
	const editable = (node) =>
		(node.closest ? node : node.parentElement)?.closest('input, textarea, select, [contenteditable]:not([contenteditable="false"])');
	let pan;
	let glide = 0;
	let swallow = false;
	addEventListener(
		'pointerdown',
		(event) => {
			swallow = glide !== 0; // touching a gliding page only stops it
			cancelAnimationFrame(glide);
			glide = 0;
			const path = event.composedPath();
			pan = event.pointerType === 'mouse' && event.button === 0 && !editable(path[0]) ? { path, x: event.clientX, y: event.clientY } : undefined;
		},
		true
	);
	addEventListener(
		'pointermove',
		(event) => {
			if (!pan) return;
			if (!pan.element) {
				const dx = event.clientX - pan.x;
				const dy = event.clientY - pan.y;
				if (Math.hypot(dx, dy) < 10) return;
				pan.axis = Math.abs(dx) > Math.abs(dy) ? 'x' : 'y';
				pan.element = scroller(pan.path, pan.axis, pan.axis === 'x' ? dx : dy);
				if (!pan.element) return void (pan = undefined);
				pan.moves = [[event.timeStamp, event[axes[pan.axis].at]]];
				swallow = true;
				return;
			}
			const { at, side } = axes[pan.axis];
			pan.element.scrollBy({ [side]: pan.moves.at(-1)[1] - event[at], behavior: 'instant' });
			pan.moves.push([event.timeStamp, event[at]]);
		},
		true
	);
	addEventListener(
		'pointerup',
		(event) => {
			setTimeout(() => (swallow = false));
			if (!pan?.element) return void (pan = undefined);
			const { element, axis, moves } = pan;
			const { offset, side } = axes[axis];
			pan = undefined;
			const [t1, at1] = moves.at(-1);
			const [t0, at0] = moves.find(([t]) => t1 - t < 100);
			let velocity = event.timeStamp - t1 < 50 && t1 > t0 ? (at1 - at0) / (t1 - t0) : 0;
			let then = event.timeStamp;
			const step = (now) => {
				velocity *= 0.998 ** (now - then); // UIScrollView's normal deceleration, per millisecond
				const before = element[offset];
				element.scrollBy({ [side]: -velocity * (now - then), behavior: 'instant' });
				then = now;
				glide = Math.abs(velocity) > 0.05 && element[offset] !== before ? requestAnimationFrame(step) : 0;
			};
			glide = requestAnimationFrame(step);
		},
		true
	);
	addEventListener('click', (event) => swallow && (event.preventDefault(), event.stopImmediatePropagation()), true);
	// Dragging text or an image or link scrolls on a phone; it neither selects nor picks anything up.
	addEventListener('selectstart', (event) => editable(event.composedPath()[0]) || event.preventDefault(), true);
	addEventListener('dragstart', (event) => event.target.getAttribute?.('draggable') === 'true' || event.preventDefault(), true);
	// Hover events too: a finger only moves over the page while touching it.
	for (const kind of ['mouse', 'pointer'])
		for (const type of ['over', 'out', 'enter', 'leave', 'move'])
			addEventListener(kind + type, (event) => event.buttons || event.stopImmediatePropagation(), true);

	const report = () => device.route(location.href, document.title);
	for (const name of ['pushState', 'replaceState']) {
		const original = History.prototype[name];
		History.prototype[name] = function (...args) {
			const result = original.apply(this, args);
			report();
			setTimeout(report, 500); // a router retitles the page after the route changes
			return result;
		};
	}
	addEventListener('popstate', report);
	addEventListener('hashchange', report);

	const deepest = (x, y) => {
		let element = document.elementFromPoint(x, y);
		while (element?.shadowRoot) {
			const inner = element.shadowRoot.elementFromPoint(x, y);
			if (!inner || inner === element) break;
			element = inner;
		}
		return element;
	};
	const dark = (element) => {
		for (; element; element = element.parentElement ?? element.getRootNode().host) {
			const [r, g, b, a = 1] = (getComputedStyle(element).backgroundColor.match(/[\d.]+/g) ?? []).map(Number);
			if (a > 0.5) return 0.2126 * r + 0.7152 * g + 0.0722 * b < 128;
		}
		return false;
	};
	// ponytail: polling the colours behind the status bar and home indicator; hook page transitions if 1.5 s lags.
	setInterval(() => device.colors(dark(deepest(40, 12)), dark(deepest(innerWidth / 2, innerHeight - 6))), 1500);

	addEventListener('load', () => {
		report();
		// Without Ionic the app ignores the variables, so the frame keeps its content clear of the bars instead.
		if (!getComputedStyle(document.documentElement).getPropertyValue('--ion-safe-area-top').trim()) {
			insets(0, 0);
			device.reserve();
		}
	});
})();
