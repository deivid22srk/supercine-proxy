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
  currentSeasons: null,        // cached seasons response for currentDetail
  currentSelectedSeason: 1,    // currently selected season number
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
  const rowIds = [
    '#rowMoviesLancamentos', '#rowMoviesDestaques',
    '#rowMoviesRecentes', '#rowMoviesSugeridos',
    '#rowTVLancamentos', '#rowTVDestaques',
    '#rowTVRecentes', '#rowTVSugeridos'
  ];
  rowIds.forEach(sel => {
    if ($(sel)) $(sel).innerHTML = Array.from({length: 6}).map(() => makeSkeletonCard()).join('');
  });

  // Load home rows for both types in parallel.
  const [moviesHome, tvHome] = await Promise.all([
    fetchJSON(`${API}/catalog/home?type=movies`),
    fetchJSON(`${API}/catalog/home?type=tvshows`),
  ]);

  // Render movies rows
  if (moviesHome && moviesHome.rows) {
    const mapping = {
      'lancamentos': '#rowMoviesLancamentos',
      'destaques':   '#rowMoviesDestaques',
      'recentes':    '#rowMoviesRecentes',
      'sugeridos':   '#rowMoviesSugeridos'
    };
    let heroCandidate = null;
    for (const row of moviesHome.rows) {
      const sel = mapping[row.category];
      if (!sel || !$(sel)) continue;
      renderRow(sel, row.items);
      // Pick the first available movie as hero
      if (!heroCandidate) {
        heroCandidate = row.items.find(i => i.available) || row.items[0];
      }
    }
    if (heroCandidate) {
      state.heroItem = heroCandidate;
      renderHero(heroCandidate);
    }
  }

  // Render TV rows
  if (tvHome && tvHome.rows) {
    const mapping = {
      'lancamentos': '#rowTVLancamentos',
      'destaques':   '#rowTVDestaques',
      'recentes':    '#rowTVRecentes',
      'sugeridos':   '#rowTVSugeridos'
    };
    for (const row of tvHome.rows) {
      const sel = mapping[row.category];
      if (!sel || !$(sel)) continue;
      renderRow(sel, row.items);
    }
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
    if (state.currentDetail.type === 'tv') {
      // For TV, find the first available episode and play it
      const firstEp = state.currentSeasons?.seasons?.[0]?.episodes?.[0];
      if (firstEp) {
        const season = state.currentSeasons.seasons[0].number;
        closeDetail();
        setTimeout(() => openPlayerForEpisode(state.currentDetail, season, firstEp.number), 50);
      } else {
        // No seasons loaded yet, just open the movie-style player (will fail gracefully)
        const item = state.currentDetail;
        closeDetail();
        setTimeout(() => openPlayer(item), 50);
      }
    } else {
      const item = state.currentDetail;
      closeDetail();
      setTimeout(() => openPlayer(item), 50);
    }
  });

  // Season selector change handler
  $('#seasonSelect').addEventListener('change', (e) => {
    state.currentSelectedSeason = parseInt(e.target.value);
    renderEpisodes();
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
  state.currentSeasons = null;
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

  // Show/hide seasons section
  if (item.type === 'tv' && item.available) {
    $('#seasonsSection').hidden = false;
    $('#modalPlayBtnLabel').textContent = 'Assistir 1º episódio';
    loadSeasons(item.imdb);
  } else {
    $('#seasonsSection').hidden = true;
    $('#modalPlayBtnLabel').textContent = item.available ? 'Assistir agora' : 'Indisponível';
  }

  // Disable Play button if unavailable
  $('#modalPlayBtn').disabled = !item.available;
  $('#modalPlayBtn').style.opacity = item.available ? '1' : '0.5';

  $('#detailModal').hidden = false;
  document.body.style.overflow = 'hidden';
}

// ============ SEASONS & EPISODES ============
async function loadSeasons(imdbID) {
  state.currentSeasons = null;
  $('#seasonSelect').innerHTML = '<option>Carregando…</option>';
  $('#episodesList').innerHTML = '<div class="loading-state"><div class="spinner"></div><p>Carregando episódios…</p></div>';
  try {
    const r = await fetch(`${API}/seasons?imdb=${encodeURIComponent(imdbID)}`);
    if (!r.ok) throw new Error(`HTTP ${r.status}`);
    const j = await r.json();
    state.currentSeasons = j;
    if (!j.seasons || j.seasons.length === 0) {
      $('#seasonSelect').innerHTML = '';
      $('#episodesList').innerHTML = '<div class="empty-state">Nenhuma temporada disponível.</div>';
      return;
    }
    // Populate season dropdown
    $('#seasonSelect').innerHTML = j.seasons.map(s =>
      `<option value="${s.number}">Temporada ${s.number}</option>`
    ).join('');
    state.currentSelectedSeason = j.seasons[0].number;
    $('#seasonSelect').value = state.currentSelectedSeason;
    renderEpisodes();
  } catch (e) {
    $('#seasonSelect').innerHTML = '';
    $('#episodesList').innerHTML = `<div class="empty-state">Erro ao carregar temporadas: ${escapeHtml(e.message)}</div>`;
  }
}

function renderEpisodes() {
  if (!state.currentSeasons || !state.currentSeasons.seasons) return;
  const season = state.currentSeasons.seasons.find(s => s.number === state.currentSelectedSeason);
  if (!season) {
    $('#episodesList').innerHTML = '<div class="empty-state">Temporada não encontrada.</div>';
    return;
  }
  const list = $('#episodesList');
  list.innerHTML = '';
  if (!season.episodes || season.episodes.length === 0) {
    list.innerHTML = '<div class="empty-state">Nenhum episódio nesta temporada.</div>';
    return;
  }
  for (const ep of season.episodes) {
    const card = document.createElement('div');
    card.className = 'episode-card';
    card.innerHTML = `
      <div class="episode-thumb" style="background-image: url('${ep.backdrop || ''}');">
        <div class="episode-num">E${ep.number}</div>
      </div>
      <div class="episode-info">
        <div class="episode-title">${ep.number}. ${escapeHtml(ep.title || `Episódio ${ep.number}`)}</div>
        ${ep.date ? `<div class="episode-date">${escapeHtml(ep.date)}</div>` : ''}
        <button class="episode-play-btn">▶ Assistir</button>
      </div>
    `;
    card.querySelector('.episode-play-btn').addEventListener('click', (e) => {
      e.stopPropagation();
      const item = state.currentDetail;
      closeDetail();
      setTimeout(() => openPlayerForEpisode(item, season.number, ep.number), 50);
    });
    // Also allow click anywhere on the card to play
    card.addEventListener('click', (e) => {
      e.stopPropagation();
      const item = state.currentDetail;
      closeDetail();
      setTimeout(() => openPlayerForEpisode(item, season.number, ep.number), 50);
    });
    list.appendChild(card);
  }
}

function closeDetail() {
  $('#detailModal').hidden = true;
  document.body.style.overflow = '';
  state.currentDetail = null;
  state.currentSeasons = null;
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
    if (j.videos && j.videos.length > 0) {
      playVideo(j.videos[0].url, j.videos[0].quality);
    } else {
      throw new Error('Nenhum dos provedores conseguiu extrair um link direto.');
    }
  } catch (e) {
    showPlayerError(e.message);
  }
}

