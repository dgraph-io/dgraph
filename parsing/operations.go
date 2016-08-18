package parsing

import "fmt"

func Plus(s Stream, p Parser) Stream {
	return Repeat(s, 1, 0, p)
}

func Star(s Stream, p Parser) Stream {
	return Repeat(s, 0, 0, p)
}

func Repeat(s Stream, min, max int, p Parser) Stream {
	for i := 0; max != 0 && i < max; i++ {
		s1, err := ParseErr(s, p)
		if err != nil {
			if i < min {
				err.AddContext(SyntaxErrorContext{
					Err:    fmt.Errorf("repetition %d failed", i),
					Stream: s,
				})
				panic(err)
			}
			break
		}
		s = s1
		// vs = append(vs, p)
	}
	return s
}

type errNoMatch struct {
	ps []Parser
}

func (me errNoMatch) Error() string {
	return fmt.Sprintf("couldn't match on of %s", me.ps)
}

func OneOf(ps ...Parser) oneOf {
	return oneOf{ps: ps}
}

type oneOf struct {
	Index int
	ps    []Parser
}

func (me *oneOf) Parse(s Stream) Stream {
	for i, p := range me.ps {
		s1, err := ParseErr(s, p)
		if err == nil {
			me.Index = i
			return s1
		}
	}
	panic(NewSyntaxError(SyntaxErrorContext{Err: errNoMatch{me.ps}}))
}
