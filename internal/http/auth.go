package http

import (
	"context"
	"errors"
	"financas/internal/data"
	"financas/internal/modules/auth"
	"financas/internal/views/home"
	stdhttp "net/http"
	"net/url"
	"strings"
	"time"

	"github.com/a-h/templ"

	viewsauth "financas/internal/views/auth"
)

// chaveContexto identifica valores guardados no contexto do pedido.
type chaveContexto string

const sessaoNoContexto chaveContexto = "sessao"

// sessaoDoPedido devolve a sessão associada ao cookie do pedido.
func (s *Server) sessaoDoPedido(r *stdhttp.Request) (data.Session, error) {
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil {
		return data.Session{}, auth.ErrSessionInvalida
	}
	return s.deps.Auth.Authenticate(r.Context(), cookie.Value)
}

// sessaoDoContexto recupera a sessão colocada pelo middleware.
func sessaoDoContexto(ctx context.Context) (data.Session, bool) {
	sessao, ok := ctx.Value(sessaoNoContexto).(data.Session)
	return sessao, ok
}

// protegido exige sessão válida e coloca-a no contexto.
func (s *Server) protegido(next stdhttp.HandlerFunc) stdhttp.Handler {
	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		sessao, err := s.sessaoDoPedido(r)
		if err != nil {
			s.irParaEntrar(w, r)
			return
		}
		ctx := context.WithValue(r.Context(), sessaoNoContexto, sessao)
		next(w, r.WithContext(ctx))
	})
}

// irParaEntrar redireciona para o ecrã de entrada, preservando o destino.
func (s *Server) irParaEntrar(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	destino := url.QueryEscape(r.URL.RequestURI())
	stdhttp.Redirect(w, r, "/entrar?destino="+destino, stdhttp.StatusSeeOther)
}

func (s *Server) handleHome(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	sessao, ok := sessaoDoContexto(r.Context())
	if !ok {
		s.irParaEntrar(w, r)
		return
	}

	estado, err := s.deps.System.Status(r.Context())
	if err != nil {
		s.deps.Log.Error("ler estado do sistema", "erro", err)
		stdhttp.Error(w, "erro interno", stdhttp.StatusInternalServerError)
		return
	}

	s.render(w, r, stdhttp.StatusOK, home.Page(estado, sessao.ActorName))
}

func (s *Server) handleLoginForm(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	precisa, err := s.deps.Auth.NeedsSetup(r.Context())
	if err != nil {
		s.erroInterno(w, r, "verificar primeiro arranque", err)
		return
	}
	if precisa {
		stdhttp.Redirect(w, r, "/primeiro-arranque", stdhttp.StatusSeeOther)
		return
	}

	// Sessão já válida: não há razão para mostrar o formulário outra vez.
	if _, err := s.sessaoDoPedido(r); err == nil {
		//nolint:gosec // G710: destinoSeguro só devolve caminhos internos.
		stdhttp.Redirect(w, r, destinoSeguro(r.URL.Query().Get("destino")), stdhttp.StatusSeeOther)
		return
	}

	nomes, err := s.deps.System.DisplayNames(r.Context())
	if err != nil {
		s.erroInterno(w, r, "ler nomes de apresentação", err)
		return
	}

	s.render(w, r, stdhttp.StatusOK, viewsauth.Login("", nomes, destinoSeguro(r.URL.Query().Get("destino"))))
}

func (s *Server) handleLoginSubmit(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderLoginErro(w, r, "Não foi possível ler o formulário.", stdhttp.StatusBadRequest)
		return
	}

	destino := destinoSeguro(r.PostFormValue("destino"))

	token, err := s.deps.Auth.Login(r.Context(), auth.LoginRequest{
		Password:  r.PostFormValue("senha"),
		ActorName: strings.TrimSpace(r.PostFormValue("ator")),
		UserAgent: r.UserAgent(),
		IP:        clientIP(r),
	})
	if err != nil {
		s.renderLoginErroDeAuth(w, r, destino, err)
		return
	}

	s.abrirSessao(w, token)
	stdhttp.Redirect(w, r, destino, stdhttp.StatusSeeOther)
}

