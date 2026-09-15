package data_test

import (
	"errors"
	"financas/internal/data"
	"financas/internal/domain"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// migrationsFS aponta para as migrações do repositório.
//
// O binário embute-as pela raiz do módulo (financas.Migrations); aqui lê-se do
// disco para que o teste continue a verificar o SQL que está versionado.
func migrationsFS() fs.FS { return os.DirFS("../../migrations") }

// novaBase abre uma base temporária com o esquema aplicado.
func novaBase(t *testing.T) *data.DB {
	t.Helper()

	caminho := filepath.Join(t.TempDir(), "teste.db")
	db, err := data.Open(t.Context(), data.Config{Path: caminho, Migrations: migrationsFS()})
	if err != nil {
		t.Fatalf("abrir base de teste: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("fechar base de teste: %v", err)
		}
	})
	return db
}

func TestOpenAplicaEsquema(t *testing.T) {
	db := novaBase(t)

	esperadas := []string{
		"schema_migrations",
		"users", "sessions", "app_settings",
		"accounts", "category_groups", "categories",
		"payees", "payee_mappings",
		"import_profiles", "import_batches", "import_rows",
		"transactions", "rules", "audit_log",
	}

	for _, tabela := range esperadas {
		t.Run(tabela, func(t *testing.T) {
			existe, err := db.HasTable(t.Context(), tabela)
			if err != nil {
				t.Fatalf("verificar tabela %q: %v", tabela, err)
			}
			if !existe {
				t.Errorf("tabela %q não foi criada pela migração", tabela)
			}
		})
	}
}

func TestOpenNaoReservaTabelasForaDeAmbito(t *testing.T) {
	// docs/09 §7: o esquema implementa o que existe. Etiquetas, anexos,
	// agendamentos, orçamento e busca textual entram na fase que os usar.
	db := novaBase(t)

	naoDevemExistir := []string{
		"tags", "transaction_tags", "attachments", "notes",
		"schedules", "budget_allocations", "budget_month_cache", "jobs",
		"transactions_fts",
	}

	for _, tabela := range naoDevemExistir {
		t.Run(tabela, func(t *testing.T) {
			existe, err := db.HasTable(t.Context(), tabela)
			if err != nil {
				t.Fatalf("verificar tabela %q: %v", tabela, err)
			}
			if existe {
				t.Errorf("tabela %q existe mas está fora do âmbito de M0/M1 (docs/09 §4)", tabela)
			}
		})
	}
}

func TestOpenEAplicacaoIdempotente(t *testing.T) {
	ctx := t.Context()
	caminho := filepath.Join(t.TempDir(), "teste.db")

	primeira, err := data.Open(ctx, data.Config{Path: caminho, Migrations: migrationsFS()})
	if err != nil {
		t.Fatalf("primeira abertura: %v", err)
	}
	aplicadas, err := primeira.AppliedMigrations(ctx)
	if err != nil {
		t.Fatalf("ler migrações aplicadas: %v", err)
	}
	if err := primeira.Close(); err != nil {
		t.Fatalf("fechar primeira: %v", err)
	}

	// Reabrir a mesma base não pode reaplicar nada: o esquema já existe e um
	// CREATE TABLE repetido falharia.
	segunda, err := data.Open(ctx, data.Config{Path: caminho, Migrations: migrationsFS()})
	if err != nil {
		t.Fatalf("segunda abertura devia ser inofensiva: %v", err)
	}
	t.Cleanup(func() { _ = segunda.Close() })

	depois, err := segunda.AppliedMigrations(ctx)
	if err != nil {
		t.Fatalf("reler migrações aplicadas: %v", err)
	}
	if len(depois) != len(aplicadas) {
		t.Errorf("migrações reaplicadas: antes %v, depois %v", aplicadas, depois)
	}
}

func TestOpenExigeConfiguracao(t *testing.T) {
	if _, err := data.Open(t.Context(), data.Config{Migrations: migrationsFS()}); err == nil {
		t.Error("abrir sem caminho devia falhar")
	}
	if _, err := data.Open(t.Context(), data.Config{Path: filepath.Join(t.TempDir(), "x.db")}); err == nil {
		t.Error("abrir sem migrações devia falhar")
	}
}

func TestPragmasAtivas(t *testing.T) {
	db := novaBase(t)

	casos := map[string]string{
		"journal_mode": "wal",
		"foreign_keys": "1",
		"busy_timeout": "5000",
	}

	for pragma, quero := range casos {
		t.Run(pragma, func(t *testing.T) {
			var obtido string
			if err := db.SQL().QueryRowContext(t.Context(), "PRAGMA "+pragma).Scan(&obtido); err != nil {
				t.Fatalf("ler PRAGMA %s: %v", pragma, err)
			}
			if !strings.EqualFold(obtido, quero) {
				t.Errorf("PRAGMA %s = %q, queria %q", pragma, obtido, quero)
			}
		})
	}
}

func TestChaveEstrangeiraAtiva(t *testing.T) {
	// Garante que foreign_keys está mesmo ligada, e não só declarada: inserir
	// um lançamento numa conta inexistente tem de ser recusado.
	db := novaBase(t)

	_, err := db.SQL().ExecContext(t.Context(),
		`INSERT INTO transactions
		   (id, account_id, date, amount_cents, currency, created_at, updated_at)
		 VALUES ('tx-1', 'conta-inexistente', '2026-09-15', -1234, 'BRL', 1, 1)`,
	)
	if err == nil {
		t.Fatal("inserir lançamento com conta inexistente devia violar a chave estrangeira")
	}
}

