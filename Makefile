SHELL := bash
.DEFAULT_GOAL := help

GO   ?= go
BIN  := bin/financas
ADDR ?= 127.0.0.1:8080

.PHONY: help
help: ## Lista os alvos disponíveis
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  %-12s %s\n", $$1, $$2}'

.PHONY: generate
generate: templ css ## Regenera código templ e CSS

.PHONY: templ
templ: ## Gera os ficheiros *_templ.go a partir de *.templ
	templ generate

.PHONY: css
css: ## Compila web/static/app.css a partir de tailwind.css e dos tokens
	tailwindcss -i web/static/tailwind.css -o web/static/app.css --minify

.PHONY: build
build: ## Compila o binário em bin/financas
	CGO_ENABLED=0 $(GO) build -o $(BIN) ./cmd/app

.PHONY: run
run: ## Arranca a aplicação (ADDR=127.0.0.1:8080)
	CGO_ENABLED=0 $(GO) run ./cmd/app -addr $(ADDR)

.PHONY: test
test: ## Corre todos os testes
	$(GO) test ./...

.PHONY: cover
cover: ## Testes com cobertura por pacote
	$(GO) test ./... -cover

.PHONY: lint
lint: ## Corre o golangci-lint
	golangci-lint run

.PHONY: fmt
fmt: ## Formata o código (formatador do próprio linter)
	golangci-lint fmt

.PHONY: tidy
tidy: ## Sincroniza go.mod e go.sum
	$(GO) mod tidy

.PHONY: check
check: generate fmt lint test build ## Verificação completa antes de entregar
	@echo "verificação completa: OK"

.PHONY: clean
clean: ## Remove binários e artefactos
	rm -rf bin
