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
	// Scripts asking for a touch screen get the phone's answer by asking the desktop the opposite; CSS still cannot.
	const desktopMedia = window.matchMedia.bind(window);
	const flipped = { fine: 'coarse', coarse: 'fine', hover: 'none', none: 'hover' };
	window.matchMedia = (query) =>
		desktopMedia(String(query).replace(/(pointer|hover)\s*:\s*(fine|coarse|hover|none)/g, (_, feature, value) => `${feature}: ${flipped[value]}`));

	// env(safe-area-inset-*) is 0 inside an iframe; Ionic reads these variables first.
	const style = document.createElement('style');
	const finger =
		"url(\"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='28' height='28'%3E%3Ccircle cx='14' cy='14' r='11' fill='%2300000033' stroke='%23ffffffd9' stroke-width='2'/%3E%3C/svg%3E\") 14 14";
	const insets = (top, bottom) => {
		style.textContent =
			`html{--safe-area-inset-top:${top}px!important;--safe-area-inset-bottom:${bottom}px!important;` +
			'--safe-area-inset-left:0px!important;--safe-area-inset-right:0px!important}' +
			'*{scrollbar-width:none!important}::-webkit-scrollbar{display:none!important}' +
			`html,html *{cursor:${finger},auto!important}`;
	};
	insets(device.insets.top, device.insets.bottom);
	document.head.append(style);

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
