package http_test

import (
	"financas"
	"financas/internal/data"
	"financas/internal/infra/log"
	"financas/internal/modules/auth"
	"financas/internal/modules/system"
	"io"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	stdhttp "net/http"

	webhttp "financas/internal/http"
)

const senhaBoa = "senha de teste suficientemente longa"

// ambiente é um servidor de teste com a sua própria base.
type ambiente struct {
	t      *testing.T
	serv   *httptest.Server
	client *stdhttp.Client
	db     *data.DB
	auth   *auth.Service
}

func novoAmbiente(t *testing.T) *ambiente {
	t.Helper()

	caminho := filepath.Join(t.TempDir(), "http.db")
	db, err := data.Open(t.Context(), data.Config{
		Path:       caminho,
		Migrations: os.DirFS(filepath.Join("..", "..", "migrations")),
	})
	if err != nil {
		t.Fatalf("abrir base de teste: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mut := data.NewMutator(db)
	authSvc := auth.New(db, mut, time.Hour)

	servidor, err := webhttp.NewServer(webhttp.Deps{
		Auth:     authSvc,
		System:   system.New(db, mut),
		Log:      log.New("error", os.Stderr),
		Assets:   financas.Web,
		Timezone: time.UTC,
	})
	if err != nil {
		t.Fatalf("criar servidor: %v", err)
	}

	ts := httptest.NewServer(servidor.Handler())
	t.Cleanup(ts.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("criar jar de cookies: %v", err)
	}

	client := &stdhttp.Client{
		Jar: jar,
		// Sem seguir redirecionamentos: o destino é o que está a ser testado.
		CheckRedirect: func(*stdhttp.Request, []*stdhttp.Request) error {
			return stdhttp.ErrUseLastResponse
		},
	}

	return &ambiente{t: t, serv: ts, client: client, db: db, auth: authSvc}
}

// resposta é o que os testes consomem de uma resposta HTTP, com o corpo já
// lido.
//
// Não é *stdhttp.Response de propósito: devolver o corpo aberto obrigaria cada
// teste a lembrar-se de o fechar, e um esquecimento passaria despercebido até
// esgotar as ligações.
type resposta struct {
	StatusCode int
	Header     stdhttp.Header
	Corpo      string
}

func (a *ambiente) pedido(metodo, caminho string, valores url.Values) resposta {
	a.t.Helper()

	var corpo *strings.Reader
	if valores == nil {
		corpo = strings.NewReader("")
	} else {
		corpo = strings.NewReader(valores.Encode())
	}

	req, err := stdhttp.NewRequestWithContext(a.t.Context(), metodo, a.serv.URL+caminho, corpo)
	if err != nil {
		a.t.Fatalf("criar pedido: %v", err)
	}
	if valores != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := a.client.Do(req)
	if err != nil {
		a.t.Fatalf("executar pedido %s %s: %v", metodo, caminho, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			a.t.Errorf("fechar corpo de %s %s: %v", metodo, caminho, err)
		}
	}()

	lido, err := io.ReadAll(resp.Body)
	if err != nil {
		a.t.Fatalf("ler corpo de %s %s: %v", metodo, caminho, err)
	}

	return resposta{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Corpo:      string(lido),
	}
}

func (a *ambiente) definirSenha() {
	a.t.Helper()

	resp := a.pedido(stdhttp.MethodPost, "/primeiro-arranque", url.Values{
		"senha":       {senhaBoa},
		"confirmacao": {senhaBoa},
	})
	if resp.StatusCode != stdhttp.StatusSeeOther {
		a.t.Fatalf("definir senha devolveu %d, queria %d", resp.StatusCode, stdhttp.StatusSeeOther)
	}
}

// TestPrimeiroArranqueObrigatorio garante que, sem senha, tudo encaminha para a
// definição inicial em vez de expor a aplicação.
func TestPrimeiroArranqueObrigatorio(t *testing.T) {
	a := novoAmbiente(t)

	resp := a.pedido(stdhttp.MethodGet, "/entrar", nil)
	if resp.StatusCode != stdhttp.StatusSeeOther {
		t.Fatalf("estado = %d, queria %d", resp.StatusCode, stdhttp.StatusSeeOther)
	}
	if destino := resp.Header.Get("Location"); destino != "/primeiro-arranque" {
		t.Errorf("destino = %q, queria %q", destino, "/primeiro-arranque")
	}
}

func TestAreaProtegidaRedireciona(t *testing.T) {
	a := novoAmbiente(t)
	a.definirSenha()

	// Depois de definida a senha, a definição inicial já não existe.
	if resp := a.pedido(stdhttp.MethodGet, "/primeiro-arranque", nil); resp.StatusCode != stdhttp.StatusNotFound {
		t.Errorf("primeiro arranque devia dar 404 depois de concluído, deu %d", resp.StatusCode)
	}

	// Sessão nova, sem cookie: a raiz encaminha para a entrada.
	a.client.Jar, _ = cookiejar.New(nil)

	resp := a.pedido(stdhttp.MethodGet, "/", nil)
	if resp.StatusCode != stdhttp.StatusSeeOther {
		t.Fatalf("estado = %d, queria %d", resp.StatusCode, stdhttp.StatusSeeOther)
	}

	localizacao := resp.Header.Get("Location")
	if !strings.HasPrefix(localizacao, "/entrar?destino=") {
		t.Errorf("destino = %q, queria começar por %q", localizacao, "/entrar?destino=")
	}
	if !strings.Contains(localizacao, "%2F") {
		t.Errorf("destino = %q, devia preservar a página pedida", localizacao)
	}
}

func TestPrimeiroArranqueEntraLogo(t *testing.T) {
	a := novoAmbiente(t)
	a.definirSenha()

	// Definir a senha já abre a sessão: obrigar a escrevê-la de novo no ecrã
	// seguinte seria fricção sem ganho.
	resp := a.pedido(stdhttp.MethodGet, "/", nil)
	if resp.StatusCode != stdhttp.StatusOK {
		t.Fatalf("estado = %d, queria %d", resp.StatusCode, stdhttp.StatusOK)
	}
}

func TestSenhaCurtaOuDiferente(t *testing.T) {
	a := novoAmbiente(t)

	casos := map[string]url.Values{
		"curta": {
			"senha":       {"curta"},
			"confirmacao": {"curta"},
		},
		"diferente": {
			"senha":       {senhaBoa},
			"confirmacao": {"outra senha igualmente longa"},
		},
	}

	for nome, valores := range casos {
		t.Run(nome, func(t *testing.T) {
			resp := a.pedido(stdhttp.MethodPost, "/primeiro-arranque", valores)
			if resp.StatusCode != stdhttp.StatusBadRequest {
				t.Errorf("estado = %d, queria %d", resp.StatusCode, stdhttp.StatusBadRequest)
			}
		})
	}

	// Nenhuma das tentativas pode ter criado a credencial.
	precisa, err := a.auth.NeedsSetup(t.Context())
	if err != nil {
		t.Fatalf("NeedsSetup: %v", err)
	}
	if !precisa {
		t.Error("uma senha inválida não pode criar a credencial")
	}
}

func TestLoginFluxoCompleto(t *testing.T) {
	a := novoAmbiente(t)
	a.definirSenha()

	// Nova sessão de navegador.
	a.client.Jar, _ = cookiejar.New(nil)

	resp := a.pedido(stdhttp.MethodPost, "/entrar", url.Values{
		"senha":   {senhaBoa},
		"ator":    {"Giovani"},
		"destino": {"/"},
	})
	if resp.StatusCode != stdhttp.StatusSeeOther {
		t.Fatalf("estado = %d, queria %d", resp.StatusCode, stdhttp.StatusSeeOther)
	}
	if destino := resp.Header.Get("Location"); destino != "/" {
		t.Errorf("destino = %q, queria %q", destino, "/")
	}

	resp = a.pedido(stdhttp.MethodGet, "/", nil)
	if resp.StatusCode != stdhttp.StatusOK {
		t.Fatalf("página inicial devolveu %d", resp.StatusCode)
	}
	corpo := resp.Corpo
	if !strings.Contains(corpo, "Giovani") {
		t.Error("o nome declarado devia aparecer no cabeçalho")
	}
	if !strings.Contains(corpo, "Fundação instalada") {
		t.Error("a página devia mostrar o estado da fundação")
	}
}

func TestLoginSenhaErrada(t *testing.T) {
	a := novoAmbiente(t)
	a.definirSenha()
	a.client.Jar, _ = cookiejar.New(nil)

	resp := a.pedido(stdhttp.MethodPost, "/entrar", url.Values{
		"senha": {"senha errada longa"},
	})
	if resp.StatusCode != stdhttp.StatusUnauthorized {
		t.Errorf("estado = %d, queria %d", resp.StatusCode, stdhttp.StatusUnauthorized)
	}
	if corpo := resp.Corpo; !strings.Contains(corpo, "incorreta") {
		t.Error("a página devia explicar que a senha está incorreta")
	}
}

func TestLogoutEncerraSessao(t *testing.T) {
	a := novoAmbiente(t)
	a.definirSenha()

	resp := a.pedido(stdhttp.MethodPost, "/sair", nil)
	if resp.StatusCode != stdhttp.StatusSeeOther {
		t.Fatalf("estado = %d, queria %d", resp.StatusCode, stdhttp.StatusSeeOther)
	}

	resp = a.pedido(stdhttp.MethodGet, "/", nil)
	if resp.StatusCode != stdhttp.StatusSeeOther {
		t.Errorf("depois de sair, a raiz devia encaminhar para a entrada, deu %d", resp.StatusCode)
	}
}

func TestCookieNaoAceitaDeOutroSitio(t *testing.T) {
	a := novoAmbiente(t)
	a.definirSenha()
	a.client.Jar, _ = cookiejar.New(nil)

	// Um token inventado não abre nada, e o valor não pode ser adivinhado por
	// ser previsível.
	req, err := stdhttp.NewRequestWithContext(t.Context(), stdhttp.MethodGet, a.serv.URL+"/", nil)
	if err != nil {
		t.Fatalf("criar pedido: %v", err)
	}
	req.AddCookie(&stdhttp.Cookie{Name: auth.SessionCookieName, Value: "token-inventado"})

	resp, err := a.client.Do(req)
	if err != nil {
		t.Fatalf("executar pedido: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != stdhttp.StatusSeeOther {
		t.Errorf("token inválido devia encaminhar para a entrada, deu %d", resp.StatusCode)
	}
}

func TestEstatiticosServidos(t *testing.T) {
	a := novoAmbiente(t)

	for _, ficheiro := range []string{"/static/app.css", "/static/tokens.css", "/static/htmx.min.js"} {
		t.Run(ficheiro, func(t *testing.T) {
			resp := a.pedido(stdhttp.MethodGet, ficheiro, nil)
			if resp.StatusCode != stdhttp.StatusOK {
				t.Fatalf("estado = %d, queria %d", resp.StatusCode, stdhttp.StatusOK)
			}
			if corpo := resp.Corpo; corpo == "" {
				t.Error("o ficheiro está vazio")
			}
		})
	}
}

func TestCabecalhosDeSeguranca(t *testing.T) {
	a := novoAmbiente(t)

	resp := a.pedido(stdhttp.MethodGet, "/entrar", nil)

	cabecalhos := map[string]string{
		"Content-Security-Policy": "",
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "same-origin",
	}

	for nome, quero := range cabecalhos {
		obtido := resp.Header.Get(nome)
		if obtido == "" {
			t.Errorf("cabeçalho %s ausente", nome)
			continue
		}
		if quero != "" && obtido != quero {
			t.Errorf("%s = %q, queria %q", nome, obtido, quero)
		}
	}

	csp := resp.Header.Get("Content-Security-Policy")
	if strings.Contains(csp, "unsafe-eval") || strings.Contains(csp, "unsafe-inline") {
		t.Errorf("a política de conteúdo não pode permitir avaliação dinâmica: %q", csp)
	}
}

func TestSaudeNaoExigeSessao(t *testing.T) {
	a := novoAmbiente(t)

	resp := a.pedido(stdhttp.MethodGet, "/saude", nil)
	if resp.StatusCode != stdhttp.StatusOK {
		t.Errorf("estado = %d, queria %d", resp.StatusCode, stdhttp.StatusOK)
	}
	if corpo := resp.Corpo; strings.TrimSpace(corpo) != "ok" {
		t.Errorf("corpo = %q, queria %q", corpo, "ok")
	}
}

func TestRotaDesconhecida(t *testing.T) {
	a := novoAmbiente(t)

	if resp := a.pedido(stdhttp.MethodGet, "/nao-existe", nil); resp.StatusCode != stdhttp.StatusNotFound {
		t.Errorf("estado = %d, queria %d", resp.StatusCode, stdhttp.StatusNotFound)
	}
}
