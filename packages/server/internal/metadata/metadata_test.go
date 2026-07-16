package metadata

import (
	"testing"

	"github.com/billstark001/latexmk/packages/server/internal/api"
	"github.com/billstark001/latexmk/packages/server/internal/config"
)

func TestValidateToolchain(t *testing.T) {
	cfg := config.Config{Engines: []string{"xelatex"}}
	meta := api.Metadata{Toolchain: map[string]string{"latexmk": "v", "xelatex": "v"}}
	if err := ValidateToolchain(meta, cfg); err != nil {
		t.Fatal(err)
	}
	delete(meta.Toolchain, "xelatex")
	if err := ValidateToolchain(meta, cfg); err == nil {
		t.Fatal("expected missing engine error")
	}
}
