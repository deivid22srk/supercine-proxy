package imdb

// PopularTitles is a curated list of ~120 popular IMDB IDs that we show on
// the streaming UI home screen. We hardcode this because (a) the IMDB
// suggestion endpoint requires a search query — there's no "list popular"
// call without an API key, and (b) it gives the UI a deterministic, fast
// home page experience.
//
// The list mixes classic and modern titles across movies and TV shows.
// Each entry is the IMDB ID; the title, year, poster, and type are all
// resolved live via the Supercine /embed-api/ endpoint or the IMDB
// suggestion endpoint, so this list stays accurate even as posters change.
var PopularTitles = []string{
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

	// ===== Popular TV series =====
	"tt0903747", // Breaking Bad
	"tt4574334", // Stranger Things
	"tt2861424", // Rick and Morty
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

	// ===== Anime =====
	"tt0990411", // Death Note
	"tt4786286", // One Punch Man
	"tt4158110", // Kimetsu no Yaiba
	"tt1905196", // Shingeki no Kyojin

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

// DedupPopular returns the popular list with duplicate IMDB IDs removed,
// preserving the original order.
func DedupPopular() []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(PopularTitles))
	for _, id := range PopularTitles {
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
