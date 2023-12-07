package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/weibocom/motan-go/http"
	"gopkg.in/yaml.v2"
)

const (
	LF = '\n'
	CR = '\r'
)

const (
	BlockStart = iota
	BlockDone
	FileDone
	Ok
	Error
)

// Directive location / { # directive location
//
//	   if ($request_uri ~= '/*') { # directive if
//	   }
//	}
type Directive struct {
	name       string
	args       []string
	directives []*Directive
}

type Parser struct {
	line   int
	reader *bufio.Reader
	args   []string
}

type token struct {
	kind int
	text string
}

func newToken(kind int, text string) token {
	return token{kind, text}
}

var (
	verbose  bool
	confFile string
	domain   string
)

func init() {
	flag.BoolVar(&verbose, "v", false, "verbose output")
	flag.StringVar(&confFile, "f", "", "file for parse")
	flag.StringVar(&domain, "d", "", "domain for configuration file")
}

func NewParser(reader io.Reader) *Parser {
	return &Parser{reader: bufio.NewReader(reader)}
}
func (p *Parser) readToken() token {
	buf := bytes.Buffer{}
	sharpComment := false
	quoted := false
	singleQuoted := false
	doubleQuoted := false
	needSpace := false
	lastIsSpace := true // last one is space
	found := false
	variable := false
	isSpace := func(ch byte) bool {
		return ch == ' ' || ch == '\t' || ch == CR || ch == LF
	}

	for {
		ch, err := p.reader.ReadByte()
		if err != nil {
			if err == io.EOF {
				return newToken(FileDone, "")
			}
			return newToken(Error, err.Error())
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "%c", rune(ch))
		}
		if ch == LF {
			p.line++
			if sharpComment {
				sharpComment = false
			}
		}
		if sharpComment {
			continue
		}
		if quoted {
			quoted = false
			buf.WriteByte(ch)
			continue
		}
		if needSpace {
			if isSpace(ch) {
				lastIsSpace = true
				needSpace = false
				continue
			}
			if ch == ';' {
				return newToken(Ok, "")
			}
			if ch == '{' {
				return newToken(BlockStart, "{")
			}
			if ch == ')' {
				lastIsSpace = true
				needSpace = false
			} else {
				return newToken(Error, fmt.Sprintf("unexpected %c", ch))
			}
		}
		if lastIsSpace {
			if isSpace(ch) {
				continue
			}
			switch ch {
			case ';', '{':
				if len(p.args) == 0 {
					return newToken(Error, fmt.Sprintf("unexpected %c", ch))
				}
				if ch == '{' {
					return newToken(BlockStart, "{")
				}
				return newToken(Ok, "")
			case '}':
				if len(p.args) != 0 {
					return newToken(Error, fmt.Sprintf("unexpected %c", ch))
				}
				return newToken(BlockDone, "}")
			case '#':
				sharpComment = true
			case '\\':
				quoted = true
				lastIsSpace = false
				buf.WriteByte(ch) // need write into buffer
			case '"':
				doubleQuoted = true
				lastIsSpace = false
			default:
				lastIsSpace = false
				buf.WriteByte(ch)
			}
		} else {
			if ch == '{' && variable {
				buf.WriteByte(ch)
				continue
			}
			variable = false
			if ch == '\\' {
				quoted = true
				buf.WriteByte(ch)
				continue
			}
			if ch == '$' {
				variable = true
				buf.WriteByte(ch)
				continue
			}
			if doubleQuoted {
				if ch == '"' {
					doubleQuoted = false
					needSpace = true
					found = true
				}
			} else if singleQuoted {
				if ch == '\'' {
					singleQuoted = false
					needSpace = true
					found = true
				}
			} else if isSpace(ch) || ch == ';' || ch == '{' {
				lastIsSpace = true
				found = true
			}
			if found {
				word := buf.Bytes()
				unescapedWordBuf := bytes.Buffer{}
				for i := 0; i < len(word); i++ {
					c := word[i]
					if c == '\\' {
						switch word[i+1] {
						case '"', '\'', '\\':
							unescapedWordBuf.WriteByte(word[i+1])
							i++
							continue
						case 't':
							unescapedWordBuf.WriteByte('\t')
							i++
							continue
						case 'r':
							unescapedWordBuf.WriteByte('\r')
							i++
							continue
						case 'n':
							unescapedWordBuf.WriteByte('\n')
							i++
							continue
						}
					}
					unescapedWordBuf.WriteByte(c)
				}
				p.args = append(p.args, string(unescapedWordBuf.Bytes()))
				buf.Reset()

				if ch == ';' {
					return newToken(Ok, "")
				}
				if ch == '{' {
					return newToken(BlockStart, "{")
				}
				found = false
			} else {
				buf.WriteByte(ch)
			}
		}
	}
}

