package parsing

import "fmt"

func Plus(s Stream, p Parser) (Stream, []Value) {
	return Repeat(s, 1, 0, p)
}

func Star(s Stream, p Parser) (Stream, []Value) {
	return Repeat(s, 0, 0, p)
}

func Repeat(s Stream, min, max int, p Parser) (Stream, []Value) {
	var vs []Value
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
		vs = append(vs, p)
	}
	return s, vs
}
