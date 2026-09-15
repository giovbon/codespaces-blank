package system_test

import (
	"financas/internal/data"
	"financas/internal/modules/system"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestStatusRefleteOMigrator(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "estado.db")
	db, err := data.Open(t.Context(), data.Config{
		Path:       caminho,
		Migrations: os.DirFS(filepath.Join("..", "..", "..", "migrations")),
	})
	if err != nil {
		t.Fatalf("abrir base: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	estado, err := system.New(db, data.NewMutator(db)).Status(t.Context())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	if estado.DBPath != caminho {
		t.Errorf("DBPath = %q, queria %q", estado.DBPath, caminho)
	}
	if !slices.Contains(estado.Migrations, "0001_init") {
		t.Errorf("migrações = %v, queria conter 0001_init", estado.Migrations)
	}
	if !slices.Contains(estado.Tables, "transactions") {
		t.Errorf("tabelas não contêm transactions: %v", estado.Tables)
	}
	if slices.Contains(estado.Tables, "tags") {
		t.Error("tags está fora do âmbito de M0/M1 e não devia existir")
	}
}