func (p *Parser) Parse() ([]*Directive, error) {
	return p.parseFromStat(0)
}

func (p *Parser) parseFromStat(depth int) ([]*Directive, error) {
	directives := make([]*Directive, 0, 10)
	directiveParsing := new(Directive)
	for {
		token := p.readToken()
		if token.kind == Error {
			return nil, errors.New(token.text)
		}
		if token.kind == BlockDone {
			if depth == 0 {
				return nil, errors.New("unexpected '}'")
			}
			return directives, nil
		}
		if token.kind == FileDone {
			if depth != 0 {
				return nil, errors.New("unexpected end of file, expecting '}', depth " + strconv.Itoa(depth))
			}
			return directives, nil
		}
		if token.kind == BlockStart {
			// we should reset args
			directiveParsing.name = p.args[0]
			if len(p.args) > 1 {
				directiveParsing.args = p.args[1:]
			}
			p.args = nil
			ds, err := p.parseFromStat(depth + 1)
			if err != nil {
				return nil, err
			}
			directiveParsing.directives = ds
			directives = append(directives, directiveParsing)
			directiveParsing = new(Directive)
		}

		if token.kind == Ok {
			// ok
			directiveParsing.name = p.args[0]
			if len(p.args) > 1 {
				directiveParsing.args = p.args[1:]
			}
			p.args = nil
			directives = append(directives, directiveParsing)
			directiveParsing = new(Directive)
		}
	}
}

func visitDirectives(context string, proxyLocations []*http.ProxyLocation, ds []*Directive) []*http.ProxyLocation {
	for _, d := range ds {
		if d.name == "location" {
			proxyLocations = visitLocation(context, proxyLocations, d)
			continue
		}
		if d.directives != nil {
			proxyLocations = visitDirectives(d.name, proxyLocations, d.directives)
		}
	}
	return proxyLocations
}

