// Comando app é o ponto de entrada único da aplicação.
//
// Faz três coisas: resolve a configuração, abre a base e serve HTTP até receber
// um sinal de terminação. Não há mais nenhum binário.
package main

import (
	"bufio"
	"context"
	"errors"
	"financas/internal/data"
	"financas/internal/http"
	"financas/internal/infra/clock"
	"financas/internal/infra/config"
	"financas/internal/infra/log"
	"financas/internal/modules/auth"
	"financas/internal/modules/system"
	"flag"
	"fmt"
	"io"
	"net"
	stdhttp "net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"financas" // ficheiros embutidos: migrações e web/static
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, errOut io.Writer) error {
	// A troca de senha é lida à parte: exige a base aberta, mas não o servidor,
	// e não pertence à configuração. É o único caminho para redefinir a senha
	// (ADR-019).
	restantes, definirSenha, err := separarFlagSenha(args)
	if err != nil {
		return err
	}

	cfg, err := config.FromFlags(restantes, os.Getenv, errOut)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	logger := log.New(cfg.LogLevel, errOut)
	logger.Info("finanças a arrancar",
		"versao", "0.1.0-m0",
		"addr", cfg.Addr,
		"base", cfg.DBPath,
	)

	ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer parar()

	db, err := data.Open(ctx, data.Config{Path: cfg.DBPath, Migrations: financas.Migrations})
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("fechar base de dados", "erro", err)
		}
	}()

	mut := data.NewMutator(db)
	authSvc := auth.New(db, mut, cfg.SessionTTL)
	systemSvc := system.New(db, mut)

	if definirSenha {
		return trocarSenha(ctx, authSvc, stdin, errOut)
	}

	servidor, err := http.NewServer(http.Deps{
		Auth:          authSvc,
		System:        systemSvc,
		Log:           logger,
		Assets:        financas.Web,
		SecureCookies: cfg.SecureCookies,
		Timezone:      clock.Location(clock.Timezone),
	})
	if err != nil {
		return err
	}

	precisa, err := authSvc.NeedsSetup(ctx)
	if err != nil {
		return err
	}
	if precisa {
		logger.Warn("senha por definir: abra o endereço no navegador para a criar",
			"endereco", "http://"+cfg.Addr)
	} else {
		logger.Info("pronto", "endereco", "http://"+cfg.Addr)
	}

	srv := &stdhttp.Server{
		Addr:              cfg.Addr,
		Handler:           servidor.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}

	erros := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
			erros <- err
			return
		}
		erros <- nil
	}()

	select {
	case err := <-erros:
		return err
	case <-ctx.Done():
		logger.Info("a encerrar")
	}

	// Prazo para terminar pedidos em curso: uma importação a meio tem de
	// poder acabar ou ser interrompida de forma limpa.
	desligarCtx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()

	if err := srv.Shutdown(desligarCtx); err != nil {
		return fmt.Errorf("encerrar servidor: %w", err)
	}
	return nil
}

// trocarSenha lê uma senha nova e substitui a existente.
//
// Lê do terminal sem eco quando há terminal; caso contrário, lê uma linha do
// stdin, para permitir uso em script.
func trocarSenha(ctx context.Context, svc *auth.Service, stdin io.Reader, errOut io.Writer) error {
	// O leitor é criado uma vez e partilhado: bufio lê para lá da linha pedida,
	// e um leitor novo por chamada perderia a segunda linha do tubo — o que
	// fazia duas senhas iguais parecerem diferentes.
	leitor := novaEntradaDeSenha(stdin)

	senha, err := lerSenha(leitor, errOut, "Nova senha: ")
	if err != nil {
		return err
	}

	confirmacao, err := lerSenha(leitor, errOut, "Repetir a nova senha: ")
	if err != nil {
		return err
	}
	if senha != confirmacao {
		return errors.New("as senhas não coincidem")
	}

	if err := svc.SetPassword(ctx, senha); err != nil {
		return err
	}

	escrever(errOut, "senha alterada; todas as sessões foram encerradas\n")
	return nil
}

// escrever emite uma mensagem para o utilizador da linha de comando.
//
// O erro é deliberadamente ignorado: se o terminal de saída falhar, não há
// onde o reportar, e a operação em curso não depende desta escrita.
func escrever(w io.Writer, texto string) {
	_, _ = io.WriteString(w, texto)
}

// entradaDeSenha distingue a leitura interativa da leitura de um tubo.
type entradaDeSenha struct {
	terminal *os.File
	leitor   *bufio.Reader
}

// novoEntradaDeSenha escolhe a via de leitura conforme a origem.
func novaEntradaDeSenha(stdin io.Reader) entradaDeSenha {
	if f, ok := stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		return entradaDeSenha{terminal: f}
	}
	return entradaDeSenha{leitor: bufio.NewReader(stdin)}
}

func lerSenha(entrada entradaDeSenha, errOut io.Writer, aviso string) (string, error) {
	escrever(errOut, aviso)

	if entrada.terminal != nil {
		b, err := term.ReadPassword(int(entrada.terminal.Fd()))
		escrever(errOut, "\n")
		if err != nil {
			return "", fmt.Errorf("ler senha: %w", err)
		}
		return string(b), nil
	}

	linha, err := entrada.leitor.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("ler senha: %w", err)
	}
	return strings.TrimRight(linha, "\r\n"), nil
}

// separarFlagSenha retira -set-password dos argumentos e devolve o seu valor.
//
// A flag é retirada antes de a configuração interpretar os restantes: se
// ficasse, o interpretador de flags recusá-la-ia por não estar registada.
func separarFlagSenha(args []string) (restantes []string, ativa bool, err error) {
	const nome = "-set-password"

	for _, arg := range args {
		if arg != nome && !strings.HasPrefix(arg, nome+"=") {
			restantes = append(restantes, arg)
			continue
		}

		valor, temValor := strings.CutPrefix(arg, nome+"=")
		if !temValor {
			ativa = true
			continue
		}

		switch valor {
		case "", "true", "1":
			ativa = true
		case "false", "0":
			ativa = false
		default:
			return nil, false, fmt.Errorf("valor inválido para %s: %q", nome, valor)
		}
	}

	return restantes, ativa, nil
}
