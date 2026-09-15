package domain_test

import (
	"financas/internal/domain"
	"testing"
	"time"
)

func TestNewIDFormato(t *testing.T) {
	id := domain.NewID()
	if len(id) != domain.LenID {
		t.Fatalf("identificador %q tem %d caracteres, queria %d", id, len(id), domain.LenID)
	}
	if !domain.IsValidID(id) {
		t.Errorf("identificador gerado %q não é válido", id)
	}
}

func TestNewIDOrdenacaoPorTempo(t *testing.T) {
	// A propriedade que justifica ULID em vez de UUID: ordenar textualmente
	// ordena por instante de criação.
	antes := domain.NewIDAt(time.Date(2026, time.September, 15, 10, 0, 0, 0, time.UTC))
	depois := domain.NewIDAt(time.Date(2026, time.September, 15, 10, 0, 1, 0, time.UTC))

	if antes >= depois {
		t.Errorf("%q devia ordenar antes de %q", antes, depois)
	}
}

func TestNewIDInstanteRecuperavel(t *testing.T) {
	instante := time.Date(2026, time.September, 15, 12, 34, 56, 789_000_000, time.UTC)
	id := domain.NewIDAt(instante)

	if obtido := domain.TimeOfID(id); !obtido.Equal(instante.Truncate(time.Millisecond)) {
		t.Errorf("TimeOfID devolveu %v, queria %v", obtido, instante.Truncate(time.Millisecond))
	}
}

func TestNewIDUnico(t *testing.T) {
	// O instante é o mesmo e a entropia é de 80 bits: colisões não devem
	// acontecer em milhares de identificadores gerados no mesmo milissegundo.
	instante := time.Now()
	vistos := make(map[string]struct{}, 10_000)

	for range 10_000 {
		id := domain.NewIDAt(instante)
		if _, existe := vistos[id]; existe {
			t.Fatalf("identificador repetido: %q", id)
		}
		vistos[id] = struct{}{}
	}
}

func TestIsValidID(t *testing.T) {
	if domain.IsValidID("") {
		t.Error("string vazia não é identificador")
	}
	if domain.IsValidID(domain.NewID() + "Z") {
		t.Error("identificador com 27 caracteres não é válido")
	}
	if domain.IsValidID(domain.NewID()[:25]) {
		t.Error("identificador com 25 caracteres não é válido")
	}
	// 'I', 'L', 'O' e 'U' estão fora do alfabeto Crockford.
	invalido := domain.NewID()[:25] + "I"
	if domain.IsValidID(invalido) {
		t.Errorf("identificador %q com 'I' não é válido", invalido)
	}
	if !domain.IsValidID(domain.NewID()) {
		t.Error("identificador gerado devia ser válido")
	}
}

func TestTimeOfIDInvalido(t *testing.T) {
	if obtido := domain.TimeOfID("invalido"); !obtido.IsZero() {
		t.Errorf("identificador inválido devia devolver instante zero, devolveu %v", obtido)
	}
}
