package main

func Tokens(s string) ([]string, error) {
	var toks []string
	for _, r := range s {
		toks = append(toks, string(r))
	}
	return toks, nil
}
