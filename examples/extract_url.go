// extract_url.go — standalone CLI that uses the extractors registry directly.
// Run with:
//   go run ./examples/extract_url.go https://mixdrop.ps/e/abc123
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/deivid22srk/supercine-proxy/internal/extractors"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: extract_url <hoster-url>")
		os.Exit(1)
	}
	url := os.Args[1]
	reg := extractors.NewRegistry()

	if reg.Find(url) == nil {
		fmt.Fprintf(os.Stderr, "Nenhum extractor suporta: %s\n", url)
		fmt.Fprintln(os.Stderr, "Hosters suportados:")
		for _, e := range reg.All() {
			fmt.Fprintf(os.Stderr, "  - %s\n", e.Name())
		}
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := reg.Dispatch(ctx, url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erro: %v\n", err)
	}
	out, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(out))
}
