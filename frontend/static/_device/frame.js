// The phone frame a desktop sees for a mobile project. shim.js builds it in place of the app's page and the app
// runs in the iframe, talking to window.__boreasDevice.
import { renderSVG } from './uqr.mjs';

const base = location.pathname.split('/').slice(0, 3).join('/') + '/';
const [, project, task] = base.split('/');
const finishes = { black: '#575356', burgundy: '#845960', glacier: '#d5deeb', silver: '#dbdbdb' };

const saved = (name, fallback) => {
	try {
		return localStorage.getItem(name) ?? fallback;
	} catch {
		return fallback;
	}
};
const save = (name, value) => {
	try {
		localStorage.setItem(name, value);
	} catch {
		// Blocked storage forgets the choice on reload.
	}
};

const icons =
	'<svg viewBox="0 0 18 12" width="18" height="12"><rect y="8" width="3" height="4" rx="1"/><rect x="5" y="5.5" width="3" height="6.5" rx="1"/><rect x="10" y="3" width="3" height="9" rx="1"/><rect x="15" width="3" height="12" rx="1"/></svg>' +
	'<svg viewBox="0 0 16 12" width="16" height="12"><path d="M8 2.2c2.3 0 4.4.9 6 2.4l1.3-1.4A10.6 10.6 0 0 0 8 .3 10.6 10.6 0 0 0 .7 3.2L2 4.6a8.7 8.7 0 0 1 6-2.4Zm0 3.8c1.3 0 2.5.5 3.4 1.3l1.3-1.4A6.8 6.8 0 0 0 8 4.1a6.8 6.8 0 0 0-4.7 1.8l1.3 1.4C5.5 6.5 6.7 6 8 6Zm0 3.8a1.7 1.7 0 0 0-1.3.6L8 11.8l1.3-1.4A1.7 1.7 0 0 0 8 9.8Z"/></svg>' +
	'<svg viewBox="0 0 27 13" width="27" height="13"><rect x=".5" y=".5" width="23" height="12" rx="3.5" fill="none" stroke="currentColor" opacity=".4"/><rect x="2" y="2" width="20" height="9" rx="2"/><path d="M25 4.5v4a2 2 0 0 0 0-4Z" opacity=".4"/></svg>';

document.title = `${project} · ${task}`;
document.body.innerHTML = `
<main class="stage">
	<div class="fit">
		<div class="device">
			<div class="screen">
				<iframe title="${project} ${task}"></iframe>
				<div class="statusbar"><span class="time"></span><span></span><span class="icons">${icons}</span></div>
				<div class="indicator"></div>
				<div class="home" hidden><button class="app" type="button"><span>${project[0].toUpperCase()}</span>${project}</button></div>
				<button class="banner" type="button"><span class="badge">${project[0].toUpperCase()}</span><span><strong></strong><span></span></span></button>
				<div class="alert" hidden><div class="sheet"><h3></h3><p></p><div class="actions"><button type="button" value="deny">Don’t Allow</button><button type="button" value="allow">Allow</button></div></div></div>
				<div class="notice" hidden><p>This page can’t be shown inside the frame.</p><button type="button" name="off">Open without frame</button></div>
			</div>
			<img class="bezel" alt="" draggable="false">
		</div>
	</div>
</main>
<aside class="panel">
	<header><strong>${project}</strong><span>${task}</span></header>
	<div class="finishes" role="radiogroup" aria-label="Finish">${Object.entries(finishes)
		.map(([name, color]) => `<button type="button" role="radio" data-finish="${name}" style="--swatch:${color}" aria-label="${name}" title="${name}"></button>`)
		.join('')}</div>
	<label class="switch"><input type="checkbox" name="native"><span>Native emulation</span></label>
	<p class="hint">Status bar, haptics, app state, push and preferences answer from this frame. Other plugins report “not emulated”.</p>
	<div class="native" hidden>
		<button type="button" name="home">Home</button>
		<form name="push">
			<input name="title" value="Test push" aria-label="Title">
			<input name="body" value="Hello from Boreas" aria-label="Body">
			<input name="data" placeholder='Data, e.g. {"url":"/orders/1"}' aria-label="Data (JSON)">
			<button>Send test push</button>
		</form>
		<ol class="log" aria-label="Native calls"></ol>
	</div>
	<figure class="qr"><div></div><figcaption>Scan to open this screen on a phone</figcaption></figure>
	<button type="button" name="off">Open without frame</button>
</aside>`;

const $ = (selector) => document.querySelector(selector);
const device = $('.device');
const iframe = $('iframe');
const log = $('.log');
const native = saved('boreas-device:native:' + base, '0') === '1';
$('[name=native]').checked = native;
$('.native').hidden = !native;

const finish = (name) => {
	device.dataset.finish = name;
	$('.bezel').src = `/_device/iphone-18-pro-max-${name}.png`;
	for (const swatch of document.querySelectorAll('[data-finish]')) {
		swatch.setAttribute('aria-checked', String(swatch.dataset.finish === name));
	}
	save('boreas-device:finish', name);
};
const chosen = saved('boreas-device:finish', 'black');
finish(chosen in finishes ? chosen : 'black');