func (s *Server) handleLogout(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if cookie, err := r.Cookie(auth.SessionCookieName); err == nil {
		if err := s.deps.Auth.Logout(r.Context(), cookie.Value); err != nil {
			s.deps.Log.Error("encerrar sessão", "erro", err)
		}
	}

	s.fecharSessao(w)
	stdhttp.Redirect(w, r, "/entrar", stdhttp.StatusSeeOther)
}

func (s *Server) handleSetupForm(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	precisa, err := s.deps.Auth.NeedsSetup(r.Context())
	if err != nil {
		s.erroInterno(w, r, "verificar primeiro arranque", err)
		return
	}
	if !precisa {
		// Depois de definida a senha, este ecrã deixa de existir: não há
		// recuperação pela interface (ADR-019).
		stdhttp.NotFound(w, r)
		return
	}

	s.render(w, r, stdhttp.StatusOK, viewsauth.Setup("", auth.MinPasswordLen))
}

func (s *Server) handleSetupSubmit(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	precisa, err := s.deps.Auth.NeedsSetup(r.Context())
	if err != nil {
		s.erroInterno(w, r, "verificar primeiro arranque", err)
		return
	}
	if !precisa {
		stdhttp.Redirect(w, r, "/entrar", stdhttp.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		s.renderSetupErro(w, r, "Não foi possível ler o formulário.", stdhttp.StatusBadRequest)
		return
	}

	senha := r.PostFormValue("senha")
	if senha != r.PostFormValue("confirmacao") {
		s.renderSetupErro(w, r, "As senhas não coincidem.", stdhttp.StatusBadRequest)
		return
	}

	if err := s.deps.Auth.Setup(r.Context(), senha); err != nil {
		switch {
		case errors.Is(err, auth.ErrPasswordTooShort):
			s.renderSetupErro(w, r, err.Error(), stdhttp.StatusBadRequest)
		case errors.Is(err, auth.ErrSetupDone):
			stdhttp.Redirect(w, r, "/entrar", stdhttp.StatusSeeOther)
		default:
			s.erroInterno(w, r, "definir senha", err)
		}
		return
	}

	// Entra logo a seguir, para que o primeiro arranque não termine num ecrã
	// de entrada a pedir a senha que acabou de ser definida.
	token, err := s.deps.Auth.Login(r.Context(), auth.LoginRequest{
		Password:  senha,
		UserAgent: r.UserAgent(),
		IP:        clientIP(r),
	})
	if err != nil {
		stdhttp.Redirect(w, r, "/entrar", stdhttp.StatusSeeOther)
		return
	}

	s.abrirSessao(w, token)
	stdhttp.Redirect(w, r, "/", stdhttp.StatusSeeOther)
}

// renderLoginErro volta a mostrar o formulário com uma mensagem.
func (s *Server) renderLoginErro(w stdhttp.ResponseWriter, r *stdhttp.Request, mensagem string, status int) {
	nomes, err := s.deps.System.DisplayNames(r.Context())
	if err != nil {
		nomes = nil
	}
	s.render(w, r, status, viewsauth.Login(mensagem, nomes, destinoSeguro(r.PostFormValue("destino"))))
}

// renderLoginErroDeAuth traduz o erro do serviço numa mensagem e num estado.
func (s *Server) renderLoginErroDeAuth(w stdhttp.ResponseWriter, r *stdhttp.Request, destino string, err error) {
	switch {
	case errors.Is(err, auth.ErrCredentials):
		s.render(w, r, stdhttp.StatusUnauthorized, viewsauth.Login("Senha incorreta.", nil, destino))
	case errors.Is(err, auth.ErrRateLimited):
		s.deps.Log.Warn("login bloqueado por tentativas", "ip", clientIP(r))
		s.render(w, r, stdhttp.StatusTooManyRequests, viewsauth.Login(err.Error(), nil, destino))
	case errors.Is(err, auth.ErrSetupIncompleto):
		stdhttp.Redirect(w, r, "/primeiro-arranque", stdhttp.StatusSeeOther)
	default:
		s.erroInterno(w, r, "entrar", err)
	}
}

func (s *Server) renderSetupErro(w stdhttp.ResponseWriter, r *stdhttp.Request, mensagem string, status int) {
	s.render(w, r, status, viewsauth.Setup(mensagem, auth.MinPasswordLen))
}

func (s *Server) render(w stdhttp.ResponseWriter, r *stdhttp.Request, status int, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := c.Render(r.Context(), w); err != nil {
		// A resposta já começou: resta registar. Renderizar para um buffer
		// evitaria isto, ao custo de duplicar cada resposta em memória.
		s.deps.Log.Error("renderizar página", "caminho", r.URL.Path, "erro", err)
	}
}

func (s *Server) erroInterno(w stdhttp.ResponseWriter, r *stdhttp.Request, contexto string, err error) {
	s.deps.Log.Error(contexto, "caminho", r.URL.Path, "erro", err)
	stdhttp.Error(w, "erro interno", stdhttp.StatusInternalServerError)
}

// novoCookieSessao monta o cookie de sessão com os atributos de segurança.
//
// O prazo do cookie é o mesmo da sessão na base: o servidor é quem decide a
// validade, e um cookie manipulado não estende nada.
//
// O atributo Secure é configurável porque o uso normal é HTTP numa rede
// privada (LAN ou rede sobreposta); passa a obrigatório em qualquer exposição
// real (docs/06). HttpOnly e SameSite estão sempre definidos.
func (s *Server) novoCookieSessao(valor string, expira time.Time, maxAge int) *stdhttp.Cookie {
	//nolint:gosec // G124: Secure é configurável (rede privada); HttpOnly e SameSite estão definidos.
	return &stdhttp.Cookie{
		Name:     auth.SessionCookieName,
		Value:    valor,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.deps.SecureCookies,
		SameSite: stdhttp.SameSiteLaxMode,
		Expires:  expira,
		MaxAge:   maxAge,
	}
}

// abrirSessao grava o cookie de sessão.
func (s *Server) abrirSessao(w stdhttp.ResponseWriter, token string) {
	stdhttp.SetCookie(w, s.novoCookieSessao(token, time.Now().Add(s.deps.Auth.TTL()), 0))
}

// fecharSessao apaga o cookie de sessão.
func (s *Server) fecharSessao(w stdhttp.ResponseWriter) {
	stdhttp.SetCookie(w, s.novoCookieSessao("", time.Unix(0, 0), -1))
}

// destinoSeguro aceita apenas caminhos internos.
//
// Sem isto, um endereço como /entrar?destino=https://exemplo.com faria da
// aplicação um trampolim para um site alheio depois do login.
func destinoSeguro(destino string) string {
	if destino == "" || !strings.HasPrefix(destino, "/") {
		return "/"
	}
	// "//host" é um endereço absoluto disfarçado de caminho.
	if strings.HasPrefix(destino, "//") || strings.Contains(destino, "\\") {
		return "/"
	}
	if _, err := url.Parse(destino); err != nil {
		return "/"
	}
	return destino
}

// clientIP devolve o endereço de origem, usado para limitar tentativas.
func clientIP(r *stdhttp.Request) string {
	if encaminhado := r.Header.Get("X-Forwarded-For"); encaminhado != "" {
		if primeiro, _, encontrou := strings.Cut(encaminhado, ","); encontrou {
			return strings.TrimSpace(primeiro)
		}
		return strings.TrimSpace(encaminhado)
	}
	return r.RemoteAddr
}
