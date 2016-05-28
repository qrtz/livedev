package gosource

import (
	"go/scanner"
	"go/token"
	"io/ioutil"
	"os"
	"strings"
)

func Parse(filename string) ([]*Line, error) {

	r, err := os.Open(filename)

	if err != nil {
		return nil, err
	}

	src, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var s scanner.Scanner

	fileSet := token.NewFileSet()

	s.Init(fileSet.AddFile(filename, fileSet.Base(), len(src)), src, nil, scanner.ScanComments)
	var lines []*Line

	for {
		pos, tok, lit := s.Scan()

		if tok == token.EOF {
			break
		}

		position := fileSet.Position(pos)
		indent := space

		if len(lines) != position.Line {

			indent = tab

			// Print empty lines
			for i := len(lines) + 1; i < position.Line; i++ {
				lines = append(lines, &Line{Num: i})
			}

			lines = append(lines, &Line{Num: position.Line, offset: 1})
		}

		current := lines[len(lines)-1]

		// Print white spaces
		if i := position.Column - current.offset; i > 0 {
			current.offset = position.Column
			current.Add(Token{t: tokenSpace, Line: current.Num, Text: strings.Repeat(indent, i)})
		}

		text := lit
		kind := "text"
		name := kind

		if n, ok := tokens[tok]; ok {
			name = n
		}

		if _, ok := literals[lit]; ok {
			kind = "keyword"
		}

		switch {
		case tok.IsKeyword():
			kind = "keyword"
		case tok.IsLiteral():
			switch tok {
			case token.INT, token.FLOAT, token.IMAG:
				kind = "number"
			}
		case tok.IsOperator():
			// Use the operator as literal. Excluding auto-inserted semi-colons
			kind = "operator"
			switch tok {
			default:
				lit = tok.String()
			case token.LPAREN:
				lit = tok.String()

				if p := len(current.Tokens) - 1; p >= 0 && current.Tokens[p].t == token.IDENT {
					current.Tokens[p].Name = "funcall"
				}
			case token.SEMICOLON:
				if lit != tok.String() {
					// Remove auto-inserted semi-colons
					lit = ""
				}
			}

			text = lit
		}

		current.offset = position.Column + len(lit)

		ln := len(text) - 1
		p := 0

		for i, ch := range text {
			if ch == '\n' {
				t := Token{t: tok, Line: current.Num, Text: text[p:i], Kind: kind, Name: name}
				current.Add(t)
				current = &Line{Num: current.Num + 1, offset: current.offset}
				lines = append(lines, current)

				p = i + 1 // Skip the newline character

			} else if i == ln {
				t := Token{t: tok, Line: current.Num, Text: text[p:], Kind: kind, Name: name}
				current.Add(t)
			}
		}
	}

	return lines, nil
}

const (
	space      = " "
	tab        = "    " // 4 space tab
	tokenSpace = token.Token(token.COMMENT + 1)
)

var literals = map[string]struct{}{
	"bool":   struct{}{},
	"close":  struct{}{},
	"error":  struct{}{},
	"iota":   struct{}{},
	"int":    struct{}{},
	"int8":   struct{}{},
	"int16":  struct{}{},
	"int32":  struct{}{},
	"int64":  struct{}{},
	"uint":   struct{}{},
	"uint8":  struct{}{},
	"uint16": struct{}{},
	"uint32": struct{}{},
	"uint64": struct{}{},
	"rune":   struct{}{},
	"string": struct{}{},
	"true":   struct{}{},
	"flase":  struct{}{},
}

var tokens = map[token.Token]string{
	token.COMMENT: "comment",

	token.IDENT:  "ident",
	token.INT:    "int",
	token.FLOAT:  "float",
	token.IMAG:   "imag",
	token.CHAR:   "char",
	token.STRING: "string",

	token.ADD: "add",
	token.SUB: "sub",
	token.MUL: "mul",
	token.QUO: "quo",
	token.REM: "rem",

	token.AND:     "and",
	token.OR:      "or",
	token.XOR:     "xor",
	token.SHL:     "shl",
	token.SHR:     "shr",
	token.AND_NOT: "and_not",

	token.ADD_ASSIGN: "add_assign",
	token.SUB_ASSIGN: "sub_assign",
	token.MUL_ASSIGN: "mul_assign",
	token.QUO_ASSIGN: "quo_assign",
	token.REM_ASSIGN: "rem_assign",

	token.AND_ASSIGN:     "and_assign",
	token.OR_ASSIGN:      "or_assign",
	token.XOR_ASSIGN:     "xor_assign",
	token.SHL_ASSIGN:     "shl_assign",
	token.SHR_ASSIGN:     "shr_assign",
	token.AND_NOT_ASSIGN: "and_not_assign",

	token.LAND:  "land",
	token.LOR:   "lor",
	token.ARROW: "arrow",
	token.INC:   "inc",
	token.DEC:   "dec",

	token.EQL:    "eql",
	token.LSS:    "lss",
	token.GTR:    "gtr",
	token.ASSIGN: "assign",
	token.NOT:    "not",

	token.NEQ:      "neq",
	token.LEQ:      "leq",
	token.GEQ:      "geq",
	token.DEFINE:   "define",
	token.ELLIPSIS: "ellipsis",

	token.LPAREN: "paren",
	token.LBRACK: "brack",
	token.LBRACE: "brace",
	token.COMMA:  "comma",
	token.PERIOD: "period",

	token.RPAREN:    "paren",
	token.RBRACK:    "brack",
	token.RBRACE:    "brace",
	token.SEMICOLON: "semicolon",
	token.COLON:     "colon",

	token.BREAK:    "break",
	token.CASE:     "case",
	token.CHAN:     "chan",
	token.CONST:    "const",
	token.CONTINUE: "continue",

	token.DEFAULT:     "default",
	token.DEFER:       "defer",
	token.ELSE:        "else",
	token.FALLTHROUGH: "fallthrough",
	token.FOR:         "for",

	token.FUNC:   "func",
	token.GO:     "go",
	token.GOTO:   "goto",
	token.IF:     "if",
	token.IMPORT: "import",

	token.INTERFACE: "interface",
	token.MAP:       "map",
	token.PACKAGE:   "package",
	token.RANGE:     "range",
	token.RETURN:    "return",

	token.SELECT: "select",
	token.STRUCT: "struct",
	token.SWITCH: "switch",
	token.TYPE:   "type",
	token.VAR:    "var",
}

type Line struct {
	Num    int
	Tokens []Token
	offset int
}

func (l *Line) Add(t Token) {
	l.Tokens = append(l.Tokens, t)
}

type Token struct {
	Name, Text, Kind string
	Line             int
	t                token.Token
}
