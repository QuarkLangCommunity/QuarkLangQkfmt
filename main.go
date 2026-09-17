// qkfmt: QuarkLang 代码格式化器（正典语法）
//
// 用法: qkfmt [-w|-l|-d] files...
//
//	-w 原地写 / -l 列出未格式化（CI，有差异退出 1）/ -d 打印差异 / 默认打印到 stdout
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

type kind int

const (
	kWord kind = iota
	kNum
	kStr
	kComment
	kOp
)

type token struct {
	k    kind
	text string
	line int
}

var ops3 = []string{"..."}
var ops2 = []string{"::", "==", "!=", "<=", ">=", "&&", "||", "<<", ">>", "+=", "-=", "*=", "/=", "%="}
var ops1 = "{}()[];,.:+-*/%<>=!&|@#?"

func isWordStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}
func isWordPart(c byte) bool { return isWordStart(c) || (c >= '0' && c <= '9') }
func isDigit(c byte) bool    { return c >= '0' && c <= '9' }

// singleCharText 单字符 token 文本：预建表，避免每个标点分配一个 1 字节字符串。
var singleCharTable = func() [256]string {
	var t [256]string
	for i := 0; i < 256; i++ {
		t[i] = string(rune(i))
	}
	return t
}()

func scan(src string) ([]token, error) {
	toks := make([]token, 0, len(src)/3+8)
	i, line := 0, 1
	n := len(src)
	for i < n {
		c := src[i]
		switch {
		case c == '\n':
			line++
			i++
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == '/' && i+1 < n && src[i+1] == '/':
			j := i
			for j < n && src[j] != '\n' {
				j++
			}
			toks = append(toks, token{kComment, strings.TrimRight(src[i:j], " \t\r"), line})
			i = j
		case c == '/' && i+1 < n && src[i+1] == '*':
			j := i + 2
			for j+1 < n && !(src[j] == '*' && src[j+1] == '/') {
				if src[j] == '\n' {
					line++
				}
				j++
			}
			if j+1 < n {
				j += 2
			} else {
				j = n
			}
			toks = append(toks, token{kComment, src[i:j], line})
			i = j
		case c == '"':
			j := i + 1
			for j < n {
				if src[j] == 0x5c {
					j += 2
					continue
				}
				if src[j] == '"' {
					j++
					break
				}
				if src[j] == '\n' {
					line++
				}
				j++
			}
			toks = append(toks, token{kStr, src[i:j], line})
			i = j
		case c == 0x60:
			j := i + 1
			for j < n && src[j] != 0x60 {
				if src[j] == '\n' {
					line++
				}
				j++
			}
			if j < n {
				j++
			}
			toks = append(toks, token{kStr, src[i:j], line})
			i = j
		case isDigit(c):
			j := i
			for j < n && (isDigit(src[j]) || isWordPart(src[j]) || src[j] == '.') {
				if src[j] == '.' && j+1 < n && !isDigit(src[j+1]) {
					break
				}
				j++
			}
			toks = append(toks, token{kNum, src[i:j], line})
			i = j
		case isWordStart(c):
			j := i
			for j < n && isWordPart(src[j]) {
				j++
			}
			toks = append(toks, token{kWord, src[i:j], line})
			i = j
		default:
			matched := false
			for _, o := range ops3 {
				if strings.HasPrefix(src[i:], o) {
					toks = append(toks, token{kOp, o, line})
					i += len(o)
					matched = true
					break
				}
			}
			if matched {
				continue
			}
			for _, o := range ops2 {
				if strings.HasPrefix(src[i:], o) {
					toks = append(toks, token{kOp, o, line})
					i += len(o)
					matched = true
					break
				}
			}
			if matched {
				continue
			}
			if strings.IndexByte(ops1, c) >= 0 {
				toks = append(toks, token{kOp, singleCharTable[c], line})
				i++
				continue
			}
			return nil, fmt.Errorf("qkfmt: 无法识别的字符 %q（第 %d 行）", string(c), line)
		}
	}
	return toks, nil
}

type writer struct {
	b         strings.Builder
	line      strings.Builder
	indent    int
	started   bool
	blank     bool
	prev      string
	prevK     kind
	lineLen   int  // 当前行已写字节数（免 Strings.HasSuffix 复制整行）
	lastSpace bool // 当前行尾是否已是空格
}

func (w *writer) flush() {
	w.b.WriteString(strings.TrimRight(w.line.String(), " "))
	w.b.WriteByte('\n')
	w.line.Reset()
	w.started = false
	w.lineLen = 0
	w.lastSpace = false
}

func (w *writer) ensure() {
	if !w.started {
		if w.blank && w.b.Len() > 0 {
			w.b.WriteByte('\n')
			w.blank = false
		}
		for i := 0; i < w.indent; i++ {
			w.line.WriteString("    ")
		}
		w.started = true
	}
}

func (w *writer) raw(s string) {
	w.ensure()
	w.line.WriteString(s)
	w.lineLen += len(s)
	w.lastSpace = len(s) > 0 && s[len(s)-1] == ' '
}

func (w *writer) space() {
	// 不调用 line.String()（会复制整行）；用「行尾是否已空格」标记判定。
	if w.started && w.lineLen > 0 && !w.lastSpace {
		w.line.WriteString(" ")
		w.lineLen++
		w.lastSpace = true
	}
}

// binary：需要两侧留空的二元运算符（一元位置由 isUnaryCtx 判定，不留空）。
var binary = map[string]bool{
	"=": true, "+": true, "-": true, "*": true, "/": true, "%": true,
	"<": true, ">": true, "<=": true, ">=": true, "==": true, "!=": true,
	"&&": true, "||": true, "<<": true, ">>": true, "&": true, "|": true,
	"+=": true, "-=": true, "*=": true, "/=": true, "%=": true,
}