func visitLocation(context string, proxyLocations []*http.ProxyLocation, d *Directive) []*http.ProxyLocation {
	pl := new(http.ProxyLocation)
	argc := len(d.args)
	if argc == 0 {
		panic(fmt.Sprintf("illegal argument count for location %v", d))
	}
	if argc == 2 {
		arg0 := d.args[0]
		arg1 := d.args[1]
		switch arg0 {
		case "=":
			pl.Type = "exact"
			pl.Match = arg1
		case "~":
			pl.Type = "regexp"
			pl.Match = arg1
		case "~*":
			pl.Type = "iregexp"
			pl.Match = arg1
		default:
			panic("unsupported match type " + arg0)
		}
	} else {
		// only one parameter
		arg0 := d.args[0]
		if arg0[0] == '=' {
			pl.Type = "exact"
			pl.Match = arg0[1:]
		} else if arg0[0] == '^' && arg0[1] == '~' {
			panic("unsupported match type ^~")
		} else if arg0[0] == '~' {
			if arg0[1] == '*' {
				pl.Type = "iregexp"
				pl.Match = arg0[2:]
			} else {
				pl.Type = "regexp"
				pl.Match = arg0[1:]
			}
		} else {
			pl.Type = "start"
			pl.Match = arg0
		}
	}
	for _, subDirective := range d.directives {
		if subDirective.name == "proxy_pass" {
			const httpPrefixLen = len("http://")
			pl.Upstream = subDirective.args[0][httpPrefixLen:]
		}
		if subDirective.name == "if" {
			parenthesesRemovedArgs := make([]string, 0, len(subDirective.args))
			for i, arg := range subDirective.args {
				// first one remove ahead '('
				argToBeAppend := arg
				if i == 0 {
					if len(arg) < 1 || arg[0] != '(' {
						panic("invalid condition " + strings.Join(subDirective.args, " "))
					}
					if arg != "(" {
						argToBeAppend = arg[1:]
					} else {
						continue
					}
				} else if i == len(subDirective.args)-1 {
					// last one remove end ')'
					if arg != ")" && strings.HasSuffix(arg, ")") {
						argToBeAppend = arg[:len(arg)-1]
					} else {
						continue
					}
				}
				parenthesesRemovedArgs = append(parenthesesRemovedArgs, argToBeAppend)
			}
			if len(parenthesesRemovedArgs) != 3 {
				fmt.Fprintf(os.Stderr, "invalid condition "+strings.Join(subDirective.args, " "))
				continue
			}
			// $request_uri !~ /2/.*
			if parenthesesRemovedArgs[0] != "$request_uri" {
				fmt.Fprintf(os.Stderr, "unsupported condition "+strings.Join(subDirective.args, " "))
				continue
			}
			conditionType := ""
			switch parenthesesRemovedArgs[1] {
			case "~":
				conditionType = "regexp"
			case "~*":
				conditionType = "iregexp"
			case "!~":
				conditionType = "!regexp"
			case "!~*":
				conditionType = "!iregexp"
			default:
				panic("unsupported write condition type")
			}
			conditionRegexp := parenthesesRemovedArgs[2]
			matchStr := ""
			replacement := ""
			for _, ifSubDirective := range subDirective.directives {
				if ifSubDirective.name == "rewrite" {
					matchStr = ifSubDirective.args[0]
					replacement = ifSubDirective.args[1]
				}
			}
			// has rewrite
			if matchStr != "" {
				pl.RewriteRules = append(pl.RewriteRules, conditionType+" "+conditionRegexp+" "+matchStr+" "+replacement)
			}
		}
		if subDirective.name == "rewrite" {
			pl.RewriteRules = append(pl.RewriteRules, "start / "+subDirective.args[0]+" "+subDirective.args[1])
		}
	}
	// has no upstream ignore it
	if pl.Upstream == "" {
		fmt.Fprintf(os.Stderr, "ignore location "+printDirectives([]*Directive{d}, 0))
		return proxyLocations
	}
	return append(proxyLocations, pl)
}

func printDirectives(ds []*Directive, depth int) string {
	prefix := ""
	for i := 0; i < depth; i++ {
		prefix += "  "
	}
	resultStr := ""
	for _, d := range ds {
		resultStr += fmt.Sprintf("%s%s %s\n", prefix, d.name, strings.Join(d.args, " "))
		if d.directives != nil {
			resultStr += printDirectives(d.directives, depth+1)
		}
	}
	return resultStr
}

func main() {
	if !flag.Parsed() {
		flag.Parse()
	}
	if confFile == "" {
		fmt.Println("use -f to specify the configuration file to parse")
		os.Exit(1)
	}
	if domain == "" {
		fmt.Println("use -d to specify the domain for configuration file")
		os.Exit(1)
	}
	f, err := os.Open(confFile)
	if err != nil {
		panic(err.Error())
	}
	content, err := ioutil.ReadAll(f)
	if err != nil {
		panic(err.Error())
	}
	parser := NewParser(bytes.NewReader(content))
	f.Close()
	directives, err := parser.Parse()
	if err != nil {
		panic(err.Error())
	}
	locations := visitDirectives("global", nil, directives)
	yamlMap := make(map[string]interface{})
	yamlMap["http-locations"] = map[string]interface{}{domain: locations}
	out, _ := yaml.Marshal(yamlMap)
	fmt.Println(string(out))
}
