package parsing

import "fmt"

func Bytes(s Stream, bs string) Stream {
	for _, b := range []byte(bs) {
		if !s.Good() {
			panic(NewSyntaxError(SyntaxErrorContext{
				Stream: s,
				Err:    fmt.Errorf("expected %q but got %s", b, s.Err()),
			}))
		}
		_b := s.Token().(byte)
		if _b != b {
			panic(NewSyntaxError(SyntaxErrorContext{
				Stream: s,
				Err:    fmt.Errorf("expected %q but got %q", b, _b),
			}))
		}
		s = s.Next()
	}
	return s
}