const fit = () => {
	const stage = $('.stage');
	const scale = Math.min(1, (stage.clientHeight - 32) / 1000, (stage.clientWidth - 32) / 490);
	document.documentElement.style.setProperty('--scale', String(Math.max(scale, 0.3)));
};
addEventListener('resize', fit);
fit();

const clock = () => {
	const now = new Date();
	$('.time').textContent = `${now.getHours()}:${String(now.getMinutes()).padStart(2, '0')}`;
};
clock();
setInterval(clock, 10_000);

const qr = (href) => {
	$('.qr div').innerHTML = renderSVG(href, { border: 2 });
};

const buzz = () => {
	device.classList.remove('buzz');
	requestAnimationFrame(() => device.classList.add('buzz'));
};

// The app decides the status bar text colour once it calls StatusBar.setStyle; until then the page colour does.
const bars = { styled: false, top: false, bottom: false };
const paint = () => {
	device.classList.toggle('status-light', bars.top);
	device.classList.toggle('indicator-light', bars.bottom);
};
const setStyle = (style, where = 'top') => {
	if (style === 'DEFAULT') {
		bars.styled = false;
		return;
	}
	bars.styled = true;
	bars[where] = style === 'DARK'; // DARK means a dark background, so light text
	paint();
};

const listeners = new Map();
let nextListener = 0;
const emit = (plugin, event, data) => {
	for (const listener of listeners.values()) {
		if (listener.plugin === plugin && listener.event === event) listener.callback(data);
	}
};

const state = { active: true };
const appDocument = () => iframe.contentDocument;
const lifecycle = (active) => {
	if (state.active === active) return;
	state.active = active;
	$('.home').hidden = active;
	emit('App', 'appStateChange', { isActive: active });
	emit('App', active ? 'resume' : 'pause', undefined);
	const doc = appDocument();
	doc?.dispatchEvent(new doc.defaultView.Event(active ? 'resume' : 'pause'));
};

const ask = (title, message) =>
	new Promise((resolve) => {
		const alert = $('.alert');
		alert.querySelector('h3').textContent = title;
		alert.querySelector('p').textContent = message;
		alert.hidden = false;
		alert.onclick = (event) => {
			const choice = event.target.closest('button')?.value;
			if (!choice) return;
			alert.hidden = true;
			resolve(choice === 'allow');
		};
	});

const pushKey = 'boreas-device:push:' + base;
const delivered = [];
const token = () => {
	let value = saved('boreas-device:token:' + base, '');
	if (!value) {
		value = [...crypto.getRandomValues(new Uint8Array(32))].map((byte) => byte.toString(16).padStart(2, '0')).join('');
		save('boreas-device:token:' + base, value);
	}
	return value;
};

let bannerTimer;
const banner = (notification) => {
	const element = $('.banner');
	element.querySelector('strong').textContent = notification.title;
	element.querySelector('strong + span').textContent = notification.body;
	element.classList.add('shown');
	clearTimeout(bannerTimer);
	bannerTimer = setTimeout(() => element.classList.remove('shown'), 5000);
	element.onclick = () => {
		element.classList.remove('shown');
		lifecycle(true);
		emit('PushNotifications', 'pushNotificationActionPerformed', { actionId: 'tap', notification });
	};
};

// Preferences live per task: the web build's CapacitorStorage.* keys are shared by every task on this origin.
let group = 'CapacitorStorage';
const prefix = () => `boreas-device:prefs:${base}${group}.`;
const preferenceKeys = () => {
	const keys = [];
	for (let i = 0; i < localStorage.length; i++) {
		const name = localStorage.key(i);
		if (name?.startsWith(prefix())) keys.push(name.slice(prefix().length));
	}
	return keys;
};