func TestMutatorGravaAuditoria(t *testing.T) {
	ctx := t.Context()
	db := novaBase(t)
	mut := data.NewMutator(db)

	const chave = "moeda-teste"

	err := mut.Run(ctx, data.Write{Actor: "Giovani", Origin: "ui"}, func(tx *data.Tx) error {
		if err := tx.PutSetting(ctx, chave, "BRL"); err != nil {
			return err
		}
		return tx.Audit("app_setting", chave, data.ActionCreate, nil, map[string]string{"value": "BRL"})
	})
	if err != nil {
		t.Fatalf("escrita devia ter sucesso: %v", err)
	}

	entradas, err := db.AuditLog(ctx, "app_setting", chave, 10)
	if err != nil {
		t.Fatalf("ler auditoria: %v", err)
	}
	if len(entradas) != 1 {
		t.Fatalf("queria 1 entrada de auditoria, obtive %d", len(entradas))
	}
	if entradas[0].Actor != "Giovani" {
		t.Errorf("ator = %q, queria %q", entradas[0].Actor, "Giovani")
	}
	if entradas[0].Origin != "ui" {
		t.Errorf("origem = %q, queria %q", entradas[0].Origin, "ui")
	}

	valor, existe, err := db.Setting(ctx, chave)
	if err != nil || !existe {
		t.Fatalf("definição não foi gravada: existe=%v err=%v", existe, err)
	}
	if valor != `"BRL"` {
		t.Errorf("valor gravado = %q, queria %q", valor, `"BRL"`)
	}
}

func TestMutatorReverteTudo(t *testing.T) {
	ctx := t.Context()
	db := novaBase(t)
	mut := data.NewMutator(db)

	const chave = "moeda-revertida"
	erroEsperado := errors.New("falha deliberada")

	err := mut.Run(ctx, data.Write{Actor: "Giovani", Origin: "ui"}, func(tx *data.Tx) error {
		if err := tx.PutSetting(ctx, chave, "USD"); err != nil {
			return err
		}
		if err := tx.Audit("app_setting", chave, data.ActionCreate, nil, "USD"); err != nil {
			return err
		}
		// Erro a meio: nem a definição nem a auditoria podem sobreviver.
		return erroEsperado
	})
	if !errors.Is(err, erroEsperado) {
		t.Fatalf("erro devolvido = %v, queria %v", err, erroEsperado)
	}

	if _, existe, err := db.Setting(ctx, chave); err != nil || existe {
		t.Errorf("definição sobreviveu ao rollback: existe=%v err=%v", existe, err)
	}
	quantas, err := db.AuditCount(ctx, "app_setting", chave)
	if err != nil {
		t.Fatalf("contar auditoria: %v", err)
	}
	if quantas != 0 {
		t.Errorf("auditoria sobreviveu ao rollback: %d entradas", quantas)
	}
}

func TestMutatorExigeFuncao(t *testing.T) {
	db := novaBase(t)
	if err := data.NewMutator(db).Run(t.Context(), data.SystemWrite, nil); err == nil {
		t.Error("mutator sem função devia falhar")
	}
}

func TestAuditoriaEncadeada(t *testing.T) {
	// Duas escritas na mesma transação têm de aparecer na ordem em que foram
	// pedidas: o undo percorre-as ao contrário e a ordem importa.
	ctx := t.Context()
	db := novaBase(t)
	mut := data.NewMutator(db)

	err := mut.Run(ctx, data.SystemWrite, func(tx *data.Tx) error {
		if err := tx.Audit("conta", "acc-1", data.ActionCreate, nil, "nova"); err != nil {
			return err
		}
		return tx.Audit("conta", "acc-1", data.ActionUpdate, "nova", "renomeada")
	})
	if err != nil {
		t.Fatalf("escrita devia ter sucesso: %v", err)
	}

	entradas, err := db.AuditLog(ctx, "conta", "acc-1", 10)
	if err != nil {
		t.Fatalf("ler auditoria: %v", err)
	}
	if len(entradas) != 2 {
		t.Fatalf("queria 2 entradas, obtive %d", len(entradas))
	}
	// A leitura devolve da mais recente para a mais antiga.
	if entradas[0].Action != data.ActionUpdate {
		t.Errorf("primeira entrada = %q, queria %q", entradas[0].Action, data.ActionUpdate)
	}
	if entradas[1].Action != data.ActionCreate {
		t.Errorf("segunda entrada = %q, queria %q", entradas[1].Action, data.ActionCreate)
	}
}

func TestAuditoriaDeEntidadeInexistente(t *testing.T) {
	db := novaBase(t)
	entradas, err := db.AuditLog(t.Context(), "nada", "nada", 10)
	if err != nil {
		t.Fatalf("ler auditoria vazia não é erro: %v", err)
	}
	if len(entradas) != 0 {
		t.Errorf("queria nenhuma entrada, obtive %d", len(entradas))
	}
}

func TestCredencialAusente(t *testing.T) {
	// Sem senha definida, a aplicação tem de entrar em modo de primeiro
	// arranque — e isso distingue-se de erro por um erro tipado.
	db := novaBase(t)

	if _, err := db.Credential(t.Context()); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("credencial ausente devia devolver ErrNotFound, devolveu %v", err)
	}

	n, err := db.UserCount(t.Context())
	if err != nil {
		t.Fatalf("contar credenciais: %v", err)
	}
	if n != 0 {
		t.Errorf("queria 0 credenciais, obtive %d", n)
	}
}
