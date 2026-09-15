package domain

import "errors"

// Erros sentinela do domínio. Quem chama compara com errors.Is, nunca com o
// texto da mensagem — o texto muda, o erro não.
var (
	// ErrNotFound indica que o registo não existe ou está tombstoned.
	ErrNotFound = errors.New("registo não encontrado")

	// ErrConflict indica que a revisão esperada não corresponde à guardada:
	// outra escrita aconteceu entre a leitura e a gravação (concorrência
	// otimista, docs/03).
	ErrConflict = errors.New("revisão desatualizada: o registo mudou entretanto")

	// ErrInvalid indica dados que violam uma invariante do domínio.
	ErrInvalid = errors.New("dados inválidos")

	// ErrReadOnly indica tentativa de escrita fora de uma transação.
	ErrReadOnly = errors.New("escrita fora de transação")
)
