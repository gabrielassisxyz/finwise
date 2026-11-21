package contract

import "testing"

func byQualified(t *testing.T) map[string]Method {
	t.Helper()
	methods, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	index := make(map[string]Method, len(methods))
	for _, method := range methods {
		index[method.Qualified()] = method
	}
	return index
}

func TestLoadReadsEveryMethodOfTheManifest(t *testing.T) {
	methods, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	counts := map[Class]int{}
	for _, method := range methods {
		counts[method.Class]++
	}
	want := map[Class]int{ClassSession: 3, ClassRead: 23, ClassEvaluate: 1, ClassWrite: 55}
	for class, n := range want {
		if counts[class] != n {
			t.Errorf("class %s: %d methods, want %d", class, counts[class], n)
		}
	}
}

func TestSessionMethodsAreClassifiedAsTheServerExpects(t *testing.T) {
	index := byQualified(t)
	want := map[string]Class{
		"session.status":    ClassSession,
		"session.unlock":    ClassSession,
		"session.lock":      ClassSession,
		"session.changePin": ClassWrite,
	}
	for name, class := range want {
		if index[name].Class != class {
			t.Errorf("%s: class %q, want %q", name, index[name].Class, class)
		}
	}
}

func TestArgumentBoundsFollowTheSchema(t *testing.T) {
	index := byQualified(t)
	cases := map[string][2]int{
		"session.status":    {0, 0},
		"session.unlock":    {1, 1},
		"session.changePin": {3, 3},
		"budget.view":       {1, 2},
	}
	for name, bounds := range cases {
		got := [2]int{index[name].MinArguments, index[name].MaxArguments}
		if got != bounds {
			t.Errorf("%s: bounds %v, want %v", name, got, bounds)
		}
	}
}

func TestParseRefusesWhatItDoesNotUnderstand(t *testing.T) {
	entry := func(route, class string) string {
		return `{"methods":[{"domain":"a","name":"b","route":"` + route + `","class":"` + class +
			`","arguments":{"type":"array","maxItems":0}}]}`
	}
	one := `{"domain":"a","name":"b","route":"/api/a/b","class":"read","arguments":{"maxItems":0}}`
	cases := map[string]string{
		"unknown class": entry("/api/a/b", "delete"),
		"wrong route":   entry("/api/a/c", "read"),
		"no methods":    `{"methods":[]}`,
		"not json":      `{`,
		"duplicate":     `{"methods":[` + one + `,` + one + `]}`,
	}
	for name, raw := range cases {
		if _, err := parse([]byte(raw)); err == nil {
			t.Errorf("%s: parse accepted it", name)
		}
	}
}
