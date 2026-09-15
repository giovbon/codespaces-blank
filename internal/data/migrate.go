package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"
)

// migrateTable guarda o que já foi aplicado.
//
// As migrações são só para a frente: nunca se escreve o caminho inverso. Um
// erro corrige-se com uma migração nova, não desfazendo a anterior.
const migrateTable = `
CREATE TABLE IF NOT EXISTS schema_migrations (
  version    TEXT PRIMARY KEY,
  applied_at INTEGER NOT NULL
);`

// Migrate aplica, por ordem, as migrações ainda não aplicadas.
//
// Cada migração corre dentro de uma transação: ou entra por inteiro, ou o
// esquema fica como estava. O nome do arquivo é a versão — '0001_init.sql'.
func (db *DB) Migrate(ctx context.Context, fsys fs.FS) error {
	if _, err := db.sql.ExecContext(ctx, migrateTable); err != nil {
		return fmt.Errorf("data: criar tabela de migrações: %w", err)
	}

	aplicadas, err := db.appliedVersions(ctx)
	if err != nil {
		return err
	}

	nomes, err := migrationNames(fsys)
	if err != nil {
		return err
	}
	// Falhar no arranque é melhor do que arrancar com uma base sem esquema: o
	// sintoma seguinte seria «no such table» a meio de uma operação, longe da
	// causa. Aconteceu uma vez, por o prefixo de pasta não ser resolvido.
	if len(nomes) == 0 {
		return errors.New("data: nenhuma migração encontrada: verificar a raiz do sistema de ficheiros")
	}

	for _, nome := range nomes {
		versao := strings.TrimSuffix(nome, ".sql")
		if _, jaAplicada := aplicadas[versao]; jaAplicada {
			continue
		}

		script, err := fs.ReadFile(fsys, nome)
		if err != nil {
			return fmt.Errorf("data: ler migração %q: %w", nome, err)
		}

		if err := db.applyMigration(ctx, versao, string(script)); err != nil {
			return err
		}
	}

	return nil
}

// applyMigration executa uma migração e registra a versão, tudo numa transação.
//
// O driver SQLite não aceita vários statements num único ExecContext, por isso
// o script é dividido. O esquema não tem procedimentos nem strings com ';'
// fora de literais, o que torna a divisão segura — e um teste cobre isso.
func (db *DB) applyMigration(ctx context.Context, versao, script string) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("data: iniciar transação da migração %q: %w", versao, err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, stmt := range splitStatements(script) {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("data: migração %q falhou: %w", versao, err)
		}
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
		versao, time.Now().UnixMilli(),
	); err != nil {
		return fmt.Errorf("data: registar migração %q: %w", versao, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("data: confirmar migração %q: %w", versao, err)
	}
	return nil
}

func (db *DB) appliedVersions(ctx context.Context) (map[string]struct{}, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("data: ler migrações aplicadas: %w", err)
	}
	defer func() { _ = rows.Close() }()

	aplicadas := make(map[string]struct{})
	for rows.Next() {
		var versao string
		if err := rows.Scan(&versao); err != nil {
			return nil, fmt.Errorf("data: ler migração aplicada: %w", err)
		}
		aplicadas[versao] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: percorrer migrações aplicadas: %w", err)
	}
	return aplicadas, nil
}

// migrationNames devolve os .sql ordenados pelo nome, que é a ordem de aplicação.
func migrationNames(fsys fs.FS) ([]string, error) {
	entradas, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return nil, fmt.Errorf("data: listar migrações: %w", err)
	}
	for _, nome := range entradas {
		if path.Base(nome) != nome {
			return nil, fmt.Errorf("data: migração %q não pode estar em subpasta", nome)
		}
	}
	sort.Strings(entradas)
	return entradas, nil
}

// splitStatements divide um script em instruções completas.
//
// Comentários de linha (--) e de bloco (/* */) são removidos, e um ponto e
// vírgula dentro de um literal de texto não conta como fim de instrução.
func splitStatements(script string) []string {
	var (
		statements []string
		atual      strings.Builder
		emTexto    bool
		emLinha    bool
		emBloco    bool
	)

	runas := []rune(script)
	for i := 0; i < len(runas); i++ {
		c := runas[i]

		switch {
		case emLinha:
			if c == '\n' {
				emLinha = false
				atual.WriteRune(c)
			}

		case emBloco:
			if c == '*' && i+1 < len(runas) && runas[i+1] == '/' {
				emBloco = false
				i++
				// Espaço de separação: sem ele, "a/*x*/b" viraria "ab".
				atual.WriteRune(' ')
			}

		case emTexto:
			atual.WriteRune(c)
			if c == '\'' {
				// '' dentro de um literal é um apóstrofo escapado, não o fim.
				if i+1 < len(runas) && runas[i+1] == '\'' {
					atual.WriteRune(runas[i+1])
					i++
					continue
				}
				emTexto = false
			}

		case c == '-' && i+1 < len(runas) && runas[i+1] == '-':
			emLinha = true
			i++

		case c == '/' && i+1 < len(runas) && runas[i+1] == '*':
			emBloco = true
			i++

		case c == '\'':
			emTexto = true
			atual.WriteRune(c)

		case c == ';':
			if stmt := strings.TrimSpace(atual.String()); stmt != "" {
				statements = append(statements, stmt)
			}
			atual.Reset()

		default:
			atual.WriteRune(c)
		}
	}

	if stmt := strings.TrimSpace(atual.String()); stmt != "" {
		statements = append(statements, stmt)
	}
	return statements
}

// AppliedMigrations devolve as versões aplicadas, para diagnóstico e testes.
func (db *DB) AppliedMigrations(ctx context.Context) ([]string, error) {
	aplicadas, err := db.appliedVersions(ctx)
	if err != nil {
		return nil, err
	}
	versoes := make([]string, 0, len(aplicadas))
	for versao := range aplicadas {
		versoes = append(versoes, versao)
	}
	sort.Strings(versoes)
	return versoes, nil
}

// Tables lista as tabelas e vistas do esquema, em ordem alfabética.
func (db *DB) Tables(ctx context.Context) ([]string, error) {
	rows, err := db.sql.QueryContext(ctx,
		`SELECT name FROM sqlite_master
		  WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%'
		  ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("data: listar tabelas: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var nomes []string
	for rows.Next() {
		var nome string
		if err := rows.Scan(&nome); err != nil {
			return nil, fmt.Errorf("data: ler nome de tabela: %w", err)
		}
		nomes = append(nomes, nome)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: percorrer tabelas: %w", err)
	}
	return nomes, nil
}

// HasTable informa se uma tabela existe. Usado pelos testes de esquema.
func (db *DB) HasTable(ctx context.Context, nome string) (bool, error) {
	var existe int
	err := db.sql.QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, nome,
	).Scan(&existe)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("data: procurar tabela %q: %w", nome, err)
	}
	return existe > 0, nil
}
