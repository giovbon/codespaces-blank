package domain

import (
	"crypto/rand"
	"time"
)

// crockford é o alfabeto Crockford base32, usado pelo ULID. Exclui as letras
// I, L, O e U para evitar confusão na leitura humana.
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// LenID é o comprimento de um identificador.
const LenID = 26

// NewID gera um identificador ULID.
//
// ULID e não UUID porque as chaves primárias são ordenáveis por tempo, o que
// ajuda na depuração e na paginação (docs/03 §1). Os primeiros 10 caracteres
// são o instante em milissegundos e os 16 restantes são aleatórios.
func NewID() string { return NewIDAt(time.Now()) }

// NewIDAt gera um identificador com o instante indicado. Existe para que os
// testes sejam determinísticos e para reprocessamentos fiéis.
func NewIDAt(t time.Time) string {
	var id [LenID]byte

	// 48 bits de instante em milissegundos, em 10 caracteres de 5 bits.
	ms := uint64(t.UnixMilli()) & 0xFFFFFFFFFFFF
	for i := 9; i >= 0; i-- {
		id[i] = crockford[ms&31]
		ms >>= 5
	}

	// 80 bits aleatórios, em 16 caracteres de 5 bits.
	var buf [10]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand não falha em plataformas suportadas; se falhar, não há
		// identificador seguro para gerar.
		panic("financas: falha ao ler entropia: " + err.Error())
	}

	acc := uint32(0)
	bits := 0
	pos := LenID
	for _, b := range buf {
		acc = acc<<8 | uint32(b)
		bits += 8
		for bits >= 5 {
			bits -= 5
			pos--
			id[pos] = crockford[(acc>>uint(bits))&31]
		}
	}

	return string(id[:])
}

// IsValidID informa se s é um identificador bem formado.
func IsValidID(s string) bool {
	if len(s) != LenID {
		return false
	}
	for i := range len(s) {
		if !isCrockford(s[i]) {
			return false
		}
	}
	return true
}

func isCrockford(c byte) bool {
	for i := range len(crockford) {
		if crockford[i] == c {
			return true
		}
	}
	return false
}

// TimeOfID recupera o instante codificado no identificador. Devolve o instante
// zero se o identificador for inválido.
func TimeOfID(s string) time.Time {
	if !IsValidID(s) {
		return time.Time{}
	}
	var ms uint64
	for i := range 10 {
		ms = ms<<5 | uint64(decodeCrockford(s[i]))
	}
	return time.UnixMilli(int64(ms))
}

func decodeCrockford(c byte) byte {
	for i := range len(crockford) {
		if crockford[i] == c {
			return byte(i)
		}
	}
	return 0
}
