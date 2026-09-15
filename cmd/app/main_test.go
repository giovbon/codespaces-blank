package main

import (
	"strings"
	"testing"
)

// TestLerSenhaPartilhaOLeitor cobre o defeito que só apareceu a correr o
// binário: um leitor novo por chamada deixava a segunda linha presa no buffer
// da primeira, e duas senhas iguais eram lidas como diferentes.
func TestLerSenhaPartilhaOLeitor(t *testing.T) {
	entrada := novaEntradaDeSenha(strings.NewReader("primeira linha\nsegunda linha\n"))
	var saida strings.Builder

	primeira, err := lerSenha(entrada, &saida, "Nova senha: ")
	if err != nil {
		t.Fatalf("primeira leitura: %v", err)
	}
	segunda, err := lerSenha(entrada, &saida, "Repetir: ")
	if err != nil {
		t.Fatalf("segunda leitura: %v", err)
	}

	if primeira != "primeira linha" {
		t.Errorf("primeira = %q, queria %q", primeira, "primeira linha")
	}
	if segunda != "segunda linha" {
		t.Errorf("segunda = %q, queria %q", segunda, "segunda linha")
	}
	if primeira == segunda {
		t.Error("as duas leituras deviam ser linhas distintas do tubo")
	}
}

func TestLerSenhaRemoveFimDeLinha(t *testing.T) {
	casos := map[string]string{
		"unix":  "senha\n",
		"dos":   "senha\r\n",
		"final": "senha",
	}

	for nome, entrada := range casos {
		t.Run(nome, func(t *testing.T) {
			var saida strings.Builder
			lida, err := lerSenha(novaEntradaDeSenha(strings.NewReader(entrada)), &saida, "")
			if err != nil {
				t.Fatalf("ler senha: %v", err)
			}
			if lida != "senha" {
				t.Errorf("senha lida = %q, queria %q", lida, "senha")
			}
		})
	}
}

func TestLerSenhaLinhaVazia(t *testing.T) {
	// Uma linha vazia é uma senha vazia, e não um erro: quem decide se serve é
	// a validação de comprimento.
	var saida strings.Builder
	lida, err := lerSenha(novaEntradaDeSenha(strings.NewReader("\n")), &saida, "")
	if err != nil {
		t.Fatalf("ler senha: %v", err)
	}
	if lida != "" {
		t.Errorf("senha lida = %q, queria vazia", lida)
	}
}

func TestSepararFlagSenha(t *testing.T) {
	casos := []struct {
		nome      string
		args      []string
		restantes []string
		ativa     bool
		erro      bool
	}{
		{
			nome:      "sem a flag",
			args:      []string{"-addr", "127.0.0.1:9000"},
			restantes: []string{"-addr", "127.0.0.1:9000"},
		},
		{
			nome:      "com a flag",
			args:      []string{"-set-password", "-addr", "127.0.0.1:9000"},
			restantes: []string{"-addr", "127.0.0.1:9000"},
			ativa:     true,
		},
		{
			nome:      "com a flag no fim",
			args:      []string{"-addr", "127.0.0.1:9000", "-set-password"},
			restantes: []string{"-addr", "127.0.0.1:9000"},
			ativa:     true,
		},
		{
			nome: "com a flag e valor verdadeiro",
			args: []string{"-set-password=true"},
			// Sem restantes: a flag sai da lista antes de a configuração a ver.
			restantes: []string{},
			ativa:     true,
		},
		{
			nome:      "com a flag e valor falso",
			args:      []string{"-set-password=false", "-addr", "127.0.0.1:9000"},
			restantes: []string{"-addr", "127.0.0.1:9000"},
			ativa:     false,
		},
		{
			nome: "com valor inválido",
			args: []string{"-set-password=sim"},
			erro: true,
		},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			restantes, ativa, err := separarFlagSenha(caso.args)
			if caso.erro {
				if err == nil {
					t.Fatal("esperava erro")
				}
				return
			}
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if ativa != caso.ativa {
				t.Errorf("ativa = %v, queria %v", ativa, caso.ativa)
			}
			if len(restantes) != len(caso.restantes) {
				t.Fatalf("restantes = %v, queria %v", restantes, caso.restantes)
			}
			for i := range restantes {
				if restantes[i] != caso.restantes[i] {
					t.Errorf("restantes[%d] = %q, queria %q", i, restantes[i], caso.restantes[i])
				}
			}
		})
	}
}
