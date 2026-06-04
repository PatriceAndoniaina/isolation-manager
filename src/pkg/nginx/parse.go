// Package nginx fournit un parseur de configuration nginx et un générateur de
// reverse proxy durci pour les conteneurs.
//
// Le parseur produit un arbre de directives (blocs http/server/location
// imbriqués) en un seul passage lexical suivi d'un parcours, soit une
// complexité linéaire O(n) sans backtracking. Le générateur (generate.go)
// s'appuie sur ce parseur pour relire sa propre sortie et garantir qu'elle
// respecte les règles de sécurité (validate.go) avant de la livrer.
package nginx

import (
	"fmt"
)

// Directive représente une directive nginx, éventuellement suivie d'un bloc.
// Block est non-nil (mais possiblement vide) si la directive ouvre un « { } ».
type Directive struct {
	Name  string
	Args  []string
	Block []*Directive
}

// tokenKind énumère les lexèmes reconnus.
type tokenKind int

const (
	tokWord  tokenKind = iota // identifiant ou valeur
	tokOpen                   // {
	tokClose                  // }
	tokSemi                   // ;
)

type token struct {
	kind tokenKind
	val  string
	line int
}

// lex découpe l'entrée en lexèmes. Les commentaires (# … fin de ligne) sont
// ignorés ; les chaînes entre guillemets simples ou doubles forment un seul mot.
// Indexation par octet : un seul passage, O(n).
func lex(input string) ([]token, error) {
	var toks []token
	line := 1
	for i, n := 0, len(input); i < n; {
		c := input[i]
		switch {
		case c == '\n':
			line++
			i++
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == '#':
			for i < n && input[i] != '\n' {
				i++
			}
		case c == '{':
			toks = append(toks, token{tokOpen, "{", line})
			i++
		case c == '}':
			toks = append(toks, token{tokClose, "}", line})
			i++
		case c == ';':
			toks = append(toks, token{tokSemi, ";", line})
			i++
		case c == '"' || c == '\'':
			quote := c
			i++
			start := i
			for i < n && input[i] != quote {
				if input[i] == '\n' {
					line++
				}
				i++
			}
			if i >= n {
				return nil, fmt.Errorf("ligne %d: chaîne non terminée", line)
			}
			toks = append(toks, token{tokWord, input[start:i], line})
			i++ // consomme le guillemet fermant
		default:
			start := i
			for i < n && !isSpecial(input[i]) {
				i++
			}
			toks = append(toks, token{tokWord, input[start:i], line})
		}
	}
	return toks, nil
}

// isSpecial indique si l'octet termine un mot non quoté.
func isSpecial(c byte) bool {
	switch c {
	case ' ', '\t', '\r', '\n', '{', '}', ';', '#', '"', '\'':
		return true
	}
	return false
}

// Parse analyse une configuration nginx en arbre de directives.
func Parse(input string) ([]*Directive, error) {
	toks, err := lex(input)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	return p.parseBlock(true)
}

type parser struct {
	toks []token
	pos  int
}

// parseBlock lit des directives jusqu'à un '}' (top=false) ou la fin de
// l'entrée (top=true).
func (p *parser) parseBlock(top bool) ([]*Directive, error) {
	dirs := []*Directive{}
	for p.pos < len(p.toks) {
		t := p.toks[p.pos]
		if t.kind == tokClose {
			if top {
				return nil, fmt.Errorf("ligne %d: '}' inattendu", t.line)
			}
			p.pos++ // consomme '}'
			return dirs, nil
		}
		d, err := p.parseDirective()
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, d)
	}
	if !top {
		return nil, fmt.Errorf("'}' manquant en fin de configuration")
	}
	return dirs, nil
}

// parseDirective lit un nom, ses arguments, puis ';' (directive simple) ou un
// bloc '{ … }'.
func (p *parser) parseDirective() (*Directive, error) {
	first := p.toks[p.pos]
	if first.kind != tokWord {
		return nil, fmt.Errorf("ligne %d: nom de directive attendu, trouvé %q", first.line, first.val)
	}
	d := &Directive{Name: first.val}
	p.pos++

	for p.pos < len(p.toks) {
		t := p.toks[p.pos]
		switch t.kind {
		case tokWord:
			d.Args = append(d.Args, t.val)
			p.pos++
		case tokSemi:
			p.pos++
			return d, nil
		case tokOpen:
			p.pos++
			block, err := p.parseBlock(false)
			if err != nil {
				return nil, err
			}
			d.Block = block
			return d, nil
		case tokClose:
			return nil, fmt.Errorf("ligne %d: '}' inattendu dans la directive %q", t.line, d.Name)
		}
	}
	return nil, fmt.Errorf("directive %q non terminée (';' ou '{' manquant)", d.Name)
}

// Walk parcourt l'arbre en profondeur et applique fn à chaque directive.
func Walk(dirs []*Directive, fn func(*Directive)) {
	for _, d := range dirs {
		fn(d)
		if d.Block != nil {
			Walk(d.Block, fn)
		}
	}
}
