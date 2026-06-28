package imdb

// popularMovies is a curated list of ~80 popular movie IMDB IDs shown
// in the "Filmes populares" row of the streaming UI home screen.
var popularMovies = []string{
	// ===== Top classics =====
	"tt0111161", // Um Sonho de Liberdade
	"tt0468569", // Batman: O Cavaleiro das Trevas
	"tt1375666", // A Origem
	"tt0133093", // Matrix
	"tt0816692", // Interestelar
	"tt0137523", // Clube da Luta
	"tt0109830", // Forrest Gump
	"tt0114369", // Seven: Os Sete Crimes Capitais
	"tt0167260", // O Senhor dos Anéis: O Retorno do Rei
	"tt0110912", // Pulp Fiction
	"tt0120737", // O Senhor dos Anéis: A Sociedade do Anel
	"tt0068646", // O Poderoso Chefão
	"tt0102926", // O Silêncio dos Inocentes
	"tt0118799", // A Vida é Bela
	"tt0167261", // O Senhor dos Anéis: As Duas Torres
	"tt0071562", // O Poderoso Chefão II
	"tt0130827", // A Lista de Schindler
	"tt0114814", // Os Suspeitos

	// ===== Modern superhero / action =====
	"tt2250912", // Homem-Aranha: De Volta ao Lar
	"tt4154796", // Vingadores: Ultimato
	"tt0848228", // Os Vingadores
	"tt3501632", // Thor: Ragnarok
	"tt1825683", // Pantera Negra
	"tt4154756", // Vingadores: Guerra Infinita
	"tt2015381", // Guardiões da Galáxia
	"tt2395427", // Capitão América: O Soldado Invernal
	"tt0800369", // Homem de Ferro
	"tt2120120", // Aquaman
	"tt2975590", // Batman vs Superman
	"tt4912910", // Mulher Maravilha

	// ===== Sci-fi / fantasy =====
	"tt0114709", // Toy Story
	"tt0241527", // Harry Potter e a Pedra Filosofal
	"tt0295297", // Harry Potter e a Câmara Secreta
	"tt0304141", // Harry Potter e o Prisioneiro de Azkaban
	"tt0926084", // Harry Potter e a Ordem da Fênix
	"tt1201607", // Harry Potter e o Enigma do Príncipe
	"tt0080684", // Star Wars: O Império Contra-Ataca
	"tt0076759", // Star Wars: Uma Nova Esperança
	"tt0121765", // Star Wars: A Ameaça Fantasma
	"tt0121766", // Star Wars: Ataque dos Clones

	// ===== Animation =====
	"tt0317705", // Os Incríveis
	"tt0382932", // Ratatouille
	"tt0990407", // Kung Fu Panda
	"tt0892769", // Kung Fu Panda 2
	"tt0435761", // Toy Story 3
	"tt1979376", // Toy Story 4
	"tt0101414", // A Pequena Sereia
	"tt0327594", // Procurando Nemo

	// ===== Brazilian / PT classics =====
	"tt0333237", // Cidade de Deus
	"tt4395816", // Tropa de Elite 2
	"tt0861732", // Tropa de Elite
	"tt0365685", // Carandiru
	"tt0118865", // Central do Brasil

	// ===== More blockbusters =====
	"tt15398776", // Oppenheimer
	"tt9362722", // Spider-Man: Através do Verso
	"tt10872600", // Spider-Man: No Way Home
	"tt9114286", // Black Panther: Wakanda Forever
	"tt1745960", // Top Gun: Maverick
	"tt6710474", // Avatar: O Caminho da Água
	"tt6791350", // Guardiões da Galáxia Vol. 3
	"tt1517268", // Barbie
	"tt7286456", // Joker
}

