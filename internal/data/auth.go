package data

import (
	"context"
	"database/sql"
	"errors"
	"financas/internal/domain"
	"fmt"
)

// User é a credencial da aplicação.
//
// Existe exatamente uma linha: senha única para até duas pessoas que não usam
// ao mesmo tempo (ADR-019). Não há email, registo nem recuperação.
type User struct {
	ID           string
	PasswordHash string
	CreatedAt    int64
	UpdatedAt    int64
}

// Session é uma sessão ativa. O token em claro nunca é guardado — só o hash.
type Session struct {
	ID        string
	UserID    string
	TokenHash string
	ActorName string // nome declarado ao entrar; vazio se não foi indicado
	ExpiresAt int64
	CreatedAt int64
	UserAgent string
	IP        string
}

// UserCount informa quantas credenciais existem.
func (db *DB) UserCount(ctx context.Context) (int, error) {
	var n int
	if err := db.sql.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
		return 0, fmt.Errorf("data: contar credenciais: %w", err)
	}
	return n, nil
}

// Credential devolve a credencial existente.
//
// Devolve domain.ErrNotFound quando ainda não foi definida senha: é o que
// distingue «primeiro arranque» de «erro».
func (db *DB) Credential(ctx context.Context) (User, error) {
	var u User
	err := db.sql.QueryRowContext(ctx,
		`SELECT id, password_hash, created_at, updated_at FROM users LIMIT 1`,
	).Scan(&u.ID, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, domain.ErrNotFound
		}
		return User{}, fmt.Errorf("data: ler credencial: %w", err)
	}
	return u, nil
}

// InsertUser cria a credencial. Só deve correr no primeiro arranque.
func (t *Tx) InsertUser(ctx context.Context, u User) error {
	_, err := t.tx.ExecContext(ctx,
		`INSERT INTO users (id, password_hash, created_at, updated_at)
		 VALUES (?, ?, ?, ?)`,
		u.ID, u.PasswordHash, u.CreatedAt, u.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("data: criar credencial: %w", err)
	}
	return nil
}

// UpdatePasswordHash troca o hash da senha.
func (t *Tx) UpdatePasswordHash(ctx context.Context, id, hash string) error {
	res, err := t.tx.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`,
		hash, t.now, id,
	)
	if err != nil {
		return fmt.Errorf("data: atualizar senha: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// InsertSession grava uma sessão nova.
func (t *Tx) InsertSession(ctx context.Context, s Session) error {
	_, err := t.tx.ExecContext(ctx,
		`INSERT INTO sessions
		   (id, user_id, token_hash, actor_name, expires_at, created_at, user_agent, ip)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.UserID, s.TokenHash,
		nullableString(s.ActorName), s.ExpiresAt, s.CreatedAt,
		nullableString(s.UserAgent), nullableString(s.IP),
	)
	if err != nil {
		return fmt.Errorf("data: gravar sessão: %w", err)
	}
	return nil
}

// SessionByTokenHash devolve a sessão cujo token confere, se ainda for válida.
func (db *DB) SessionByTokenHash(ctx context.Context, tokenHash string, now int64) (Session, error) {
	var s Session
	var actor, agent, ip sql.NullString

	err := db.sql.QueryRowContext(ctx,
		`SELECT id, user_id, token_hash, actor_name, expires_at, created_at, user_agent, ip
		   FROM sessions
		  WHERE token_hash = ? AND expires_at > ?`,
		tokenHash, now,
	).Scan(&s.ID, &s.UserID, &s.TokenHash, &actor, &s.ExpiresAt, &s.CreatedAt, &agent, &ip)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Session{}, domain.ErrNotFound
		}
		return Session{}, fmt.Errorf("data: ler sessão: %w", err)
	}

	s.ActorName = actor.String
	s.UserAgent = agent.String
	s.IP = ip.String
	return s, nil
}

// DeleteSession encerra uma sessão.
func (t *Tx) DeleteSession(ctx context.Context, id string) error {
	if _, err := t.tx.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id); err != nil {
		return fmt.Errorf("data: encerrar sessão: %w", err)
	}
	return nil
}

// DeleteSessionsForUser encerra todas as sessões de uma credencial. É o efeito
// de trocar a senha: quem estava dentro deixa de estar.
func (t *Tx) DeleteSessionsForUser(ctx context.Context, userID string) error {
	if _, err := t.tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("data: encerrar sessões: %w", err)
	}
	return nil
}

// DeleteExpiredSessions remove sessões já vencidas.
//
// Sessões não têm tombstone: são estado operacional, e não uma entidade de
// negócio (docs/03 §1). Apagá-las não perde histórico — o journal fica.
func (t *Tx) DeleteExpiredSessions(ctx context.Context, now int64) error {
	if _, err := t.tx.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, now); err != nil {
		return fmt.Errorf("data: limpar sessões vencidas: %w", err)
	}
	return nil
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
