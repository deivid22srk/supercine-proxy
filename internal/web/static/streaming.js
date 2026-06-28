/* Supercine Streaming UI - main JavaScript */
/* Depends on: hls.js (loaded via CDN before this file) */

const API = '/v1';
const $ = (s) => document.querySelector(s);
const $$ = (s) => document.querySelectorAll(s);

// State
const state = {
  popular: { movies: [], tv: [] },
  heroItem: null,
  searchResults: [],
  currentDetail: null,
  currentServers: [],
  currentServerIdx: 0,
  hls: null,
};

// ============ INIT ============
document.addEventListener('DOMContentLoaded', () => {
  setupNavbar();
  setupSearch();
  setupModals();
  loadHome();
});

// ============ NAVBAR SCROLL EFFECT ============
function setupNavbar() {
  const nav = $('.navbar');
  const onScroll = () => {
    nav.classList.toggle('scrolled', window.scrollY > 80);
  };
  window.addEventListener('scroll', onScroll, { passive: true });
  onScroll();
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
    if (!q) {
      showHome();
      return;
    }
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
  $('#searchGrid').innerHTML = '<div class="loading-state">Buscando…</div>';
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
  // Load popular movies + TV in parallel.
  const [movies, tv] = await Promise.all([
    fetchJSON(`${API}/catalog/popular?type=movies&limit=40`),
    fetchJSON(`${API}/catalog/popular?type=tvshows&limit=30`),
  ]);

  state.popular.movies = (movies && movies.items) || [];
  state.popular.tv = (tv && tv.items) || [];

  // Pick hero — first available movie.
  state.heroItem = state.popular.movies.find(m => m.available) || state.popular.movies[0];
  if (state.heroItem) renderHero(state.heroItem);

  // Render rows.
  renderRow('#rowMovies', state.popular.movies);
  renderRow('#rowTV', state.popular.tv);

  // "Classics" row = older movies (year < 2000) from the movies list.
  const classics = state.popular.movies.filter(m => m.year && m.year < 2005);
  if (classics.length > 0) {
    renderRow('#rowClassics', classics);
  } else {
    $('#rowClassics').parentElement.hidden = true;
  }
}

// ============ RENDER HERO ============
function renderHero(item) {
  $('#heroTitle').textContent = item.title_ptbr || item.title_orig || item.imdb;
  $('#heroMeta').textContent = [
    item.year && item.year > 0 ? item.year : null,
    item.type === 'tv' ? 'Série' : 'Filme',
    item.server_count > 0 ? `${item.server_count} servidor${item.server_count > 1 ? 'es' : ''}` : 'Indisponível',
  ].filter(Boolean).join(' · ');
  $('#heroDesc').textContent = item.cast ? `Elenco: ${item.cast}` : '';
  $('#heroBadge').textContent = item.type === 'tv' ? '📺 Série em destaque' : '⚡ Filme em destaque';
  $('#heroBadge').className = 'hero-badge' + (item.type === 'tv' ? '' : '');

  if (item.backdrop_url) {
    $('#heroBackdrop').style.backgroundImage = `url("${item.backdrop_url}")`;
  }

  $('#heroPlayBtn').onclick = () => openPlayer(item);
  $('#heroInfoBtn').onclick = () => openDetail(item);
}

// ============ RENDER CARD ROW ============
function renderRow(selector, items) {
  const row = $(selector);
  row.innerHTML = '';
  if (!items || items.length === 0) {
    row.innerHTML = '<div class="empty-state">Nenhum item disponível.</div>';
    return;
  }
  for (const item of items) {
    row.appendChild(makeCard(item));
  }
}

function renderSearchResults(items, query) {
  const grid = $('#searchGrid');
  grid.innerHTML = '';
  if (!items || items.length === 0) {
    grid.innerHTML = `<div class="empty-state">Nenhum resultado para "${escapeHtml(query)}".</div>`;
    return;
  }
  for (const item of items) {
    grid.appendChild(makeCard(item));
  }
}

