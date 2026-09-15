// Package clock fornece o instante atual de forma injetável.
//
// Datas de transação são civis (texto 'YYYY-MM-DD') e não instantes; o relógio
// existe para saber que dia é hoje, e por isso é sempre lido com o fuso da
// aplicação e nunca em UTC.
package clock

import (
	"time"

	// tzdata embutida: a imagem final é distroless e não tem
	// /usr/share/zoneinfo, o que faria America/Sao_Paulo cair silenciosamente
	// em UTC e deslocar todas as datas em três horas.
	_ "time/tzdata"
)

// Timezone por omissão da aplicação.
const Timezone = "America/Sao_Paulo"

// Clock devolve o instante atual.
type Clock interface {
	Now() time.Time
}

// System é o relógio real.
type System struct{}

// Now devolve o instante atual.
func (System) Now() time.Time { return time.Now() }

// Location devolve o fuso pedido, com recurso a America/Sao_Paulo.
func Location(name string) *time.Location {
	if name == "" {
		name = Timezone
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}

// Today devolve a data civil de hoje no fuso indicado.
func Today(agora time.Time, loc *time.Location) string {
	return agora.In(loc).Format("2006-01-02")
}

// Fixed é um relógio fixo, para testes.
type Fixed struct{ T time.Time }

// Now devolve sempre o mesmo instante.
func (f Fixed) Now() time.Time { return f.T }
