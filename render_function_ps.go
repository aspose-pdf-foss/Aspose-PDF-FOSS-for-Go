// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"strconv"
)

// fnPostScript is a Type 4 (PostScript calculator) function (ISO 32000-1 §7.10.5)
// — a small stack-based program mapping m inputs to n outputs. These are the
// common tint transforms for Separation/DeviceN colour and some shadings.
type fnPostScript struct {
	prog     []psItem
	domain   []float64 // [min0 max0 …] input clamps
	rangeArr []float64 // [min0 max0 …] output clamps; count/2 = nout
}

// psItem is one program element: a number (float64), an operator (string), or a
// nested procedure ([]psItem, consumed by if/ifelse).
type psItem interface{}

func (f *fnPostScript) eval(in []float64) []float64 {
	st := make([]float64, 0, 32)
	for i, v := range in {
		if 2*i+1 < len(f.domain) {
			v = clampf(v, f.domain[2*i], f.domain[2*i+1])
		}
		st = append(st, v)
	}
	st = execPS(f.prog, st, 0)

	nout := len(f.rangeArr) / 2
	out := make([]float64, nout)
	base := len(st) - nout // outputs are the top n stack values
	for j := 0; j < nout; j++ {
		if k := base + j; k >= 0 && k < len(st) {
			out[j] = clampf(st[k], f.rangeArr[2*j], f.rangeArr[2*j+1])
		}
	}
	return out
}

// execPS runs a program against the stack and returns the new stack.
func execPS(prog []psItem, st []float64, depth int) []float64 {
	if depth > 64 {
		return st
	}
	pop := func() float64 {
		if len(st) == 0 {
			return 0
		}
		v := st[len(st)-1]
		st = st[:len(st)-1]
		return v
	}
	for i := 0; i < len(prog); i++ {
		switch v := prog[i].(type) {
		case float64:
			st = append(st, v)
		case []psItem:
			// A procedure: consumed by a following if / ifelse.
			if i+1 < len(prog) {
				if op, _ := prog[i+1].(string); op == "if" {
					if pop() != 0 {
						st = execPS(v, st, depth+1)
					}
					i++
					continue
				}
			}
			if i+2 < len(prog) {
				if p2, ok := prog[i+1].([]psItem); ok {
					if op, _ := prog[i+2].(string); op == "ifelse" {
						if pop() != 0 {
							st = execPS(v, st, depth+1)
						} else {
							st = execPS(p2, st, depth+1)
						}
						i += 2
						continue
					}
				}
			}
			// stray procedure with no consumer — ignore
		case string:
			st = applyPSOp(v, st)
		}
	}
	return st
}

