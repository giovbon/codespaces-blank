// Package auth implementa a credencial única da aplicação.
//
// Uma senha para até duas pessoas que não usam ao mesmo tempo, sem registo,
// sem permissões e sem recuperação pela interface (ADR-019).
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Parâmetros do argon2id.
//
// t=3 e 64 MiB situam-se acima do mínimo recomendado pela OWASP para 2026 e
// continuam confortáveis num ARM modesto: o custo só se paga no login, que é
// raro, e não em cada pedido.
const (
	argonTime    = 3
	argonMemory  = 64 * 1024 // KiB, ou seja 64 MiB
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

// MinPasswordLen é o comprimento mínimo aceito para a senha.
const MinPasswordLen = 10

// Erros de senha.
var (
	ErrPasswordTooShort = fmt.Errorf("a senha tem de ter pelo menos %d caracteres", MinPasswordLen)
	ErrPasswordHash     = errors.New("hash de senha inválido")
)

// HashPassword devolve a senha codificada em formato PHC.
//
// O formato é `$argon2id$v=19$m=...,t=...,p=...$salt$hash`, que guarda os
// parâmetros junto ao hash — permitir mudá-los mais tarde sem invalidar as
// senhas existentes.
func HashPassword(password string) (string, error) {
	if err := validatePassword(password); err != nil {
		return "", err
	}

	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: gerar sal: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword compara a senha com o hash guardado.
//
// Devolve false sem erro quando a senha está simplesmente errada, e erro apenas
// quando o hash guardado está corrompido — que é um problema diferente.
func VerifyPassword(hash, password string) (bool, error) {
	partes := strings.Split(hash, "$")
	// "", "argon2id", "v=19", "m=..,t=..,p=..", sal, hash
	if len(partes) != 6 || partes[1] != "argon2id" {
		return false, ErrPasswordHash
	}

	var versao int
	if _, err := fmt.Sscanf(partes[2], "v=%d", &versao); err != nil {
		return false, ErrPasswordHash
	}
	if versao != argon2.Version {
		return false, fmt.Errorf("%w: versão %d não suportada", ErrPasswordHash, versao)
	}

	var memoria uint32
	var tempo uint32
	var threads uint8
	if _, err := fmt.Sscanf(partes[3], "m=%d,t=%d,p=%d", &memoria, &tempo, &threads); err != nil {
		return false, ErrPasswordHash
	}

	sal, err := base64.RawStdEncoding.DecodeString(partes[4])
	if err != nil {
		return false, ErrPasswordHash
	}
	esperado, err := base64.RawStdEncoding.DecodeString(partes[5])
	if err != nil {
		return false, ErrPasswordHash
	}
	// O comprimento da chave é fixo na geração. Exigi-lo aqui evita calcular
	// um hash de comprimento arbitrário por causa de um PHC adulterado.
	if len(esperado) != argonKeyLen {
		return false, ErrPasswordHash
	}

	obtido := argon2.IDKey([]byte(password), sal, tempo, memoria, threads, argonKeyLen)

	// Comparação em tempo constante: uma comparação normal revelaria, pelo
	// tempo de resposta, quantos bytes iniciais estão certos.
	return subtle.ConstantTimeCompare(obtido, esperado) == 1, nil
}

func validatePassword(password string) error {
	if len([]rune(password)) < MinPasswordLen {
		return ErrPasswordTooShort
	}
	return nil
}
