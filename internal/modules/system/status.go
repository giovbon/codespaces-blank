// Package system expõe o estado do próprio sistema.
//
// Existe para que a fundação seja verificável a partir do navegador: sem isto,
// confirmar que o esquema foi aplicado exigiria abrir a base com um cliente
// SQL — precisamente o tipo de passo manual que o projeto quer evitar.
package system

import (
	"context"
	"financas/internal/data"
)

// Status descreve o estado da base de dados.
type Status struct {
	DBPath     string
	Migrations []string
	Tables     []string
}

// Service lê o estado do sistema.
type Service struct {
	db  *data.DB
	mut *data.Mutator
}

// New cria o serviço.
func New(db *data.DB, mut *data.Mutator) *Service { return &Service{db: db, mut: mut} }

// Status devolve o estado atual.
func (s *Service) Status(ctx context.Context) (Status, error) {
	migracoes, err := s.db.AppliedMigrations(ctx)
	if err != nil {
		return Status{}, err
	}
	tabelas, err := s.db.Tables(ctx)
	if err != nil {
		return Status{}, err
	}

	return Status{
		DBPath:     s.db.Path(),
		Migrations: migracoes,
		Tables:     tabelas,
	}, nil
}

// DisplayNames devolve os nomes oferecidos no campo «quem está a usar».
//
// É uma lista opcional, guardada nas definições: serve apenas para atribuir o
// que fica no registo de auditoria, e não é autenticação (ADR-019). Quando
// está vazia, o campo simplesmente não aparece.
func (s *Service) DisplayNames(ctx context.Context) ([]string, error) {
	var nomes []string
	if _, err := s.db.SettingJSON(ctx, data.SettingDisplayNames, &nomes); err != nil {
		return nil, err
	}
	return nomes, nil
}

// DefineDisplayNames grava os nomes oferecidos no ecrã de entrada.
func (s *Service) DefineDisplayNames(ctx context.Context, w data.Write, nomes []string) error {
	return s.mut.Run(ctx, w, func(tx *data.Tx) error {
		if err := tx.PutSetting(ctx, data.SettingDisplayNames, nomes); err != nil {
			return err
		}
		return tx.Audit("app_setting", data.SettingDisplayNames, data.ActionUpdate, nil, nomes)
	})
}
