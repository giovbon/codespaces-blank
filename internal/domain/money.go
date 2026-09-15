// Package domain contém os tipos e as regras do domínio.
//
// Este pacote não importa data, infra, http, adapters, views nem modules — a
// regra é verificada por teste (fronteira_test.go) e não por disciplina.
package domain

import (
	"errors"
	"strconv"
	"strings"
)

// Cents é uma quantia monetária em cêntimos.
//
// Dinheiro nunca é representado por ponto flutuante — nem numa expressão
// intermédia (docs/03 §1). Negativo significa saída.
type Cents int64

// Erros de conversão de quantias.
var (
	ErrAmountEmpty    = errors.New("quantia vazia")
	ErrAmountFormat   = errors.New("formato de quantia inválido")
	ErrAmountTooLarge = errors.New("quantia fora do intervalo representável")
)

// ParseCents interpreta uma quantia no formato usado pelos extratos.
//
// Regras aceitas:
//
//	"1.234,56"  ->  123456     ponto como milhar, vírgula como decimal
//	"-1.234,56" -> -123456     sinal antes
//	"(1.234,56)" -> -123456    parênteses como negativo (notação contabilística)
//	"1234,5"    ->  123450     uma casa decimal
//	"1234"      ->  123400     sem parte decimal
//	"1234.56"   ->  123456     ponto como decimal, quando não há vírgula
//
// Não decide ambiguidades de formato do ficheiro: quando o separador é
// ambíguo, quem chama tem de o resolver pelo perfil e pelo conjunto de linhas
// (docs/04 §5.2). Aqui a interpretação é determinística.
func ParseCents(s string) (Cents, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return 0, ErrAmountEmpty
	}

	negative := false

	// Parênteses: convenção contabilística para negativo.
	if strings.HasPrefix(raw, "(") && strings.HasSuffix(raw, ")") {
		negative = true
		raw = strings.TrimSpace(raw[1 : len(raw)-1])
	}

	// Sinal explícito.
	switch {
	case strings.HasPrefix(raw, "-"):
		negative = !negative
		raw = strings.TrimSpace(raw[1:])
	case strings.HasPrefix(raw, "+"):
		raw = strings.TrimSpace(raw[1:])
	}

	if raw == "" {
		return 0, ErrAmountFormat
	}

	decimalSep := ","
	if !strings.Contains(raw, ",") {
		decimalSep = "."
	}

	intPart := raw
	fracPart := ""
	if i := strings.LastIndex(raw, decimalSep); i >= 0 {
		intPart = raw[:i]
		fracPart = raw[i+1:]
	}

	// O separador de milhar só pode separar grupos de três dígitos.
	intDigits := strings.ReplaceAll(intPart, ".", "")
	if intDigits == "" {
		intDigits = "0"
	}
	if err := validThousands(intPart, decimalSep); err != nil {
		return 0, err
	}
	if !allDigits(intDigits) {
		return 0, ErrAmountFormat
	}
	if len(fracPart) > 2 {
		return 0, ErrAmountFormat
	}
	if fracPart != "" && !allDigits(fracPart) {
		return 0, ErrAmountFormat
	}

	// Normaliza a parte decimal para exatamente dois dígitos.
	if len(fracPart) == 1 {
		fracPart += "0"
	}
	if fracPart == "" {
		fracPart = "00"
	}

	total := intDigits + fracPart
	n, err := strconv.ParseInt(total, 10, 64)
	if err != nil {
		return 0, ErrAmountTooLarge
	}
	if negative {
		n = -n
	}
	return Cents(n), nil
}

// validThousands confirma que o separador de milhar, quando presente, agrupa
// dígitos de três em três. É o que distingue "1.234,56" de "1.2.3,56".
func validThousands(intPart, decimalSep string) error {
	if decimalSep != "," || intPart == "" {
		// Sem vírgula, o ponto é decimal: não há separador de milhar. Sem parte
		// inteira (",50"), também não há agrupamento a validar.
		return nil
	}
	if !strings.Contains(intPart, ".") {
		// Parte inteira sem ponto: "1234" é o número inteiro, e não há
		// agrupamento a validar.
		return nil
	}

	groups := strings.Split(intPart, ".")
	if len(groups[0]) == 0 || len(groups[0]) > 3 {
		return ErrAmountFormat
	}
	for _, g := range groups[1:] {
		if len(g) != 3 {
			return ErrAmountFormat
		}
	}
	return nil
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// String devolve a quantia no formato brasileiro: "1.234,56" e "-1.234,56".
func (c Cents) String() string {
	n := int64(c)
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}

	units := strconv.FormatInt(n/100, 10)
	decs := strconv.FormatInt(n%100, 10)
	if len(decs) == 1 {
		decs = "0" + decs
	}

	return sign + groupThousands(units) + "," + decs
}

// groupThousands separa os milhares com ponto: "1234567" -> "1.234.567".
func groupThousands(digits string) string {
	if len(digits) <= 3 {
		return digits
	}
	var out strings.Builder
	lead := len(digits) % 3
	if lead > 0 {
		out.WriteString(digits[:lead])
	}
	for i := lead; i < len(digits); i += 3 {
		if out.Len() > 0 {
			out.WriteByte('.')
		}
		out.WriteString(digits[i : i+3])
	}
	return out.String()
}

// Abs devolve a quantia sem sinal.
func (c Cents) Abs() Cents {
	if c < 0 {
		return -c
	}
	return c
}

// IsZero indica se a quantia é exatamente zero.
func (c Cents) IsZero() bool { return c == 0 }
