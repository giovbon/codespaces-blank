// Ficheiro de assets embutidos no binário.
//
// Vive na raiz do módulo porque //go:embed só alcança ficheiros da própria
// pasta para baixo, e as migrações e os estáticos ficam na raiz por decisão de
// estrutura (docs/10 §2). Sem isto, o binário não seria autossuficiente e a
// imagem final deixaria de poder ser distroless (ADR-010).
package financas

import (
	"embed"
	"io/fs"
)

var (
	//go:embed migrations/*.sql
	migrationsFS embed.FS

	//go:embed all:web/static
	webFS embed.FS
)

// Migrations contém as migrações, com os .sql na raiz do sistema de ficheiros.
//
// Quem consome recebe o caminho já resolvido e não precisa de saber que os
// ficheiros vivem numa subpasta do repositório. A simetria com Web não é
// estética: enquanto o servidor resolveu o prefixo e o migrator não, as
// migrações deixaram de ser encontradas no binário e o arranque falhou com
// «no such table: users» — falha que os testes não apanhavam, porque liam do
// disco com a raiz certa.
var Migrations = subFS(migrationsFS, "migrations")

// Web contém os ficheiros servidos em /static, com app.css e tokens.css na
// raiz do sistema de ficheiros.
var Web = subFS(webFS, "web/static")

func subFS(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		// Só acontece se a diretiva //go:embed deixar de corresponder à
		// estrutura de pastas: é um erro de programação sem recuperação.
		panic("financas: assets embutidos sem a pasta " + dir + ": " + err.Error())
	}
	return sub
}