const plugins = {
	StatusBar: {
		setStyle: ({ style }) => setStyle(style),
		show: () => device.classList.remove('status-hidden'),
		hide: () => device.classList.add('status-hidden'),
		getInfo: () => ({ visible: !device.classList.contains('status-hidden'), style: bars.top ? 'DARK' : 'LIGHT', overlays: true }),
		setBackgroundColor: () => {},
		setOverlaysWebView: () => {}
	},
	SystemBars: {
		setStyle: ({ style, bar }) => {
			if (bar !== 'StatusBar') setStyle(style, 'bottom');
			if (bar !== 'NavigationBar') setStyle(style);
		},
		show: ({ bar } = {}) => {
			if (bar !== 'NavigationBar') device.classList.remove('status-hidden');
			if (bar !== 'StatusBar') device.classList.remove('indicator-hidden');
		},
		hide: ({ bar } = {}) => {
			if (bar !== 'NavigationBar') device.classList.add('status-hidden');
			if (bar !== 'StatusBar') device.classList.add('indicator-hidden');
		},
		setAnimation: () => {}
	},
	Haptics: {
		impact: buzz,
		notification: buzz,
		vibrate: buzz,
		selectionStart: () => {},
		selectionChanged: buzz,
		selectionEnd: () => {}
	},
	App: {
		getInfo: () => ({ name: project, id: `dev.boreas.${project}`, build: task, version: task }),
		getState: () => ({ isActive: state.active }),
		getLaunchUrl: () => undefined,
		minimizeApp: () => lifecycle(false)
	},
	PushNotifications: {
		checkPermissions: () => ({ receive: saved(pushKey, 'prompt') }),
		requestPermissions: async () => {
			if (saved(pushKey, 'prompt') === 'prompt') {
				const allowed = await ask(
					`“${project}” Would Like to Send You Notifications`,
					'Notifications may include alerts, sounds, and icon badges. These can be configured in Settings.'
				);
				save(pushKey, allowed ? 'granted' : 'denied');
			}
			return { receive: saved(pushKey, 'prompt') };
		},
		register: () => {
			setTimeout(() => emit('PushNotifications', 'registration', { value: token() }), 300);
		},
		unregister: () => {},
		getDeliveredNotifications: () => ({ notifications: delivered }),
		removeDeliveredNotifications: ({ notifications = [] }) => {
			const ids = new Set(notifications.map((notification) => notification.id));
			delivered.splice(0, delivered.length, ...delivered.filter((notification) => !ids.has(notification.id)));
		},
		removeAllDeliveredNotifications: () => {
			delivered.length = 0;
		},
		createChannel: () => {},
		deleteChannel: () => {},
		listChannels: () => ({ channels: [] })
	},
	Preferences: {
		configure: ({ group: name } = {}) => {
			group = name ?? 'CapacitorStorage';
		},
		get: ({ key }) => ({ value: localStorage.getItem(prefix() + key) }),
		set: ({ key, value }) => localStorage.setItem(prefix() + key, value),
		remove: ({ key }) => localStorage.removeItem(prefix() + key),
		keys: () => ({ keys: preferenceKeys() }),
		clear: () => preferenceKeys().forEach((key) => localStorage.removeItem(prefix() + key)),
		migrate: () => ({ migrated: [], existing: [] }),
		removeOld: () => {}
	}
};

const record = (text, missing) => {
	const item = document.createElement('li');
	item.textContent = text.length > 140 ? text.slice(0, 139) + '…' : text;
	item.classList.toggle('missing', missing);
	log.prepend(item);
	log.querySelector('li:nth-child(61)')?.remove();
};

window.__boreasDevice = {
	insets: { top: 62, bottom: 34 },
	bridge: native,
	plugins: Object.keys(plugins),
	route(href, title) {
		if (href !== location.href) history.replaceState(null, '', href);
		document.title = title ? `${title} · ${task}` : `${project} · ${task}`;
		qr(href);
	},
	colors(top, bottom) {
		if (bars.styled) return;
		bars.top = top;
		bars.bottom = bottom;
		paint();
	},
	reserve() {
		device.classList.add('reserve');
	},
	call(plugin, method, options = {}) {
		if (method === 'removeListener') return void listeners.delete(options.callbackId);
		if (method === 'removeAllListeners') {
			for (const [id, listener] of listeners) if (listener.plugin === plugin) listeners.delete(id);
			return;
		}
		const run = plugins[plugin]?.[method];
		record(`${plugin}.${method}(${options && Object.keys(options).length ? JSON.stringify(options) : ''})`, !run);
		if (!run) {
			return Promise.reject({ message: `${plugin}.${method}() is not emulated in the Boreas frame.`, code: 'UNIMPLEMENTED' });
		}
		return Promise.resolve().then(() => run(options));
	},
	listen(plugin, event, callback) {
		const id = String(++nextListener);
		listeners.set(id, { plugin, event, callback });
		return id;
	}
};

iframe.addEventListener('load', () => {
	let inside = true;
	try {
		void iframe.contentWindow.location.href;
	} catch {
		inside = false;
	}
	$('.notice').hidden = inside;
	state.active = true;
	$('.home').hidden = true;
});
iframe.src = location.href;
qr(location.href);

document.addEventListener('click', (event) => {
	const swatch = event.target.closest('[data-finish]');
	if (swatch?.matches('button')) finish(swatch.dataset.finish);
	if (event.target.closest('[name=off]')) {
		try {
			sessionStorage.setItem('boreas-device-off:' + base, '1');
		} catch {
			// Without storage the frame cannot be turned off.
		}
		location.reload();
	}
	if (event.target.closest('[name=home]') || event.target.closest('.home .app')) lifecycle(!state.active);
});
$('[name=native]').addEventListener('change', (event) => {
	save('boreas-device:native:' + base, event.target.checked ? '1' : '0');
	location.reload();
});
$('form[name=push]').addEventListener('submit', (event) => {
	event.preventDefault();
	const form = new FormData(event.target);
	let data;
	try {
		data = form.get('data') ? JSON.parse(form.get('data')) : {};
	} catch {
		record('Push data must be JSON.', true);
		return;
	}
	const notification = { id: String(Date.now()), title: form.get('title'), body: form.get('body'), data };
	delivered.push(notification);
	banner(notification);
	if (state.active) emit('PushNotifications', 'pushNotificationReceived', notification);
	record(`push → ${JSON.stringify(notification)}`, false);
});
