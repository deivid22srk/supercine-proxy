/* Supercine Streaming UI - main JavaScript (v2)
 *
 * Major changes vs v1:
 *   - Use addEventListener with proper event delegation instead of inline onclick
 *   - Stop propagation on modal inner clicks so the backdrop handler doesn't
 *     intercept button clicks (this was the "click feels like it didn't happen"
 *     bug)
 *   - Use the new /v1/resolve endpoint that goes through the provider registry
 *   - Render distinct rows for movies, TV, and classics (no more duplication)
 *   - Add provider status indicator in the navbar
 */

const API = '/v1';
const $ = (s, root = document) => root.querySelector(s);
const $$ = (s, root = document) => Array.from(root.querySelectorAll(s));

// State
const state = {
  popular: { movies: [], tv: [], classics: [] },
  heroItem: null,
  searchResults: [],
  currentDetail: null,
  currentResolve: null,
  currentServerIdx: 0,
  hls: null,
};

// ============ INIT ============
document.addEventListener('DOMContentLoaded', () => {
  setupNavbar();
  setupSearch();
  setupModals();
  loadProviders();
  loadHome();
});

// ============ NAVBAR SCROLL EFFECT ============
function setupNavbar() {
  const nav = $('.navbar');
  const onScroll = () => nav.classList.toggle('scrolled', window.scrollY > 80);
  window.addEventListener('scroll', onScroll, { passive: true });
  onScroll();
}

// ============ PROVIDERS ============
async function loadProviders() {
  try {
    const r = await fetch(`${API}/providers`);
    const j = await r.json();
    const box = $('#providerStatus');
    if (!j.providers || j.providers.length === 0) {
      box.innerHTML = '<span class="provider-dot err"></span><span>Nenhum provedor</span>';
      return;
    }
    const html = j.providers.map(p => {
      const cls = p.healthy ? 'ok' : 'err';
      const label = p.healthy ? 'online' : 'offline';
      return `<span class="provider-pill"><span class="provider-dot ${cls}"></span>${escapeHtml(p.display_name)} · ${label}</span>`;
    }).join('');
    box.innerHTML = html;
  } catch (e) {
    $('#providerStatus').innerHTML = '<span class="provider-dot err"></span><span>erro</span>';
  }
}

// ============ SEARCH ============
function setupSearch() {
  const input = $('#searchInput');
  const clearBtn = $('#searchClear');
  let debounce = null;

  input.addEventListener('input', (e) => {
    const q = e.target.value.trim();
    clearBtn.hidden = !q;
    clearTimeout(debounce);
    if (!q) { showHome(); return; }
    debounce = setTimeout(() => doSearch(q), 350);
  });

  clearBtn.addEventListener('click', () => {
    input.value = '';
    clearBtn.hidden = true;
    showHome();
    input.focus();
  });
}

async function doSearch(q) {
  $('#searchQuery').textContent = q;
  $('#searchGrid').innerHTML = '<div class="loading-state"><div class="spinner"></div><p>Buscando…</p></div>';
  $('#searchResults').hidden = false;
  $('#homeContent').hidden = true;
  try {
    const r = await fetch(`${API}/catalog/search?q=${encodeURIComponent(q)}&limit=20`);
    if (!r.ok) throw new Error(`HTTP ${r.status}`);
    const j = await r.json();
    state.searchResults = j.items || [];
    renderSearchResults(state.searchResults, q);
  } catch (e) {
    $('#searchGrid').innerHTML = `<div class="empty-state">Erro ao buscar: ${escapeHtml(e.message)}</div>`;
  }
}

function showHome() {
  $('#searchResults').hidden = true;
  $('#homeContent').hidden = false;
  $('#searchInput').value = '';
  $('#searchClear').hidden = true;
}

// ============ LOAD HOME ============
async function loadHome() {
  // Render loading placeholders so the user sees something immediately.
  ['#rowMovies', '#rowTV', '#rowClassics'].forEach(sel => {
    $(sel).innerHTML = Array.from({length: 8}).map(() => makeSkeletonCard()).join('');
  });

  // Load popular movies + TV + classics in parallel.
  const [movies, tv, classics] = await Promise.all([
    fetchJSON(`${API}/catalog/popular?type=movies&limit=40`),
    fetchJSON(`${API}/catalog/popular?type=tvshows&limit=20`),
    fetchJSON(`${API}/catalog/popular?type=movies&limit=30`), // we'll filter classics client-side
  ]);

  state.popular.movies = (movies && movies.items) || [];
  state.popular.tv = (tv && tv.items) || [];
  // Classics = movies with year < 2005 from the popular list.
  state.popular.classics = ((classics && classics.items) || []).filter(m => m.year && m.year < 2005);

  // Pick hero — first available movie.
  state.heroItem = state.popular.movies.find(m => m.available) || state.popular.movies[0];
  if (state.heroItem) renderHero(state.heroItem);

  // Render rows.
  renderRow('#rowMovies', state.popular.movies);
  renderRow('#rowTV', state.popular.tv);
  if (state.popular.classics.length > 0) {
    renderRow('#rowClassics', state.popular.classics);
  } else {
    $('#rowClassicsContainer').hidden = true;
  }
}

