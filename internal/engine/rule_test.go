package engine

import (
	"testing"
	"time"
)

func testEnv() Env {
	return Env{
		TX:  Quote{A1: 44284, B1: 44272, Price: 44280, Volume: 55986, Time: time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)},
		MTX: Quote{A1: 44275, B1: 44271, Price: 44274, Volume: 129055, Time: time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)},
		Now: time.Date(2026, 8, 7, 10, 30, 0, 0, time.UTC),
	}
}

func TestCompile_Valid(t *testing.T) {
	if _, err := Compile("TX.a1 > TX.b1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompile_UnknownField(t *testing.T) {
	if _, err := Compile("TX.ask1 > TX.b1"); err == nil {
		t.Fatal("expected a compile error for an unknown field")
	}
}

func TestCompile_NonBoolResult(t *testing.T) {
	if _, err := Compile("TX.price"); err == nil {
		t.Fatal("expected a compile error for a non-bool expression")
	}
}

func TestCompile_SyntaxError(t *testing.T) {
	if _, err := Compile("TX.a1 >"); err == nil {
		t.Fatal("expected a compile error for invalid syntax")
	}
}

func TestEval(t *testing.T) {
	cases := []struct {
		name string
		expr string
		want bool
	}{
		{"a1 above b1 and volume filter", "TX.a1 > TX.b1 && TX.volume > 1000", true},
		{"a1 below b1 is false", "TX.a1 < TX.b1", false},
		{"volume filter fails", "TX.volume > 1000000", false},
		{"cross-symbol comparison", "MTX.price < TX.price", true},
		{"logical or", "TX.volume > 1000000 || MTX.b1 > 0", true},
		{"now field usable", "now.Hour() >= 0", true},
	}
	env := testEnv()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rule, err := Compile(c.expr)
			if err != nil {
				t.Fatalf("Compile(%q): %v", c.expr, err)
			}
			got, err := rule.Eval(env)
			if err != nil {
				t.Fatalf("Eval(%q): %v", c.expr, err)
			}
			if got != c.want {
				t.Errorf("Eval(%q) = %v, want %v", c.expr, got, c.want)
			}
		})
	}
}
