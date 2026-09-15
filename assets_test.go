package financas_test

import (
	"financas"
	"financas/internal/data"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestMigrationsEmbutidasNaRaiz verifica o contrato do sistema de ficheiros.
//
// Os testes das outras camadas leem as migrações do disco, com o diretório
// correto como raiz. Só aqui se testa o que o binário realmente embute — e foi
// exatamente esta diferença que deixou o arranque sem esquema.
func TestMigrationsEmbutidasNaRaiz(t *testing.T) {
	nomes, err := fs.Glob(financas.Migrations, "*.sql")
	if err != nil {
		t.Fatalf("listar migrações embutidas: %v", err)
	}
	if len(nomes) == 0 {
		t.Fatal("nenhuma migração embutida na raiz: o migrator não aplicaria nada")
	}

	var encontrouInicial bool
	for _, nome := range nomes {
		if nome == "0001_init.sql" {
			encontrouInicial = true
		}
		if filepath.Base(nome) != nome {
			t.Errorf("migração %q tem prefixo de pasta: o migrator não a encontra", nome)
		}
	}
	if !encontrouInicial {
		t.Errorf("migrações embutidas = %v, queria conter 0001_init.sql", nomes)
	}
}

func TestWebEmbutidoNaRaiz(t *testing.T) {
	for _, arquivo := range []string{"app.css", "tokens.css", "htmx.min.js", "alpine.min.js"} {
		t.Run(arquivo, func(t *testing.T) {
			if _, err := fs.Stat(financas.Web, arquivo); err != nil {
				t.Errorf("%s não está embutido na raiz de Web: %v", arquivo, err)
			}
		})
	}
}

// TestEsquemaEmbutidoCriaAsTabelas percorre o caminho real de arranque: abre a
// base com as migrações que o binário embute.
func TestEsquemaEmbutidoCriaAsTabelas(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "embutido.db")

	db, err := data.Open(t.Context(), data.Config{
		Path:       caminho,
		Migrations: financas.Migrations,
	})
	if err != nil {
		t.Fatalf("abrir base com migrações embutidas: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, tabela := range []string{"users", "sessions", "transactions", "audit_log"} {
		existe, err := db.HasTable(t.Context(), tabela)
		if err != nil {
			t.Fatalf("verificar tabela %q: %v", tabela, err)
		}
		if !existe {
			t.Errorf("tabela %q não foi criada a partir das migrações embutidas", tabela)
		}
	}

	// O ficheiro tem de existir em disco, e não apenas em memória.
	if _, err := os.Stat(caminho); err != nil {
		t.Errorf("base de dados não foi criada: %v", err)
	}
}