// applyPSOp applies one operator to the stack and returns it.
func applyPSOp(op string, st []float64) []float64 {
	push := func(v float64) { st = append(st, v) }
	pop := func() float64 {
		if len(st) == 0 {
			return 0
		}
		v := st[len(st)-1]
		st = st[:len(st)-1]
		return v
	}
	b2f := func(b bool) float64 {
		if b {
			return 1
		}
		return 0
	}
	switch op {
	case "add":
		x := pop()
		push(pop() + x)
	case "sub":
		x := pop()
		push(pop() - x)
	case "mul":
		x := pop()
		push(pop() * x)
	case "div":
		x := pop()
		if x != 0 {
			push(pop() / x)
		} else {
			pop()
			push(0)
		}
	case "idiv":
		x := pop()
		y := pop()
		if int(x) != 0 {
			push(float64(int(y) / int(x)))
		} else {
			push(0)
		}
	case "mod":
		x := pop()
		y := pop()
		if int(x) != 0 {
			push(float64(int(y) % int(x)))
		} else {
			push(0)
		}
	case "neg":
		push(-pop())
	case "abs":
		push(math.Abs(pop()))
	case "sqrt":
		push(math.Sqrt(math.Max(0, pop())))
	case "sin":
		push(math.Sin(pop() * math.Pi / 180))
	case "cos":
		push(math.Cos(pop() * math.Pi / 180))
	case "atan":
		den := pop()
		num := pop()
		a := math.Atan2(num, den) * 180 / math.Pi
		if a < 0 {
			a += 360
		}
		push(a)
	case "exp":
		e := pop()
		push(math.Pow(pop(), e))
	case "ln":
		push(math.Log(math.Max(1e-12, pop())))
	case "log":
		push(math.Log10(math.Max(1e-12, pop())))
	case "cvi", "truncate":
		push(math.Trunc(pop()))
	case "cvr":
		// no-op (already real)
	case "floor":
		push(math.Floor(pop()))
	case "ceiling":
		push(math.Ceil(pop()))
	case "round":
		push(math.Round(pop()))
	case "dup":
		v := pop()
		push(v)
		push(v)
	case "pop":
		pop()
	case "exch":
		a := pop()
		b := pop()
		push(a)
		push(b)
	case "copy":
		n := int(pop())
		if n > 0 && n <= len(st) {
			st = append(st, st[len(st)-n:]...)
		}
	case "index":
		n := int(pop())
		if n >= 0 && n < len(st) {
			push(st[len(st)-1-n])
		} else {
			push(0)
		}
	case "roll":
		j := int(pop())
		n := int(pop())
		if n > 0 && n <= len(st) {
			s := st[len(st)-n:]
			j = ((j % n) + n) % n
			rolled := append(append([]float64(nil), s[n-j:]...), s[:n-j]...)
			copy(s, rolled)
		}
	case "eq":
		push(b2f(pop() == pop()))
	case "ne":
		push(b2f(pop() != pop()))
	case "gt":
		x := pop()
		push(b2f(pop() > x))
	case "ge":
		x := pop()
		push(b2f(pop() >= x))
	case "lt":
		x := pop()
		push(b2f(pop() < x))
	case "le":
		x := pop()
		push(b2f(pop() <= x))
	case "and":
		x := int(pop())
		push(float64(int(pop()) & x))
	case "or":
		x := int(pop())
		push(float64(int(pop()) | x))
	case "xor":
		x := int(pop())
		push(float64(int(pop()) ^ x))
	case "not":
		push(b2f(pop() == 0))
	case "true":
		push(1)
	case "false":
		push(0)
	case "bitshift":
		s := int(pop())
		v := int(pop())
		if s >= 0 {
			push(float64(v << uint(s)))
		} else {
			push(float64(v >> uint(-s)))
		}
	}
	return st
}

// parsePSProgram tokenizes and parses a Type 4 program (the body inside the
// outer { }). Returns nil if no program is found.
func parsePSProgram(data []byte) []psItem {
	toks := tokenizePS(data)
	i := 0
	for i < len(toks) && toks[i] != "{" {
		i++
	}
	if i >= len(toks) {
		return nil
	}
	prog, _ := parsePSBlock(toks, i+1)
	return prog
}

func parsePSBlock(toks []string, pos int) ([]psItem, int) {
	var out []psItem
	for pos < len(toks) {
		t := toks[pos]
		switch {
		case t == "}":
			return out, pos + 1
		case t == "{":
			sub, np := parsePSBlock(toks, pos+1)
			out = append(out, sub)
			pos = np
		default:
			if v, err := strconv.ParseFloat(t, 64); err == nil {
				out = append(out, v)
			} else {
				out = append(out, t)
			}
			pos++
		}
	}
	return out, pos
}

func tokenizePS(data []byte) []string {
	var toks []string
	i := 0
	for i < len(data) {
		c := data[i]
		switch {
		case c == '{' || c == '}':
			toks = append(toks, string(c))
			i++
		case c == '%': // comment to end of line
			for i < len(data) && data[i] != '\n' && data[i] != '\r' {
				i++
			}
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == 0:
			i++
		default:
			start := i
			for i < len(data) {
				d := data[i]
				if d == '{' || d == '}' || d == '%' || d == ' ' || d == '\t' || d == '\n' || d == '\r' || d == '\f' || d == 0 {
					break
				}
				i++
			}
			toks = append(toks, string(data[start:i]))
		}
	}
	return toks
}
