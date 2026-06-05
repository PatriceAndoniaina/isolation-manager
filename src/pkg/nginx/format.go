package nginx

import "strings"

// indentUnit est l'unité d'indentation du formateur (4 espaces).
const indentUnit = "    "

// Format rend un arbre de directives en configuration nginx canonique :
// indentation à 4 espaces, un point-virgule par directive simple, blocs
// « { } » pour les directives à bloc. Format est l'inverse de Parse — à la
// présentation près — et est idempotent : Format(Parse(Format(x))) == Format(x).
func Format(dirs []*Directive) string {
	var b strings.Builder
	formatBlock(&b, dirs, 0)
	return b.String()
}

// formatBlock écrit récursivement les directives avec l'indentation de depth.
func formatBlock(b *strings.Builder, dirs []*Directive, depth int) {
	indent := strings.Repeat(indentUnit, depth)
	for _, d := range dirs {
		b.WriteString(indent)
		b.WriteString(d.Name)
		for _, a := range d.Args {
			b.WriteByte(' ')
			b.WriteString(quoteArg(a))
		}
		if d.Block != nil {
			b.WriteString(" {\n")
			formatBlock(b, d.Block, depth+1)
			b.WriteString(indent)
			b.WriteString("}\n")
			continue
		}
		b.WriteString(";\n")
	}
}

// quoteArg ré-entoure un argument de guillemets doubles s'il contient des
// caractères qui, sans cela, briseraient la syntaxe (espaces, séparateurs).
// Le lexer ayant retiré les guillemets, c'est ici qu'on les restitue.
func quoteArg(a string) string {
	if a == "" || strings.ContainsAny(a, " \t\r\n\"';{}#") {
		return `"` + strings.ReplaceAll(a, `"`, `\"`) + `"`
	}
	return a
}
