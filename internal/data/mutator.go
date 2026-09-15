package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Write identifica quem escreve e por que caminho.
//
// O ator é uma declaração, não uma autenticação: com senha única para até duas
// pessoas, o nome escolhido ao entrar é o que fica no journal (ADR-019). O
// objetivo é poder responder «quem mudou isto» sem introduzir contas separadas.
type Write struct {
	Actor  string // nome declarado por quem está a usar; "system" se não houver
	Origin string // 'ui','import','rule','api','system'
}

// Write do sistema, usado por jobs e migrações de dados.
var SystemWrite = Write{Actor: "system", Origin: "system"}

// Mutator é o único caminho de escrita da aplicação.
//
// Não escrever diretamente nas tabelas de negócio: o mutator é o que garante
// transação, revision e auditoria no mesmo âmbito (ADR-007).
type Mutator struct {
	db *DB
}

// NewMutator devolve o caminho de escrita para uma base já aberta.
func NewMutator(db *DB) *Mutator { return &Mutator{db: db} }

// Run executa fn dentro de uma transação e só confirma se fn terminar sem erro.
//
// As entradas de auditoria acumuladas por fn são gravadas na mesma transação:
// ou a alteração e o seu registo entram os dois, ou não entra nenhum.
func (m *Mutator) Run(ctx context.Context, w Write, fn func(*Tx) error) error {
	if fn == nil {
		return fmt.Errorf("data: mutator sem função de escrita")
	}
	if w.Actor == "" {
		w.Actor = "system"
	}
	if w.Origin == "" {
		w.Origin = "system"
	}

	// O DSN usa _txlock=immediate: a transação toma o bloqueio de escrita no
	// início, em vez de o tentar promover a meio, que é a origem do
	// SQLITE_BUSY em padrões difíceis de reproduzir.
	tx, err := m.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("data: iniciar transação: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	t := &Tx{tx: tx, w: w, now: time.Now().UnixMilli()}

	if err := fn(t); err != nil {
		// Rollback pelo defer: o erro de quem chamou é o que interessa.
		return err
	}

	if err := t.flushAudit(ctx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("data: confirmar transação: %w", err)
	}
	return nil
}

// Tx é o acesso de escrita dentro de uma transação.
type Tx struct {
	tx      *sql.Tx
	w       Write
	now     int64
	entries []AuditEntry
}

// Now devolve o instante da transação, em milissegundos. Fixo durante a
// transação para que created_at e updated_at de um lote sejam coerentes.
func (t *Tx) Now() int64 { return t.now }

// Write devolve quem está a escrever.
func (t *Tx) Write() Write { return t.w }

// Exec executa uma instrução de escrita.
func (t *Tx) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return t.tx.ExecContext(ctx, query, args...)
}

// QueryRow executa uma consulta de uma linha dentro da transação.
func (t *Tx) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return t.tx.QueryRowContext(ctx, query, args...)
}

// Query executa uma consulta dentro da transação.
func (t *Tx) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return t.tx.QueryContext(ctx, query, args...)
}

// Audit registra uma alteração no journal.
//
// before e after são gravados como JSON; nil significa «não havia» ou «deixou
// de haver». Não falha por erro de serialização em tipos do domínio — se
// falhar, é defeito de programação e não se perdoa em silêncio.
func (t *Tx) Audit(entity, entityID, action string, before, after any) error {
	antes, err := marshalOrNull(before)
	if err != nil {
		return fmt.Errorf("data: auditoria de %s/%s: estado anterior: %w", entity, entityID, err)
	}
	depois, err := marshalOrNull(after)
	if err != nil {
		return fmt.Errorf("data: auditoria de %s/%s: estado posterior: %w", entity, entityID, err)
	}

	t.entries = append(t.entries, AuditEntry{
		Entity:     entity,
		EntityID:   entityID,
		Action:     action,
		BeforeJSON: antes,
		AfterJSON:  depois,
		Actor:      t.w.Actor,
		Origin:     t.w.Origin,
		At:         t.now,
	})
	return nil
}

// flushAudit grava as entradas acumuladas, pela ordem em que foram pedidas.
func (t *Tx) flushAudit(ctx context.Context) error {
	for _, e := range t.entries {
		if _, err := t.tx.ExecContext(ctx,
			`INSERT INTO audit_log
			   (entity, entity_id, action, before_json, after_json, actor, origin, at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			e.Entity, e.EntityID, e.Action, e.BeforeJSON, e.AfterJSON, e.Actor, e.Origin, e.At,
		); err != nil {
			return fmt.Errorf("data: gravar auditoria de %s/%s: %w", e.Entity, e.EntityID, err)
		}
	}
	t.entries = nil
	return nil
}

func marshalOrNull(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