func needSpace(p string, pk kind, cur string, ck kind) bool {
	if p == "" {
		return false
	}
	switch cur {
	case ")", "]", ",", ";", ".", "::":
		return false
	}
	switch p {
	case "(", "[", ".", "::", "@", "#":
		return false
	}
	if cur == "(" || cur == "[" {
		return kwSpaceBeforeParen[p]
	}
	// `fn f(int a) int { ... }`：形参表之后是返回类型，必须留一个空格
	if p == ")" && (ck == kWord || ck == kNum || ck == kStr) {
		return true
	}
	if ck == kComment || pk == kComment {
		return true
	}
	if (pk == kWord || pk == kNum || pk == kStr) && (ck == kWord || ck == kNum || ck == kStr) {
		return true
	}
	return false
}

var kwSpaceBeforeParen = map[string]bool{"if": true, "while": true, "for": true, "catch": true, "return": true}

// isUnaryCtx 判断当前运算符是否处于一元位置（行首/开括号/逗号/等号/二元运算符之后）。
func isUnaryCtx(p string, pk kind) bool {
	if p == "" {
		return true
	}
	switch p {
	case "(", "[", ",", "=", "==", "!=", "<", ">", "<=", ">=", "&&", "||", "+", "-", "*", "/", "%", "<<", ">>", "!", ":", "::", "{", ";", "return":
		return true
	}
	return pk == kOp && p != ")" && p != "]"
}

func format(src string) (string, error) {
	toks, err := scan(src)
	if err != nil {
		return "", err
	}
	var w writer
	w.b.Grow(len(src) + len(src)/8)
	prevLine := 0
	lastBinary := false
	spaceNext := false
	for idx, t := range toks {
		if prevLine > 0 && t.line-prevLine >= 2 && !w.started && w.indent == 0 {
			w.blank = true
		}
		if t.k == kComment {
			if w.started {
				w.space()
				w.line.WriteString(strings.TrimRight(t.text, " \t"))
				w.flush()
			} else if strings.Contains(t.text, "\n") {
				w.ensure()
				w.line.WriteString(strings.TrimRight(t.text, " \t"))
				w.flush()
			} else {
				w.raw(strings.TrimRight(t.text, " \t"))
				w.flush()
			}
			prevLine = t.line
			continue
		}
		if t.k == kOp && binary[t.text] {
			if !isUnaryCtx(w.prev, w.prevK) {
				w.space()
				lastBinary = true
			} else {
				lastBinary = false
			}
			w.raw(t.text)
			w.prev, w.prevK, prevLine = t.text, t.k, t.line
			continue
		}
		switch t.text {
		case "{":
			if w.started {
				w.space()
			}
			w.raw("{")
			w.flush()
			w.indent++
			lastBinary = false
		case "}":
			if w.started {
				w.flush()
			}
			if w.indent > 0 {
				w.indent--
			}
			w.raw("}")
			if idx+1 < len(toks) && (toks[idx+1].text == "else" || toks[idx+1].text == "catch") {
				w.prev, w.prevK, prevLine = "}", kOp, t.line
				spaceNext = true
				continue
			}
			w.flush()
			lastBinary = false
		case ";":
			w.raw(";")
			w.flush()
			lastBinary = false
		default:
			if spaceNext || lastBinary || needSpace(w.prev, w.prevK, t.text, t.k) {
				w.space()
			}
			w.raw(t.text)
			spaceNext = false
			lastBinary = false
		}
		w.prev, w.prevK, prevLine = t.text, t.k, t.line
	}
	if w.started {
		w.flush()
	}
	out := w.b.String()
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	return strings.TrimRight(out, "\n") + "\n", nil
}

func main() {
	args := os.Args[1:]
	var files []string
	write, list, diff := false, false, false
	for _, a := range args {
		switch a {
		case "-w":
			write = true
		case "-l":
			list = true
		case "-d":
			diff = true
		default:
			files = append(files, a)
		}
	}
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "usage: qkfmt [-w|-l|-d] files...")
		os.Exit(2)
	}
	sort.Strings(files)
	changed := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintln(os.Stderr, "qkfmt:", err)
			os.Exit(2)
		}
		out, err := format(string(data))
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", f, err)
			os.Exit(2)
		}
		if out == string(data) {
			if !write && !list && !diff {
				fmt.Print(out)
			}
			continue
		}
		changed++
		switch {
		case list:
			fmt.Println(f)
		case diff:
			fmt.Printf("--- %s\n+++ %s (formatted)\n", f, f)
			al, bl := strings.Split(string(data), "\n"), strings.Split(out, "\n")
			n := len(al)
			if len(bl) > n {
				n = len(bl)
			}
			for i := 0; i < n; i++ {
				var x, y string
				if i < len(al) {
					x = al[i]
				}
				if i < len(bl) {
					y = bl[i]
				}
				if x != y {
					fmt.Printf("-%s\n+%s\n", x, y)
				}
			}
		case write:
			mode := os.FileMode(0o644)
			if info, err := os.Stat(f); err == nil {
				mode = info.Mode()
			}
			if err := os.WriteFile(f, []byte(out), mode); err != nil {
				fmt.Fprintln(os.Stderr, "qkfmt:", err)
				os.Exit(2)
			}
		default:
			fmt.Print(out)
		}
	}
	if list && changed > 0 {
		os.Exit(1)
	}
}
