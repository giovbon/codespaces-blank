package domain_test

import (
	"errors"
	"financas/internal/domain"
	"testing"
	"time"
)

func TestParseDate(t *testing.T) {
	validas := []string{"2026-01-01", "2026-02-28", "2024-02-29", "2026-12-31"}

	for _, texto := range validas {
		t.Run(texto, func(t *testing.T) {
			data, err := domain.ParseDate(texto)
			if err != nil {
				t.Fatalf("ParseDate(%q) falhou: %v", texto, err)
			}
			if data.String() != texto {
				t.Errorf("ParseDate(%q) = %q", texto, data.String())
			}
		})
	}
}

func TestParseDateRejeitadas(t *testing.T) {
	// 31/02/2026 não existe, e o parser do Go normalizaria para 03/03 —
	// aceitar isso corromperia a data do lançamento em silêncio.
	invalidas := []string{
		"",
		"2026-1-1",
		"01/01/2026",
		"2026-02-31",
		"2026-13-01",
		"2026-00-10",
		"2026-02-30",
		"2026-02-29",
		"ontem",
		"2026-12-31T00:00:00Z",
	}

	for _, texto := range invalidas {
		t.Run(texto, func(t *testing.T) {
			if _, err := domain.ParseDate(texto); !errors.Is(err, domain.ErrDateFormat) {
				t.Errorf("ParseDate(%q) devia devolver ErrDateFormat, devolveu %v", texto, err)
			}
		})
	}
}

func TestDateAddDays(t *testing.T) {
	casos := []struct {
		origem string
		dias   int
		quero  string
	}{
		{"2026-09-15", 7, "2026-09-22"},  // soma simples
		{"2026-09-28", 7, "2026-10-05"},  // atravessa o fim do mês
		{"2026-12-30", 3, "2027-01-02"},  // atravessa o fim do ano
		{"2026-03-01", -1, "2026-02-28"}, // subtração
		{"2024-02-28", 1, "2024-02-29"},  // ano bissexto
		{"2026-09-15", 0, "2026-09-15"},  // identidade
	}

	for _, caso := range casos {
		t.Run(caso.origem, func(t *testing.T) {
			data, err := domain.ParseDate(caso.origem)
			if err != nil {
				t.Fatalf("data de origem inválida: %v", err)
			}
			if obtido := data.AddDays(caso.dias).String(); obtido != caso.quero {
				t.Errorf("%s + %d dias = %q, queria %q", caso.origem, caso.dias, obtido, caso.quero)
			}
		})
	}
}

func TestDateOrdenacao(t *testing.T) {
	// A comparação é textual, e é por isso que o formato é fixo e com zeros à
	// esquerda: 'YYYY-MM-DD' ordena corretamente como texto.
	menor, _ := domain.ParseDate("2026-09-09")
	maior, _ := domain.ParseDate("2026-09-15")

	if !menor.Before(maior) {
		t.Errorf("%s devia ser anterior a %s", menor, maior)
	}
	if !maior.After(menor) {
		t.Errorf("%s devia ser posterior a %s", maior, menor)
	}
	if menor.Before(menor) || menor.After(menor) {
		t.Error("uma data não é anterior nem posterior a si mesma")
	}
}

func TestDateMonth(t *testing.T) {
	data, _ := domain.ParseDate("2026-09-15")
	if obtido := data.Month(); obtido != "2026-09" {
		t.Errorf("Month() = %q, queria %q", obtido, "2026-09")
	}
}

func TestNewDate(t *testing.T) {
	if obtido := domain.NewDate(2026, time.September, 5).String(); obtido != "2026-09-05" {
		t.Errorf("NewDate devolveu %q, queria %q", obtido, "2026-09-05")
	}
}

func TestDateZero(t *testing.T) {
	var vazia domain.Date
	if !vazia.IsZero() {
		t.Error("a data vazia devia ser zero")
	}
	data, _ := domain.ParseDate("2026-01-01")
	if data.IsZero() {
		t.Error("uma data preenchida não é zero")
	}
}
