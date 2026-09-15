// Package data é a única camada que fala SQL.
//
// Nenhum outro pacote escreve consultas (docs/10 §3). Toda a escrita passa por
// um Mutator, que abre transação, incrementa revision e registra auditoria
// (ADR-007).
package data

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // driver SQLite em Go puro: CGO_ENABLED=0 sempre
)

// Config reúne o que a camada de dados precisa para abrir a base.
type Config struct {
	// Path é o caminho do arquivo SQLite. A pasta é criada se não existir.
	Path string

	// Migrations é o sistema de ficheiros com os .sql, aplicados por ordem.
	// Vem da raiz do módulo (financas.Migrations) para que este pacote não
	// dependa do pacote raiz.
	Migrations fs.FS
}

// DB é o acesso à base de dados.
type DB struct {
	sql  *sql.DB
	path string
}

// Open abre a base, aplica as PRAGMAs, cria o esquema e devolve o acesso.
//
// As PRAGMAs vão no DSN e não numa instrução solta: com um pool de conexões,
// uma PRAGMA executada uma vez só vale para aquela conexão, e o modo WAL
// silenciosamente deixaria de estar ativo nas restantes.
func Open(ctx context.Context, cfg Config) (*DB, error) {
	if cfg.Path == "" {
		return nil, fmt.Errorf("data: caminho da base de dados é obrigatório")
	}
	if cfg.Migrations == nil {
		return nil, fmt.Errorf("data: sistema de ficheiros das migrações é obrigatório")
	}

	if dir := filepath.Dir(cfg.Path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("data: criar pasta %q: %w", dir, err)
		}
	}

	dsn := dsnFor(cfg.Path)

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("data: abrir base %q: %w", cfg.Path, err)
	}

	// Uma única conexão. Com WAL, leitores concorrentes seriam possíveis, mas
	// um escritor e um leitor no mesmo pool produzem SQLITE_BUSY em padrões
	// difíceis de reproduzir. Para um serviço de duas pessoas, serializar é a
	// escolha que remove a classe inteira de erros; reavaliar se uma consulta
	// lenta passar a bloquear a interface.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("data: ligar à base %q: %w", cfg.Path, err)
	}

	db := &DB{sql: sqlDB, path: cfg.Path}

	if err := db.Migrate(ctx, cfg.Migrations); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	return db, nil
}

// dsnFor monta o DSN com as PRAGMAs de docs/03 §1.
func dsnFor(path string) string {
	pragmas := []string{
		"journal_mode(WAL)",   // leituras concorrentes com um escritor
		"foreign_keys(1)",     // nenhuma referência órfã
		"busy_timeout(5000)",  // espera em vez de falhar de imediato
		"synchronous(NORMAL)", // durabilidade adequada com WAL
		"temp_store(MEMORY)",  // temporários em memória
	}

	query := url.Values{}
	for _, p := range pragmas {
		query.Add("_pragma", p)
	}
	// Bloqueio de escrita tomado no início da transação, e não promovido a
	// meio: evita SQLITE_BUSY em padrões difíceis de reproduzir.
	query.Set("_txlock", "immediate")

	return "file:" + path + "?" + query.Encode()
}

// Close fecha a base.
func (db *DB) Close() error { return db.sql.Close() }

// Path é o caminho do arquivo em uso.
func (db *DB) Path() string { return db.path }

// SQL devolve o handle de leitura. Existe para as consultas desta camada e para
// os testes; nenhum outro pacote deve usá-lo para escrever.
func (db *DB) SQL() *sql.DB { return db.sql }
