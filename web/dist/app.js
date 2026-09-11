// Auto-detect secret path prefix from current browser location (e.g. /mysecret or /enter)
        const apiPrefix = (() => {
            const parts = window.location.pathname.split('/').filter(Boolean);
            if (parts.length > 0 && parts[0] !== 'api' && parts[0] !== 'metrics') {
                return '/' + parts[0];
            }
            return '';
        })();
        window.__apiPrefix = apiPrefix;

        // Automatically route all /api/ and /metrics requests through the current secret path prefix
        // so that the browser always sends Basic Auth credentials and avoids cross-path 404s
        const originalFetch = window.fetch;
        window.fetch = async function(input, init) {
            if (typeof input === 'string' && apiPrefix) {
                if (input.startsWith('/api/') || input === '/api' || input.startsWith('/metrics')) {
                    input = apiPrefix + input;
                }
            }
            try {
                const response = await originalFetch(input, init);
                if (response.status >= 500) {
                    showAppAlert(`服务返回 ${response.status}，部分数据可能未更新。`);
                } else if (response.ok) {
                    clearAppAlert();
                }
                return response;
            } catch (error) {
                showAppAlert('无法连接管理服务，请检查网络或服务状态。');
                throw error;
            }
        };

        let allNodes = [];
        let currentState = null;
        let cachedUnlockMap = {};
        let activeQuickFilter = 'all';

        const countryNames = {
            JP: '日本', US: '美国', KR: '韩国', TW: '台湾', HK: '香港',
            SG: '新加坡', GB: '英国', DE: '德国', FR: '法国', CA: '加拿大',
            AU: '澳大利亚', VN: '越南', TH: '泰国', MY: '马来西亚', IN: '印度',
            RU: '俄罗斯', NL: '荷兰', BR: '巴西', PH: '菲律宾', ID: '印尼'
        };

        const flagSVGs = {
            JP: '<svg aria-hidden="true" class="flag-img" viewBox="0 0 900 600"><rect fill="#fff" width="900" height="600"/><circle fill="#bc002d" cx="450" cy="300" r="180"/></svg>',
            US: '<svg aria-hidden="true" class="flag-img" viewBox="0 0 7410 3900"><rect fill="#b22234" width="7410" height="3900"/><path stroke="#fff" stroke-width="300" d="M0,450H7410M0,1050H7410M0,1650H7410M0,2250H7410M0,2850H7410M0,3450H7410"/><rect fill="#3c3b6e" width="2964" height="2100"/></svg>',
            KR: '<svg aria-hidden="true" class="flag-img" viewBox="0 0 900 600"><rect fill="#fff" width="900" height="600"/><path fill="#cd2e3a" d="M450,150a150,150 0 0,1 0,300a75,75 0 0,1 0,-150z"/><path fill="#0047a0" d="M450,450a150,150 0 0,1 0,-300a75,75 0 0,1 0,150z"/></svg>',
            TW: '<svg aria-hidden="true" class="flag-img" viewBox="0 0 900 600"><rect fill="#fe0000" width="900" height="600"/><rect fill="#000095" width="450" height="300"/><circle fill="#fff" cx="225" cy="150" r="75"/></svg>',
            HK: '<svg aria-hidden="true" class="flag-img" viewBox="0 0 900 600"><rect fill="#c8102e" width="900" height="600"/><circle fill="#fff" cx="450" cy="300" r="110" opacity="0.9"/></svg>',
            SG: '<svg aria-hidden="true" class="flag-img" viewBox="0 0 900 600"><rect fill="#ed2939" width="900" height="300"/><rect fill="#fff" y="300" width="900" height="300"/><circle fill="#fff" cx="225" cy="150" r="100"/><circle fill="#ed2939" cx="265" cy="150" r="100"/></svg>',
            DE: '<svg aria-hidden="true" class="flag-img" viewBox="0 0 5 3"><rect width="5" height="1" y="0" fill="#000"/><rect width="5" height="1" y="1" fill="#D00"/><rect width="5" height="1" y="2" fill="#FFCE00"/></svg>',
            FR: '<svg aria-hidden="true" class="flag-img" viewBox="0 0 900 600"><rect fill="#002395" width="300" height="600"/><rect fill="#fff" x="300" width="300" height="600"/><rect fill="#ed2939" x="600" width="300" height="600"/></svg>',
            GB: '<svg aria-hidden="true" class="flag-img" viewBox="0 0 60 30"><rect fill="#012169" width="60" height="30"/><path d="M0,0 L60,30 M60,0 L0,30" stroke="#fff" stroke-width="6"/><path d="M0,0 L60,30 M60,0 L0,30" stroke="#C8102E" stroke-width="4"/><path d="M30,0 v30 M0,15 h60" stroke="#fff" stroke-width="10"/><path d="M30,0 v30 M0,15 h60" stroke="#C8102E" stroke-width="6"/></svg>',
            CA: '<svg aria-hidden="true" class="flag-img" viewBox="0 0 900 450"><rect fill="#f00" width="225" height="450"/><rect fill="#fff" x="225" width="450" height="450"/><rect fill="#f00" x="675" width="225" height="450"/><polygon fill="#f00" points="450,112 470,170 515,160 480,200 500,240 450,225 400,240 420,200 385,160 430,170"/></svg>',
            VN: '<svg aria-hidden="true" class="flag-img" viewBox="0 0 900 600"><rect fill="#da251d" width="900" height="600"/><polygon fill="#ffff00" points="450,140 487,254 607,254 510,324 547,438 450,368 353,438 390,324 293,254 413,254"/></svg>',
            TH: '<svg aria-hidden="true" class="flag-img" viewBox="0 0 900 600"><rect fill="#a51931" width="900" height="600"/><rect fill="#f4f5f8" y="100" width="900" height="400"/><rect fill="#2d2a4a" y="200" width="900" height="200"/></svg>'
        };

        function getCountryFlagSVG(code) {
            code = (code || '').toUpperCase();
            if (flagSVGs[code]) return flagSVGs[code];
            return '<svg aria-hidden="true" class="flag-img flag-fallback" viewBox="0 0 24 24"><circle cx="12" cy="12" r="10" stroke="#64748b" fill="none"/><line x1="2" y1="12" x2="22" y2="12" stroke="#64748b"/></svg>';
        }

        function getCountryName(code) { return countryNames[code] || code; }

        function escapeHtml(value) {
            return String(value ?? '').replace(/[&<>"']/g, c => ({
                '&': '&amp;',
                '<': '&lt;',
                '>': '&gt;',
                '"': '&quot;',
                "'": '&#39;'
            })[c]);
        }

        function jsonAttr(value) {
            return escapeHtml(JSON.stringify(value));
        }

        function safeLogLevel(level) {
            const value = String(level || 'info').toLowerCase();
            return ['info', 'warning', 'error'].includes(value) ? value : 'info';
        }

        function formatBytes(bytes) {
            if (!bytes || bytes === 0) return '0 B';
            const k = 1024;
            const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
            const i = Math.floor(Math.log(bytes) / Math.log(k));
            return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
        }

        function formatSpeed(bps) { return formatBytes(bps) + '/s'; }

        function appendLog(entry) {
            const term = document.getElementById('terminal');
            const row = document.createElement('div');
            row.className = 'log-line';
            const timeStr = entry.timestamp ? new Date(entry.timestamp).toLocaleTimeString() : new Date().toLocaleTimeString();
            row.innerHTML = `<span class="log-time">[${escapeHtml(timeStr)}]</span> <span class="log-level-${safeLogLevel(entry.level)}">[${escapeHtml(entry.module || 'System')}]</span> ${escapeHtml(entry.message)}`;
            term.appendChild(row);
            term.scrollTop = term.scrollHeight;
        }

        function clearLogs() { document.getElementById('terminal').innerHTML = ''; }

        function renderUnlockBadges(u) {
            if (!u) return '<span  class="text-xs text-muted">-</span>';
            const badges = [];
            if (u.openai === 'unlocked') badges.push('<span class="badge unlock-badge unlock-open" title="OpenAI / ChatGPT 解锁正常">GPT 可用</span>');
            else if (u.openai === 'blocked') badges.push('<span class="badge unlock-badge unlock-blocked" title="OpenAI 阻断拦截">GPT 阻断</span>');

            if (u.claude === 'unlocked') badges.push('<span class="badge unlock-badge unlock-open" title="Claude / Anthropic 访问正常">Claude 可用</span>');
            else if (u.claude === 'blocked') badges.push('<span class="badge unlock-badge unlock-blocked" title="Claude 风控拦截">Claude 阻断</span>');

            if (u.netflix === 'unlocked') badges.push('<span class="badge unlock-badge unlock-warn" title="Netflix 原生流媒体解锁">NF 可用</span>');
            else if (u.netflix === 'blocked') badges.push('<span class="badge unlock-badge unlock-blocked" title="Netflix 限制访问">NF 限制</span>');

            if (u.google === 'unlocked') badges.push('<span class="badge unlock-badge text-accent" title="Google Search 干净无验证码">Google 可用</span>');
            else if (u.google === 'blocked') badges.push('<span class="badge unlock-badge unlock-blocked" title="Google 出现验证码异常">Google 验证</span>');

            if (badges.length === 0) return '<span class="text-xs text-muted">-</span>';
            return `<div class="unlock-badges">${badges.join('')}</div>`;
        }

        async function fetchUnlockCache() {
            try {
                const res = await fetch('/api/unlock');
                if (res.ok) cachedUnlockMap = await res.json() || {};
            } catch(e) {}
        }

        async function probeTunnelUnlock(tunnelId) {
            appendLog({ level: 'INFO', module: 'Unlock', message: `正在对隧道 ${tunnelId} 启动流媒体与 AI 解锁探测...` });
            try {
                await fetch('/api/unlock/probe', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ tunnel_id: tunnelId })
                });
                setTimeout(async () => {
                    await fetchUnlockCache();
                    fetchStatus();
                }, 3500);
            } catch(err) {
                alert('探测请求失败: ' + err);
            }
        }

        async function testTelegramAlert() {
            try {
                const res = await fetch('/api/telegram/test', { method: 'POST' });
                const ret = await res.json();
                if (res.ok) alert(ret.message || '测试消息发送成功！');
                else alert('测试失败: ' + (ret.error || '未知错误'));
            } catch(err) {
                alert('请求失败: ' + err);
            }
        }

        async function fetchStatus() {
            try {
                const res = await fetch('/api/status');
                if (!res.ok) return;
                const data = await res.json();
                currentState = data;

                const vpn = data.vpn || {};
                const traffic = data.traffic || {};

                if (data.version) {
                    const vStr = 'v' + data.version.replace(/^v/, '');
                    const vBadge = document.getElementById('app-version-badge');
                    if (vBadge) vBadge.innerText = vStr;
                    const sbVer = document.getElementById('sidebar-version-badge');
                    if (sbVer) sbVer.innerText = vStr;
                }
                const sbPort = document.getElementById('sidebar-proxy-port');
                if (sbPort && data.proxy_addr) {
                    sbPort.innerText = data.proxy_addr.split(':').pop();
                }
                const portBadge = document.getElementById('nav-port-badge');
                if (portBadge) {
                    portBadge.innerText = (data.port_rules || []).length || 1;
                }

                // Speed stats
                document.getElementById('stat-down-speed').innerText = formatSpeed(traffic.download_speed_bps || 0);
                document.getElementById('stat-up-speed').innerText = formatSpeed(traffic.upload_speed_bps || 0);
                document.getElementById('stat-total-down').innerText = '总计下载: ' + formatBytes(traffic.total_download_bytes || 0);
                document.getElementById('stat-total-up').innerText = '总计上传: ' + formatBytes(traffic.total_upload_bytes || 0);
                document.getElementById('stat-active-conns').innerText = traffic.active_connections || 0;
                document.getElementById('stat-proxy-port').innerText = `默认端口: ${data.proxy_addr ? data.proxy_addr.split(':').pop() : '7928'}`;

                // Status Badge & Buttons
                const badge = document.getElementById('conn-badge');
                const btnConnect = document.getElementById('btn-quick-connect');
                const btnDisconnect = document.getElementById('btn-disconnect');

                badge.className = 'badge ' + (vpn.status || 'disconnected');
                if (vpn.status === 'connected') {
                    badge.innerHTML = '<span class="status-dot"></span> 已连通就绪';
                    btnConnect.classList.add('hidden');
                    btnDisconnect.classList.remove('hidden');
                } else if (vpn.status === 'connecting' || vpn.status === 'reconnecting') {
                    badge.innerHTML = '<span class="status-dot"></span> 正在连接中...';
                    btnConnect.disabled = true;
                    btnDisconnect.classList.remove('hidden');
                } else {
                    badge.innerHTML = '<span class="status-dot"></span> 未连接';
                    btnConnect.classList.remove('hidden');
                    btnConnect.disabled = false;
                    btnDisconnect.classList.add('hidden');
                }

                // Info card
                document.getElementById('vpn-status-text').innerText = vpn.status_text || vpn.status || '未连接';
                document.getElementById('vpn-node-ip').innerText = vpn.active_node ? `${vpn.active_node.ip}:${vpn.active_node.port}` : '-';

                if (vpn.active_node) {
                    let typeBadge = '<span class="badge badge-residential">住宅宽带</span>';
                    if (vpn.active_node.ip_type === 'hosting') {
                        typeBadge = '<span class="badge badge-hosting">机房网络</span>';
                    } else if (vpn.active_node.ip_type === 'mobile') {
                        typeBadge = '<span class="badge badge-mobile">移动网络</span>';
                    }
                    document.getElementById('vpn-node-type').innerHTML = `${typeBadge} ${escapeHtml(vpn.active_node.isp || '')}`;
                    const loc = [getCountryName(vpn.active_node.country_short), vpn.active_node.region, vpn.active_node.city].filter(Boolean).join(' · ');
                    document.getElementById('vpn-node-country').innerHTML = `
                        <span class="flag-box">${getCountryFlagSVG(vpn.active_node.country_short)} ${escapeHtml(loc)} (${escapeHtml(vpn.active_node.country_short)})</span>
                    `;
                } else {
                    document.getElementById('vpn-node-type').innerText = '-';
                    document.getElementById('vpn-node-country').innerText = '-';
                }

                document.getElementById('vpn-proxy-addr').innerText = data.proxy_addr ? `${data.proxy_addr} (HTTP/SOCKS5)` : '-';
                document.getElementById('vpn-last-msg').innerText = vpn.last_message || '服务正常运行中';

                if (vpn.uptime_seconds > 0) {
                    const m = Math.floor(vpn.uptime_seconds / 60);
                    const s = vpn.uptime_seconds % 60;
                    document.getElementById('vpn-uptime').innerText = `已连接运行: ${m}分${s}秒`;
                } else {
                    document.getElementById('vpn-uptime').innerText = '运行时间: -';
                }

                const totalStr = data.total_node_count && data.total_node_count > data.node_count ? ` (历史全库: ${data.total_node_count})` : '';
                document.getElementById('stat-nodes-count').innerText = `${data.node_count || 0}${totalStr} / 屏蔽 ${data.blacklist_count || 0}`;
                document.getElementById('stat-node-source').innerText = `数据源: ${data.node_source || '未知'}`;

                // Render active tunnels strip sorted stably by virtual interface index (tun0, tun1, tun2...)
                const tunnels = (data.tunnels || []).sort((a, b) => {
                    const idxA = parseInt((a.dev_name || '').replace(/\D+/g, '')) || 0;
                    const idxB = parseInt((b.dev_name || '').replace(/\D+/g, '')) || 0;
                    return idxA - idxB;
                });
                document.getElementById('tunnels-count').innerText = tunnels.length;
                const tunListEl = document.getElementById('active-tunnels-list');
                if (tunListEl) {
                    if (tunnels.length === 0) {
                        tunListEl.innerHTML = '<div  class="empty-copy">暂无独立并发出口，在下方节点列表中点击「+并发」即可多节点同时在线</div>';
                    } else {
                        tunListEl.innerHTML = tunnels.map(t => {
                            const cCode = t.node ? t.node.country_short : '';
                            const flag = cCode ? getCountryFlagSVG(cCode) : '';
                            const ip = t.node ? `${t.node.ip}:${t.node.port}` : '';
                            const isUp = t.status === 'connected';
                            const badgeClass = isUp ? 'connected' : (t.status === 'connecting' ? 'connecting' : 'disconnected');
                            const unlockBadges = renderUnlockBadges(t.unlock || (t.node ? cachedUnlockMap[t.node.ip] : null));
                            const pingStr = t.node && t.node.latency_ms > 0 ? `<span class="ping-ok">${t.node.latency_ms}ms</span>` : '';
                            return `
                                <div class="tunnel-chip">
                                    <strong  class="mono text-accent">${escapeHtml(t.dev_name)}</strong>
                                    <span class="badge ${badgeClass} badge-mini" ><span class="status-dot"></span> ${escapeHtml(t.status === 'connected' ? '在线' : t.status)}</span>
                                    <span class="flag-box text-note">${flag} ${escapeHtml(ip)}</span>
                                    ${pingStr}
                                    <div class="tunnel-flags">${unlockBadges}</div>
                                    <button class="btn btn-outline btn-xs" data-action="probeTunnelUnlock" data-args="${jsonAttr([t.id])}" title="探测该出口的AI与流媒体解锁状态">测解锁</button>
                                    <button class="btn btn-danger btn-xs" data-action="stopTunnel" data-args="${jsonAttr([t.id])}">断开</button>
                                </div>
                            `;
                        }).join('');
                    }
                }

            } catch (err) {
                console.error("fetch status error:", err);
            }
        }

        async function fetchNodes() {
            try {
                const res = await fetch('/api/nodes');
                if (!res.ok) return;
                allNodes = await res.json();
                const nBadge = document.getElementById('nav-node-badge');
                if (nBadge) {
                    nBadge.innerText = allNodes.length;
                }
                updateCountryFilter();
                renderNodes();
                document.getElementById('nodes-table-wrap')?.setAttribute('aria-busy', 'false');
            } catch (err) {
                console.error("fetch nodes error:", err);
                document.getElementById('nodes-table-wrap')?.setAttribute('aria-busy', 'false');
            }
        }

        function updateCountryFilter() {
            const select = document.getElementById('country-filter');
            const curr = select.value;
            const countries = [...new Set(allNodes.map(n => n.country_short))].filter(Boolean).sort();

            select.innerHTML = '<option value="">全部国家/地区</option>' +
                countries.map(c => `<option value="${escapeHtml(c)}">${escapeHtml(getCountryName(c))} (${escapeHtml(c)})</option>`).join('');
            select.value = curr;
        }

        let currentSort = 'latency_asc';

        function toggleSort(field) {
            if (field === 'latency') {
                currentSort = currentSort === 'latency_asc' ? 'score_desc' : 'latency_asc';
            } else if (field === 'speed') {
                currentSort = currentSort === 'speed_desc' ? 'latency_asc' : 'speed_desc';
            } else if (field === 'score') {
                currentSort = currentSort === 'score_desc' ? 'latency_asc' : 'score_desc';
            }
            document.getElementById('sort-filter').value = currentSort;
            renderNodes();
        }

        function selectQuickFilter(filter) {
            activeQuickFilter = filter;
            document.querySelectorAll('.chip').forEach(c => c.classList.remove('active'));
            const el = document.getElementById('chip-' + filter);
            if (el) el.classList.add('active');
            renderNodes();
        }

        async function toggleFavorite(nodeId) {
            try {
                const res = await fetch('/api/nodes/favorite', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ node_id: nodeId })
                });
                const ret = await res.json();
                if (res.ok) {
                    const node = allNodes.find(n => n.id === nodeId);
                    if (node) {
                        node.is_favorite = ret.is_favorite;
                        renderNodes();
                    }
                }
            } catch (err) {}
        }

        function renderNodes() {
            const tbody = document.getElementById('nodes-tbody');
            const search = document.getElementById('search-filter').value.toLowerCase();
            const country = document.getElementById('country-filter').value;
            const ipType = document.getElementById('ip-type-filter').value;
            const sortMode = document.getElementById('sort-filter').value;

            const favCount = allNodes.filter(n => n.is_favorite).length;
            const favCountEl = document.getElementById('fav-count');
            if (favCountEl) favCountEl.innerText = favCount;

            const filtered = allNodes.filter(n => {
                // Quick Chips Filter Logic
                if (activeQuickFilter === 'fav' && !n.is_favorite) return false;
                if (activeQuickFilter === 'res' && n.ip_type !== 'residential') return false;
                if (activeQuickFilter === 'host' && n.ip_type !== 'hosting') return false;
                if (activeQuickFilter === 'fast' && (n.latency_ms <= 0 || n.latency_ms >= 150)) return false;
                if (activeQuickFilter === 'gpt') {
                    const u = n.unlock || cachedUnlockMap[n.ip];
                    if (!u || u.openai !== 'unlocked') return false;
                }
                if (activeQuickFilter === 'nf') {
                    const u = n.unlock || cachedUnlockMap[n.ip];
                    if (!u || u.netflix !== 'unlocked') return false;
                }

                // Standard Filters
                if (country && n.country_short !== country) return false;
                if (ipType && n.ip_type !== ipType) return false;
                if (search) {
                    const cName = getCountryName(n.country_short).toLowerCase();
                    const str = `${n.ip} ${n.country_long} ${n.country_short} ${cName} ${n.hostname} ${n.isp || ''} ${n.city || ''} ${n.region || ''}`.toLowerCase();
                    if (!str.includes(search)) return false;
                }
                return true;
            });

            filtered.sort((a, b) => {
                if (sortMode === 'latency_asc') {
                    const aAvail = a.latency_ms > 0 ? 1 : (a.latency_ms === -1 ? -1 : 0);
                    const bAvail = b.latency_ms > 0 ? 1 : (b.latency_ms === -1 ? -1 : 0);
                    if (aAvail !== bAvail) return bAvail - aAvail;
                    if (a.latency_ms > 0 && b.latency_ms > 0) return a.latency_ms - b.latency_ms;
                    return b.score - a.score;
                } else if (sortMode === 'speed_desc') {
                    return b.speed - a.speed;
                } else if (sortMode === 'score_desc') {
                    return b.score - a.score;
                } else if (sortMode === 'ping_asc') {
                    return a.ping - b.ping;
                }
                return 0;
            });

            document.getElementById('filtered-count').innerText = `已筛选出 ${filtered.length} / ${allNodes.length} 个节点`;

            if (filtered.length === 0) {
                tbody.innerHTML = '<tr><td colspan="9"  class="empty-state">未找到匹配条件的节点</td></tr>';
                return;
            }

            tbody.innerHTML = filtered.map(n => {
                const isCurrent = currentState && currentState.vpn && currentState.vpn.active_node_id === n.id;
                const activeTun = (currentState && currentState.tunnels) ? currentState.tunnels.find(t => t.node && (t.node.id === n.id || t.node.ip === n.ip)) : null;

                let latencyBadge = '';
                if (n.latency_ms > 0) {
                    latencyBadge = `<span class="badge latency-badge latency-available"><span class="status-dot"></span> 可用 ${n.latency_ms} ms</span>`;
                } else if (n.latency_ms === -1) {
                    latencyBadge = `<span class="badge latency-badge latency-timeout"><span class="status-dot"></span> 超时不可达</span>`;
                } else {
                    latencyBadge = `<span class="badge latency-badge latency-unknown"><span class="status-dot"></span> 未测速</span>`;
                }

                const speedMbps = (n.speed / 1000000).toFixed(1) + ' Mbps';

                let typeTag = '<span class="badge badge-residential">住宅宽带</span>';
                if (n.ip_type === 'hosting') {
                    typeTag = '<span class="badge badge-hosting">机房网络</span>';
                } else if (n.ip_type === 'mobile') {
                    typeTag = '<span class="badge badge-mobile">移动网络</span>';
                }

                const ispInfo = n.isp ? `<div class="node-isp" title="${escapeHtml(n.isp)}">${escapeHtml(n.isp)}</div>` : '';
                const cityInfo = n.city ? `<span class="node-city"> · ${escapeHtml(n.city)}</span>` : '';

                const repVal = n.reputation_score !== undefined ? n.reputation_score : 60;
                const repClass = repVal < 45 ? 'rep-bad' : (repVal < 70 ? 'rep-warn' : 'rep-good');
                const repBadge = `<span class="badge reputation-badge ${repClass}">${repVal}分</span>`;

                const unlockData = n.unlock || cachedUnlockMap[n.ip];
                const unlockInfo = unlockData ? renderUnlockBadges(unlockData) : '<span  class="text-xs text-muted">未检测</span>';

                return `
                    <tr class="${isCurrent ? 'node-row-current' : (activeTun ? 'node-row-active' : '')}">
                        <td data-label=""  class="text-center">
                            <button class="star-btn ${n.is_favorite ? 'active' : ''}" data-action="toggleFavorite" data-args="${jsonAttr([n.id])}" title="${n.is_favorite ? '取消收藏' : '加入收藏'}">
                                ${n.is_favorite ? '★' : '☆'}
                            </button>
                        </td>
                        <td data-label="地区">
                            <div class="flag-box">
                                ${getCountryFlagSVG(n.country_short)}
                                <span>${escapeHtml(getCountryName(n.country_short))}</span>
                                ${cityInfo}
                            </div>
                        </td>
                        <td data-label="出口端点">
                            <div class="node-endpoint">${escapeHtml(n.ip)}:${escapeHtml(n.port)}</div>
                            <span class="badge badge-proto">${escapeHtml(String(n.proto || '').toUpperCase())}</span>
                        </td>
                        <td data-label="网络类型">${typeTag}${ispInfo}</td>
                        <td data-label="延迟">${latencyBadge}</td>
                        <td data-label="信誉">${repBadge}</td>
                        <td data-label="解锁能力">${unlockInfo}</td>
                        <td data-label="带宽">
                            <div class="node-speed">${speedMbps}</div>
                            <div  class="text-xs text-muted">评分: ${n.score}</div>
                        </td>
                        <td data-label="操作"  class="text-right">
                            ${isCurrent ?
                                '<span class="badge connected"><span class="status-dot"></span> 当前主连</span>' :
                                (activeTun ?
                                    `<div class="action-group">
                                        <span class="badge badge-current"><span class="status-dot"></span> ${escapeHtml(activeTun.dev_name)}</span>
                                        <button class="btn btn-danger btn-xs" data-action="stopTunnel" data-args="${jsonAttr([activeTun.id])}">断开</button>
                                    </div>` :
                                    `<div class="action-group">
                                        <button class="btn btn-xs" data-action="connectToNode" data-args="${jsonAttr([n.id])}" title="设置为主网关出口">主连</button>
                                        <button class="btn btn-outline btn-xs" data-action="startNewTunnel" data-args="${jsonAttr([n.id])}" title="启动为新的独立并发出口">+并发</button>
                                        <button class="btn btn-outline btn-xs btn-icon-danger" data-action="addNodeToBlacklist" data-args="${jsonAttr([n.id, n.ip, n.country_short])}" title="屏蔽/拉黑此节点24小时">×</button>
                                    </div>`
                                )
                            }
                        </td>
                    </tr>
                `;
            }).join('');
        }

        async function probeCurrentNodes() {
            const search = document.getElementById('search-filter').value.toLowerCase();
            const country = document.getElementById('country-filter').value;
            const ipType = document.getElementById('ip-type-filter').value;

            const filtered = allNodes.filter(n => {
                if (country && n.country_short !== country) return false;
                if (ipType && n.ip_type !== ipType) return false;
                if (search) {
                    const cName = getCountryName(n.country_short).toLowerCase();
                    const str = `${n.ip} ${n.country_long} ${n.country_short} ${cName} ${n.hostname} ${n.isp || ''} ${n.city || ''} ${n.region || ''}`.toLowerCase();
                    if (!str.includes(search)) return false;
                }
                return true;
            });

            const ids = filtered.map(n => n.id);
            if (ids.length === 0) {
                alert('当前筛选条件下没有节点');
                return;
            }

            const btn = document.getElementById('btn-probe-nodes');
            const oldHtml = btn.innerHTML;
            btn.disabled = true;
            btn.innerHTML = `<svg aria-hidden="true" viewBox="0 0 24 24" class="spin"><path d="M21 12a9 9 0 1 1-6.219-8.56"/></svg> 测速与解锁检测中 (${ids.length}个)...`;
            appendLog({ level: 'INFO', module: 'Action', message: `正在对当前筛选出的 ${ids.length} 个节点并发执行可用性测速与 AI/流媒体解锁同步检测...` });

            await fetch('/api/nodes/probe', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ node_ids: ids })
            });

            let count = 0;
            const timer = setInterval(async () => {
                count++;
                await fetchNodes();
                await fetchUnlockCache();
                if (count >= 5) {
                    clearInterval(timer);
                    btn.disabled = false;
                    btn.innerHTML = oldHtml;
                    appendLog({ level: 'INFO', module: 'Action', message: '当前筛选节点可用性与 AI/流媒体解锁检测完成！' });
                }
            }, 700);
        }

        async function quickConnect() {
            appendLog({ level: 'INFO', module: 'Action', message: '正在请求快速连接最优节点...' });
            await fetch('/api/connect', { method: 'POST' });
            fetchStatus();
        }

        async function connectToNode(nodeId) {
            appendLog({ level: 'INFO', module: 'Action', message: `正在请求切换至节点: ${nodeId}...` });
            await fetch('/api/connect', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ node_id: nodeId })
            });
            fetchStatus();
        }

        async function disconnectVPN() {
            appendLog({ level: 'INFO', module: 'Action', message: '正在断开当前 VPN 连接...' });
            await fetch('/api/disconnect', { method: 'POST' });
            fetchStatus();
        }

        async function refreshNodes() {
            appendLog({ level: 'INFO', module: 'Action', message: '正在刷新远端节点列表...' });
            await fetch('/api/refresh', { method: 'POST' });
            setTimeout(fetchNodes, 1500);
            fetchStatus();
        }

        // Settings Modal & Tabs
        function switchSettingsTab(tabKey) {
            document.querySelectorAll('#settings-modal .modal-tab-btn').forEach(b => b.classList.remove('active'));
            ['base', 'rotate', 'tg'].forEach(k => {
                const el = document.getElementById('tab-content-' + k);
                if (el) el.classList.toggle('hidden', k !== tabKey);
            });
            const activeBtn = document.getElementById('tab-btn-' + tabKey);
            if (activeBtn) activeBtn.classList.add('active');
        }

        async function loadSettingsForm() {
            try {
                const res = await fetch('/api/settings');
                if (!res.ok) return;
                const data = await res.json();
                document.getElementById('cfg-web-port').value = data.ui_port || 8787;
                document.getElementById('cfg-web-path').value = data.ui_path || 'aimili';
                document.getElementById('cfg-username').value = data.ui_username || 'admin';
                document.getElementById('cfg-password').value = '';
                document.getElementById('cfg-proxy-port').value = data.proxy_port || 7928;
                document.getElementById('cfg-auto-rotate').value = data.auto_rotate_minutes || 0;
                document.getElementById('cfg-rotate-iptype').value = data.auto_rotate_ip_type || 'all';
                document.getElementById('cfg-discovery-countries').value = (data.discovery_countries || []).join(',');
                document.getElementById('cfg-tg-token').value = data.telegram_bot_token || '';
                document.getElementById('cfg-tg-chatid').value = data.telegram_chat_id || '';
                switchSettingsTab('base');
            } catch (err) {
                console.warn('获取系统配置失败:', err);
            }
        }

        async function openSettingsModal() {
            switchView('settings');
        }

        function closeSettingsModal() { switchView('dashboard'); }

        function randomPath() {
            const chars = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';
            let res = '';
            for (let i = 0; i < 8; i++) res += chars.charAt(Math.floor(Math.random() * chars.length));
            document.getElementById('cfg-web-path').value = res;
        }

        function randomPassword() {
            const chars = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*';
            let res = '';
            for (let i = 0; i < 14; i++) res += chars.charAt(Math.floor(Math.random() * chars.length));
            document.getElementById('cfg-password').value = res;
        }

        async function saveSettings(e) {
            e.preventDefault();
            const webPort = parseInt(document.getElementById('cfg-web-port').value);
            const webPath = document.getElementById('cfg-web-path').value.trim().replace(/^\/+|\/+$/g, '');
            const user = document.getElementById('cfg-username').value.trim();
            const pass = document.getElementById('cfg-password').value.trim();
            const proxyPort = parseInt(document.getElementById('cfg-proxy-port').value);

            if (webPort === proxyPort) {
                alert('错误: Web 管理端口不能与本地代理端口相同！');
                return;
            }

            const autoRotate = parseInt(document.getElementById('cfg-auto-rotate').value) || 0;
            const rotateIPType = document.getElementById('cfg-rotate-iptype').value;
            const countriesRaw = document.getElementById('cfg-discovery-countries').value.trim();
            const countries = countriesRaw ? countriesRaw.split(',').map(c => c.trim().toUpperCase()).filter(Boolean) : [];
            const tgToken = document.getElementById('cfg-tg-token').value.trim();
            const tgChatID = document.getElementById('cfg-tg-chatid').value.trim();

            const payload = {
                ui_port: webPort,
                ui_path: webPath,
                ui_username: user,
                proxy_port: proxyPort,
                auto_rotate_minutes: autoRotate,
                auto_rotate_ip_type: rotateIPType,
                discovery_countries: countries,
                telegram_bot_token: tgToken,
                telegram_chat_id: tgChatID
            };
            if (pass) payload.ui_password = pass;

            try {
                const res = await fetch('/api/settings', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload)
                });
                const ret = await res.json();
                if (!res.ok) {
                    alert('保存失败: ' + (ret.error || '未知错误'));
                    return;
                }

                alert('配置修改成功并已生效！');
                closeSettingsModal();

                const curPort = window.location.port || (window.location.protocol === 'https:' ? '443' : '80');
                if (webPort.toString() !== curPort || !window.location.pathname.includes(webPath)) {
                    const newUrl = `${window.location.protocol}//${window.location.hostname}:${webPort}/${webPath}`;
                    alert(`Web 访问入口已变更，即将跳转至新地址:\n${newUrl}`);
                    window.location.href = newUrl;
                } else {
                    fetchStatus();
                }
            } catch (err) {
                alert('网络请求失败: ' + err);
            }
        }

        // Setup SSE connection
        function setupSSE() {
            const es = new EventSource((apiPrefix || '') + '/api/events');
            es.onopen = () => {
                appendLog({ level: 'INFO', module: 'SSE', message: '已成功连通服务器实时推流总线' });
            };
            es.addEventListener('log', (e) => {
                try {
                    const entry = JSON.parse(e.data);
                    appendLog(entry);
                } catch(err){}
            });
            es.addEventListener('status', (e) => {
                try {
                    const data = JSON.parse(e.data);
                    currentState = data;
                    fetchStatus();
                } catch(err){}
            });
            es.onerror = () => {};
        }

        // Port Matrix Modal & Dynamic Groups
        let currentPortRules = [];
        let currentDynamicGroups = [];
        let editingPort = null;
        let editingGroupId = null;

        function switchMatrixTab(tabKey) {
            document.querySelectorAll('#port-matrix-modal .modal-tab-btn').forEach(b => b.classList.remove('active'));
            document.getElementById('matrix-content-ports').classList.toggle('hidden', tabKey !== 'ports');
            document.getElementById('matrix-content-groups').classList.toggle('hidden', tabKey !== 'groups');
            const activeBtn = document.getElementById('matrix-tab-' + tabKey);
            if (activeBtn) activeBtn.classList.add('active');
        }

        async function startNewTunnel(nodeId) {
            appendLog({ level: 'INFO', module: 'Tunnel', message: `正在启动并发独立出口隧道: ${nodeId}...` });
            try {
                const res = await fetch('/api/tunnels/start', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ node_id: nodeId })
                });
                const ret = await res.json();
                if (!res.ok) {
                    alert('启动并发隧道失败: ' + (ret.error || '未知错误'));
                } else {
                    appendLog({ level: 'INFO', module: 'Tunnel', message: `新隧道启动成功: ${ret.tunnel.id} (${ret.tunnel.dev_name})` });
                    fetchStatus();
                }
            } catch (err) {
                alert('请求失败: ' + err);
            }
        }

        async function stopTunnel(tunnelId) {
            if (!confirm('确认断开并释放该并发隧道吗？')) return;
            try {
                await fetch('/api/tunnels/stop', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ tunnel_id: tunnelId })
                });
                fetchStatus();
            } catch (err) {
                alert('断开失败: ' + err);
            }
        }

        async function openPortMatrixModal() {
            switchView('matrix');
        }

        function closePortMatrixModal() { switchView('dashboard'); }

        async function fetchPortRules() {
            try {
                const res = await fetch('/api/proxy/ports');
                if (!res.ok) return;
                currentPortRules = await res.json();
                renderPortRules();
            } catch (err) {
                console.error("fetch port rules error:", err);
            }
        }

        async function fetchDynamicGroups() {
            try {
                const res = await fetch('/api/tunnel-groups');
                if (!res.ok) return;
                currentDynamicGroups = await res.json();
                renderDynamicGroups();
            } catch (err) {
                console.error("fetch dynamic groups error:", err);
            }
        }

        function renderDynamicGroups() {
            const container = document.getElementById('dynamic-groups-container');
            if (!container) return;
            if (!currentDynamicGroups || currentDynamicGroups.length === 0) {
                container.innerHTML = '<div class="list-empty">暂无自适应隧道池，点击右上角「+ 新建自适应组」即可自动化按指标维持出口</div>';
                return;
            }

            container.innerHTML = currentDynamicGroups.map(g => {
                let metricText = '延迟最低优先';
                if (g.sort_by === 'speed') metricText = '带宽最大优先';
                if (g.sort_by === 'score') metricText = '评分最高优先';

                let ipTypeText = '全部网络类型';
                if (g.ip_type === 'residential') ipTypeText = '住宅宽带 IP';
                if (g.ip_type === 'hosting') ipTypeText = '机房 IP';
                if (g.ip_type === 'mobile') ipTypeText = '移动网络';

                let unlockText = '';
                if (g.unlock_filter === 'ai') unlockText = '<span>解锁: <strong class="unlock-ai">仅AI模型</strong></span>';
                else if (g.unlock_filter === 'streaming') unlockText = '<span>解锁: <strong class="unlock-stream">仅流媒体</strong></span>';
                else if (g.unlock_filter === 'full' || g.unlock_filter === 'all') unlockText = '<span>解锁: <strong class="unlock-full">全解锁 (AI+流媒体)</strong></span>';

                const isSys = g.is_system || g.id === 'system-primary';
                const sysBadge = isSys ? '<span class="badge badge-system">系统主连网关 (tun0)</span>' : `<span class="badge badge-accent">Top ${g.target_count} 隧道</span>`;
                const deleteBtn = isSys ? '' : `<button class="btn btn-danger btn-xs" data-action="deleteDynamicGroup" data-args="${jsonAttr([g.id])}">删除</button>`;
                const editLabel = isSys ? '配置主连策略' : '编辑';

                const countryStr = g.country ? `${getCountryName(g.country)} (${g.country})` : '全部国家/地区';
                const activeCount = (g.active_tunnel_ids || []).length;
                const statusClass = activeCount >= g.target_count ? 'connected' : (activeCount > 0 ? 'connecting' : 'disconnected');

                return `
                    <div class="dynamic-group-card ${isSys ? 'is-system' : ''}">
                        <div>
                            <div class="dynamic-group-title-row">
                                <span class="badge ${statusClass}"><span class="status-dot"></span> ${escapeHtml(g.status_text || '正常')}</span>
                                <strong class="dynamic-group-name">${escapeHtml(g.name)}</strong>
                                ${sysBadge}
                            </div>
                            <div class="dynamic-group-meta">
                                <span>目标: <strong class="text-strong">${escapeHtml(countryStr)}</strong></span>
                                <span>类型: <strong class="text-strong">${escapeHtml(ipTypeText)}</strong></span>
                                ${unlockText}
                                <span>指标: <strong class="dynamic-metric">${metricText}</strong></span>
                                <span>周期: <strong class="text-strong">${g.interval_minutes}分钟</strong></span>
                            </div>
                        </div>
                        <div class="row gap-1">
                            <button class="btn btn-outline btn-xs" data-action="editDynamicGroup" data-args="${jsonAttr([g.id])}">${editLabel}</button>
                            ${deleteBtn}
                        </div>
                    </div>
                `;
            }).join('');
        }

        function showAddDynamicGroupForm() {
            editingGroupId = null;
            document.getElementById('dynamic-group-edit-title').innerText = '新建动态自适应隧道组';
            document.getElementById('dg-name').value = '';
            document.getElementById('dg-country').value = 'JP';
            document.getElementById('dg-iptype').value = 'residential';
            document.getElementById('dg-unlock').value = 'none';
            document.getElementById('dg-sortby').value = 'latency';
            document.getElementById('dg-count-container').classList.remove('hidden');
            document.getElementById('dg-count').value = 3;
            document.getElementById('dg-interval').value = 15;
            openEditorDrawer('dynamic-group-edit-card');
        }

        function editDynamicGroup(id) {
            const g = currentDynamicGroups.find(item => item.id === id);
            if (!g) return;
            editingGroupId = id;
            const isSys = g.is_system || g.id === 'system-primary';
            if (isSys) {
                document.getElementById('dynamic-group-edit-title').innerText = '配置系统主连接自适应策略 (tun0)';
                document.getElementById('dg-count-container').classList.add('hidden');
            } else {
                document.getElementById('dynamic-group-edit-title').innerText = `编辑自适应组: ${g.name}`;
                document.getElementById('dg-count-container').classList.remove('hidden');
            }
            document.getElementById('dg-name').value = g.name;
            document.getElementById('dg-country').value = g.country || '';
            document.getElementById('dg-iptype').value = g.ip_type || 'all';
            document.getElementById('dg-unlock').value = g.unlock_filter || 'none';
            document.getElementById('dg-sortby').value = g.sort_by || 'latency';
            document.getElementById('dg-count').value = g.target_count || (isSys ? 1 : 3);
            document.getElementById('dg-interval').value = g.interval_minutes || 15;
            openEditorDrawer('dynamic-group-edit-card');
        }

        function hideDynamicGroupForm() { closeEditorDrawer('dynamic-group-edit-card'); }

        async function saveDynamicGroup() {
            const name = document.getElementById('dg-name').value.trim();
            if (!name) { alert('请输入自适应组名称'); return; }
            const country = document.getElementById('dg-country').value;
            const ipType = document.getElementById('dg-iptype').value;
            const unlockFilter = document.getElementById('dg-unlock').value;
            const sortBy = document.getElementById('dg-sortby').value;
            const count = parseInt(document.getElementById('dg-count').value) || 3;
            const interval = parseInt(document.getElementById('dg-interval').value) || 15;

            const payload = {
                id: editingGroupId || '',
                name: name,
                enabled: true,
                is_system: editingGroupId === 'system-primary',
                country: country,
                ip_type: ipType,
                unlock_filter: unlockFilter,
                sort_by: sortBy,
                target_count: editingGroupId === 'system-primary' ? 1 : count,
                interval_minutes: interval
            };

            try {
                const res = await fetch('/api/tunnel-groups', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload)
                });
                const ret = await res.json();
                if (!res.ok) { alert('保存失败: ' + (ret.error || '未知错误')); return; }
                hideDynamicGroupForm();
                appendLog({ level: 'INFO', module: 'Action', message: `动态自适应组 [${name}] 已成功保存并启动评估！` });
                await fetchDynamicGroups();
                fetchStatus();
            } catch(err) {
                alert('请求异常: ' + err);
            }
        }

        async function deleteDynamicGroup(id) {
            if (!confirm('确认删除该动态自适应组吗？其维护的隧道将被安全释放。')) return;
            try {
                const res = await fetch(`/api/tunnel-groups?id=${id}`, { method: 'DELETE' });
                const ret = await res.json();
                if (!res.ok) { alert('删除失败: ' + (ret.error || '未知错误')); return; }
                await fetchDynamicGroups();
                fetchStatus();
            } catch(err) {
                alert('请求异常: ' + err);
            }
        }

        async function evaluateDynamicGroups() {
            appendLog({ level: 'INFO', module: 'Action', message: '正在触发所有自适应组重新探活并轮换...' });
            try {
                await fetch('/api/tunnel-groups/evaluate', { method: 'POST' });
                setTimeout(async () => {
                    await fetchDynamicGroups();
                    fetchStatus();
                }, 1500);
            } catch(err){}
        }

        function showToast(text, type = 'success', duration = 2500) {
            const t = document.getElementById('toast');
            const msg = document.getElementById('toast-msg');
            if (t && msg) {
                msg.innerText = text;
                t.dataset.type = type;
                t.classList.add('open');
                clearTimeout(t._timer);
                t._timer = setTimeout(() => { t.classList.remove('open'); }, duration);
            }
        }

        function alert(message) {
            const text = String(message ?? '操作未完成');
            const type = /失败|错误|异常|无法|不能|不存在|请输入|未安装|尚未|暂无/.test(text) ? 'error' : 'info';
            showToast(text, type, type === 'error' ? 5000 : 3200);
        }

        function showAppAlert(message) {
            const el = document.getElementById('app-alert');
            if (!el) return;
            el.textContent = message;
            el.hidden = false;
        }

        function clearAppAlert() {
            const el = document.getElementById('app-alert');
            if (!el) return;
            el.hidden = true;
            el.textContent = '';
        }

        function copyText(text, label) {
            if (navigator.clipboard && navigator.clipboard.writeText) {
                navigator.clipboard.writeText(text);
            } else {
                const ta = document.createElement('textarea');
                ta.value = text;
                document.body.appendChild(ta);
                ta.select();
                document.execCommand('copy');
                document.body.removeChild(ta);
            }
            showToast(`已复制: ${label || text}`);
        }

        // ==========================================
        //  屏蔽库与故障熔断隔离交互 (Blacklist Manager)
        // ==========================================
        let currentBlacklist = [];

        async function openBlacklistModal() {
            document.getElementById('blacklist-modal').classList.add('open');
            await fetchBlacklist();
        }

        function closeBlacklistModal() {
            document.getElementById('blacklist-modal').classList.remove('open');
        }

        async function fetchBlacklist(showNotice = false) {
            const container = document.getElementById('blacklist-items-container');
            if (!container) return;
            try {
                const res = await fetch('/api/blacklist');
                if (!res.ok) return;
                currentBlacklist = await res.json() || [];
                renderBlacklist(currentBlacklist);
                if (showNotice) {
                    showToast('屏蔽库列表已刷新');
                }
            } catch (err) {
                container.innerHTML = '<div class="list-error">加载屏蔽库失败: ' + escapeHtml(err && err.message ? err.message : err) + '</div>';
            }
        }

        function renderBlacklist(items) {
            const container = document.getElementById('blacklist-items-container');
            if (!container) return;

            if (!items || items.length === 0) {
                container.innerHTML = `
                    <div class="blacklist-empty">
                        <div class="blacklist-empty-title">当前无任何被隔离屏蔽的节点</div>
                        <div class="blacklist-empty-copy">
                            当节点在连接时发生多次超时、认证拒绝或异常断线时，系统会自动临时隔离；目前所有节点运行正常。
                        </div>
                    </div>
                `;
                return;
            }

            const now = Date.now();
            container.innerHTML = `
                <div class="blacklist-list">
                    ${items.map(item => {
                        const untilTime = new Date(item.until).getTime();
                        const diffSec = Math.max(0, Math.floor((untilTime - now) / 1000));
                        let leftStr = '即将解封';
                        if (diffSec > 3600) {
                            leftStr = `${Math.floor(diffSec / 3600)}小时${Math.floor((diffSec % 3600) / 60)}分后解封`;
                        } else if (diffSec > 0) {
                            leftStr = `${Math.floor(diffSec / 60)}分${diffSec % 60}秒后解封`;
                        }

                        const cCode = item.country || '';
                        const flag = cCode ? getCountryFlagSVG(cCode) : '';
                        const failBadge = item.fail_count > 1 ? `<span class="badge unlock-blocked badge-mini">失败 ${item.fail_count} 次</span>` : '';

                        return `
                            <div class="blacklist-item">
                                <div class="blacklist-main">
                                    <span class="flag-box">${flag}</span>
                                    <div>
                                        <div class="blacklist-id">
                                            ${escapeHtml(item.id || item.ip)}
                                        </div>
                                        <div class="blacklist-meta">
                                            <span>原因: <strong class="blacklist-reason">${escapeHtml(item.reason || '故障断线')}</strong></span>
                                            <span>·</span>
                                            <span>状态: <span class="blacklist-expiry">${leftStr}</span></span>
                                            ${failBadge}
                                        </div>
                                    </div>
                                </div>
                                <div>
                                    <button class="btn btn-outline btn-xs" data-action="removeNodeFromBlacklist" data-args="${jsonAttr([item.id])}">
                                        解除屏蔽
                                    </button>
                                </div>
                            </div>
                        `;
                    }).join('')}
                </div>
            `;
        }

        async function removeNodeFromBlacklist(nodeId) {
            try {
                const res = await fetch('/api/blacklist/remove', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ node_id: nodeId })
                });
                const ret = await res.json();
                if (!res.ok) {
                    alert('解除失败: ' + (ret.error || '未知错误'));
                    return;
                }
                showToast(`节点 [${nodeId}] 已成功移出屏蔽库`);
                await fetchBlacklist();
                fetchStatus();
                fetchNodes();
            } catch (err) {
                alert('请求失败: ' + err);
            }
        }

        async function clearAllBlacklist() {
            if (!confirm('确定清空整个屏蔽库吗？所有被隔离的节点将被立即释放回候选池。')) return;
            try {
                const res = await fetch('/api/blacklist/clear', { method: 'POST' });
                const ret = await res.json();
                if (!res.ok) {
                    alert('清空失败: ' + (ret.error || '未知错误'));
                    return;
                }
                showToast('已清空全部屏蔽节点');
                await fetchBlacklist();
                fetchStatus();
                fetchNodes();
            } catch (err) {
                alert('请求失败: ' + err);
            }
        }

        async function resurrectBlacklist() {
            const btn = document.getElementById('btn-resurrect-bl');
            if (btn) {
                btn.disabled = true;
                btn.innerText = '⏳ 正在探活复活...';
            }
            try {
                const res = await fetch('/api/blacklist/resurrect', { method: 'POST' });
                const ret = await res.json();
                if (!res.ok) {
                    alert('探活检测失败: ' + (ret.error || '未知错误'));
                    return;
                }
                const revivedCount = ret.revived_count || 0;
                if (revivedCount > 0) {
                    showToast(`探活成功！已复活并释放 ${revivedCount} 个节点`);
                } else {
                    showToast('探活完成：当前被屏蔽节点均未响应，暂无复活');
                }
                await fetchBlacklist();
                fetchStatus();
                fetchNodes();
            } catch (err) {
                alert('请求失败: ' + err);
            } finally {
                if (btn) {
                    btn.disabled = false;
                    btn.innerText = '立即探活复活节点';
                }
            }
        }

        async function addNodeToBlacklist(nodeId, ip, country) {
            if (!confirm(`确定手动屏蔽节点 [${nodeId}] 吗？\n该节点将在 24 小时内不再被连接或调度。`)) return;
            try {
                const res = await fetch('/api/blacklist/add', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        node_id: nodeId,
                        ip: ip,
                        country: country,
                        duration_minutes: 1440,
                        reason: '用户手动屏蔽'
                    })
                });
                const ret = await res.json();
                if (!res.ok) {
                    alert('屏蔽失败: ' + (ret.error || '未知错误'));
                    return;
                }
                showToast(`已将节点 [${nodeId}] 屏蔽 24 小时`);
                fetchStatus();
                fetchNodes();
            } catch (err) {
                alert('请求失败: ' + err);
            }
        }

        function renderPortRules() {
            const container = document.getElementById('port-rules-cards');
            if (!container) return;
            if (!currentPortRules || currentPortRules.length === 0) {
                container.innerHTML = '<div class="list-empty">暂未配置自定义端口，点击上方「+ 新增代理端口」添加</div>';
                return;
            }

            const tunnels = (currentState && currentState.tunnels) ? currentState.tunnels : [];

            container.innerHTML = currentPortRules.map(r => {
                let policyBadge = '<span class="badge policy-round"><svg aria-hidden="true" viewBox="0 0 24 24" class="icon-xs icon-stroke"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg> 单连接轮询</span>';
                if (r.policy === 'random') {
                    policyBadge = '<span class="badge policy-random"><svg aria-hidden="true" viewBox="0 0 24 24" class="icon-xs icon-stroke"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg> 随机分发</span>';
                } else if (r.policy === 'interval') {
                    policyBadge = `<span class="badge policy-interval"><svg aria-hidden="true" viewBox="0 0 24 24" class="icon-xs icon-stroke"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg> 定时轮换 (${r.interval_seconds || 300}s)</span>`;
                }

                let authBadge = '<span class="auth-note">跟随管理密码</span>';
                if (r.auth_mode === 'none') {
                    authBadge = '<span class="auth-note auth-open">免密直连</span>';
                } else if (r.auth_mode === 'custom') {
                    authBadge = `<span class="auth-note text-accent">独立账号: ${escapeHtml(r.auth_user || '未设')}</span>`;
                }

                let boundBadges = [];
                if (r.bound_group_ids && r.bound_group_ids.length > 0) {
                    r.bound_group_ids.forEach(gid => {
                        const g = currentDynamicGroups.find(item => item.id === gid);
                        const gName = g ? g.name : gid;
                        boundBadges.push(`<span class="badge badge-system">动态池: ${escapeHtml(gName)}</span>`);
                    });
                }
                if (r.bound_tunnel_ids && r.bound_tunnel_ids.length > 0) {
                    r.bound_tunnel_ids.forEach(id => {
                        const t = tunnels.find(item => item.id === id);
                        if (t && t.node) {
                            const flag = getCountryFlagSVG(t.node.country_short);
                            boundBadges.push(`<span class="badge badge-proto text-xs" >${flag} ${escapeHtml(t.dev_name)} (${escapeHtml(t.node.ip)})</span>`);
                        } else {
                            boundBadges.push(`<span class="badge badge-proto text-xs" >${escapeHtml(id)}</span>`);
                        }
                    });
                }

                let boundHtml = boundBadges.join(' ');
                if (boundBadges.length === 0) {
                    boundHtml = '<span class="auth-note text-accent">全部在线隧道 (动态负载均衡)</span>';
                }

                const httpUrl = `http://127.0.0.1:${r.port}`;
                const socksUrl = `socks5://127.0.0.1:${r.port}`;

                return `
                    <div class="port-card">
                        <div class="port-card-top">
                            <div class="port-card-badge">
                                <span class="status-dot status-dot-success"></span>
                                PORT ${r.port}
                                <span class="badge badge-proto badge-mini">HTTP / SOCKS5</span>
                            </div>
                            <div  class="row items-center gap-2 wrap">
                                <span class="copy-pill" data-action="copyText" data-args="${jsonAttr([httpUrl, 'HTTP 代理地址'])}">
                                    <svg aria-hidden="true"  viewBox="0 0 24 24" class="icon-xs icon-stroke"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
                                    http://127.0.0.1:${r.port}
                                </span>
                                <span class="copy-pill" data-action="copyText" data-args="${jsonAttr([socksUrl, 'SOCKS5 代理地址'])}">
                                    <svg aria-hidden="true"  viewBox="0 0 24 24" class="icon-xs icon-stroke"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
                                    socks5://127.0.0.1:${r.port}
                                </span>
                                <button class="btn btn-outline btn-xs" data-action="editPortRule" data-args="${jsonAttr([r.port])}">编辑</button>
                                <button class="btn btn-danger btn-xs" data-action="deletePortRule" data-args="${jsonAttr([r.port])}">删除</button>
                            </div>
                        </div>
                        <div class="port-card-body">
                            <div class="port-card-col">
                                <span class="port-card-label">分流调度策略</span>
                                <div class="port-card-val">${policyBadge}</div>
                            </div>
                            <div class="port-card-col">
                                <span class="port-card-label">绑定出口 / 动态自适应池</span>
                                <div class="port-card-val">${boundHtml}</div>
                            </div>
                            <div class="port-card-col">
                                <span class="port-card-label">代理鉴权状态</span>
                                <div class="port-card-val">${authBadge}</div>
                            </div>
                        </div>
                    </div>
                `;
            }).join('');
        }

        function showAddPortForm() {
            editingPort = null;
            document.getElementById('port-edit-title').innerHTML = `新增代理监听端口与分流绑定`;
            document.getElementById('rule-port').value = '';
            document.getElementById('rule-port').disabled = false;
            selectPolicy('round_robin');
            document.getElementById('rule-interval').value = '300';
            selectAuthMode('default_web');
            document.getElementById('rule-auth-user').value = '';
            document.getElementById('rule-auth-pass').value = '';
            renderTunnelCheckboxes([], []);
            suggestNextPort();
            openEditorDrawer('port-edit-card');
        }

        function editPortRule(port) {
            const rule = currentPortRules.find(r => r.port === port);
            if (!rule) return;
            editingPort = port;
            document.getElementById('port-edit-title').innerHTML = `编辑端口 [${port}] 绑定与调度规则`;
            document.getElementById('rule-port').value = rule.port;
            document.getElementById('rule-port').disabled = true;
            selectPolicy(rule.policy || 'round_robin');
            document.getElementById('rule-interval').value = rule.interval_seconds || 300;
            selectAuthMode(rule.auth_mode || 'default_web');
            document.getElementById('rule-auth-user').value = rule.auth_user || '';
            document.getElementById('rule-auth-pass').value = rule.auth_pass || '';
            renderTunnelCheckboxes(rule.bound_tunnel_ids || [], rule.bound_group_ids || []);
            openEditorDrawer('port-edit-card');
        }

        function hideEditPortForm() { closeEditorDrawer('port-edit-card'); }

        function selectPolicy(pol) {
            document.getElementById('rule-policy').value = pol;
            ['round_robin', 'random', 'interval'].forEach(p => {
                const el = document.getElementById('card-policy-' + p);
                if (el) {
                    if (p === pol) el.classList.add('selected');
                    else el.classList.remove('selected');
                }
            });
            document.getElementById('rule-interval-group').classList.toggle('hidden', pol !== 'interval');
        }

        function selectAuthMode(mode) {
            document.getElementById('rule-auth-mode').value = mode;
            ['default_web', 'none', 'custom'].forEach(m => {
                const el = document.getElementById('card-auth-' + m);
                if (el) {
                    if (m === mode) el.classList.add('selected');
                    else el.classList.remove('selected');
                }
            });
            document.getElementById('rule-custom-auth-group').classList.toggle('hidden', mode !== 'custom');
        }

        function suggestNextPort() {
            let max = 7927;
            if (currentPortRules && currentPortRules.length > 0) {
                currentPortRules.forEach(r => { if (r.port > max) max = r.port; });
            }
            document.getElementById('rule-port').value = max + 1;
        }

        function setPortVal(val) { document.getElementById('rule-port').value = val; }
        function setIntervalVal(sec) { document.getElementById('rule-interval').value = sec; }

        function renderTunnelCheckboxes(selectedTunnelIds, selectedGroupIds) {
            const container = document.getElementById('rule-tunnels-checkboxes');
            const tunnels = (currentState && currentState.tunnels) ? currentState.tunnels : [];
            selectedTunnelIds = selectedTunnelIds || [];
            selectedGroupIds = selectedGroupIds || [];
            const selTunMap = {};
            selectedTunnelIds.forEach(id => selTunMap[id] = true);
            const selGrpMap = {};
            selectedGroupIds.forEach(id => selGrpMap[id] = true);

            const isAllChecked = (selectedTunnelIds.length === 0 && selectedGroupIds.length === 0);

            let html = `
                <label class="check-row check-row-all">
                    <input type="checkbox" id="chk-tunnel-all" ${isAllChecked ? 'checked' : ''} data-change-action="onAllTunnelsCheckChanged">
                    <span><strong>全部在线隧道 (默认)</strong> - 自动根据当前所有运行的隧道动态负载均衡</span>
                </label>
            `;

            if (currentDynamicGroups && currentDynamicGroups.length > 0) {
                html += '<div class="check-section-label text-accent">动态自适应组出口 (自动维持Top N并定期轮换):</div>';
                currentDynamicGroups.forEach(g => {
                    const isChecked = !isAllChecked && selGrpMap[g.id];
                    let metricText = g.sort_by === 'speed' ? '最大带宽' : (g.sort_by === 'score' ? '最高评分' : '最低延迟');
                    html += `
                        <label  class="check-row">
                            <input type="checkbox" class="chk-dynamic-group" value="${escapeHtml(g.id)}" ${isChecked ? 'checked' : ''} data-change-action="onSpecificTunnelCheckChanged">
                            <span class="badge badge-system">自适应组</span>
                            <strong class="check-name">${escapeHtml(g.name)}</strong>
                            <span  class="text-xs text-muted">(${escapeHtml(g.country || '全部')} · ${escapeHtml(g.ip_type === 'residential' ? '家宽' : (g.ip_type === 'hosting' ? '机房' : '不限'))} · ${escapeHtml(metricText)} Top${escapeHtml(g.target_count)})</span>
                        </label>
                    `;
                });
            }

            if (tunnels.length > 0) {
                html += '<div class="check-section-label text-muted">固定独立在线隧道:</div>';
                tunnels.forEach(t => {
                    const ip = t.node ? t.node.ip : '';
                    const cName = t.node ? getCountryName(t.node.country_short) : '';
                    const flag = t.node ? getCountryFlagSVG(t.node.country_short) : '';
                    const isChecked = !isAllChecked && selTunMap[t.id];
                    const pingStr = t.node && t.node.latency_ms > 0 ? `(${t.node.latency_ms}ms)` : '';
                    html += `
                        <label  class="check-row">
                            <input type="checkbox" class="chk-single-tunnel" value="${escapeHtml(t.id)}" ${isChecked ? 'checked' : ''} data-change-action="onSpecificTunnelCheckChanged">
                            <span class="flag-box">${flag}</span>
                            <strong  class="mono text-accent">${escapeHtml(t.dev_name)}</strong>
                            <span>${escapeHtml(cName)} (${escapeHtml(ip)})</span>
                            ${pingStr ? `<span class="text-xs ping-ok">${pingStr}</span>` : ''}
                        </label>
                    `;
                });
            }

            container.innerHTML = html;
        }

        function onAllTunnelsCheckChanged(elOrEvent) {
            const el = elOrEvent?.currentTarget || elOrEvent;
            if (el.checked) {
                document.querySelectorAll('.chk-single-tunnel').forEach(c => c.checked = false);
                document.querySelectorAll('.chk-dynamic-group').forEach(c => c.checked = false);
            }
        }

        function onSpecificTunnelCheckChanged() {
            const anyTunChecked = Array.from(document.querySelectorAll('.chk-single-tunnel')).some(c => c.checked);
            const anyGrpChecked = Array.from(document.querySelectorAll('.chk-dynamic-group')).some(c => c.checked);
            const allChk = document.getElementById('chk-tunnel-all');
            if (anyTunChecked || anyGrpChecked) {
                allChk.checked = false;
            } else {
                allChk.checked = true;
            }
        }

        async function savePortRule() {
            const port = parseInt(document.getElementById('rule-port').value);
            if (!port || port < 1 || port > 65535) {
                alert('请输入有效的端口号 (1-65535)');
                return;
            }

            const policy = document.getElementById('rule-policy').value;
            const interval = parseInt(document.getElementById('rule-interval').value) || 300;
            const authMode = document.getElementById('rule-auth-mode').value;
            const authUser = document.getElementById('rule-auth-user').value.trim();
            const authPass = document.getElementById('rule-auth-pass').value.trim();

            let boundTunnels = [];
            let boundGroups = [];
            const allChk = document.getElementById('chk-tunnel-all');
            if (!allChk || !allChk.checked) {
                document.querySelectorAll('.chk-single-tunnel:checked').forEach(c => boundTunnels.push(c.value));
                document.querySelectorAll('.chk-dynamic-group:checked').forEach(c => boundGroups.push(c.value));
            }

            const newRule = {
                port: port,
                enabled: true,
                bound_tunnel_ids: boundTunnels,
                bound_group_ids: boundGroups,
                policy: policy,
                interval_seconds: interval,
                auth_mode: authMode,
                auth_user: authUser,
                auth_pass: authPass
            };

            let updatedRules = currentPortRules ? [...currentPortRules] : [];
            const idx = updatedRules.findIndex(r => r.port === port);
            if (idx >= 0) updatedRules[idx] = newRule;
            else updatedRules.push(newRule);

            try {
                const res = await fetch('/api/proxy/ports', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ rules: updatedRules })
                });
                const ret = await res.json();
                if (!res.ok) {
                    alert('保存端口规则失败: ' + (ret.error || '未知错误'));
                    return;
                }
                currentPortRules = ret.rules || updatedRules;
                renderPortRules();
                hideEditPortForm();
                alert(`端口 [${port}] 规则已保存并实时生效！`);
                fetchStatus();
            } catch (err) {
                alert('请求异常: ' + err);
            }
        }

        async function deletePortRule(port) {
            if (!confirm(`确认删除并停止代理端口 [${port}] 吗？`)) return;
            const updatedRules = currentPortRules.filter(r => r.port !== port);
            try {
                const res = await fetch('/api/proxy/ports', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ rules: updatedRules })
                });
                const ret = await res.json();
                if (!res.ok) { alert('删除失败: ' + (ret.error || '未知错误')); return; }
                currentPortRules = ret.rules || updatedRules;
                renderPortRules();
                fetchStatus();
            } catch (err) {
                alert('请求异常: ' + err);
            }
        }

        // ==========================================
        //  页面视图导航切换引擎 (View Router)
        // ==========================================
        function switchView(viewName) {
            const views = ['dashboard', 'singbox', 'matrix', 'nodes', 'settings'];
            if (!views.includes(viewName)) viewName = 'dashboard';

            // 1. 切换页面卡片视图
            views.forEach(v => {
                const el = document.getElementById('view-' + v);
                if (el) el.classList.remove('active');
            });
            const targetView = document.getElementById('view-' + viewName);
            if (targetView) targetView.classList.add('active');

            // 2. 切换侧边栏高亮项
            document.querySelectorAll('.nav-item').forEach(el => {
                el.classList.remove('active');
                el.removeAttribute('aria-current');
            });
            const navEl = document.getElementById('nav-' + viewName);
            if (navEl) {
                navEl.classList.add('active');
                navEl.setAttribute('aria-current', 'page');
            }

            // 3. 切换移动端底部菜单高亮项
            document.querySelectorAll('.bottom-nav-item').forEach(el => {
                el.classList.remove('active');
                el.removeAttribute('aria-current');
            });
            const bnavEl = document.getElementById('bnav-' + viewName);
            if (bnavEl) {
                bnavEl.classList.add('active');
                bnavEl.setAttribute('aria-current', 'page');
            }

            // 4. 更新顶部大标题
            const titles = {
                dashboard: '运行概览',
                singbox: '边缘抗封锁入站 (sing-box)',
                matrix: '多端口代理与自适应分流矩阵',
                nodes: '优质 VPN 节点列表',
                settings: '系统与安全参数设置'
            };
            const titleEl = document.getElementById('page-title');
            if (titleEl && titles[viewName]) {
                titleEl.innerText = titles[viewName];
            }
            document.title = `${titles[viewName] || '控制台'} · AimiliVPN`;

            // 5. 同步浏览器 Hash 路由，便于后退/前进
            if (window.location.hash !== '#' + viewName) {
                history.replaceState(null, '', '#' + viewName);
            }

            // 6. 按需触发视图专项数据刷新
            if (viewName === 'singbox') fetchSingBoxOverview();
            if (viewName === 'matrix') {
                hideEditPortForm();
                hideDynamicGroupForm();
                fetchPortRules();
                fetchDynamicGroups();
            }
            if (viewName === 'settings') loadSettingsForm();
            if (viewName === 'nodes') { updateCountryFilter(); renderNodes(); }

            window.scrollTo({ top: 0, behavior: 'auto' });
        }

        let activeEditorDrawer = null;

        function openEditorDrawer(id) {
            const drawer = document.getElementById(id);
            const backdrop = document.getElementById('drawer-backdrop');
            if (!drawer) return;
            if (activeEditorDrawer && activeEditorDrawer !== drawer) {
                closeEditorDrawer(activeEditorDrawer.id);
            }
            activeEditorDrawer = drawer;
            drawer.hidden = false;
            if (backdrop) backdrop.hidden = false;
            requestAnimationFrame(() => {
                drawer.classList.add('open');
                backdrop?.classList.add('open');
            });
            document.body.classList.add('drawer-open');
            const firstInput = drawer.querySelector('input:not([disabled]), select');
            firstInput?.focus();
        }

        function closeEditorDrawer(id) {
            const drawer = typeof id === 'string' ? document.getElementById(id) : id;
            const backdrop = document.getElementById('drawer-backdrop');
            if (!drawer) return;
            drawer.classList.remove('open');
            backdrop?.classList.remove('open');
            activeEditorDrawer = null;
            document.body.classList.remove('drawer-open');
            window.setTimeout(() => {
                if (!drawer.classList.contains('open')) drawer.hidden = true;
                if (backdrop && !document.querySelector('.editor-drawer.open')) backdrop.hidden = true;
            }, 230);
        }

        function enhanceInteractiveElements(root = document) {
            root.querySelectorAll('[data-action]').forEach(element => {
                if (element.matches('button, a, input, select, textarea')) return;
                element.setAttribute('role', 'button');
                element.setAttribute('tabindex', '0');
            });
        }

        function openSingBoxModal() {
            switchView('singbox');
        }

        // ==========================================
        //  侧边栏展开/收起切换引擎 (Sidebar Collapse)
        // ==========================================
        function toggleSidebar() {
            const sb = document.getElementById('app-sidebar');
            if (!sb) return;
            const isCollapsed = sb.classList.toggle('collapsed');
            try {
                localStorage.setItem('aimili_sidebar_collapsed', isCollapsed ? '1' : '0');
            } catch(e){}
            updateSidebarUI(isCollapsed);
        }

        function updateSidebarUI(isCollapsed) {
            const btn = document.getElementById('sidebar-collapse-btn');
            const hBtn = document.getElementById('btn-toggle-sidebar');
            const tooltip = isCollapsed ? '展开侧边栏' : '收起侧边栏';
            if (btn) btn.setAttribute('title', tooltip);
            if (hBtn) hBtn.setAttribute('title', tooltip);
        }

        function initSidebarState() {
            try {
                if (localStorage.getItem('aimili_sidebar_collapsed') === '1') {
                    const sb = document.getElementById('app-sidebar');
                    if (sb) {
                        sb.classList.add('collapsed');
                        updateSidebarUI(true);
                    }
                }
            } catch(e){}
        }

        // ==========================================
        //  sing-box 边缘抗封锁入站 & 链式代理交互引擎
        // ==========================================
        let singBoxOverview = null;
        let currentSelectedProto = 'reality';

        async function fetchSingBoxOverview(showToastNotice = false) {
            try {
                const res = await fetch('/api/singbox/overview');
                if (!res.ok) {
                    renderSingBox({ ok: false, installed: false });
                    return;
                }
                const data = await res.json();
                singBoxOverview = data;
                renderSingBox(data);
                if (showToastNotice) {
                    showToast('sing-box 入站与链式状态已同步刷新');
                }
            } catch (err) {
                console.warn('拉取 sing-box 概览失败:', err);
                renderSingBox({ ok: false, installed: false });
            }
        }

        function renderSingBox(data) {
            const statusBadge = document.getElementById('sb-status-badge');
            const subBadge = document.getElementById('sb-sub-badge');
            const container = document.getElementById('sb-nodes-container');
            const grid = document.getElementById('sb-nodes-grid');
            const guide = document.getElementById('sb-empty-guide');
            const addBtn = document.getElementById('btn-add-sb-node');

            if (!statusBadge || !grid) return;

            // 1. 服务状态指示徽章
            if (data && data.installed) {
                const isRunning = data.status && data.status.core && data.status.core.running;
                const coreVer = (data.status && data.status.core && data.status.core.version) || '已安装';
                if (isRunning) {
                    statusBadge.className = 'badge connected';
                    statusBadge.innerHTML = `<span class="status-dot"></span> 运行中 (${escapeHtml(coreVer)})`;
                } else {
                    statusBadge.className = 'badge connecting';
                    statusBadge.innerHTML = `<span class="status-dot"></span> 服务就绪 (${escapeHtml(coreVer)})`;
                }
                if (addBtn) addBtn.disabled = false;
            } else {
                statusBadge.className = 'badge disconnected';
                statusBadge.innerHTML = `<span class="status-dot"></span> 未安装 / 未运行`;
            }

            // 2. 远程订阅状态指示
            if (data && data.subscription && data.subscription.enabled) {
                subBadge.classList.remove('hidden');
                subBadge.innerText = `订阅: :${data.subscription.port} (${data.subscription.node_count || 0} 节点)`;
            } else {
                subBadge.classList.add('hidden');
            }

            const sbNavBadge = document.getElementById('nav-sb-badge');
            if (sbNavBadge) {
                const count = (data && data.nodes) ? data.nodes.length : 0;
                sbNavBadge.innerText = count;
                sbNavBadge.classList.toggle('hidden', count === 0);
            }

            // 3. 未安装引导状态
            if (!data || !data.installed) {
                container.classList.add('hidden');
                guide.classList.remove('hidden');
                guide.innerHTML = `
                    <div  class="empty-title">
                        尚未在系统中检测到 sing-box 边缘服务端
                    </div>
                    <div class="empty-guide-copy">
                        在 VPS 终端执行安装后，即可在此直接纳管 VLESS-REALITY、Hysteria2、TUIC、Shadowsocks 2022 等顶级抗封锁协议，并一键将其流量通过 AimiliVPN 的全球家宽住宅池分流出海。
                    </div>
                    <div class="command-box">
                        <span>bash &lt;(curl -fsSL https://raw.githubusercontent.com/xiumuzidiao0/sing-box/main/install.sh)</span>
                        <button type="button" class="btn btn-outline btn-xs" data-action="copyText" data-args="${jsonAttr(['bash <(curl -fsSL https://raw.githubusercontent.com/xiumuzidiao0/sing-box/main/install.sh)', 'sing-box 一键安装指令'])}">复制</button>
                    </div>
                `;
                return;
            }

            // 4. 已安装但 0 节点引导
            if (!data.nodes || data.nodes.length === 0) {
                container.classList.add('hidden');
                guide.classList.remove('hidden');
                guide.innerHTML = `
                    <div  class="empty-title">
                        暂无活跃的抗封锁入站配置
                    </div>
                    <div class="empty-guide-copy compact">
                        点击下方按钮即可一键新建 VLESS-REALITY 或 Hysteria2 入站，系统将自动分配端口、计算 TLS 凭证，并链式绑定至 AimiliVPN 代理出口。
                    </div>
                    <button class="btn" data-action="openAddSingBoxModal">
                        + 新建第一个抗封锁入站节点
                    </button>
                `;
                return;
            }

            // 5. 渲染活跃节点卡片
            guide.classList.add('hidden');
            container.classList.remove('hidden');

            const outbounds = data.available_outbounds || [];

            grid.innerHTML = data.nodes.map(n => {
                let protoPillClass = 'sb-proto-other';
                const pUpper = (n.protocol || '').toUpperCase();
                if (pUpper.includes('REALITY')) protoPillClass = 'sb-proto-reality';
                else if (pUpper.includes('HYSTERIA')) protoPillClass = 'sb-proto-hy2';
                else if (pUpper.includes('TUIC')) protoPillClass = 'sb-proto-tuic';
                else if (pUpper.includes('SHADOWSOCKS') || pUpper === 'SS') protoPillClass = 'sb-proto-ss';

                // 生成出口下拉选单
                const outboundOptions = outbounds.map(ob => {
                    let isSelected = false;
                    if (ob.addr === 'direct') {
                        isSelected = (!n.outbound || n.outbound === 'direct');
                    } else if (ob.port && n.outbound_port) {
                        isSelected = (ob.port === n.outbound_port);
                    } else if (n.outbound) {
                        isSelected = n.outbound.includes(ob.addr);
                    }
                    return `<option value="${escapeHtml(ob.addr)}" ${isSelected ? 'selected' : ''}>${escapeHtml(ob.label)}</option>`;
                }).join('');

                const isChained = n.outbound && n.outbound !== 'direct';
                const chainSelectClass = isChained ? 'sb-chain-select active-chain' : 'sb-chain-select';

                const sniText = n.sni || n.host || '无伪装域名';
                const networkText = `${n.network || 'tcp'}${n.flow ? ' (' + n.flow + ')' : ''}`;

                return `
                    <div class="sb-node-card">
                        <div class="sb-node-header">
                            <div class="node-header-left">
                                <span class="sb-proto-pill ${protoPillClass}">${escapeHtml(n.protocol)}</span>
                                <strong class="node-port">:${n.port}</strong>
                            </div>
                            <span class="badge connected badge-mini"><span class="status-dot"></span> 在网监听</span>
                        </div>

                        <!-- 链式出口动态选择器 -->
                        <div class="sb-chain-box">
                            <div class="sb-chain-label">
                                <svg aria-hidden="true" viewBox="0 0 24 24" class="icon-xs icon-stroke"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>
                                链式出口路由 (Forwarding Exit)
                            </div>
                            <select class="${chainSelectClass}" data-change-action="updateNodeOutboundFromSelect" data-node-name="${escapeHtml(n.name)}">
                                ${outboundOptions}
                            </select>
                        </div>

                        <!-- 节点核心参数摘要 -->
                        <div class="sb-node-info">
                            <div class="sb-info-item">
                                <span class="sb-info-lbl">SNI / 伪装域名</span>
                                <span class="sb-info-val" title="${escapeHtml(sniText)}">${escapeHtml(sniText)}</span>
                            </div>
                            <div class="sb-info-item">
                                <span class="sb-info-lbl">传输层 / 流控</span>
                                <span class="sb-info-val" title="${escapeHtml(networkText)}">${escapeHtml(networkText)}</span>
                            </div>
                        </div>

                        <!-- 卡片底部快捷操作 -->
                        <div class="sb-action-bar">
                            <div  class="row gap-1">
                                <button class="btn btn-outline btn-xs" data-action="copyNodeShareLink" data-args="${jsonAttr([n.url, n.protocol])}">
                                    <svg aria-hidden="true" viewBox="0 0 24 24"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
                                    复制链接
                                </button>
                                <button class="btn btn-outline btn-xs" data-action="showNodeQRCode" data-args="${jsonAttr([n.url, `${n.protocol} :${n.port}`])}">
                                    <svg aria-hidden="true" viewBox="0 0 24 24"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/></svg>
                                    二维码
                                </button>
                            </div>
                            <button class="btn btn-danger btn-xs" title="删除此入站配置" data-action="deleteSingBoxNode" data-args="${jsonAttr([n.name])}">
                                <svg aria-hidden="true" viewBox="0 0 24 24"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
                            </button>
                        </div>
                    </div>
                `;
            }).join('');
        }

        function openAddSingBoxModal() {
            if (!singBoxOverview || !singBoxOverview.installed) {
                alert('系统尚未安装 sing-box 服务端，请先在终端执行一键安装！');
                return;
            }
            // 填充出口选择器
            const sel = document.getElementById('sb-add-outbound');
            const outbounds = singBoxOverview.available_outbounds || [];
            sel.innerHTML = outbounds.map(ob => {
                return `<option value="${escapeHtml(ob.addr)}" ${ob.is_default ? 'selected' : ''}>${escapeHtml(ob.label)}</option>`;
            }).join('');

            selectSingBoxProto('reality');
            document.getElementById('sb-add-port').value = 'auto';
            document.getElementById('sb-add-sni').value = 'auto';
            document.getElementById('sb-add-cred').value = 'auto';

            document.getElementById('singbox-add-modal').classList.add('open');
        }

        function closeAddSingBoxModal() {
            document.getElementById('singbox-add-modal').classList.remove('open');
        }

        function selectSingBoxProto(proto) {
            currentSelectedProto = proto;
            ['reality', 'hy2', 'tuic', 'ss'].forEach(p => {
                const el = document.getElementById('proto-card-' + p);
                if (el) {
                    if (p === proto) el.classList.add('selected');
                    else el.classList.remove('selected');
                }
            });
        }

        function setSingBoxPortAuto() {
            const input = document.getElementById('sb-add-port');
            if (input) input.value = 'auto';
        }

        async function submitAddSingBoxNode(e) {
            if (e) e.preventDefault();
            const btn = document.getElementById('btn-submit-add-sb');
            btn.disabled = true;
            btn.innerText = '正在生成并部署...';

            const portVal = document.getElementById('sb-add-port').value.trim() || 'auto';
            const sniVal = document.getElementById('sb-add-sni').value.trim() || 'auto';
            const credVal = document.getElementById('sb-add-cred').value.trim() || 'auto';
            const outboundVal = document.getElementById('sb-add-outbound').value;

            const payload = {
                protocol: currentSelectedProto,
                port: portVal,
                sni: sniVal,
                uuid: credVal,
                outbound: outboundVal
            };

            try {
                const res = await fetch('/api/singbox/nodes', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload)
                });
                const ret = await res.json();
                if (!res.ok) {
                    alert('创建节点失败: ' + (ret.error || '未知错误'));
                    return;
                }
                closeAddSingBoxModal();
                showToast(`已成功创建 ${ret.node?.protocol || currentSelectedProto} 入站节点 (端口: ${ret.node?.port || 'auto'})`);
                await fetchSingBoxOverview();
            } catch (err) {
                alert('网络请求失败: ' + err);
            } finally {
                btn.disabled = false;
                btn.innerText = '立即创建并部署';
            }
        }

        async function updateNodeOutbound(nodeName, outboundAddr) {
            try {
                const res = await fetch('/api/singbox/nodes/outbound', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ target: nodeName, outbound: outboundAddr })
                });
                const ret = await res.json();
                if (!res.ok) {
                    alert('修改出口失败: ' + (ret.error || '未知错误'));
                    await fetchSingBoxOverview();
                    return;
                }
                showToast(`节点 [${nodeName}] 链式出口已切换为: ${outboundAddr}`);
                await fetchSingBoxOverview();
            } catch (err) {
                alert('网络请求失败: ' + err);
            }
        }

        function updateNodeOutboundFromSelect(event) {
            const select = event.currentTarget;
            updateNodeOutbound(select.dataset.nodeName, select.value);
        }

        async function batchSetSingBoxOutbound(targetType) {
            if (!singBoxOverview || !singBoxOverview.installed || !singBoxOverview.nodes || singBoxOverview.nodes.length === 0) {
                alert('当前没有活跃的 sing-box 节点可供操作');
                return;
            }

            let targetOutbound = 'direct';
            let label = '直连 (VPS 原生机房网络)';
            if (targetType === 'default') {
                const defOb = (singBoxOverview.available_outbounds || []).find(o => o.is_default);
                targetOutbound = defOb ? defOb.addr : '127.0.0.1:7928';
                label = `AimiliVPN 默认住宅出口 (${targetOutbound})`;
            }

            if (!confirm(`确定将所有 sing-box 入站节点批量切换至【${label}】吗？`)) return;

            try {
                const res = await fetch('/api/singbox/nodes/outbound', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ target: 'all', outbound: targetOutbound })
                });
                const ret = await res.json();
                if (!res.ok) {
                    alert('批量修改出口失败: ' + (ret.error || '未知错误'));
                    return;
                }
                showToast(`已成功将 ${ret.updated_count || '全部'} 个节点切换至: ${targetOutbound}`);
                await fetchSingBoxOverview();
            } catch (err) {
                alert('网络请求失败: ' + err);
            }
        }

        async function deleteSingBoxNode(nodeName) {
            if (!confirm(`确认彻底删除 sing-box 入站配置 [${nodeName}] 吗？`)) return;
            try {
                const res = await fetch(`/api/singbox/nodes?target=${encodeURIComponent(nodeName)}`, {
                    method: 'DELETE'
                });
                const ret = await res.json();
                if (!res.ok) {
                    alert('删除失败: ' + (ret.error || '未知错误'));
                    return;
                }
                showToast(`节点 [${nodeName}] 已成功删除并重构订阅`);
                await fetchSingBoxOverview();
            } catch (err) {
                alert('网络请求失败: ' + err);
            }
        }

        async function copySingBoxSubURL() {
            if (!singBoxOverview || !singBoxOverview.installed) {
                alert('sing-box 未安装，无法获取远程订阅');
                return;
            }
            const sub = singBoxOverview.subscription;
            if (sub && sub.enabled && sub.sub_url) {
                copyText(sub.sub_url, 'sing-box 全量远程订阅链接');
            } else {
                if (confirm('当前尚未开启远程订阅服务，是否立即初始化开启？')) {
                    try {
                        const res = await fetch('/api/singbox/subscription/init', { method: 'POST' });
                        const ret = await res.json();
                        if (ret.ok && ret.sub_url) {
                            showToast('远程订阅服务已成功开启！');
                            copyText(ret.sub_url, '远程订阅链接');
                            await fetchSingBoxOverview();
                        } else {
                            alert('开启订阅失败: ' + (ret.error || '未知错误'));
                        }
                    } catch (err) {
                        alert('请求失败: ' + err);
                    }
                }
            }
        }

        function getClashSubURL() {
            if (singBoxOverview && singBoxOverview.subscription && singBoxOverview.subscription.clash_sub_url) {
                return singBoxOverview.subscription.clash_sub_url;
            }
            const origin = window.location.origin;
            const prefix = window.__apiPrefix || '';
            return `${origin}${prefix}/api/singbox/subscription/clash`;
        }

        function copyClashSubURL() {
            if (!singBoxOverview || !singBoxOverview.installed) {
                alert('sing-box 未安装，无法获取 Clash 订阅');
                return;
            }
            const url = getClashSubURL();
            copyText(url, 'Clash Meta / Mihomo 专属订阅链接');
        }

        function downloadClashConfig() {
            const url = getClashSubURL();
            const a = document.createElement('a');
            a.href = url;
            a.download = 'singbox-clash.yaml';
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            showToast('已触发下载 singbox-clash.yaml');
        }

        function copyNodeShareLink(url, proto) {
            if (!url) {
                alert('该节点暂无有效客户端分享链接');
                return;
            }
            copyText(url, `${proto || '代理'} 客户端分享链接`);
        }

        function showNodeQRCode(url, title) {
            if (!url) {
                alert('暂无分享链接');
                return;
            }
            document.getElementById('sb-qr-title').innerText = title || '客户端配置链接与二维码';
            document.getElementById('sb-qr-url-text').value = url;
            const qrImg = document.getElementById('sb-qr-img');
            qrImg.src = `https://api.qrserver.com/v1/create-qr-code/?size=200x200&data=${encodeURIComponent(url)}`;
            document.getElementById('singbox-qr-modal').classList.add('open');
        }

        function closeSingBoxQRModal() {
            document.getElementById('singbox-qr-modal').classList.remove('open');
        }

        function copyFromElement(elId) {
            const el = document.getElementById(elId);
            if (el) {
                copyText(el.value, '链接');
            }
        }

        const uiActions = {
            toggleSidebar,
            quickConnect,
            disconnectVPN,
            openBlacklistModal,
            openPortMatrixModal,
            clearLogs,
            openAddSingBoxModal,
            batchSetSingBoxOutbound,
            copySingBoxSubURL,
            copyClashSubURL,
            downloadClashConfig,
            fetchSingBoxOverview,
            probeCurrentNodes,
            refreshNodes,
            selectQuickFilter,
            toggleSort,
            closeSettingsModal,
            switchSettingsTab,
            randomPath,
            randomPassword,
            testTelegramAlert,
            switchView,
            closePortMatrixModal,
            switchMatrixTab,
            showAddPortForm,
            suggestNextPort,
            setPortVal,
            selectPolicy,
            setIntervalVal,
            selectAuthMode,
            hideEditPortForm,
            savePortRule,
            evaluateDynamicGroups,
            showAddDynamicGroupForm,
            hideDynamicGroupForm,
            saveDynamicGroup,
            closeAddSingBoxModal,
            selectSingBoxProto,
            setSingBoxPortAuto,
            closeSingBoxQRModal,
            copyFromElement,
            closeBlacklistModal,
            resurrectBlacklist,
            clearAllBlacklist,
            fetchBlacklist,
            renderNodes,
            saveSettings,
            submitAddSingBoxNode,
            probeTunnelUnlock,
            stopTunnel,
            toggleFavorite,
            connectToNode,
            startNewTunnel,
            addNodeToBlacklist,
            deleteDynamicGroup,
            editDynamicGroup,
            removeNodeFromBlacklist,
            copyText,
            editPortRule,
            deletePortRule,
            onAllTunnelsCheckChanged,
            onSpecificTunnelCheckChanged,
            updateNodeOutbound,
            updateNodeOutboundFromSelect,
            copyNodeShareLink,
            showNodeQRCode,
            deleteSingBoxNode
        };

        function runDataAction(element, dataKey, event) {
            const actionName = element?.dataset?.[dataKey];
            const action = uiActions[actionName];
            if (!action) return false;
            let args = [];
            try {
                args = JSON.parse(element.dataset.args || '[]');
            } catch (error) {
                console.error('Invalid data-args', actionName, error);
            }
            action(...args, event);
            return true;
        }

        document.addEventListener('click', event => {
            const actionTarget = event.target.closest('[data-action]');
            if (runDataAction(actionTarget, 'action', event)) return;
            const drawerClose = event.target.closest('[data-drawer-close]');
            if (drawerClose) {
                const drawer = drawerClose.closest('.editor-drawer');
                if (drawer) closeEditorDrawer(drawer.id);
                return;
            }
            if (event.target.id === 'drawer-backdrop') {
                closeEditorDrawer(activeEditorDrawer);
                return;
            }
            const target = event.target.closest('[data-view]');
            if (!target) return;
            switchView(target.dataset.view);
        });

        document.addEventListener('change', event => {
            const target = event.target.closest('[data-change-action]');
            runDataAction(target, 'changeAction', event);
        });

        document.addEventListener('input', event => {
            const target = event.target.closest('[data-input-action]');
            runDataAction(target, 'inputAction', event);
        });

        document.addEventListener('submit', event => {
            const target = event.target.closest('[data-submit-action]');
            if (runDataAction(target, 'submitAction', event)) event.preventDefault();
        });

        document.addEventListener('keydown', event => {
            if (event.key === 'Escape' && activeEditorDrawer) {
                closeEditorDrawer(activeEditorDrawer);
                return;
            }
            if ((event.key === 'Enter' || event.key === ' ') && !['BUTTON', 'A', 'INPUT', 'SELECT', 'TEXTAREA'].includes(event.target.tagName)) {
                const target = event.target.closest('[data-action]');
                if (target && runDataAction(target, 'action', event)) {
                    event.preventDefault();
                }
            }
        });

        window.onload = () => {
            enhanceInteractiveElements();
            new MutationObserver(mutations => {
                mutations.forEach(mutation => {
                    mutation.addedNodes.forEach(node => {
                        if (node.nodeType === Node.ELEMENT_NODE) enhanceInteractiveElements(node);
                    });
                });
            }).observe(document.body, { childList: true, subtree: true });
            initSidebarState();
            fetchUnlockCache();
            fetchStatus();
            fetchNodes();
            fetchSingBoxOverview();
            setupSSE();

            const initialHash = (window.location.hash || '').replace(/^#/, '');
            if (['dashboard', 'singbox', 'matrix', 'nodes', 'settings'].includes(initialHash)) {
                switchView(initialHash);
            } else {
                switchView('dashboard');
            }

            setInterval(fetchStatus, 2000);
            setInterval(fetchSingBoxOverview, 10000);
            setInterval(fetchNodes, 15000);
            setInterval(fetchUnlockCache, 30000);
        };
    
