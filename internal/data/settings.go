package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// Chaves de definições usadas pela aplicação.
const (
	// SettingDisplayNames guarda os nomes oferecidos no campo «quem está a
	// usar» (ADR-019). É declaração, não autenticação.
	SettingDisplayNames = "display_names"

	// SettingCurrency é a moeda base, 'BRL'.
	SettingCurrency = "currency"

	// SettingTimezone é o fuso usado para datas civis, 'America/Sao_Paulo'.
	SettingTimezone = "timezone"
)

// Setting devolve o valor cru de uma definição.
func (db *DB) Setting(ctx context.Context, key string) (string, bool, error) {
	var valor string
	err := db.sql.QueryRowContext(ctx,
		`SELECT value_json FROM app_settings WHERE key = ?`, key,
	).Scan(&valor)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("data: ler definição %q: %w", key, err)
	}
	return valor, true, nil
}

// SettingJSON descodifica uma definição para dest. Devolve false se não existir.
func (db *DB) SettingJSON(ctx context.Context, key string, dest any) (bool, error) {
	valor, existe, err := db.Setting(ctx, key)
	if err != nil || !existe {
		return false, err
	}
	if err := json.Unmarshal([]byte(valor), dest); err != nil {
		return false, fmt.Errorf("data: descodificar definição %q: %w", key, err)
	}
	return true, nil
}

// PutSetting grava uma definição, substituindo a anterior.
func (t *Tx) PutSetting(ctx context.Context, key string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("data: codificar definição %q: %w", key, err)
	}

	_, err = t.tx.ExecContext(ctx,
		`INSERT INTO app_settings (key, value_json, updated_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json,
		                                updated_at = excluded.updated_at`,
		key, string(b), t.now,
	)
	if err != nil {
		return fmt.Errorf("data: gravar definição %q: %w", key, err)
	}
	return nil
}
