package domain_test

import (
	"errors"
	"financas/internal/domain"
	"testing"
)

func TestParseCents(t *testing.T) {
	casos := []struct {
		nome  string
		texto string
		quero domain.Cents
	}{
		{"simples", "1234", 123400},
		{"virgula com dois digitos", "1.234,56", 123456},
		{"virgula com um digito", "1234,5", 123450},
		{"ponto como decimal sem virgula", "1234.56", 123456},
		{"negativo com sinal", "-1.234,56", -123456},
		{"negativo com parenteses", "(1.234,56)", -123456},
		{"positivo com sinal", "+42,00", 4200},
		{"zero", "0", 0},
		{"zero com decimais", "0,00", 0},
		{"sem parte inteira", ",50", 50},
		{"espacos em volta", "  1.234,56  ", 123456},
		{"milhoes", "1.234.567,89", 123456789},
		{"negativo de parenteses com sinal", "(-10,00)", 1000},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			obtido, err := domain.ParseCents(caso.texto)
			if err != nil {
				t.Fatalf("ParseCents(%q) devolveu erro inesperado: %v", caso.texto, err)
			}
			if obtido != caso.quero {
				t.Errorf("ParseCents(%q) = %d, queria %d", caso.texto, obtido, caso.quero)
			}
		})
	}
}

func TestParseCentsRejeitados(t *testing.T) {
	// "1.2.3,45" tem agrupamento de milhar inválido e "12,345" tem três casas
	// decimais: em ambos a leitura seria ambígua, e adivinhar é pior do que
	// falhar (docs/04 §5.2).
	entradas := []string{
		"",
		"   ",
		"abc",
		"12,345",
		"1.2.3,45",
		"1,23,45",
		"R$ 10,00",
		"10 00",
		"1e5",
		"-",
		"()",
	}

	for _, entrada := range entradas {
		t.Run(entrada, func(t *testing.T) {
			_, err := domain.ParseCents(entrada)
			if err == nil {
				t.Fatalf("ParseCents(%q) devia ter falhado", entrada)
			}
			if !errors.Is(err, domain.ErrAmountEmpty) &&
				!errors.Is(err, domain.ErrAmountFormat) &&
				!errors.Is(err, domain.ErrAmountTooLarge) {
				t.Errorf("erro %v não é um dos erros de quantia previstos", err)
			}
		})
	}
}

func TestParseCentsErroTipado(t *testing.T) {
	if _, err := domain.ParseCents(""); !errors.Is(err, domain.ErrAmountEmpty) {
		t.Errorf("quantia vazia devia devolver ErrAmountEmpty, devolveu %v", err)
	}
	if _, err := domain.ParseCents("abc"); !errors.Is(err, domain.ErrAmountFormat) {
		t.Errorf("formato inválido devia devolver ErrAmountFormat, devolveu %v", err)
	}
	// 20 dígitos não cabem num int64 de cêntimos.
	grande := "99999999999999999999"
	if _, err := domain.ParseCents(grande); !errors.Is(err, domain.ErrAmountTooLarge) {
		t.Errorf("quantia fora de intervalo devia devolver ErrAmountTooLarge, devolveu %v", err)
	}
}

func TestFormatCents(t *testing.T) {
	casos := []struct {
		valor domain.Cents
		quero string
	}{
		{0, "0,00"},
		{5, "0,05"},
		{50, "0,50"},
		{123456, "1.234,56"},
		{-123456, "-1.234,56"},
		{123456789, "1.234.567,89"},
		{-1, "-0,01"},
	}

	for _, caso := range casos {
		t.Run(caso.quero, func(t *testing.T) {
			if obtido := caso.valor.String(); obtido != caso.quero {
				t.Errorf("%d formatado = %q, queria %q", caso.valor, obtido, caso.quero)
			}
		})
	}
}

func TestCentsRoundTrip(t *testing.T) {
	// Formatar e voltar a ler tem de devolver o mesmo valor: é a garantia de
	// que a UI e o banco não divergem.
	for _, valor := range []domain.Cents{0, 1, -1, 99, -99, 100, 123456, -987654321} {
		texto := valor.String()
		volta, err := domain.ParseCents(texto)
		if err != nil {
			t.Fatalf("ParseCents(%q) falhou: %v", texto, err)
		}
		if volta != valor {
			t.Errorf("%d -> %q -> %d", valor, texto, volta)
		}
	}
}

func TestCentsAbs(t *testing.T) {
	if obtido := domain.Cents(-4200).Abs(); obtido != 4200 {
		t.Errorf("Abs(-4200) = %d, queria 4200", obtido)
	}
	if obtido := domain.Cents(4200).Abs(); obtido != 4200 {
		t.Errorf("Abs(4200) = %d, queria 4200", obtido)
	}
	if domain.Cents(1).IsZero() {
		t.Error("1 não é zero")
	}
	if !domain.Cents(0).IsZero() {
		t.Error("0 é zero")
	}
}