// popularTV is a curated list of ~40 popular TV series IMDB IDs shown
// in the "Séries populares" row. These are distinct from the movies so
// the two rows don't show the same items.
var popularTV = []string{
	"tt0903747", // Breaking Bad
	"tt4574334", // Stranger Things
	"tt2861424", // Rick and Morty
	"tt0944947", // Game of Thrones
	"tt0141842", // Os Sopranos
	"tt1475582", // Sherlock
	"tt5491994", // Planeta Terra II
	"tt2306299", // Vikings
	"tt1190634", // The Boys
	"tt0098904", // Seinfeld
	"tt0108778", // Friends
	"tt0096697", // Os Simpsons
	"tt1632701", // Suits
	"tt2707408", // Narcos
	"tt5180504", // The Witcher
	"tt7660850", // Succession
	"tt2356777", // True Detective
	"tt6468322", // La Casa de Papel
	"tt1520211", // The Walking Dead
	"tt0944947", // Game of Thrones (will be deduped)
	"tt5491994", // Planeta Terra II (will be deduped)
	"tt0386676", // The Office (US)
	"tt0285331", // 24 Horas
	"tt0401083", // Alista
	"tt0098904", // Seinfeld (will be deduped)
	"tt0773262", // Dexter
	"tt1442437", // Modern Family
	"tt0795176", // Um Maluco no Pedaço
	"tt0096697", // Os Simpsons (will be deduped)
	"tt1475582", // Sherlock (will be deduped)
	"tt0387128", // The Big Bang Theory
	"tt1439629", // Community
	"tt0898266", // The IT Crowd
	"tt5311514", // Atlantis
	"tt4276898", // Dark
	"tt6468322", // La Casa de Papel (will be deduped)
	"tt11192306", // Merlí
	"tt1865718", // House of Cards (US)
	"tt1837492", // Como Eu Conheci Sua Mãe
	"tt2741602", // Marco Polo
	"tt2098220", // The Newsroom
	"tt4188926", // Line of Duty
}

// classicMovies is a curated list of classic films (pre-2005) shown in
// the "Clássicos atemporais" row. These overlap with popularMovies
// intentionally — but we filter for old year at render time so the row
// only shows classics, not recent ones.
var classicMovies = []string{
	"tt0111161", // Um Sonho de Liberdade (1994)
	"tt0468569", // Batman: O Cavaleiro das Trevas (2008) — borderline
	"tt0133093", // Matrix (1999)
	"tt0137523", // Clube da Luta (1999)
	"tt0109830", // Forrest Gump (1994)
	"tt0114369", // Seven (1995)
	"tt0167260", // Senhor dos Anéis: Retorno do Rei (2003)
	"tt0110912", // Pulp Fiction (1994)
	"tt0120737", // Senhor dos Anéis: Sociedade do Anel (2001)
	"tt0068646", // O Poderoso Chefão (1972)
	"tt0102926", // O Silêncio dos Inocentes (1991)
	"tt0118799", // A Vida é Bela (1997)
	"tt0167261", // Senhor dos Anéis: As Duas Torres (2002)
	"tt0071562", // O Poderoso Chefão II (1974)
	"tt0130827", // A Lista de Schindler (1993)
	"tt0114814", // Os Suspeitos (1995)
	"tt0114709", // Toy Story (1995)
	"tt0118865", // Central do Brasil (1998)
	"tt0080684", // Star Wars: Império Contra-Ataca (1980)
	"tt0076759", // Star Wars: Uma Nova Esperança (1977)
	"tt0121765", // Star Wars: A Ameaça Fantasma (1999)
	"tt0121766", // Star Wars: Ataque dos Clones (2002)
	"tt0101414", // A Pequena Sereia (1989)
	"tt0333237", // Cidade de Deus (2002)
	"tt0861732", // Tropa de Elite (2007) — borderline
	"tt0365685", // Carandiru (2003)
	"tt0110912", // Pulp Fiction (will dedup)
	"tt0099685", // Bastidores (1990)
	"tt0088763", // De Volta para o Futuro (1985)
	"tt0086250", // Caça-Fantasmas (1984)
	"tt0083658", // Blade Runner (1982)
	"tt0084787", // Os Goonies (1985)
	"tt0097576", // A Princesa Prometida (1987)
	"tt0082971", // Indiana Jones e os Caçadores da Arca Perdida (1981)
	"tt0099685", // Bastidores (dup)
}

// PopularMovies returns the deduplicated list of popular movie IMDB IDs.
func PopularMovies() []string {
	return dedup(popularMovies)
}

// PopularTV returns the deduplicated list of popular TV series IMDB IDs.
func PopularTV() []string {
	return dedup(popularTV)
}

// ClassicMovies returns the deduplicated list of classic movie IMDB IDs.
func ClassicMovies() []string {
	return dedup(classicMovies)
}

// dedup returns the input slice with duplicate IMDB IDs removed,
// preserving the original order.
func dedup(ids []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// PopularTitles is kept for backwards compatibility — it returns the
// full popular movies list (same as PopularMovies()).
//
// Deprecated: use PopularMovies() instead.
var PopularTitles = popularMovies

// DedupPopular is kept for backwards compatibility.
//
// Deprecated: use PopularMovies() instead.
func DedupPopular() []string {
	return PopularMovies()
}
