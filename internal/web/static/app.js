/* Supercine Proxy dashboard - SPA logic */

const $ = (sel) => document.querySelector(sel);
const $$ = (sel) => document.querySelectorAll(sel);

// ============ TABS ============
$$('.tab').forEach(btn => {
  btn.addEventListener('click', () => {
    $$('.tab').forEach(b => b.classList.remove('active'));
    $$('.tab-panel').forEach(p => p.classList.remove('active'));
    btn.classList.add('active');
    const tabId = btn.dataset.tab;
    $(`#tab-${tabId}`).classList.add('active');
    if (tabId === 'overview') refreshOverview();
    if (tabId === 'logs') refreshLogs();
    if (tabId === 'extract') loadExtractors();
  });
});

// ============ HEALTH ============
async function checkHealth() {
  try {
    const r = await fetch('/v1/health');
    const j = await r.json();
    $('#healthDot').classList.remove('err');
    $('#healthDot').classList.add('ok');
    $('#healthText').textContent = 'online · ' + j.upstream;
  } catch (e) {
    $('#healthDot').classList.remove('ok');
    $('#healthDot').classList.add('err');
    $('#healthText').textContent = 'offline';
  }
}
checkHealth();
setInterval(checkHealth, 15000);

// ============ OVERVIEW ============
async function refreshOverview() {
  try {
    const r = await fetch('/v1/stats');
    const s = await r.json();
    $('#statTotal').textContent = s.total_requests.toLocaleString('pt-BR');
    $('#statHits').textContent = s.cache_hits.toLocaleString('pt-BR');
    $('#statMisses').textContent = s.cache_misses.toLocaleString('pt-BR');
    $('#statErrors').textContent = s.errors.toLocaleString('pt-BR');
    $('#statBytes').textContent = formatBytes(s.bytes_proxied);
    $('#statExt').textContent = s.extractor_calls.toLocaleString('pt-BR')
      + (s.extractor_errors > 0 ? ` (${s.extractor_errors} err)` : '');

    // Bar chart for status
    const chart = $('#statusBreakdown');
    chart.innerHTML = '';
    const entries = Object.entries(s.by_status || {}).sort();
    const max = Math.max(1, ...entries.map(([, v]) => v));
    for (const [k, v] of entries) {
      const cls = k.replace(/[() ]/g, '').toLowerCase();
      const item = document.createElement('div');
      item.className = 'bar-item';
      item.innerHTML = `
        <div class="bar-fill ${cls}" style="height:${(v / max) * 80}px" title="${v}"></div>
        <div class="bar-label">${k} (${v})</div>
      `;
      chart.appendChild(item);
    }

    // Top paths
    const pl = $('#topPaths');
    pl.innerHTML = '';
    const paths = Object.entries(s.by_path || {})
      .sort((a, b) => b[1] - a[1])
      .slice(0, 12);
    for (const [p, c] of paths) {
      const div = document.createElement('div');
      div.className = 'path-item';
      div.innerHTML = `<span class="path-name">${escapeHtml(p)}</span><span class="path-count">${c}</span>`;
      pl.appendChild(div);
    }
  } catch (e) {
    console.error('stats', e);
  }
}
refreshOverview();
setInterval(refreshOverview, 5000);

// ============ LOGS ============
async function refreshLogs() {
  try {
    const limit = $('#logLimit').value;
    const r = await fetch('/v1/logs?limit=' + limit);
    const logs = await r.json();
    const body = $('#logBody');
    body.innerHTML = '';
    for (const l of logs) {
      const tr = document.createElement('tr');
      const statusCls = statusClass(l.status_code);
      tr.innerHTML = `
        <td>${formatTime(l.timestamp)}</td>
        <td>${l.method}</td>
        <td title="${escapeHtml(l.upstream)}">${escapeHtml(l.path)}</td>
        <td><span class="status-pill ${statusCls}">${l.status_code || '—'}</span></td>
        <td>${l.duration}</td>
        <td>${formatBytes(l.size)}</td>
        <td class="${l.cached ? 'cache-yes' : 'cache-no'}">${l.cached ? '✓ HIT' : 'miss'}</td>
        <td class="${l.error ? 'err' : ''}" title="${escapeHtml(l.error || '')}">${escapeHtml((l.error || '').slice(0, 60))}</td>
      `;
      body.appendChild(tr);
    }
  } catch (e) {
    console.error('logs', e);
  }
}

let logsTimer = null;
function setupLogsAutorefresh() {
  if (logsTimer) clearInterval(logsTimer);
  if ($('#logAutorefresh').checked) {
    logsTimer = setInterval(refreshLogs, 3000);
  }
}
$('#logLimit').addEventListener('change', refreshLogs);
$('#logAutorefresh').addEventListener('change', setupLogsAutorefresh);
$('#btnClearLogs').addEventListener('click', async () => {
  if (!confirm('Limpar todos os logs?')) return;
  await fetch('/v1/logs/clear', { method: 'POST' });
  refreshLogs();
  refreshOverview();
});
setupLogsAutorefresh();

// ============ API EXPLORER ============
$('#btnApiSend').addEventListener('click', async () => {
  const path = $('#apiPath').value;
  const method = $('#apiMethod').value;
  const btn = $('#btnApiSend');
  btn.disabled = true;
  btn.textContent = 'Enviando...';
  const t0 = performance.now();
  try {
    const r = await fetch(path, { method });
    const text = await r.text();
    const t1 = performance.now();
    let formatted = text;
    try { formatted = JSON.stringify(JSON.parse(text), null, 2); } catch (e) {}
    $('#apiMeta').textContent = `${method} ${path} → HTTP ${r.status} · ${text.length} bytes · ${(t1 - t0).toFixed(0)}ms`;
    $('#apiResult').textContent = formatted;
  } catch (e) {
    $('#apiMeta').textContent = `Erro: ${e.message}`;
    $('#apiResult').textContent = '';
  } finally {
    btn.disabled = false;
    btn.textContent = 'Enviar';
  }
});

