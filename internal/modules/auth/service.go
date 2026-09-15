package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"financas/internal/data"
	"financas/internal/domain"
	"fmt"
	"time"
)

// SessionCookieName é o nome do cookie de sessão.
const SessionCookieName = "financas_sessao"

// tokenBytes é a entropia do token de sessão.
const tokenBytes = 32

// Erros do serviço de autenticação.
var (
	ErrSetupDone       = errors.New("a senha já foi definida")
	ErrCredentials     = errors.New("senha incorreta")
	ErrRateLimited     = errors.New("demasiadas tentativas falhadas")
	ErrSessionInvalida = errors.New("sessão inválida ou expirada")
	ErrSetupIncompleto = errors.New("a senha ainda não foi definida")
)

// Service resolve autenticação e sessões.
type Service struct {
	db    *data.DB
	mut   *data.Mutator
	ttl   time.Duration
	lim   *limiter
	agora func() time.Time
}

// New cria o serviço. ttl é a validade de cada sessão.
func New(db *data.DB, mut *data.Mutator, ttl time.Duration) *Service {
	return &Service{
		db:    db,
		mut:   mut,
		ttl:   ttl,
		lim:   newLimiter(time.Now),
		agora: time.Now,
	}
}

// TTL é a validade das sessões criadas por este serviço. Existe para que a
// camada HTTP dê ao cookie o mesmo prazo que a base de dados impõe.
func (s *Service) TTL() time.Duration { return s.ttl }

// NeedsSetup indica se a aplicação está em primeiro arranque.
func (s *Service) NeedsSetup(ctx context.Context) (bool, error) {
	n, err := s.db.UserCount(ctx)
	if err != nil {
		return false, err
	}
	return n == 0, nil
}

// Setup define a senha no primeiro arranque.
//
// Recusa correr se já existir credencial: sem isto, um pedido repetido poderia
// redefinir a senha de um sistema em uso.
func (s *Service) Setup(ctx context.Context, password string) error {
	jaDefinida, err := s.NeedsSetup(ctx)
	if err != nil {
		return err
	}
	if !jaDefinida {
		return ErrSetupDone
	}

	hash, err := HashPassword(password)
	if err != nil {
		return err
	}

	agora := s.agora().UnixMilli()
	u := data.User{
		ID:           domain.NewID(),
		PasswordHash: hash,
		CreatedAt:    agora,
		UpdatedAt:    agora,
	}

	return s.mut.Run(ctx, data.Write{Actor: "system", Origin: "ui"}, func(tx *data.Tx) error {
		if err := tx.InsertUser(ctx, u); err != nil {
			return err
		}
		return tx.Audit("user", u.ID, data.ActionCreate, nil, map[string]any{
			"origin": "primeiro arranque",
		})
	})
}

// SetPassword troca a senha e encerra todas as sessões.
//
// É o caminho da linha de comando previsto no ADR-019: não há recuperação de
// senha pela interface, porque quem perde a senha tem acesso ao host.
func (s *Service) SetPassword(ctx context.Context, password string) error {
	cred, err := s.db.Credential(ctx)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return ErrSetupIncompleto
		}
		return err
	}

	hash, err := HashPassword(password)
	if err != nil {
		return err
	}

	return s.mut.Run(ctx, data.Write{Actor: "system", Origin: "system"}, func(tx *data.Tx) error {
		if err := tx.UpdatePasswordHash(ctx, cred.ID, hash); err != nil {
			return err
		}
		// Trocar a senha tem de expulsar quem estava dentro.
		if err := tx.DeleteSessionsForUser(ctx, cred.ID); err != nil {
			return err
		}
		return tx.Audit("user", cred.ID, data.ActionUpdate, nil, map[string]any{
			"campo": "password_hash",
		})
	})
}

// LoginRequest reúne o que um login traz.
type LoginRequest struct {
	Password  string
	ActorName string // «quem está a usar»; opcional (ADR-019)
	UserAgent string
	IP        string
}

// Login valida a senha e abre uma sessão, devolvendo o token em claro.
//
// O token em claro existe apenas nesta resposta: na base fica só o hash.
func (s *Service) Login(ctx context.Context, req LoginRequest) (string, error) {
	if permitido, espera := s.lim.permitido(req.IP); !permitido {
		return "", fmt.Errorf("%w: espere %s", ErrRateLimited, espera.Round(time.Second))
	}

	cred, err := s.db.Credential(ctx)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return "", ErrSetupIncompleto
		}
		return "", err
	}

	senhaConfere, err := VerifyPassword(cred.PasswordHash, req.Password)
	if err != nil {
		return "", err
	}
	if !senhaConfere {
		s.lim.falhou(req.IP)
		return "", ErrCredentials
	}
	s.lim.sucesso(req.IP)

	token, hash, err := newToken()
	if err != nil {
		return "", err
	}

	agora := s.agora()
	sessao := data.Session{
		ID:        domain.NewID(),
		UserID:    cred.ID,
		TokenHash: hash,
		ActorName: req.ActorName,
		ExpiresAt: agora.Add(s.ttl).UnixMilli(),
		CreatedAt: agora.UnixMilli(),
		UserAgent: req.UserAgent,
		IP:        req.IP,
	}

	err = s.mut.Run(ctx, data.Write{Actor: actorOuSistema(req.ActorName), Origin: "ui"}, func(tx *data.Tx) error {
		// Aproveita a escrita para limpar sessões vencidas, em vez de ter um
		// job só para isso.
		if err := tx.DeleteExpiredSessions(ctx, agora.UnixMilli()); err != nil {
			return err
		}
		return tx.InsertSession(ctx, sessao)
	})
	if err != nil {
		return "", err
	}

	return token, nil
}

// Authenticate devolve a sessão correspondente ao token, se ainda for válida.
func (s *Service) Authenticate(ctx context.Context, token string) (data.Session, error) {
	if token == "" {
		return data.Session{}, ErrSessionInvalida
	}

	sessao, err := s.db.SessionByTokenHash(ctx, hashToken(token), s.agora().UnixMilli())
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return data.Session{}, ErrSessionInvalida
		}
		return data.Session{}, err
	}
	return sessao, nil
}

// Logout encerra a sessão do token indicado.
func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}

	sessao, err := s.db.SessionByTokenHash(ctx, hashToken(token), 0)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// Sair de uma sessão já inexistente é o resultado desejado.
			return nil
		}
		return err
	}

	return s.mut.Run(ctx, data.Write{Actor: actorOuSistema(sessao.ActorName), Origin: "ui"}, func(tx *data.Tx) error {
		return tx.DeleteSession(ctx, sessao.ID)
	})
}

// newToken gera um token de sessão e o seu hash.
func newToken() (token, hash string, err error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("auth: gerar token de sessão: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, hashToken(token), nil
}

// hashToken devolve o SHA-256 do token, em hexadecimal.
//
// SHA-256 e não argon2: o token tem 256 bits de entropia, pelo que não há
// dicionário a resistir — o objetivo é apenas não guardar o segredo em claro.
func hashToken(token string) string {
	soma := sha256.Sum256([]byte(token))
	return hex.EncodeToString(soma[:])
}

func actorOuSistema(nome string) string {
	if nome == "" {
		return "sistema"
	}
	return nome
}