// ============ RENDER HERO ============
function renderHero(item) {
  $('#heroTitle').textContent = item.title_ptbr || item.title_orig || item.imdb;
  $('#heroMeta').textContent = [
    item.year && item.year > 0 ? item.year : null,
    item.type === 'tv' ? 'Série' : 'Filme',
    item.server_count > 0 ? `${item.server_count} servidor${item.server_count > 1 ? 'es' : ''}` : 'Indisponível',
    item.provider ? `via ${item.provider}` : null,
  ].filter(Boolean).join(' · ');
  $('#heroDesc').textContent = item.cast ? `Elenco: ${item.cast}` : '';
  $('#heroBadge').textContent = item.type === 'tv' ? '📺 Série em destaque' : '⚡ Filme em destaque';

  if (item.backdrop_url) {
    $('#heroBackdrop').style.backgroundImage = `url("${item.backdrop_url}")`;
  }

  $('#heroPlayBtn').onclick = (e) => {
    e.stopPropagation();
    openPlayer(item);
  };
  $('#heroInfoBtn').onclick = (e) => {
    e.stopPropagation();
    openDetail(item);
  };
}

// ============ RENDER CARD ROW ============
function renderRow(selector, items) {
  const row = $(selector);
  row.innerHTML = '';
  if (!items || items.length === 0) {
    row.innerHTML = '<div class="empty-state">Nenhum item disponível.</div>';
    return;
  }
  const frag = document.createDocumentFragment();
  for (const item of items) {
    frag.appendChild(makeCard(item));
  }
  row.appendChild(frag);
}

function renderSearchResults(items, query) {
  const grid = $('#searchGrid');
  grid.innerHTML = '';
  if (!items || items.length === 0) {
    grid.innerHTML = `<div class="empty-state">Nenhum resultado para "${escapeHtml(query)}".</div>`;
    return;
  }
  const frag = document.createDocumentFragment();
  for (const item of items) {
    frag.appendChild(makeCard(item));
  }
  grid.appendChild(frag);
}

// ============ CARD ============
function makeCard(item) {
  const card = document.createElement('div');
  card.className = 'card';
  card.addEventListener('click', (e) => {
    e.stopPropagation();
    openDetail(item);
  });

  const poster = document.createElement('div');
  poster.className = 'card-poster';
  if (item.poster_url) {
    const img = document.createElement('img');
    img.src = item.poster_url;
    img.alt = item.title_ptbr || item.title_orig || item.imdb;
    img.loading = 'lazy';
    img.onerror = () => {
      poster.innerHTML = `<div class="card-poster-fallback">${escapeHtml(item.title_ptbr || item.title_orig || 'Sem capa')}</div>`;
    };
    poster.appendChild(img);
  } else {
    poster.innerHTML = `<div class="card-poster-fallback">${escapeHtml(item.title_ptbr || item.title_orig || item.imdb)}</div>`;
  }

  // Badges
  if (!item.available) {
    const badge = document.createElement('div');
    badge.className = 'card-badge unavailable';
    badge.textContent = 'Indisponível';
    poster.appendChild(badge);
  } else if (item.type === 'tv') {
    const badge = document.createElement('div');
    badge.className = 'card-badge tv';
    badge.textContent = 'Série';
    poster.appendChild(badge);
  }

  // Info overlay on hover
  const info = document.createElement('div');
  info.className = 'card-info';
  info.innerHTML = `
    <div class="card-title">${escapeHtml(item.title_ptbr || item.title_orig || item.imdb)}</div>
    <div class="card-meta">${item.year && item.year > 0 ? item.year + ' · ' : ''}${item.type === 'tv' ? 'Série' : 'Filme'}</div>
  `;
  poster.appendChild(info);

  card.appendChild(poster);
  return card;
}

function makeSkeletonCard() {
  return `<div class="card skeleton-card"><div class="card-poster skeleton"></div></div>`;
}

