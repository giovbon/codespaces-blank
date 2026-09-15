package auth

import (
	"sync"
	"time"
)

// Parâmetros do bloqueio por tentativas falhadas.
const (
	// maxFalhas é o número de erros tolerados antes do primeiro bloqueio.
	maxFalhas = 5

	// bloqueioBase é a duração do bloqueio inicial.
	bloqueioBase = time.Minute

	// bloqueioMaximo limita o crescimento exponencial.
	bloqueioMaximo = 15 * time.Minute

	// limpezaLimite é o número de chaves a partir do qual se tenta limpar.
	limpezaLimite = 1024
)

// limiter conta tentativas falhadas por chave (o IP de origem).
//
// Vive em memória e perde-se no reinício, o que é aceitável: serve para tornar
// a força bruta remota impraticável, não para auditoria. A aplicação tem um
// processo único, e um bloqueio perdido no arranque custa uma janela de
// minutos.
type limiter struct {
	mu      sync.Mutex
	falhas  map[string]*estadoTentativas
	relogio func() time.Time
}

type estadoTentativas struct {
	falhas       int
	bloqueadoAte time.Time
}

func newLimiter(relogio func() time.Time) *limiter {
	return &limiter{falhas: make(map[string]*estadoTentativas), relogio: relogio}
}

// permitido indica se a chave pode tentar entrar agora.
func (l *limiter) permitido(chave string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	estado, existe := l.falhas[chave]
	if !existe {
		return true, 0
	}

	restante := estado.bloqueadoAte.Sub(l.relogio())
	if restante > 0 {
		return false, restante
	}
	return true, 0
}

// falhou registra uma tentativa falhada e devolve o bloqueio resultante.
func (l *limiter) falhou(chave string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.falhas) > limpezaLimite {
		l.limparExpirados()
	}

	estado, existe := l.falhas[chave]
	if !existe {
		estado = &estadoTentativas{}
		l.falhas[chave] = estado
	}

	estado.falhas++

	if estado.falhas < maxFalhas {
		return 0
	}

	// Cresce a dobrar por cada falha acima do limite, até ao teto.
	excesso := min(estado.falhas-maxFalhas, 5)
	bloqueio := min(bloqueioBase<<uint(excesso), bloqueioMaximo)

	estado.bloqueadoAte = l.relogio().Add(bloqueio)
	return bloqueio
}

// sucesso limpa o histórico da chave.
func (l *limiter) sucesso(chave string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.falhas, chave)
}

// limparExpirados liberta as entradas já sem bloqueio em curso.
func (l *limiter) limparExpirados() {
	agora := l.relogio()
	for chave, estado := range l.falhas {
		if estado.bloqueadoAte.Before(agora) {
			delete(l.falhas, chave)
		}
	}
}
