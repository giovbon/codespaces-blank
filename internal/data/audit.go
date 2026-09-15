package data

import (
	"context"
	"database/sql"
	"errors"
	"financas/internal/domain"
	"fmt"
)

// Ações registradas no journal. São valores estáveis: a interface e os testes
// reagem a eles, nunca a texto de mensagem.
const (
	ActionCreate       = "create"
	ActionUpdate       = "update"
	ActionDelete       = "delete"
	ActionMerge        = "merge"
	ActionImportCommit = "import_commit"
	ActionImportUndo   = "import_undo"
)

// AuditEntry é uma linha do journal de auditoria.
type AuditEntry struct {
	ID         int64
	Entity     string
	EntityID   string
	Action     string
	BeforeJSON string
	AfterJSON  string
	Actor      string
	Origin     string
	At         int64
}

// AuditLog lê o journal de uma entidade, do mais recente para o mais antigo.
//
// É a matéria-prima do undo de um lote (ADR-007): reverter consiste em
// percorrer estas entradas ao contrário, numa transação nova.
func (db *DB) AuditLog(ctx context.Context, entity, entityID string, limit int) ([]AuditEntry, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := db.sql.QueryContext(ctx,
		`SELECT id, entity, entity_id, action,
		        coalesce(before_json, ''), coalesce(after_json, ''),
		        coalesce(actor, ''), coalesce(origin, ''), at
		   FROM audit_log
		  WHERE entity = ? AND entity_id = ?
		  ORDER BY at DESC, id DESC
		  LIMIT ?`,
		entity, entityID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("data: ler auditoria de %s/%s: %w", entity, entityID, err)
	}
	defer func() { _ = rows.Close() }()

	var entradas []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(
			&e.ID, &e.Entity, &e.EntityID, &e.Action,
			&e.BeforeJSON, &e.AfterJSON, &e.Actor, &e.Origin, &e.At,
		); err != nil {
			return nil, fmt.Errorf("data: ler entrada de auditoria: %w", err)
		}
		entradas = append(entradas, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("data: percorrer auditoria: %w", err)
	}
	return entradas, nil
}

// AuditCount conta as entradas de uma entidade. Usado pelos testes do mutator.
func (db *DB) AuditCount(ctx context.Context, entity, entityID string) (int, error) {
	var n int
	err := db.sql.QueryRowContext(ctx,
		`SELECT count(*) FROM audit_log WHERE entity = ? AND entity_id = ?`,
		entity, entityID,
	).Scan(&n)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, domain.ErrNotFound
		}
		return 0, fmt.Errorf("data: contar auditoria de %s/%s: %w", entity, entityID, err)
	}
	return n, nil
}
