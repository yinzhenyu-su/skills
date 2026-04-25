// ==UserScript==
// @name         哔哩视频下载
// @namespace    http://tampermonkey.net/
// @version      2026.4.25.211634
// @description  B站视频流下载工具（DASH 视频+音频分流）
// @author       yinzhenyu
// @homepage     https://github.com/yinzhenyu-su/skills
// @match        https://www.bilibili.com/video/*
// @icon         https://www.google.com/s2/favicons?sz=64&domain=bilibili.com
// @grant        GM_download
// @grant        GM_xmlhttpRequest
// @grant        GM_setClipboard
// @grant        unsafeWindow
// @run-at       document-idle
// @updateURL    https://raw.githubusercontent.com/yinzhenyu-su/skills/main/skills/bili/tampermonkey-video-dowload.js
// @downloadURL  https://raw.githubusercontent.com/yinzhenyu-su/skills/main/skills/bili/tampermonkey-video-dowload.js
// @license MIT
// ==/UserScript==

(function () {
	'use strict';

	const pageWindow = (typeof unsafeWindow !== 'undefined') ? unsafeWindow : window;

	let host = null;
	let titleObserver = null;
	let latestPlayInfo = null;
	let latestPlayInfoMeta = null;
	let activePlayinfoSignature = null;

	function cleanup() {
		if (titleObserver) { titleObserver.disconnect(); titleObserver = null; }
		if (host) { host.remove(); host = null; }
	}

	function clamp(value, min, max) {
		return Math.min(Math.max(value, min), max);
	}

	function getInitialHostPosition() {
		return {
			left: Math.max(window.innerWidth - 156, 16),
			top: 16,
		};
	}

	function mountHost() {
		if (!host || host.isConnected)
			return;

		document.body.appendChild(host);
	}

	function init() {
		const playInfo = getMatchedPlayInfo();
		if (!playInfo) {
			console.warn('[bili-dl] No matched playinfo found for current page');
			return;
		}
		cleanup();
		activePlayinfoSignature = getPlayinfoSignature(playInfo);
		run(playInfo);
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

	function getCurrentPageInfo() {
		const state = pageWindow.__INITIAL_STATE__ || {};
		const pathnameMatch = location.pathname.match(/\/video\/([^/?]+)/);
		const search = new URLSearchParams(location.search);
		const pageNo = Number.parseInt(search.get('p') || '1', 10) || 1;
		const pages = state.videoData && Array.isArray(state.videoData.pages) ? state.videoData.pages : [];
		const matchedPage = pages.find((item) => item.page === pageNo);
		return {
			bvid: pathnameMatch ? pathnameMatch[1] : (state.bvid || ''),
			cid: matchedPage ? String(matchedPage.cid) : (state.cid ? String(state.cid) : ''),
			pageNo,
		};
	}

	function extractPlayinfoMeta(playInfo, requestUrl = '') {
		const data = playInfo && playInfo.data;
		const url = new URL(requestUrl || location.href, location.origin);
		return {
			bvid: url.searchParams.get('bvid') || '',
			cid: data && data.cid ? String(data.cid) : (url.searchParams.get('cid') || ''),
		};
	}

	function isPlayinfoPayload(payload) {
		return !!(payload && payload.code === 0 && payload.data && (payload.data.dash || payload.data.durl));
	}

	function getMatchedPlayInfo() {
		const current = getCurrentPageInfo();

		if (latestPlayInfo && isPlayInfoForCurrentPage(latestPlayInfoMeta, current))
			return latestPlayInfo;

		const pagePlayInfo = pageWindow.__playinfo__;
		if (!isPlayinfoPayload(pagePlayInfo))
			return null;

		if (activePlayinfoSignature === null)
			return pagePlayInfo;

		const meta = extractPlayinfoMeta(pagePlayInfo);
		return isPlayInfoForCurrentPage(meta, current) ? pagePlayInfo : null;
	}

	function isPlayInfoForCurrentPage(meta, current = getCurrentPageInfo()) {
		if (!meta)
			return false;

		if (current.cid && meta.cid)
			return current.cid === meta.cid;

		if (current.bvid && meta.bvid)
			return current.bvid === meta.bvid;

		return false;
	}

	function storePlayInfo(playInfo, requestUrl = '') {
		if (!isPlayinfoPayload(playInfo))
			return;

		latestPlayInfo = playInfo;
		latestPlayInfoMeta = extractPlayinfoMeta(playInfo, requestUrl);

		if (isPlayInfoForCurrentPage(latestPlayInfoMeta)) {
			const signature = getPlayinfoSignature(playInfo);
			if (signature && signature !== activePlayinfoSignature) {
				console.log('[bili-dl] playurl updated, refreshing panel...');
				init();
			}
		}
	}

	function getPlayinfoSignature(playInfo) {
		if (!playInfo || !playInfo.data)
			return null;

		const data = playInfo.data;
		const dashVideo = data.dash && data.dash.video && data.dash.video[0];
		const durlVideo = data.durl && data.durl[0];
		const videoUrl = (dashVideo && (dashVideo.baseUrl || dashVideo.base_url)) || (durlVideo && durlVideo.url) || '';
		const acceptQuality = data.accept_quality ? data.accept_quality.join(',') : '';
		return [data.cid || '', data.quality || '', acceptQuality, videoUrl].join('|');
	}

	function waitForPlayinfo(cb, timeout = 10000, previousSignature = null) {
		// 页面内切换后优先等待新的 playurl 响应，其次回退到首屏内联的 __playinfo__
		const start = Date.now();
		const timer = setInterval(() => {
			if (Date.now() - start > timeout) {
				clearInterval(timer);
				console.error('[bili-dl] playinfo not found after timeout');
				return;
			}
			const playInfo = getMatchedPlayInfo();
			const currentSignature = getPlayinfoSignature(playInfo);
			if (currentSignature && currentSignature !== previousSignature) {
				clearInterval(timer);
				cb();
			}
		}, 200);
	}

	function hookPlayurlResponses() {
		const PLAYURL_RE = /\/x\/player\/(wbi\/)?playurl/i;

		const originalFetch = window.fetch;
		if (typeof originalFetch === 'function') {
			window.fetch = async function (...args) {
				const response = await originalFetch.apply(this, args);
				try {
					const requestUrl = typeof args[0] === 'string' ? args[0] : (args[0] && args[0].url) || '';
					if (PLAYURL_RE.test(requestUrl)) {
						response.clone().json().then((payload) => {
							storePlayInfo(payload, requestUrl);
						}).catch(() => { });
					}
				}
				catch (error) {
					console.warn('[bili-dl] fetch hook failed', error);
				}
				return response;
			};
		}

		const originalOpen = XMLHttpRequest.prototype.open;
		const originalSend = XMLHttpRequest.prototype.send;
		XMLHttpRequest.prototype.open = function (method, url, ...rest) {
			this.__biliDlUrl = url;
			return originalOpen.call(this, method, url, ...rest);
		};
		XMLHttpRequest.prototype.send = function (...args) {
			this.addEventListener('load', () => {
				try {
					const requestUrl = this.__biliDlUrl || '';
					if (!PLAYURL_RE.test(requestUrl) || typeof this.responseText !== 'string')
						return;

					storePlayInfo(JSON.parse(this.responseText), requestUrl);
				}
				catch (error) {
					console.warn('[bili-dl] xhr hook failed', error);
				}
			});
			return originalSend.apply(this, args);
		};
	}

	storePlayInfo(pageWindow.__playinfo__);
	hookPlayurlResponses();
	waitForPlayinfo(init);

	// 监听 B 站 SPA 路由跳转
	let currentUrl = location.href;
	const onUrlChange = () => {
		if (location.href === currentUrl)
			return;
		const previousSignature = activePlayinfoSignature;
		currentUrl = location.href;
		if (/\/video\//.test(location.pathname)) {
			console.log('[bili-dl] URL changed, reinitializing...');
			waitForPlayinfo(init, 10000, previousSignature);
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

	function run(playInfo) {
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
		host.className = 'bili-dl-entry';
		const initialPosition = getInitialHostPosition();
		// 用 setAttribute 写 style 以最高优先级覆盖 B 站可能的全局 CSS
		host.setAttribute('style', [
			'display:flex !important',
			'align-items:center !important',
			'flex-shrink:0 !important',
			'position:fixed !important',
			`left:${initialPosition.left}px !important`,
			`top:${initialPosition.top}px !important`,
			'overflow:visible !important',
			'z-index:2147483647 !important',
			'height:auto !important',
			'margin:0 !important',
			'line-height:normal !important',
			'font-family:sans-serif !important',
			'font-size:13px !important',
			'pointer-events:auto !important',
			'visibility:visible !important',
			'opacity:1 !important',
		].join(';'));

		const shadow = host.attachShadow({ mode: 'open' });

		const style = document.createElement('style');
		style.textContent = `
        :host { all: initial; }

		.wrapper { position: relative; display: flex; align-items: center; height: 28px; overflow: visible; }

        .main-btn {
			display: inline-flex; align-items: center; gap: 4px;
			height: 28px; padding: 0 10px; cursor: pointer;
			background: #f6f7f8; color: #18191c;
			border: 1px solid #e3e5e7; border-radius: 14px;
			font-size: 13px; font-family: sans-serif; font-weight: 500;
			white-space: nowrap;
			box-sizing: border-box;
			transition: background 0.2s, border-color 0.2s, color 0.2s;
        }
		.main-btn:hover {
			background: #e8f3ff;
			border-color: #91caff;
			color: #1677ff;
		}

        .panel {
			position: absolute; right: 0; top: calc(100% + 8px);
			width: 400px; max-width: min(400px, calc(100vw - 24px));
			min-width: 270px; max-height: 420px; overflow-y: auto; overflow-x: hidden;
            background: #fff; border: 1px solid #e2e2e2; border-radius: 6px;
			box-shadow: 0 10px 30px rgba(0,0,0,.18); padding: 8px;
            box-sizing: border-box;
			z-index: 2147483647;
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
			white-space: pre-wrap; overflow-wrap: anywhere; word-break: break-word;
			cursor: pointer;
        }
        .hint-bar code:hover { background: #e8f4ff; }

        .video-title {
            padding: 6px 4px 8px;
            font-size: 12px; font-family: sans-serif; color: #333; font-weight: bold;
            border-bottom: 1px solid #f0f0f0; margin-bottom: 6px;
			white-space: normal; overflow-wrap: anywhere; word-break: break-word;
			line-height: 1.5;
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
			const codeEl = hint.querySelector('code');
			codeEl.__biliDlCmd = cmd;
			codeEl.addEventListener('click', () => {
				GM_setClipboard(cmd);
				codeEl.textContent = '✓ 已复制！';
				clearTimeout(codeEl.__biliDlRestoreTimer);
				codeEl.__biliDlRestoreTimer = setTimeout(() => {
					codeEl.textContent = codeEl.__biliDlCmd;
				}, 2000);
			});
			panel.appendChild(hint);
		}

		let hideTimer = null;
		let dragState = null;
		let suppressClick = false;
		mainBtn.addEventListener('click', (event) => {
			if (suppressClick) {
				suppressClick = false;
				event.preventDefault();
				event.stopPropagation();
				return;
			}
			panel.classList.toggle('open');
		});

		mainBtn.addEventListener('pointerdown', (event) => {
			if (event.button !== 0)
				return;

			const rect = host.getBoundingClientRect();
			dragState = {
				startX: event.clientX,
				startY: event.clientY,
				originLeft: rect.left,
				originTop: rect.top,
				dragged: false,
				pointerId: event.pointerId,
			};
			mainBtn.setPointerCapture(event.pointerId);
		});

		mainBtn.addEventListener('pointermove', (event) => {
			if (!dragState || event.pointerId !== dragState.pointerId)
				return;

			const deltaX = event.clientX - dragState.startX;
			const deltaY = event.clientY - dragState.startY;
			if (!dragState.dragged && Math.hypot(deltaX, deltaY) > 4)
				dragState.dragged = true;

			if (!dragState.dragged)
				return;

			event.preventDefault();
			panel.classList.remove('open');
			const nextLeft = clamp(dragState.originLeft + deltaX, 8, Math.max(window.innerWidth - host.offsetWidth - 8, 8));
			const nextTop = clamp(dragState.originTop + deltaY, 8, Math.max(window.innerHeight - host.offsetHeight - 8, 8));
			host.style.left = `${nextLeft}px`;
			host.style.top = `${nextTop}px`;
		});

		const stopDragging = (event) => {
			if (!dragState || event.pointerId !== dragState.pointerId)
				return;

			if (mainBtn.hasPointerCapture(event.pointerId))
				mainBtn.releasePointerCapture(event.pointerId);
			suppressClick = dragState.dragged;
			dragState = null;
		};

		mainBtn.addEventListener('pointerup', stopDragging);
		mainBtn.addEventListener('pointercancel', stopDragging);
		wrapper.addEventListener('mouseleave', () => {
			hideTimer = setTimeout(() => panel.classList.remove('open'), 1000);
		});
		wrapper.addEventListener('mouseenter', () => {
			clearTimeout(hideTimer);
		});

		wrapper.appendChild(mainBtn);
		wrapper.appendChild(panel);
		shadow.appendChild(wrapper);

		mountHost();

		console.log('[bili-dl] injected, videos:', bestVideoStreams.length, 'audio:', !!bestAudio);
	} // end run()
})();
