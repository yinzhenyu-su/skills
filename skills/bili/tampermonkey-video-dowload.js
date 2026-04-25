// ==UserScript==
// @name         哔哩视频下载
// @namespace    http://tampermonkey.net/
// @version      2026-04-25
// @description  B站视频流下载工具（DASH 视频+音频分流）
// @author       You
// @match        https://www.bilibili.com/video/*
// @icon         https://www.google.com/s2/favicons?sz=64&domain=bilibili.com
// @grant        GM_download
// @grant        GM_xmlhttpRequest
// @grant        GM_setClipboard
// @grant        unsafeWindow
// @run-at       document-idle
// ==/UserScript==

(function () {
	'use strict';

	const pageWindow = (typeof unsafeWindow !== 'undefined') ? unsafeWindow : window;

	let host = null;
	let titleObserver = null;

	function cleanup() {
		if (titleObserver) { titleObserver.disconnect(); titleObserver = null; }
		if (host) { host.remove(); host = null; }
	}

	function init() {
		cleanup();
		run();
	}

	function getVideoTitle() {
		const raw = document.title.replace(/_哔哩哔哩_.*$/, '').replace(/\s*[-|]\s*哔哩哔哩.*$/, '').trim() || 'video';
		return raw.replace(/[/\\:*?"<>|]/g, '_');
	}

	function dlFile(url, filename) {
		console.log('[bili-dl] download:', filename, url);
		GM_xmlhttpRequest({
			method: 'GET',
			url,
			headers: {
				Referer: 'https://www.bilibili.com',
				Origin: 'https://www.bilibili.com',
			},
			responseType: 'blob',
			onload: (res) => {
				const a = document.createElement('a');
				a.href = URL.createObjectURL(res.response);
				a.download = filename;
				document.body.appendChild(a);
				a.click();
				setTimeout(() => {
					URL.revokeObjectURL(a.href);
					a.remove();
				}, 1000);
			},
			onerror: (err) => {
				console.error('[bili-dl] download error', err);
				window.open(url, '_blank');
			},
		});
	}

	function waitForPlayinfo(cb, timeout = 10000) {
		const expectedUrl = location.href;
		// 先等 __playinfo__ 消失（B 站切换时会清掉），再等新数据出现
		const start = Date.now();
		const timer = setInterval(() => {
			if (Date.now() - start > timeout) {
				clearInterval(timer);
				console.error('[bili-dl] __playinfo__ not found after timeout');
				return;
			}
			if (typeof pageWindow.__playinfo__ !== 'undefined' && location.href === expectedUrl) {
				clearInterval(timer);
				cb();
			}
		}, 200);
	}

	waitForPlayinfo(init);

	// 监听 B 站 SPA 路由跳转
	let currentUrl = location.href;
	const onUrlChange = () => {
		if (location.href === currentUrl)
			return;
		currentUrl = location.href;
		if (/\/video\//.test(location.pathname)) {
			console.log('[bili-dl] URL changed, reinitializing...');
			waitForPlayinfo(init);
		}
		else {
			cleanup();
		}
	};

	// 拦截 pushState / replaceState
	['pushState', 'replaceState'].forEach((method) => {
		const orig = history[method];
		history[method] = function (...args) {
			const ret = orig.apply(this, args);
			onUrlChange();
			return ret;
		};
	});
	window.addEventListener('popstate', onUrlChange);

	function run() {
		const playInfo = pageWindow.__playinfo__;
		const title = getVideoTitle();
		const data = playInfo.data || {};
		const videoStreams = (data.dash && data.dash.video) || data.durl || [];
		const audioStreams = (data.dash && data.dash.audio) || [];
		const durlStreams = data.durl || [];

		if (!videoStreams.length && !durlStreams.length) {
			console.warn('[bili-dl] No video streams found');
			return;
		}

		const QUALITY_MAP = {
			127: '超高清 8K',
			126: '杜比视界',
			125: 'HDR 真彩',
			120: '超清 4K',
			116: '高清 1080P60',
			112: '高清 1080P+',
			80: '高清 1080P',
			74: '高清 720P60',
			64: '高清 720P',
			32: '清晰 480P',
			16: '流畅 360P',
		};

		const CODEC_MAP = { 7: 'AVC', 12: 'HEVC', 13: 'AV1' };

		const AUDIO_QUALITY_MAP = {
			30280: '高品质 (320kbps)',
			30232: '中品质 (128kbps)',
			30216: '低品质 (64kbps)',
		};

		// 按画质去重，保留带宽最高的
		const videoGroups = {};
		videoStreams.forEach((s) => {
			if (!videoGroups[s.id] || s.bandwidth > videoGroups[s.id].bandwidth) {
				videoGroups[s.id] = s;
			}
		});
		const bestVideoStreams = Object.values(videoGroups).sort((a, b) => b.id - a.id);
		const bestAudio = audioStreams.length
			? audioStreams.reduce((a, b) => a.bandwidth > b.bandwidth ? a : b)
			: null;

		/* ---- Shadow DOM（隔离 B 站样式） ---- */
		host = document.createElement('div');
		// 用 setAttribute 写 style 以最高优先级覆盖 B 站可能的全局 CSS
		host.setAttribute('style', [
			'position:fixed !important',
			'top:20px !important',
			'right:20px !important',
			'z-index:2147483647 !important',
			'display:block !important',
			'line-height:normal !important',
			'font-family:sans-serif !important',
			'font-size:14px !important',
			'pointer-events:auto !important',
			'visibility:visible !important',
			'opacity:1 !important',
		].join(';'));

		const shadow = host.attachShadow({ mode: 'open' });

		const style = document.createElement('style');
		style.textContent = `
        :host { all: initial; }

        .wrapper { position: relative; display: inline-block; }

        .main-btn {
            display: inline-flex; align-items: center; gap: 6px;
            padding: 8px 16px; cursor: pointer;
            background: #00aeec; color: #fff;
            border: none; border-radius: 4px;
            font-size: 14px; font-family: sans-serif; font-weight: bold;
            box-shadow: 0 2px 8px rgba(0,174,236,.35);
            transition: background 0.2s;
        }
        .main-btn:hover { background: #0099d4; }

        .panel {
            position: absolute; right: 0; top: calc(100% + 6px);
            min-width: 270px; max-height: 420px; overflow-y: auto;
            background: #fff; border: 1px solid #e2e2e2; border-radius: 6px;
            box-shadow: 0 6px 20px rgba(0,0,0,.15); padding: 8px;
            box-sizing: border-box;
            opacity: 0; transform: translateY(-6px);
            visibility: hidden; pointer-events: none;
            transition: opacity 0.3s ease, transform 0.3s ease, visibility 0.3s;
        }
        .panel.open {
            opacity: 1; transform: translateY(0);
            visibility: visible; pointer-events: auto;
        }

        .section-title {
            font-size: 11px; color: #999; padding: 4px 4px 2px;
            border-bottom: 1px solid #f0f0f0; margin-bottom: 4px;
            font-family: sans-serif;
        }

        .panel-item {
            display: flex; align-items: center; justify-content: space-between;
            width: 100%; box-sizing: border-box;
            padding: 7px 10px; margin: 2px 0;
            background: #f8f9fa; border: 1px solid #e9ecef; border-radius: 4px;
            font-size: 12px; font-family: sans-serif; color: #333; cursor: pointer;
            transition: background 0.15s, border-color 0.15s;
        }
        .panel-item:hover { background: #e8f4ff; border-color: #b3d7ff; color: #1677ff; }
        .panel-item .label { flex: 1; text-align: left; white-space: nowrap;
            overflow: hidden; text-overflow: ellipsis; }
        .panel-item .badge {
            font-size: 10px; background: #e6f0ff; color: #1677ff;
            border-radius: 3px; padding: 1px 5px; margin-left: 6px; flex-shrink: 0;
        }

        .audio-bar {
            margin-top: 6px; padding: 6px 8px;
            background: #f6ffed; border: 1px solid #b7eb8f; border-radius: 4px;
            font-size: 11px; font-family: sans-serif; color: #389e0d;
            display: flex; align-items: center; gap: 6px;
        }
        .audio-bar a {
            color: #389e0d; text-decoration: underline; cursor: pointer;
        }

        .hint-bar {
            margin-top: 6px; padding: 6px 8px;
            background: #fffbe6; border: 1px solid #ffe58f; border-radius: 4px;
            font-size: 11px; font-family: sans-serif; color: #874d00; line-height: 1.5;
        }
        .hint-bar code {
            display: block; margin-top: 4px; background: #f5f5f5;
            padding: 3px 6px; border-radius: 3px;
            font-family: monospace; font-size: 10px;
            word-break: break-all; cursor: pointer;
        }
        .hint-bar code:hover { background: #e8f4ff; }

        .video-title {
            padding: 6px 4px 8px;
            font-size: 12px; font-family: sans-serif; color: #333; font-weight: bold;
            border-bottom: 1px solid #f0f0f0; margin-bottom: 6px;
            white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
            max-width: 100%; box-sizing: border-box;
        }
    `;
		shadow.appendChild(style);

		const wrapper = document.createElement('div');
		wrapper.className = 'wrapper';

		const mainBtn = document.createElement('button');
		mainBtn.className = 'main-btn';
		mainBtn.innerHTML = '⬇ 下载视频';

		const panel = document.createElement('div');
		panel.className = 'panel';

		/* ---- 视频标题 ---- */
		const titleEl = document.createElement('div');
		titleEl.className = 'video-title';
		titleEl.title = title;
		titleEl.textContent = title;
		panel.appendChild(titleEl);

		// 监听 document.title 变化，实时更新面板标题
		titleObserver = new MutationObserver(() => {
			const updated = getVideoTitle();
			titleEl.title = updated;
			titleEl.textContent = updated;
		});
		titleObserver.observe(document.querySelector('title'), { childList: true });

		/* ---- 视频流列表 ---- */
		if (bestVideoStreams.length) {
			const sectionTitle = document.createElement('div');
			sectionTitle.className = 'section-title';
			sectionTitle.textContent = '视频流（需配合音频合并）';
			panel.appendChild(sectionTitle);

			bestVideoStreams.forEach((stream) => {
				const qualityName = QUALITY_MAP[stream.id] || `质量 ID ${stream.id}`;
				const codec = CODEC_MAP[stream.codecid] || `codec ${stream.codecid}`;
				const res = stream.width ? `${stream.width}×${stream.height}` : '';
				const url = stream.baseUrl || stream.base_url || '';

				const item = document.createElement('div');
				item.className = 'panel-item';
				item.innerHTML = `
                <span class="label">${qualityName}${res ? ` · ${res}` : ''}</span>
                <span class="badge">${codec}</span>
            `;
				item.title = bestAudio ? '点击下载并复制合并命令' : '点击下载';
				item.addEventListener('click', () => {
					if (url) {
						dlFile(url, `${title}_video.m4s`);
						if (bestAudio) {
							const cmd = `ffmpeg -i "${title}_video.m4s" -i "${title}_audio.m4s" -c copy "${title}.mp4"`;
							GM_setClipboard(cmd);
							const labelEl = item.querySelector('.label');
							const orig = labelEl.textContent;
							labelEl.textContent = '✓ 已复制合并命令';
							setTimeout(() => { labelEl.textContent = orig; }, 2000);
						}
					}
					else { alert('无法获取该流的有效 URL'); }
				});
				panel.appendChild(item);
			});
		}

		/* ---- durl 格式（非 DASH，视频音频合并） ---- */
		durlStreams.forEach((stream, i) => {
			const item = document.createElement('div');
			item.className = 'panel-item';
			const mb = stream.size ? `${Math.round(stream.size / 1024 / 1024)}MB` : '';
			item.innerHTML = `<span class="label">流 ${i + 1}（含音频）</span>
            ${mb ? `<span class="badge">${mb}</span>` : ''}`;
			item.addEventListener('click', () => {
				if (stream.url)
					dlFile(stream.url, `${title}.mp4`);
			});
			panel.appendChild(item);
		});

		/* ---- 音频流 ---- */
		if (audioStreams.length) {
			const sectionTitle = document.createElement('div');
			sectionTitle.className = 'section-title';
			sectionTitle.textContent = '音频流';
			panel.appendChild(sectionTitle);

			const sortedAudio = [...audioStreams].sort((a, b) => b.id - a.id);
			sortedAudio.forEach((audio) => {
				const audioUrl = audio.baseUrl || audio.base_url || '';
				const audioQuality = AUDIO_QUALITY_MAP[audio.id] || `音频 ID ${audio.id}`;

				const item = document.createElement('div');
				item.className = 'panel-item';
				item.innerHTML = `<span class="label">🎵 ${audioQuality}</span>`;
				item.title = '点击下载';
				item.addEventListener('click', () => {
					if (audioUrl)
						dlFile(audioUrl, `${title}_audio.m4s`);
				});
				panel.appendChild(item);
			});
		}

		/* ---- FFmpeg 合并提示 ---- */
		if (videoStreams.length && bestAudio) {
			const cmd = `ffmpeg -i "${title}_video.m4s" -i "${title}_audio.m4s" -c copy "${title}.mp4"`;
			const hint = document.createElement('div');
			hint.className = 'hint-bar';
			hint.innerHTML = `⚠ DASH 格式需分别下载视频和音频，再用 FFmpeg 合并：
            <code title="点击复制">${cmd}</code>`;
			hint.querySelector('code').addEventListener('click', (e) => {
				GM_setClipboard(cmd);
				e.target.textContent = '✓ 已复制！';
				setTimeout(() => { e.target.textContent = cmd; }, 2000);
			});
			panel.appendChild(hint);
		}

		let hideTimer = null;
		mainBtn.addEventListener('click', () => panel.classList.toggle('open'));
		wrapper.addEventListener('mouseleave', () => {
			hideTimer = setTimeout(() => panel.classList.remove('open'), 1000);
		});
		wrapper.addEventListener('mouseenter', () => {
			clearTimeout(hideTimer);
		});

		wrapper.appendChild(mainBtn);
		wrapper.appendChild(panel);
		shadow.appendChild(wrapper);
		document.body.appendChild(host);

		console.log('[bili-dl] injected, videos:', bestVideoStreams.length, 'audio:', !!bestAudio);
	} // end run()
})();