// ============ CARD ============
function makeCard(item) {
  const card = document.createElement('div');
  card.className = 'card';
  card.onclick = () => openDetail(item);

  const poster = document.createElement('div');
  poster.className = 'card-poster';
  if (item.poster_url) {
    const img = document.createElement('img');
    img.src = item.poster_url;
    img.alt = item.title_ptbr || item.title_orig || item.imdb;
    img.loading = 'lazy';
    img.onerror = () => poster.innerHTML = `<div class="card-poster-fallback">${escapeHtml(item.title_ptbr || item.title_orig || 'Sem capa')}</div>`;
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

// ============ DETAIL MODAL ============
function setupModals() {
  $('#modalClose').onclick = closeDetail;
  $('#modalBackdrop').onclick = closeDetail;
  $('#modalPlayBtn').onclick = () => {
    if (state.currentDetail) {
      closeDetail();
      openPlayer(state.currentDetail);
    }
  };
  $('#playerClose').onclick = closePlayer;
  $('#playerRetryBtn').onclick = () => {
    if (state.currentServers.length === 0) return;
    state.currentServerIdx = (state.currentServerIdx + 1) % state.currentServers.length;
    loadCurrentServer();
  };
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
    item.server_count > 0 ? `${item.server_count} servidor${item.server_count > 1 ? 'es' : ''}` : 'Indisponível no Supercine',
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
  ].filter(Boolean).join(' · ');

  $('#playerError').hidden = true;
  $('#playerLoading').hidden = false;
  $('#videoPlayer').hidden = true;
  $('#serverButtons').innerHTML = '';
  $('#playerModal').hidden = false;
  document.body.style.overflow = 'hidden';

  // Fetch the embed page to get server list.
  try {
    const r = await fetch(`${API}/embed?imdb=${encodeURIComponent(item.imdb)}&type=${encodeURIComponent(item.embed_type)}`);
    if (!r.ok) throw new Error(`HTTP ${r.status}`);
    const j = await r.json();
    state.currentServers = j.servers || [];
    if (state.currentServers.length === 0) {
      throw new Error('Nenhum servidor disponível para este título.');
    }
    renderServerList();
    state.currentServerIdx = 0;
    await loadCurrentServer();
  } catch (e) {
    showPlayerError(e.message);
  }
}

function renderServerList() {
  const container = $('#serverButtons');
  container.innerHTML = '';
  state.currentServers.forEach((s, idx) => {
    const btn = document.createElement('button');
    btn.className = 'server-btn' + (idx === state.currentServerIdx ? ' active' : '');
    btn.innerHTML = `
      <span class="server-btn-name">${escapeHtml(s.title || `Servidor ${idx + 1}`)}</span>
      <span class="server-btn-desc">${escapeHtml(s.description || '')}</span>
    `;
    btn.onclick = () => {
      state.currentServerIdx = idx;
      $$('.server-btn').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      loadCurrentServer();
    };
    container.appendChild(btn);
  });
}

async function loadCurrentServer() {
  if (state.currentServers.length === 0) return;
  const server = state.currentServers[state.currentServerIdx];
  $('#playerError').hidden = true;
  $('#playerLoading').hidden = false;
  $('#videoPlayer').hidden = true;

  // Destroy previous hls instance
  if (state.hls) {
    state.hls.destroy();
    state.hls = null;
  }
  const video = $('#videoPlayer');
  video.pause();
  video.removeAttribute('src');
  video.load();

  try {
    // Extract direct video URL
    const r = await fetch(`${API}/extract?imdb=${encodeURIComponent(state.currentDetail.imdb)}&type=${encodeURIComponent(state.currentDetail.embed_type)}&server=${state.currentServerIdx}`);
    if (!r.ok) throw new Error(`HTTP ${r.status}`);
    const j = await r.json();
    if (!j.videos || j.videos.length === 0) {
      throw new Error('Extractor não retornou nenhum vídeo.');
    }
    const videoURL = j.videos[0].url;
    const quality = j.videos[0].quality;

    $('#playerLoading').hidden = true;
    $('#videoPlayer').hidden = false;

    if (videoURL.includes('.m3u8')) {
      // Use hls.js
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
        video.addEventListener('loadedmetadata', () => video.play().catch(() => {}));
      } else {
        throw new Error('Navegador não suporta HLS.');
      }
    } else {
      // Direct mp4/mkv
      video.src = videoURL;
      video.play().catch(() => {});
    }

    // Update title with quality
    $('#playerMeta').textContent = [
      state.currentDetail.year && state.currentDetail.year > 0 ? state.currentDetail.year : null,
      state.currentDetail.type === 'tv' ? 'Série' : 'Filme',
      `Servidor ${state.currentServerIdx + 1}/${state.currentServers.length}`,
      quality,
    ].filter(Boolean).join(' · ');
  } catch (e) {
    showPlayerError(e.message);
  }
}

function showPlayerError(msg) {
  $('#playerLoading').hidden = true;
  $('#videoPlayer').hidden = true;
  $('#playerError').hidden = false;
  $('#playerErrorDetail').textContent = msg;
  if (state.currentServers.length > 1) {
    $('#playerRetryBtn').textContent = `Tentar outro servidor (${state.currentServerIdx + 1}/${state.currentServers.length})`;
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
  state.currentServers = [];
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
