package http

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

const (
	scriptVarRequestURI = "request_uri"
)

const pcRet = -1

type scriptContext struct {
	variables map[string]string
	boolFlag  bool
}

func scriptGetVarName(s string) (string, bool) {
	if len(s) > 1 && s[0] == '$' {
		return s[1:], true
	}
	return "", false
}

func newScriptContext() *scriptContext {
	return &scriptContext{variables: make(map[string]string, 16)}
}

func (c *scriptContext) get(name string) string {
	return c.variables[name]
}

func (c *scriptContext) set(name, value string) {
	c.variables[name] = value
}

type instruction interface {
	// if next instruction address is -1, it present ret
	exec(pc int, ctx *scriptContext) (int, error)
}

type compiledCode struct {
	codes []instruction
}

func (c *compiledCode) exec(ctx *scriptContext) error {
	for pc := 0; pc != pcRet; {
		nextPC, err := c.codes[pc].exec(pc, ctx)
		if err != nil {
			return err
		}
		pc = nextPC
	}
	return nil
}

// set $a 1
// set $b 2
// rmatch $http_host "^(.*)"
// smatch vname matchstring
// equal vname matchstring
// jt L1 # jt: true跳转  jf: false 跳转
// ret
// L1:
// rewrite /test1/(.*) /backend/test1/$1
// ret

type _set struct {
	p1 string
	p2 string
}

func (i *_set) exec(pc int, ctx *scriptContext) (int, error) {
	v := i.p2
	if strings.HasSuffix(i.p2, "$") {
		v = ctx.get(i.p2[1:])
	}
	ctx.set(i.p1, v)
	return pc + 1, nil
}

type _rmatch struct {
	p1      string
	p2      string
	pattern *regexp.Regexp
}

func (i *_rmatch) exec(pc int, ctx *scriptContext) (int, error) {
	ctx.boolFlag = i.pattern.MatchString(ctx.get(i.p1))
	return pc + 1, nil
}

type _smatch struct {
	p1 string
	p2 string
}

func (i *_smatch) exec(pc int, ctx *scriptContext) (int, error) {
	ctx.boolFlag = strings.HasPrefix(ctx.get(i.p1), i.p2)
	return pc + 1, nil
}

type _equal struct {
	p1 string
	p2 string
}

func (i *_equal) exec(pc int, ctx *scriptContext) (int, error) {
	match := i.p2
	if strings.HasSuffix(i.p2, "$") {
		match = ctx.get(i.p2[1:])
	}
	ctx.boolFlag = ctx.get(i.p1) == match
	return pc + 1, nil
}

type _jt struct {
	label string
	pc    int
}

func (i *_jt) exec(pc int, ctx *scriptContext) (int, error) {
	if ctx.boolFlag {
		return i.pc, nil
	}
	return pc + 1, nil
}

type _jf struct {
	label string
	pc    int
}

func (i *_jf) exec(pc int, ctx *scriptContext) (int, error) {
	if ctx.boolFlag {
		return pc + 1, nil
	}
	return i.pc, nil
}

type _rewrite struct {
	p1      string
	p2      string
	pattern *regexp.Regexp
}

func (i *_rewrite) exec(pc int, ctx *scriptContext) (int, error) {
	uri := ctx.get(scriptVarRequestURI)
	expand := i.pattern.ExpandString(nil, i.p2, uri, i.pattern.FindStringSubmatchIndex(uri))
	ctx.set(scriptVarRequestURI, string(expand))
	return pc + 1, nil
}

type _ret struct {
}

func (i *_ret) exec(pc int, ctx *scriptContext) (int, error) {
	return pcRet, nil
}

func scriptCompile(script string) (*compiledCode, error) {
	r := bufio.NewReader(strings.NewReader(script))
	lines := 0
	codeIndex := 0
	labelIndex := make(map[string]int)
	codes := make([]instruction, 0, 8)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if io.EOF == err {
				break
			}
			return nil, err
		}
		lines++
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		insAndParam := PatternSplit(line, WhitespaceSplitPattern)
		if len(insAndParam) == 0 {
			return nil, fmt.Errorf("illegal instruction [%s] at line %d", line, lines)
		}

		insCode := insAndParam[0]
		switch insCode {
		case "set":
			varName, ok := scriptGetVarName(insAndParam[1])
			if !ok {
				return nil, fmt.Errorf("illegal set instruction [%s] at line %d: first parameter must be a variable", line, lines)
			}
			codes = append(codes, &_set{p1: varName, p2: insAndParam[2]})
		case "smatch":
			varName, ok := scriptGetVarName(insAndParam[1])
			if !ok {
				return nil, fmt.Errorf("illegal smatch instruction [%s] at line %d: first parameter must be a variable", line, lines)
			}
			codes = append(codes, &_smatch{p1: varName, p2: insAndParam[2]})
		case "rmatch":
			varName, ok := scriptGetVarName(insAndParam[1])
			if !ok {
				return nil, fmt.Errorf("illegal rmatch instruction [%s] at line %d: first parameter must be a variable", line, lines)
			}
			pattern, err := regexp.Compile(insAndParam[2])
			if err != nil {
				return nil, fmt.Errorf("compile regex failed for instruction [%s] at line %d: %s", line, lines, err.Error())
			}
			codes = append(codes, &_rmatch{p1: varName, p2: insAndParam[2], pattern: pattern})
		case "irmatch":
			varName, ok := scriptGetVarName(insAndParam[1])
			if !ok {
				return nil, fmt.Errorf("illegal irmatch instruction [%s] at line %d: first parameter must be a variable", line, lines)
			}
			pattern, err := regexp.Compile("(?i)" + insAndParam[2])
			if err != nil {
				return nil, fmt.Errorf("compile case insensitive regex failed for instruction [%s] at line %d: %s", line, lines, err.Error())
			}
			codes = append(codes, &_rmatch{p1: varName, pattern: pattern})
		case "equal":
			varName, ok := scriptGetVarName(insAndParam[1])
			if !ok {
				return nil, fmt.Errorf("illegal equal instruction [%s] at line %d: first parameter must be a variable", line, lines)
			}
			codes = append(codes, &_equal{p1: varName, p2: insAndParam[2]})
		case "jt":
			codes = append(codes, &_jt{label: insAndParam[1]})
		case "jf":
			codes = append(codes, &_jf{label: insAndParam[1]})
		case "ret":
			codes = append(codes, &_ret{})
		case "rewrite":
			pattern, err := regexp.Compile(insAndParam[1])
			if err != nil {
				return nil, fmt.Errorf("compile regex failed for instruction [%s] at line %d: %s", line, lines, err.Error())
			}
			codes = append(codes, &_rewrite{p1: insAndParam[1], p2: insAndParam[2], pattern: pattern})
		default:
			// label do not include in the instruction set
			if len(insAndParam) == 1 && strings.HasSuffix(line, ":") {
				labelIndex[line[:len(line)-1]] = codeIndex
				continue
			}
			return nil, errors.New("illegal instruction [" + line + "] at line " + strconv.Itoa(lines))
		}
		codeIndex++
	}
	for _, ins := range codes {
		if jt, ok := ins.(*_jt); ok {
			pc, ok := labelIndex[jt.label]
			if !ok {
				return nil, errors.New("jump label [" + jt.label + "] not found")
			}
			jt.pc = pc
		}
		if jf, ok := ins.(*_jf); ok {
			pc, ok := labelIndex[jf.label]
			if !ok {
				return nil, errors.New("jump label [" + jf.label + "] not found")
			}
			jf.pc = pc
		}
	}
	return &compiledCode{codes: codes}, nil
}
