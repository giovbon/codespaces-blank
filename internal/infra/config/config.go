// Package config resolve a configuração da aplicação a partir de flags e de
// variáveis de ambiente. Flags têm precedência sobre o ambiente.
package config

import (
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Prefixo das variáveis de ambiente reconhecidas.
const EnvPrefix = "FINANCAS_"

// Config é a configuração resolvida.
type Config struct {
	// Addr é o endereço de escuta, no formato host:porta.
	Addr string

	// DataDir é a pasta dos dados persistentes: base, uploads e anexos.
	DataDir string

	// DBPath é o arquivo SQLite. Por padrão vive dentro de DataDir.
	DBPath string

	// UploadsDir guarda os arquivos originais, para reprocessar um lote.
	UploadsDir string

	// InboxDir é vigiada para importação automática (fase M2).
	InboxDir string

	// SessionTTL é a validade de uma sessão.
	SessionTTL time.Duration

	// SecureCookies exige HTTPS para o cookie de sessão. Desligado por padrão
	// para funcionar em rede local sem TLS; ligar em qualquer exposição real.
	SecureCookies bool

	// LogLevel é um dos níveis de log/slog: debug, info, warn, error.
	LogLevel string
}

// FromFlags interpreta args e o ambiente, e devolve a configuração validada.
//
// getenv existe para que o ambiente seja injetado nos testes em vez de lido
// diretamente.
func FromFlags(args []string, getenv func(string) string, errOut io.Writer) (Config, error) {
	cfg := Config{
		Addr:          envOr(getenv, "ADDR", "127.0.0.1:8080"),
		DataDir:       envOr(getenv, "DATA_DIR", "./data"),
		SessionTTL:    30 * 24 * time.Hour,
		SecureCookies: envBool(getenv, "SECURE_COOKIES", false),
		LogLevel:      envOr(getenv, "LOG_LEVEL", "info"),
	}

	fs := flag.NewFlagSet("financas", flag.ContinueOnError)
	fs.SetOutput(errOut)

	var ttlHoras int
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "endereço de escuta (host:porta)")
	fs.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "pasta dos dados persistentes")
	fs.StringVar(&cfg.DBPath, "db", "", "arquivo SQLite (por padrão dentro de data-dir)")
	fs.StringVar(&cfg.UploadsDir, "uploads-dir", "", "pasta dos arquivos originais")
	fs.StringVar(&cfg.InboxDir, "inbox-dir", "", "pasta vigiada para importação automática")
	fs.IntVar(&ttlHoras, "session-ttl-hours", int(cfg.SessionTTL.Hours()), "validade da sessão, em horas")
	fs.BoolVar(&cfg.SecureCookies, "secure-cookies", cfg.SecureCookies, "exigir HTTPS no cookie de sessão")
	fs.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "nível de log: debug, info, warn, error")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	cfg.SessionTTL = time.Duration(ttlHoras) * time.Hour

	if err := cfg.normalize(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// normalize preenche os caminhos derivados e valida a coerência.
func (c *Config) normalize() error {
	if strings.TrimSpace(c.Addr) == "" {
		return fmt.Errorf("config: endereço de escuta é obrigatório")
	}
	if strings.TrimSpace(c.DataDir) == "" {
		return fmt.Errorf("config: pasta de dados é obrigatória")
	}
	if c.SessionTTL <= 0 {
		return fmt.Errorf("config: validade da sessão tem de ser positiva")
	}

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("config: nível de log inválido: %q", c.LogLevel)
	}

	if c.DBPath == "" {
		c.DBPath = filepath.Join(c.DataDir, "financas.db")
	}
	if c.UploadsDir == "" {
		c.UploadsDir = filepath.Join(c.DataDir, "uploads")
	}
	if c.InboxDir == "" {
		c.InboxDir = filepath.Join(c.DataDir, "inbox")
	}

	return nil
}

func envOr(getenv func(string) string, sufixo, porOmissao string) string {
	if v := strings.TrimSpace(getenv(EnvPrefix + sufixo)); v != "" {
		return v
	}
	return porOmissao
}

func envBool(getenv func(string) string, sufixo string, porOmissao bool) bool {
	v := strings.TrimSpace(getenv(EnvPrefix + sufixo))
	if v == "" {
		return porOmissao
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return porOmissao
	}
	return b
}
