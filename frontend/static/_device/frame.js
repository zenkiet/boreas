// The phone frame a desktop sees for a mobile project. shim.js builds it in place of the app's page and the app
// runs in the iframe, talking to window.__boreasDevice.
const [, project, task] = location.pathname.split('/');

const icons =
	'<svg viewBox="0 0 18 12" width="18" height="12"><rect y="8" width="3" height="4" rx="1"/><rect x="5" y="5.5" width="3" height="6.5" rx="1"/><rect x="10" y="3" width="3" height="9" rx="1"/><rect x="15" width="3" height="12" rx="1"/></svg>' +
	'<svg viewBox="0 0 16 12" width="16" height="12"><path d="M8 2.2c2.3 0 4.4.9 6 2.4l1.3-1.4A10.6 10.6 0 0 0 8 .3 10.6 10.6 0 0 0 .7 3.2L2 4.6a8.7 8.7 0 0 1 6-2.4Zm0 3.8c1.3 0 2.5.5 3.4 1.3l1.3-1.4A6.8 6.8 0 0 0 8 4.1a6.8 6.8 0 0 0-4.7 1.8l1.3 1.4C5.5 6.5 6.7 6 8 6Zm0 3.8a1.7 1.7 0 0 0-1.3.6L8 11.8l1.3-1.4A1.7 1.7 0 0 0 8 9.8Z"/></svg>' +
	'<svg viewBox="0 0 27 13" width="27" height="13"><rect x=".5" y=".5" width="23" height="12" rx="3.5" fill="none" stroke="currentColor" opacity=".4"/><rect x="2" y="2" width="20" height="9" rx="2"/><path d="M25 4.5v4a2 2 0 0 0 0-4Z" opacity=".4"/></svg>';

document.title = `${project} · ${task}`;
document.body.innerHTML = `
<div class="fit">
	<div class="device">
		<div class="screen">
			<iframe title="${project} ${task}"></iframe>
			<div class="statusbar"><span class="time"></span><span></span><span class="icons">${icons}</span></div>
			<div class="indicator"></div>
		</div>
		<img class="bezel" src="/_device/iphone-18-pro-max-black.png" alt="" draggable="false">
	</div>
</div>`;

const $ = (selector) => document.querySelector(selector);
const device = $('.device');

const fit = () => {
	const scale = Math.min(1, (innerHeight - 32) / 1000, (innerWidth - 32) / 490);
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

window.__boreasDevice = {
	insets: { top: 62, bottom: 34 },
	route(href, title) {
		if (href !== location.href) history.replaceState(null, '', href);
		document.title = title ? `${title} · ${task}` : `${project} · ${task}`;
	},
	colors(top, bottom) {
		device.classList.toggle('status-light', top);
		device.classList.toggle('indicator-light', bottom);
	},
	reserve() {
		device.classList.add('reserve');
	}
};

$('iframe').src = location.href;
