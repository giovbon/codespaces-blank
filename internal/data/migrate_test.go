package data

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitStatements(t *testing.T) {
	casos := []struct {
		nome   string
		script string
		quero  []string
	}{
		{
			nome:   "duas instruções",
			script: "CREATE TABLE a (id TEXT);\nCREATE TABLE b (id TEXT);",
			quero:  []string{"CREATE TABLE a (id TEXT)", "CREATE TABLE b (id TEXT)"},
		},
		{
			nome:   "sem ponto e vírgula final",
			script: "SELECT 1",
			quero:  []string{"SELECT 1"},
		},
		{
			nome:   "ponto e vírgula dentro de literal",
			script: "INSERT INTO t VALUES ('a;b'); SELECT 1;",
			quero:  []string{"INSERT INTO t VALUES ('a;b')", "SELECT 1"},
		},
		{
			nome:   "apóstrofo escapado",
			script: "INSERT INTO t VALUES ('it''s; ok');",
			quero:  []string{"INSERT INTO t VALUES ('it''s; ok')"},
		},
		{
			nome:   "comentário de linha removido",
			script: "-- comentário; com ponto e vírgula\nSELECT 1;",
			quero:  []string{"SELECT 1"},
		},
		{
			nome:   "comentário de bloco removido",
			script: "SELECT 1 /* aqui; nada */; SELECT 2;",
			quero:  []string{"SELECT 1", "SELECT 2"},
		},
		{
			nome:   "comentário de bloco entre tokens separa",
			script: "SELECT a/*x*/b;",
			quero:  []string{"SELECT a b"},
		},
		{
			nome:   "comentário de linha com barra e asterisco dentro",
			script: "-- caminho profiles/*.yaml; não é bloco\nSELECT 1;",
			quero:  []string{"SELECT 1"},
		},
		{
			nome:   "apenas comentário",
			script: "-- nada aqui",
			quero:  nil,
		},
		{
			nome:   "apenas comentário de bloco",
			script: "/* nada; aqui */",
			quero:  nil,
		},
		{
			nome:   "espaços em branco não contam",
			script: "  \n\n ; \n ; ",
			quero:  nil,
		},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			obtido := splitStatements(caso.script)
			if len(obtido) != len(caso.quero) {
				t.Fatalf("splitStatements devolveu %d instruções (%q), queria %d (%q)",
					len(obtido), obtido, len(caso.quero), caso.quero)
			}
			for i := range obtido {
				if gota := strings.TrimSpace(obtido[i]); gota != caso.quero[i] {
					t.Errorf("instrução %d = %q, queria %q", i, gota, caso.quero[i])
				}
			}
		})
	}
}

func TestMigracoesProduzemInstrucoes(t *testing.T) {
	// Um script que não produza instruções seria aplicado «com sucesso» sem
	// criar nada — falha silenciosa que só se veria em produção.
	arquivos, err := filepath.Glob("../../migrations/*.sql")
	if err != nil {
		t.Fatalf("listar migrações: %v", err)
	}
	if len(arquivos) == 0 {
		t.Fatal("nenhuma migração encontrada")
	}

	for _, arquivo := range arquivos {
		conteudo, err := os.ReadFile(filepath.Clean(arquivo))
		if err != nil {
			t.Fatalf("ler %s: %v", arquivo, err)
		}
		instrucoes := splitStatements(string(conteudo))
		if len(instrucoes) == 0 {
			t.Errorf("%s não produziu nenhuma instrução", filepath.Base(arquivo))
		}
		for i, instrucao := range instrucoes {
			if strings.TrimSpace(instrucao) == "" {
				t.Errorf("%s: instrução %d está vazia", filepath.Base(arquivo), i)
			}
		}
	}
}

func TestMigrationNamesOrdenados(t *testing.T) {
	fsys := os.DirFS("../../migrations")
	nomes, err := migrationNames(fsys)
	if err != nil {
		t.Fatalf("listar migrações: %v", err)
	}
	if len(nomes) == 0 {
		t.Fatal("nenhuma migração encontrada")
	}

	for i := 1; i < len(nomes); i++ {
		if nomes[i-1] > nomes[i] {
			t.Errorf("migrações fora de ordem: %q antes de %q", nomes[i-1], nomes[i])
		}
	}
	if nomes[0] != "0001_init.sql" {
		t.Errorf("primeira migração = %q, queria %q", nomes[0], "0001_init.sql")
	}
}
