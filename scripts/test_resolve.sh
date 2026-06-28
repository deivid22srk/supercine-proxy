#!/usr/bin/env bash
# Test script to verify that the supercine-proxy resolves video URLs
# correctly for a range of movies and TV episodes.
#
# Usage:
#   ./scripts/test_resolve.sh [base_url]
#
# Defaults to http://localhost:8080.
#
# Exit codes:
#   0 — all tested titles resolved at least one video URL
#   1 — at least one title failed to resolve (excluding known-unavailable)
set -euo pipefail

BASE="${1:-http://localhost:8080}"

# Movies known to be available on Supercine (should all resolve)
MOVIES=(
  tt2250912  # Homem-Aranha: De Volta ao Lar
  tt0133093  # Matrix
  tt0111161  # Um Sonho de Liberdade
  tt0468569  # Batman: O Cavaleiro das Trevas
  tt1375666  # A Origem
  tt0816692  # Interestelar
  tt0109830  # Forrest Gump
  tt0114709  # Toy Story
  tt0120338  # Vida de Inseto
  tt0167260  # O Senhor dos Anéis: A Sociedade do Anel
  tt0137523  # Clube da Luta
  tt1517268  # Avatar: O Caminho da Água
)

# TV episodes (should all resolve)
TV_EPISODES=(
  "tt0903747:1:1"  # Breaking Bad S1E1
  "tt0944947:1:1"  # Game of Thrones S1E1
  "tt4574334:1:1"  # Stranger Things S1E1
  "tt2861424:1:1"  # Rick and Morty S1E1
)

PASS=0
FAIL=0
FAILED_TITLES=()

echo "=========================================="
echo "  Supercine Proxy — Resolution Test Suite"
echo "  Base URL: $BASE"
echo "=========================================="
echo ""

echo "--- Movies ---"
for imdb in "${MOVIES[@]}"; do
  RESP=$(timeout 90 curl -fsS "${BASE}/v1/resolve?imdb=${imdb}&type=movies" 2>/dev/null || echo "")
  VIDEOS=$(echo "$RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d.get('videos',[])))" 2>/dev/null || echo "0")
  if [ "$VIDEOS" -gt 0 ] 2>/dev/null; then
    echo "  PASS ${imdb}: ${VIDEOS} video(s)"
    PASS=$((PASS + 1))
  else
    echo "  FAIL ${imdb}: FAILED"
    FAILED_TITLES+=("$imdb")
    FAIL=$((FAIL + 1))
  fi
done

echo ""
echo "--- TV Episodes ---"
for entry in "${TV_EPISODES[@]}"; do
  IMDB="${entry%%:*}"
  REST="${entry#*:}"
  SEASON="${REST%%:*}"
  EPISODE="${REST##*:}"
  RESP=$(timeout 90 curl -fsS "${BASE}/v1/resolveEpisode?imdb=${IMDB}&season=${SEASON}&episode=${EPISODE}" 2>/dev/null || echo "")
  VIDEOS=$(echo "$RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d.get('videos',[])))" 2>/dev/null || echo "0")
  if [ "$VIDEOS" -gt 0 ] 2>/dev/null; then
    echo "  PASS ${IMDB} S${SEASON}E${EPISODE}: ${VIDEOS} video(s)"
    PASS=$((PASS + 1))
  else
    echo "  FAIL ${IMDB} S${SEASON}E${EPISODE}: FAILED"
    FAILED_TITLES+=("${IMDB} S${SEASON}E${EPISODE}")
    FAIL=$((FAIL + 1))
  fi
done

echo ""
echo "=========================================="
echo "  Results: ${PASS} passed, ${FAIL} failed"
if [ "$FAIL" -gt 0 ]; then
  echo "  Failed titles:"
  for t in "${FAILED_TITLES[@]}"; do
    echo "    - $t"
  done
fi
echo "=========================================="

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
