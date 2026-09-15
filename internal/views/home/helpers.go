package home

import "strconv"

// texto converte um número para texto.
//
// templ só interpola strings diretamente; manter a conversão aqui evita
// repetir a mesma chamada em cada ponto do markup.
func texto(n int) string { return strconv.Itoa(n) }
