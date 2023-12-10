package amigosecreto

import (
	_ "embed"
)

var (
	//go:embed web/templates/amigo.html
	TemplateAmigo string

	//go:embed web/templates/index.html
	TemplateIndex string

	//go:embed web/templates/compartilhar.html
	TemplateLinks string
)