$('#btnLoadRoutes').addEventListener('click', async () => {
  const btn = $('#btnLoadRoutes');
  btn.disabled = true;
  btn.textContent = 'Carregando...';
  try {
    const r = await fetch('/v1/routes');
    const j = await r.json();
    const list = $('#routesList');
    list.innerHTML = '';
    for (const route of (j.routes || [])) {
      const div = document.createElement('div');
      div.className = 'route-item';
      const methods = (route.methods || []).map(m => `<span class="method-tag">${m}</span>`).join('');
      div.innerHTML = `<span class="path">${escapeHtml(route.path)}</span><div class="methods">${methods}</div>`;
      list.appendChild(div);
    }
  } catch (e) {
    $('#routesList').innerHTML = `<div class="route-item">Erro: ${escapeHtml(e.message)}</div>`;
  } finally {
    btn.disabled = false;
    btn.textContent = 'Carregar rotas';
  }
});

$('#btnLoadPlans').addEventListener('click', async () => {
  const btn = $('#btnLoadPlans');
  btn.disabled = true;
  btn.textContent = 'Carregando...';
  try {
    const r = await fetch('/v1/auth/plans');
    const j = await r.json();
    const list = $('#plansList');
    list.innerHTML = '';
    if (!j.plans) {
      list.innerHTML = `<div class="plan-item">Nenhum plano retornado</div>`;
      return;
    }
    for (const p of j.plans) {
      const div = document.createElement('div');
      div.className = 'plan-item';
      div.innerHTML = `
        <div class="label">${escapeHtml(p.label)}</div>
        <div class="price">R$ ${p.amount.toFixed(2).replace('.', ',')}</div>
        <div class="meta">${p.days} dias · id: ${escapeHtml(p.id)}</div>
      `;
      list.appendChild(div);
    }
  } catch (e) {
    $('#plansList').innerHTML = `<div class="plan-item">Erro: ${escapeHtml(e.message)}</div>`;
  } finally {
    btn.disabled = false;
    btn.textContent = 'Carregar planos';
  }
});

// ============ EXTRACTOR ============
async function loadExtractors() {
  try {
    const r = await fetch('/v1/extractors');
    const arr = await r.json();
    const list = $('#extractorsList');
    list.innerHTML = '';
    const icons = {
      doodstream: '🔵', streamwish: '⭐', vidhide: '🎬',
      filemoon: '🌙', filelions: '🦁', mixdrop: '💧',
      streamtape: '📹', voe: '⚡',
    };
    for (const e of arr) {
      const div = document.createElement('div');
      div.className = 'extractor-item';
      div.innerHTML = `<span class="icon">${icons[e.name] || '🎬'}</span><span class="name">${escapeHtml(e.name)}</span>`;
      list.appendChild(div);
    }
  } catch (e) {
    console.error('extractors', e);
  }
}

$('#btnExtract').addEventListener('click', async () => {
  const imdb = $('#exImdb').value.trim();
  const type = $('#exType').value;
  const server = $('#exServer').value;
  const btn = $('#btnExtract');
  btn.disabled = true;
  btn.textContent = 'Extraindo...';
  const t0 = performance.now();
  try {
    let url = `/v1/extract?imdb=${encodeURIComponent(imdb)}&type=${encodeURIComponent(type)}`;
    if (server !== '') url += `&server=${parseInt(server)}`;
    const r = await fetch(url);
    const text = await r.text();
    const t1 = performance.now();
    let formatted = text;
    try { formatted = JSON.stringify(JSON.parse(text), null, 2); } catch (e) {}
    $('#extractMeta').textContent = `HTTP ${r.status} · ${text.length} bytes · ${(t1 - t0).toFixed(0)}ms`;
    $('#extractResult').textContent = formatted;
  } catch (e) {
    $('#extractMeta').textContent = `Erro: ${e.message}`;
    $('#extractResult').textContent = '';
  } finally {
    btn.disabled = false;
    btn.textContent = 'Extrair';
  }
});

$('#btnHostExtract').addEventListener('click', async () => {
  const hostUrl = $('#hostUrl').value.trim();
  if (!hostUrl) return;
  $('#hostResult').textContent = 'Extraindo...';
  try {
    // We don't have a direct hoster-extract endpoint in v1, so use the
    // /v1/extract behavior by faking an embed. For now, this is a client-side
    // hint: the user should use /v1/extract with imdb instead.
    $('#hostResult').textContent = `Use POST /v1/extract com imdb para resolver automaticamente.\n\nPara extrair diretamente de um hoster URL, você pode usar o extractor Go via:\n  go run ./examples/extract_url.go ` + hostUrl;
  } catch (e) {
    $('#hostResult').textContent = `Erro: ${e.message}`;
  }
});

// ============ HELPERS ============
function formatBytes(n) {
  if (n < 1024) return n + ' B';
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB';
  if (n < 1024 * 1024 * 1024) return (n / 1024 / 1024).toFixed(1) + ' MB';
  return (n / 1024 / 1024 / 1024).toFixed(2) + ' GB';
}

function formatTime(iso) {
  const d = new Date(iso);
  return d.toLocaleTimeString('pt-BR', { hour12: false }) + '.' + String(d.getMilliseconds()).padStart(3, '0');
}

function statusClass(code) {
  if (code === 0) return 's0xx';
  if (code < 300) return 's2xx';
  if (code < 400) return 's3xx';
  if (code < 500) return 's4xx';
  return 's5xx';
}

function escapeHtml(s) {
  if (s == null) return '';
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}
