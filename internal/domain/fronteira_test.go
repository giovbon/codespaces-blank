package domain_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestFronteiraDeDependencia impede que o domínio passe a depender das camadas
// de fora.
//
// A regra de arquitetura é http → views → modules → domain e modules → data →
// infra (docs/10 §3). Sem este teste, a regra é apenas boa intenção: o ciclo
// aparece meses depois, quando já é caro desfazer.
func TestFronteiraDeDependencia(t *testing.T) {
	proibidos := []string{
		"financas/internal/data",
		"financas/internal/infra",
		"financas/internal/http",
		"financas/internal/adapters",
		"financas/internal/views",
		"financas/internal/modules",
	}

	entradas, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("não foi possível ler o diretório do pacote: %v", err)
	}

	fset := token.NewFileSet()
	for _, entrada := range entradas {
		nome := entrada.Name()
		if entrada.IsDir() || !strings.HasSuffix(nome, ".go") {
			continue
		}

		arquivo, err := parser.ParseFile(fset, nome, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("não foi possível analisar %s: %v", nome, err)
		}

		for _, imp := range arquivo.Imports {
			caminho, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("import inválido em %s: %v", nome, err)
			}
			for _, proibido := range proibidos {
				if caminho == proibido || strings.HasPrefix(caminho, proibido+"/") {
					t.Errorf(
						"%s importa %q: o domínio não pode depender de camadas externas (docs/10 §3)",
						filepath.Base(nome), caminho,
					)
				}
			}
		}
	}
}