// openPlayerForEpisode opens the player for a specific TV episode.
// Calls /v1/resolveEpisode instead of /v1/resolve.
async function openPlayerForEpisode(item, season, episode) {
  state.currentDetail = item;
  $('#playerTitle').textContent = `${item.title_ptbr || item.title_orig || item.imdb} — S${season}E${episode}`;
  $('#playerMeta').textContent = [
    item.year && item.year > 0 ? item.year : null,
    'Série',
    `Temporada ${season} · Episódio ${episode}`,
    `via ${item.provider || 'supercine'}`,
  ].filter(Boolean).join(' · ');

  $('#playerError').hidden = true;
  $('#playerLoading').hidden = false;
  $('#videoPlayer').hidden = true;
  $('#serverButtons').innerHTML = '';
  $('#playerModal').hidden = false;
  document.body.style.overflow = 'hidden';

  try {
    const url = `${API}/resolveEpisode?imdb=${encodeURIComponent(item.imdb)}&season=${season}&episode=${episode}`;
    const r = await fetch(url);
    if (!r.ok) {
      const errBody = await r.json().catch(() => ({}));
      throw new Error(errBody.error || `HTTP ${r.status}`);
    }
    const j = await r.json();
    state.currentResolve = j;
    if (!j.servers || j.servers.length === 0) {
      throw new Error('Nenhum servidor disponível para este episódio.');
    }
    renderServerList(j.servers);
    state.currentServerIdx = 0;
    if (j.videos && j.videos.length > 0) {
      playVideo(j.videos[0].url, j.videos[0].quality);
    } else {
      throw new Error('Nenhum dos provedores conseguiu extrair um link direto para este episódio.');
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

  // Route the video through our /v1/stream proxy. The hoster CDNs
  // (MixDrop, StreamWish, VidHide, etc.) reject browser requests that
  // don't carry the hoster's own Origin/Referer, so the browser can't
  // fetch the CDN URL directly. The /v1/stream endpoint fetches the
  // video server-side with the correct headers and streams it back to
  // the browser with permissive CORS.
  //
  // For HLS (m3u8), the proxy also rewrites segment URLs in the
  // playlist to route through /v1/stream, so hls.js fetches segments
  // from the proxy instead of the CDN.
  const proxiedURL = `${API}/stream?url=${encodeURIComponent(videoURL)}`;

  if (videoURL.includes('.m3u8')) {
    // Use hls.js for HLS streams
    if (window.Hls && Hls.isSupported()) {
      state.hls = new Hls({ enableWorker: true, lowLatencyMode: false });
      state.hls.loadSource(proxiedURL);
      state.hls.attachMedia(video);
      state.hls.on(Hls.Events.MANIFEST_PARSED, () => video.play().catch(() => {}));
      state.hls.on(Hls.Events.ERROR, (e, data) => {
        if (data.fatal) {
          showPlayerError(`Erro HLS: ${data.details || data.type}`);
        }
      });
    } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
      // Native HLS (Safari)
      video.src = proxiedURL;
      video.addEventListener('loadedmetadata', () => video.play().catch(() => {}), { once: true });
    } else {
      showPlayerError('Navegador não suporta HLS.');
      return;
    }
  } else {
    // Direct mp4/mkv — but still through the proxy so the CDN's
    // Origin check doesn't block the browser.
    video.src = proxiedURL;
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
