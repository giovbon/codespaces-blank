package auth

import (
	"testing"
	"time"
)

func TestLimiterLiberaAbaixoDoLimite(t *testing.T) {
	agora := time.Now()
	l := newLimiter(func() time.Time { return agora })

	for i := 1; i < maxFalhas; i++ {
		if bloquear := l.falhou("192.0.2.1"); bloquear != 0 {
			t.Fatalf("falha %d não devia bloquear, bloqueou %s", i, bloquear)
		}
	}

	if permitido, _ := l.permitido("192.0.2.1"); !permitido {
		t.Error("abaixo do limite a chave continua a poder tentar")
	}
}

func TestLimiterBloqueiaNoLimite(t *testing.T) {
	agora := time.Now()
	l := newLimiter(func() time.Time { return agora })

	for i := range maxFalhas {
		_ = i
		l.falhou("192.0.2.2")
	}

	permitido, espera := l.permitido("192.0.2.2")
	if permitido {
		t.Fatal("no limite a chave devia estar bloqueada")
	}
	if espera <= 0 || espera > bloqueioBase {
		t.Errorf("espera = %s, queria algo até %s", espera, bloqueioBase)
	}
}

func TestLimiterCresceEDeixaDeBloquear(t *testing.T) {
	agora := time.Now()
	l := newLimiter(func() time.Time { return agora })

	for range maxFalhas {
		l.falhou("192.0.2.3")
	}
	_, primeiro := l.permitido("192.0.2.3")

	// Mais uma falha: o bloqueio seguinte tem de ser maior.
	l.falhou("192.0.2.3")
	_, segundo := l.permitido("192.0.2.3")

	if segundo <= primeiro {
		t.Errorf("bloqueio devia crescer: %s depois de %s", segundo, primeiro)
	}

	// Ao passar o tempo, a chave volta a poder tentar.
	agora = agora.Add(bloqueioMaximo + time.Second)
	if permitido, _ := l.permitido("192.0.2.3"); !permitido {
		t.Error("passado o bloqueio, a chave devia poder tentar de novo")
	}
}

func TestLimiterTetoDeBloqueio(t *testing.T) {
	agora := time.Now()
	l := newLimiter(func() time.Time { return agora })

	for range maxFalhas + 20 {
		l.falhou("192.0.2.4")
	}

	_, espera := l.permitido("192.0.2.4")
	if espera > bloqueioMaximo {
		t.Errorf("espera = %s, não pode exceder %s", espera, bloqueioMaximo)
	}
}

func TestLimiterSucessoLimpa(t *testing.T) {
	agora := time.Now()
	l := newLimiter(func() time.Time { return agora })

	for range maxFalhas + 1 {
		l.falhou("192.0.2.5")
	}
	l.sucesso("192.0.2.5")

	if permitido, _ := l.permitido("192.0.2.5"); !permitido {
		t.Error("depois de um sucesso, a chave devia ficar limpa")
	}
}

func TestLimiterChavesIndependentes(t *testing.T) {
	agora := time.Now()
	l := newLimiter(func() time.Time { return agora })

	for range maxFalhas {
		l.falhou("192.0.2.6")
	}

	if permitido, _ := l.permitido("198.51.100.7"); !permitido {
		t.Error("o bloqueio de uma chave não pode afetar outra")
	}
}

func TestLimiterLimpaExpirados(t *testing.T) {
	agora := time.Now()
	l := newLimiter(func() time.Time { return agora })

	// Enche o mapa acima do limite de limpeza para forçar a limpeza.
	for i := range limpezaLimite + 10 {
		chave := string(rune('a'+i%26)) + string(rune('a'+i/26))
		l.falhou(chave)
	}

	agora = agora.Add(bloqueioMaximo + time.Second)
	l.limparExpirados()

	if len(l.falhas) != 0 {
		t.Errorf("queria o mapa vazio, restaram %d entradas", len(l.falhas))
	}
}
