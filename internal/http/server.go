// Package http expõe a aplicação ao navegador: rotas, fragmentos HTML e
// autenticação.
//
// O nome do pacote coincide com o da biblioteca padrão; por isso o import de
// net/http é apelidado de stdhttp em todo o pacote, para que não haja dúvida
// sobre qual dos dois está a ser usado.
package http

import (
	"errors"
	"financas/internal/infra/clock"
	"financas/internal/modules/auth"
	"financas/internal/modules/system"
	"fmt"
	"io/fs"
	"log/slog"
	stdhttp "net/http"
	"time"
)

// Deps reúne o que o servidor precisa.
type Deps struct {
	Auth          *auth.Service
	System        *system.Service
	Log           *slog.Logger
	Assets        fs.FS // ficheiros de /static, com app.css na raiz (financas.Web)
	SecureCookies bool
	Timezone      *time.Location
}

// Server serve a aplicação.
type Server struct {
	deps  Deps
	mux   *stdhttp.ServeMux
	files stdhttp.Handler
}

// NewServer valida as dependências e monta as rotas.
func NewServer(deps Deps) (*Server, error) {
	switch {
	case deps.Auth == nil:
		return nil, errors.New("http: serviço de autenticação é obrigatório")
	case deps.System == nil:
		return nil, errors.New("http: serviço de sistema é obrigatório")
	case deps.Log == nil:
		return nil, errors.New("http: registrador é obrigatório")
	case deps.Assets == nil:
		return nil, errors.New("http: ficheiros embutidos são obrigatórios")
	}

	// O sistema de ficheiros chega com a raiz já resolvida. Validar o conteúdo
	// aqui faz a aplicação falhar no arranque, e não no primeiro pedido a
	// /static/app.css.
	if _, err := fs.Stat(deps.Assets, "app.css"); err != nil {
		return nil, fmt.Errorf("http: app.css ausente dos ficheiros embutidos: %w", err)
	}

	if deps.Timezone == nil {
		deps.Timezone = clock.Location(clock.Timezone)
	}

	s := &Server{
		deps:  deps,
		mux:   stdhttp.NewServeMux(),
		files: comCache(stdhttp.FileServerFS(deps.Assets)),
	}

	s.routes()
	return s, nil
}

// Handler devolve o manipulador pronto a servir, com os cabeçalhos de
// segurança aplicados.
func (s *Server) Handler() stdhttp.Handler {
	return cabecalhosDeSeguranca(s.deps.Log, s.mux)
}

// routes declara todas as rotas. É intencionalmente uma lista curta e legível:
// é aqui que se vê o que a aplicação expõe.
func (s *Server) routes() {
	// Público.
	s.mux.Handle("GET /static/", stdhttp.StripPrefix("/static/", s.files))
	s.mux.HandleFunc("GET /saude", s.handleSaude)

	// Primeiro arranque: só existe enquanto não houver senha definida.
	s.mux.HandleFunc("GET /primeiro-arranque", s.handleSetupForm)
	s.mux.HandleFunc("POST /primeiro-arranque", s.handleSetupSubmit)

	// Entrada e saída.
	s.mux.HandleFunc("GET /entrar", s.handleLoginForm)
	s.mux.HandleFunc("POST /entrar", s.handleLoginSubmit)
	s.mux.HandleFunc("POST /sair", s.handleLogout)

	// Área autenticada.
	s.mux.Handle("GET /{$}", s.protegido(s.handleHome))
}

// handleSaude responde ao teste de vida do contentor. Não exige sessão: quem
// verifica já está dentro da rede do serviço.
func (s *Server) handleSaude(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(stdhttp.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// cabecalhosDeSeguranca aplica os cabeçalhos comuns a todas as respostas.
//
// A política de conteúdo não permite scripts nem estilos inline: o Alpine.js é
// a build compatível com CSP e o HTMX corre com allowEval desligado (memorando
// em docs/06). Sem isto, a aplicação funcionaria por acidente e falharia ao
// endurecer a política.
func cabecalhosDeSeguranca(log *slog.Logger, next stdhttp.Handler) stdhttp.Handler {
	const csp = "default-src 'self'; script-src 'self'; style-src 'self'; " +
		"img-src 'self' data:; connect-src 'self'; form-action 'self'; " +
		"frame-ancestors 'none'; base-uri 'none'; object-src 'none'"

	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "DENY")

		defer func() {
			// Um panico num manipulador derruba o processo se não for contido:
			// com um servidor de duas pessoas, isso é uma indisponibilidade.
			if recuperado := recover(); recuperado != nil {
				log.Error("panico ao tratar pedido",
					"caminho", r.URL.Path, "erro", recuperado)
				stdhttp.Error(w, "erro interno", stdhttp.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// comCache evita revalidar os ficheiros estáticos em cada navegação. O prazo é
// curto para que uma atualização apareça sem obrigar a limpar a cache.
func comCache(next stdhttp.Handler) stdhttp.Handler {
	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		w.Header().Set("Cache-Control", "public, max-age=300")
		next.ServeHTTP(w, r)
	})
}