// ============ MODAL: DETAIL ============
function setupModals() {
  // Use addEventListener with stopPropagation to prevent the backdrop
  // handler from intercepting clicks inside the modal card.
  $('#modalClose').addEventListener('click', (e) => { e.stopPropagation(); closeDetail(); });
  $('#modalBackdrop').addEventListener('click', closeDetail);
  $('#modalCard').addEventListener('click', (e) => e.stopPropagation());

  $('#modalPlayBtn').addEventListener('click', (e) => {
    e.stopPropagation();
    if (!state.currentDetail) return;
    const item = state.currentDetail;
    closeDetail();
    // Small delay so the close animation finishes before the player opens.
    setTimeout(() => openPlayer(item), 50);
  });

  // Player modal
  $('#playerClose').addEventListener('click', (e) => { e.stopPropagation(); closePlayer(); });
  $('#playerContainer').addEventListener('click', (e) => e.stopPropagation());
  $('#playerRetryBtn').addEventListener('click', (e) => {
    e.stopPropagation();
    if (!state.currentResolve || state.currentResolve.servers.length === 0) return;
    state.currentServerIdx = (state.currentServerIdx + 1) % state.currentResolve.servers.length;
    loadCurrentServer();
  });

  // ESC to close
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
      if (!$('#playerModal').hidden) closePlayer();
      else if (!$('#detailModal').hidden) closeDetail();
    }
  });
}

function openDetail(item) {
  state.currentDetail = item;
  $('#modalTitle').textContent = item.title_ptbr || item.title_orig || item.imdb;
  $('#modalMeta').textContent = [
    item.year && item.year > 0 ? item.year : null,
    item.type === 'tv' ? 'Série' : 'Filme',
    item.server_count > 0 ? `${item.server_count} servidor${item.server_count > 1 ? 'es' : ''}` : 'Indisponível no catálogo',
    item.provider ? `via ${item.provider}` : null,
  ].filter(Boolean).join(' · ');
  $('#modalCast').textContent = item.cast ? `Elenco: ${item.cast}` : '';
  if (item.backdrop_url) {
    $('#modalBackdropImg').style.backgroundImage = `url("${item.backdrop_url}")`;
  } else {
    $('#modalBackdropImg').style.backgroundImage = '';
  }
  // Disable Play button if unavailable
  $('#modalPlayBtn').disabled = !item.available;
  $('#modalPlayBtn').style.opacity = item.available ? '1' : '0.5';
  $('#modalPlayBtn').textContent = item.available ? '▶ Assistir agora' : 'Indisponível';

  $('#detailModal').hidden = false;
  document.body.style.overflow = 'hidden';
}

function closeDetail() {
  $('#detailModal').hidden = true;
  document.body.style.overflow = '';
  state.currentDetail = null;
}

// ============ PLAYER ============
async function openPlayer(item) {
  state.currentDetail = item;
  $('#playerTitle').textContent = item.title_ptbr || item.title_orig || item.imdb;
  $('#playerMeta').textContent = [
    item.year && item.year > 0 ? item.year : null,
    item.type === 'tv' ? 'Série' : 'Filme',
    `via ${item.provider || 'supercine'}`,
  ].filter(Boolean).join(' · ');

  $('#playerError').hidden = true;
  $('#playerLoading').hidden = false;
  $('#videoPlayer').hidden = true;
  $('#serverButtons').innerHTML = '';
  $('#playerModal').hidden = false;
  document.body.style.overflow = 'hidden';

  // Call /v1/resolve which goes through the provider registry.
  try {
    const url = `${API}/resolve?imdb=${encodeURIComponent(item.imdb)}&type=${encodeURIComponent(item.embed_type)}`;
    const r = await fetch(url);
    if (!r.ok) {
      const errBody = await r.json().catch(() => ({}));
      throw new Error(errBody.error || `HTTP ${r.status}`);
    }
    const j = await r.json();
    state.currentResolve = j;
    if (!j.servers || j.servers.length === 0) {
      throw new Error('Nenhum servidor disponível para este título.');
    }
    renderServerList(j.servers);
    state.currentServerIdx = 0;
    // If the provider already returned videos (it tries 3 servers internally),
    // play the first one immediately.
    if (j.videos && j.videos.length > 0) {
      playVideo(j.videos[0].url, j.videos[0].quality);
    } else {
      // Otherwise, no provider could resolve — show error.
      throw new Error('Nenhum dos provedores conseguiu extrair um link direto.');
    }
  } catch (e) {
    showPlayerError(e.message);
  }
}

