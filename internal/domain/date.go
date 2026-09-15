package domain

import (
	"errors"
	"time"
)

// LayoutDate é o único formato aceito para datas civis.
const LayoutDate = "2006-01-02"

// ErrDateFormat indica uma data fora do formato 'YYYY-MM-DD' ou inexistente
// no calendário (ex.: 31 de fevereiro).
var ErrDateFormat = errors.New("data inválida: esperado 'YYYY-MM-DD'")

// Date é uma data civil, guardada como texto 'YYYY-MM-DD'.
//
// Um lançamento bancário é um dia de calendário, não um instante. Representá-lo
// como instante UTC é a origem clássica do lançamento a «cair» no dia anterior
// (docs/03 §1). Por isso não existe conversão implícita para time.Time.
type Date string

// ParseDate valida e devolve uma data civil.
func ParseDate(s string) (Date, error) {
	if len(s) != len(LayoutDate) {
		return "", ErrDateFormat
	}
	t, err := time.Parse(LayoutDate, s)
	if err != nil {
		return "", ErrDateFormat
	}
	// Rejeita datas normalizadas pelo parser (ex.: 31 de fevereiro -> 3 de março).
	if t.Format(LayoutDate) != s {
		return "", ErrDateFormat
	}
	return Date(s), nil
}

// NewDate constrói uma data civil a partir dos componentes.
func NewDate(year int, month time.Month, day int) Date {
	return Date(time.Date(year, month, day, 0, 0, 0, 0, time.UTC).Format(LayoutDate))
}

// String devolve a data no formato de armazenamento.
func (d Date) String() string { return string(d) }

// IsZero indica ausência de data.
func (d Date) IsZero() bool { return d == "" }

// AddDays devolve a data deslocada em n dias, sem fuso horário envolvido.
func (d Date) AddDays(n int) Date {
	t, err := time.Parse(LayoutDate, string(d))
	if err != nil {
		return d
	}
	return Date(t.AddDate(0, 0, n).Format(LayoutDate))
}

// Before indica se d é anterior a other.
func (d Date) Before(other Date) bool { return string(d) < string(other) }

// After indica se d é posterior a other.
func (d Date) After(other Date) bool { return string(d) > string(other) }

// Month devolve o mês no formato 'YYYY-MM', usado pelo painel de cobertura.
func (d Date) Month() string {
	if len(d) < 7 {
		return ""
	}
	return string(d[:7])
}
