package main

import "testing"

func TestHelloWorld(t *testing.T) {
	s := setup(t, `
		name: string @index(term) .
	`, `
		_:pj <name> "Peter Jackson" .
		_:pp <name> "Peter Pan" .
	`)
	defer s.cleanup()

	t.Run("Pan and Jackson", s.strtest(`
	{
		q(func: anyofterms(name, "Peter")) {
			name
		}
	}
	`, `
	{
		"data": {
			"q": [
				{ "name": "Peter Pan" },
				{ "name": "Peter Jackson" }
			]
		}
	}
	`))

	t.Run("Pan only", s.strtest(`
	{
		q(func: anyofterms(name, "Pan")) {
			name
		}
	}
	`, `
	{
		"data": {
			"q": [
				{ "name": "Peter Pan" }
			]
		}
	}
	`))

	t.Run("Jackson only", s.strtest(`
	{
		q(func: anyofterms(name, "Jackson")) {
			name
		}
	}
	`, `
	{
		"data": {
			"q": [
				{ "name": "Peter Jackson" }
			]
		}
	}
	`))
}