function renderServerList(servers) {
  const container = $('#serverButtons');
  container.innerHTML = '';
  servers.forEach((s, idx) => {
    const btn = document.createElement('button');
    btn.className = 'server-btn' + (idx === state.currentServerIdx ? ' active' : '');
    const isOk = s.description && s.description.startsWith('[OK]');
    const desc = isOk ? s.description.substring(5) : s.description;
    btn.innerHTML = `
      <span class="server-btn-name">${escapeHtml(s.name || `Servidor ${idx + 1}`)}${isOk ? ' ✓' : ''}</span>
      <span class="server-btn-desc">${escapeHtml(desc || '')}</span>
    `;
    btn.addEventListener('click', (e) => {
      e.stopPropagation();
      state.currentServerIdx = idx;
      $$('.server-btn').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      // Re-resolve just this server.
      loadCurrentServer();
    });
    container.appendChild(btn);
  });
}

async function loadCurrentServer() {
  if (!state.currentResolve || state.currentResolve.servers.length === 0) return;
  // Currently the /v1/resolve endpoint auto-resolves up to 3 servers.
  // Manual server selection is a future enhancement — for now we just
  // re-fetch and try again.
  const item = state.currentDetail;
  if (!item) return;

  $('#playerError').hidden = true;
  $('#playerLoading').hidden = false;
  $('#videoPlayer').hidden = true;

  try {
    const url = `${API}/resolve?imdb=${encodeURIComponent(item.imdb)}&type=${encodeURIComponent(item.embed_type)}`;
    const r = await fetch(url);
    const j = await r.json();
    if (j.videos && j.videos.length > 0) {
      playVideo(j.videos[0].url, j.videos[0].quality);
    } else {
      throw new Error('Servidor não retornou vídeo.');
    }
  } catch (e) {
    showPlayerError(e.message);
  }
}

function playVideo(videoURL, quality) {
  // Destroy previous hls instance
  if (state.hls) {
    state.hls.destroy();
    state.hls = null;
  }
  const video = $('#videoPlayer');
  video.pause();
  video.removeAttribute('src');
  video.load();

  $('#playerLoading').hidden = true;
  $('#videoPlayer').hidden = false;

  if (videoURL.includes('.m3u8')) {
    // Use hls.js for HLS streams
    if (window.Hls && Hls.isSupported()) {
      state.hls = new Hls({ enableWorker: true, lowLatencyMode: false });
      state.hls.loadSource(videoURL);
      state.hls.attachMedia(video);
      state.hls.on(Hls.Events.MANIFEST_PARSED, () => video.play().catch(() => {}));
      state.hls.on(Hls.Events.ERROR, (e, data) => {
        if (data.fatal) {
          showPlayerError(`Erro HLS: ${data.details || data.type}`);
        }
      });
    } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
      // Native HLS (Safari)
      video.src = videoURL;
      video.addEventListener('loadedmetadata', () => video.play().catch(() => {}), { once: true });
    } else {
      showPlayerError('Navegador não suporta HLS.');
      return;
    }
  } else {
    // Direct mp4/mkv
    video.src = videoURL;
    video.play().catch(() => {});
  }

  // Update title with quality
  const item = state.currentDetail;
  $('#playerMeta').textContent = [
    item && item.year && item.year > 0 ? item.year : null,
    item && item.type === 'tv' ? 'Série' : 'Filme',
    `via ${state.currentResolve?.provider || 'supercine'}`,
    quality,
  ].filter(Boolean).join(' · ');
}

function showPlayerError(msg) {
  $('#playerLoading').hidden = true;
  $('#videoPlayer').hidden = true;
  $('#playerError').hidden = false;
  $('#playerErrorDetail').textContent = msg;
  if (state.currentResolve && state.currentResolve.servers.length > 1) {
    $('#playerRetryBtn').textContent = `Tentar outro servidor`;
    $('#playerRetryBtn').hidden = false;
  } else {
    $('#playerRetryBtn').hidden = true;
  }
}

function closePlayer() {
  $('#playerModal').hidden = true;
  document.body.style.overflow = '';
  const video = $('#videoPlayer');
  video.pause();
  video.removeAttribute('src');
  video.load();
  if (state.hls) {
    state.hls.destroy();
    state.hls = null;
  }
  state.currentResolve = null;
  state.currentServerIdx = 0;
}

// ============ HELPERS ============
async function fetchJSON(url) {
  try {
    const r = await fetch(url);
    if (!r.ok) return null;
    return await r.json();
  } catch (e) {
    console.error('fetchJSON', url, e);
    return null;
  }
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
