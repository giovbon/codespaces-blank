package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestHashVerifyRoundTrip(t *testing.T) {
	const senha = "uma senha longa o suficiente"

	hash, err := HashPassword(senha)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	confere, err := VerifyPassword(hash, senha)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !confere {
		t.Error("a senha correta devia conferir")
	}
}

func TestHashFormato(t *testing.T) {
	hash, err := HashPassword("senha de teste longa")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	// O formato guarda os parâmetros junto ao hash: é o que permite mudá-los
	// mais tarde sem invalidar as senhas existentes.
	for _, parte := range []string{"$argon2id$", "$v=19$", "$m=65536,t=3,p=4$"} {
		if !strings.Contains(hash, parte) {
			t.Errorf("hash %q não contém %q", hash, parte)
		}
	}
	if strings.Contains(hash, "senha de teste") {
		t.Error("o hash não pode conter a senha em claro")
	}
}

func TestHashSalDiferente(t *testing.T) {
	// Duas senhas iguais têm de produzir hashes diferentes, senão um ataque de
	// dicionário pré-computado reaproveitaria resultados.
	const senha = "mesma senha para os dois"

	primeiro, err := HashPassword(senha)
	if err != nil {
		t.Fatalf("primeiro hash: %v", err)
	}
	segundo, err := HashPassword(senha)
	if err != nil {
		t.Fatalf("segundo hash: %v", err)
	}
	if primeiro == segundo {
		t.Error("hashes de senhas iguais não podem ser iguais")
	}
}

func TestVerifySenhaErrada(t *testing.T) {
	hash, err := HashPassword("senha correta e longa")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	confere, err := VerifyPassword(hash, "senha errada e longa")
	if err != nil {
		t.Fatalf("VerifyPassword não devia dar erro: %v", err)
	}
	if confere {
		t.Error("a senha errada não pode conferir")
	}
}

func TestHashSenhaCurta(t *testing.T) {
	if _, err := HashPassword("curta"); !errors.Is(err, ErrPasswordTooShort) {
		t.Errorf("senha curta devia devolver ErrPasswordTooShort, devolveu %v", err)
	}
	if _, err := HashPassword(""); !errors.Is(err, ErrPasswordTooShort) {
		t.Errorf("senha vazia devia devolver ErrPasswordTooShort, devolveu %v", err)
	}
}

func TestVerifyHashCorrompido(t *testing.T) {
	casos := map[string]string{
		"vazio":                "",
		"sem prefixo":          "argon2id$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA",
		"algoritmo errado":     "$bcrypt$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA",
		"versao nao numerica":  "$argon2id$v=xx$m=65536,t=3,p=4$c2FsdA$aGFzaA",
		"parametros invalidos": "$argon2id$v=19$m=xx,t=3,p=4$c2FsdA$aGFzaA",
		"sal invalido":         "$argon2id$v=19$m=65536,t=3,p=4$!!!$aGFzaA",
		"hash invalido":        "$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$!!!",
		"partes a mais":        "$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA$extra",
	}

	for nome, hash := range casos {
		t.Run(nome, func(t *testing.T) {
			if _, err := VerifyPassword(hash, "qualquer senha longa"); !errors.Is(err, ErrPasswordHash) {
				t.Errorf("hash inválido devia devolver ErrPasswordHash, devolveu %v", err)
			}
		})
	}
}

func TestVerifyVersaoNaoSuportada(t *testing.T) {
	// v=18 é argon2i/argon2d em codificações antigas: recusar explicitamente é
	// melhor do que calcular um resultado que nunca vai conferir.
	hash := "$argon2id$v=18$m=65536,t=3,p=4$c2FsdA$aGFzaA"

	_, err := VerifyPassword(hash, "senha longa qualquer")
	if !errors.Is(err, ErrPasswordHash) {
		t.Errorf("versão não suportada devia devolver ErrPasswordHash, devolveu %v", err)
	}
}
