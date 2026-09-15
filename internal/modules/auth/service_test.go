package auth

import (
	"errors"
	"financas/internal/data"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const senhaBoa = "senha de teste suficientemente longa"

// novaAuth monta o serviço sobre uma base temporária, com o tempo controlado.
func novaAuth(t *testing.T) (*Service, *time.Time) {
	t.Helper()

	caminho := filepath.Join(t.TempDir(), "auth.db")
	db, err := data.Open(t.Context(), data.Config{
		Path:       caminho,
		Migrations: os.DirFS(filepath.Join("..", "..", "..", "migrations")),
	})
	if err != nil {
		t.Fatalf("abrir base de teste: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	agora := time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC)

	s := New(db, data.NewMutator(db), time.Hour)
	s.agora = func() time.Time { return agora }
	s.lim = newLimiter(func() time.Time { return agora })

	return s, &agora
}

func TestNeedsSetupInicial(t *testing.T) {
	s, _ := novaAuth(t)

	precisa, err := s.NeedsSetup(t.Context())
	if err != nil {
		t.Fatalf("NeedsSetup: %v", err)
	}
	if !precisa {
		t.Error("uma base nova precisa de definição de senha")
	}
}

func TestSetupEDepoisNaoPrecisa(t *testing.T) {
	s, _ := novaAuth(t)
	ctx := t.Context()

	if err := s.Setup(ctx, senhaBoa); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	precisa, err := s.NeedsSetup(ctx)
	if err != nil {
		t.Fatalf("NeedsSetup: %v", err)
	}
	if precisa {
		t.Error("depois de definida a senha, o primeiro arranque acabou")
	}
}

func TestSetupRecusaSegundaVez(t *testing.T) {
	// Sem esta recusa, um pedido repetido redefiniria a senha de um sistema em
	// uso — e quem o enviasse passaria a ter acesso.
	s, _ := novaAuth(t)
	ctx := t.Context()

	if err := s.Setup(ctx, senhaBoa); err != nil {
		t.Fatalf("primeiro Setup: %v", err)
	}
	if err := s.Setup(ctx, "outra senha longa qualquer"); !errors.Is(err, ErrSetupDone) {
		t.Errorf("segundo Setup devia devolver ErrSetupDone, devolveu %v", err)
	}
}

func TestSetupSenhaCurta(t *testing.T) {
	s, _ := novaAuth(t)
	if err := s.Setup(t.Context(), "curta"); !errors.Is(err, ErrPasswordTooShort) {
		t.Errorf("senha curta devia ser recusada, devolveu %v", err)
	}
}

func TestLoginEAutenticacao(t *testing.T) {
	s, _ := novaAuth(t)
	ctx := t.Context()

	if err := s.Setup(ctx, senhaBoa); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	token, err := s.Login(ctx, LoginRequest{
		Password:  senhaBoa,
		ActorName: "Giovani",
		UserAgent: "teste/1.0",
		IP:        "192.0.2.10",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if token == "" {
		t.Fatal("Login devia devolver um token")
	}

	sessao, err := s.Authenticate(ctx, token)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if sessao.ActorName != "Giovani" {
		t.Errorf("ator = %q, queria %q", sessao.ActorName, "Giovani")
	}
}

func TestLoginSemSetup(t *testing.T) {
	s, _ := novaAuth(t)

	_, err := s.Login(t.Context(), LoginRequest{Password: senhaBoa, IP: "192.0.2.11"})
	if !errors.Is(err, ErrSetupIncompleto) {
		t.Errorf("login sem senha definida devia devolver ErrSetupIncompleto, devolveu %v", err)
	}
}

func TestLoginSenhaErrada(t *testing.T) {
	s, _ := novaAuth(t)
	ctx := t.Context()

	if err := s.Setup(ctx, senhaBoa); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	_, err := s.Login(ctx, LoginRequest{Password: "senha errada longa", IP: "192.0.2.12"})
	if !errors.Is(err, ErrCredentials) {
		t.Errorf("senha errada devia devolver ErrCredentials, devolveu %v", err)
	}
}

func TestLoginBloqueiaAposTentativas(t *testing.T) {
	s, _ := novaAuth(t)
	ctx := t.Context()

	if err := s.Setup(ctx, senhaBoa); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	const ip = "203.0.113.20"
	for range maxFalhas {
		_, _ = s.Login(ctx, LoginRequest{Password: "senha errada longa", IP: ip})
	}

	// A partir daqui, nem a senha certa passa: o bloqueio é por origem.
	_, err := s.Login(ctx, LoginRequest{Password: senhaBoa, IP: ip})
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("após o limite devia devolver ErrRateLimited, devolveu %v", err)
	}
}

func TestAuthenticateTokenInvalido(t *testing.T) {
	s, _ := novaAuth(t)
	ctx := t.Context()

	if err := s.Setup(ctx, senhaBoa); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	for nome, token := range map[string]string{
		"vazio":     "",
		"lixo":      "isto-nao-e-um-token",
		"aleatório": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	} {
		t.Run(nome, func(t *testing.T) {
			if _, err := s.Authenticate(ctx, token); !errors.Is(err, ErrSessionInvalida) {
				t.Errorf("token inválido devia devolver ErrSessionInvalida, devolveu %v", err)
			}
		})
	}
}

func TestSessaoExpira(t *testing.T) {
	s, agora := novaAuth(t)
	ctx := t.Context()

	if err := s.Setup(ctx, senhaBoa); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	token, err := s.Login(ctx, LoginRequest{Password: senhaBoa, IP: "192.0.2.13"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	*agora = agora.Add(2 * time.Hour)

	if _, err := s.Authenticate(ctx, token); !errors.Is(err, ErrSessionInvalida) {
		t.Errorf("sessão vencida devia ser inválida, devolveu %v", err)
	}
}

func TestLogoutInvalidaToken(t *testing.T) {
	s, _ := novaAuth(t)
	ctx := t.Context()

	if err := s.Setup(ctx, senhaBoa); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	token, err := s.Login(ctx, LoginRequest{Password: senhaBoa, IP: "192.0.2.14"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if err := s.Logout(ctx, token); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := s.Authenticate(ctx, token); !errors.Is(err, ErrSessionInvalida) {
		t.Errorf("depois do logout o token devia ser inválido, devolveu %v", err)
	}

	// Sair duas vezes não é erro: o resultado desejado já está alcançado.
	if err := s.Logout(ctx, token); err != nil {
		t.Errorf("segundo logout devia ser inócuo, devolveu %v", err)
	}
}

func TestSetPasswordExpulsaSessoes(t *testing.T) {
	s, _ := novaAuth(t)
	ctx := t.Context()

	if err := s.Setup(ctx, senhaBoa); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	token, err := s.Login(ctx, LoginRequest{Password: senhaBoa, IP: "192.0.2.15"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	const novaSenha = "outra senha igualmente longa"
	if err := s.SetPassword(ctx, novaSenha); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	// A sessão antiga deixa de valer, e a senha antiga deixa de entrar.
	if _, err := s.Authenticate(ctx, token); !errors.Is(err, ErrSessionInvalida) {
		t.Error("trocar a senha tem de expulsar as sessões existentes")
	}
	if _, err := s.Login(ctx, LoginRequest{Password: senhaBoa, IP: "192.0.2.15"}); !errors.Is(err, ErrCredentials) {
		t.Errorf("a senha antiga devia deixar de entrar, devolveu %v", err)
	}
	if _, err := s.Login(ctx, LoginRequest{Password: novaSenha, IP: "192.0.2.15"}); err != nil {
		t.Errorf("a senha nova devia entrar: %v", err)
	}
}

func TestSetPasswordSemSetup(t *testing.T) {
	s, _ := novaAuth(t)

	if err := s.SetPassword(t.Context(), senhaBoa); !errors.Is(err, ErrSetupIncompleto) {
		t.Errorf("trocar senha sem a ter definido devia devolver ErrSetupIncompleto, devolveu %v", err)
	}
}

func TestLoginLimpaSessoesVencidas(t *testing.T) {
	s, agora := novaAuth(t)
	ctx := t.Context()

	if err := s.Setup(ctx, senhaBoa); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	if _, err := s.Login(ctx, LoginRequest{Password: senhaBoa, IP: "192.0.2.16"}); err != nil {
		t.Fatalf("primeiro login: %v", err)
	}

	*agora = agora.Add(2 * time.Hour)

	if _, err := s.Login(ctx, LoginRequest{Password: senhaBoa, IP: "192.0.2.16"}); err != nil {
		t.Fatalf("segundo login: %v", err)
	}

	var total int
	if err := s.db.SQL().QueryRowContext(ctx, `SELECT count(*) FROM sessions`).Scan(&total); err != nil {
		t.Fatalf("contar sessões: %v", err)
	}
	// A sessão vencida é removida no login seguinte, sem precisar de um job.
	if total != 1 {
		t.Errorf("queria 1 sessão ativa, obtive %d", total)
	}
}

func TestTokenEmClaroNaoEGuardado(t *testing.T) {
	s, _ := novaAuth(t)
	ctx := t.Context()

	if err := s.Setup(ctx, senhaBoa); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	token, err := s.Login(ctx, LoginRequest{Password: senhaBoa, IP: "192.0.2.17"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	var guardado string
	if err := s.db.SQL().QueryRowContext(ctx, `SELECT token_hash FROM sessions LIMIT 1`).Scan(&guardado); err != nil {
		t.Fatalf("ler token guardado: %v", err)
	}
	if guardado == token {
		t.Error("o token em claro não pode ser guardado na base")
	}
	if guardado != hashToken(token) {
		t.Error("o que é guardado tem de ser o hash do token")
	}
}
