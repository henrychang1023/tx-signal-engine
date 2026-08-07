package engine

import (
	"fmt"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// Rule is a compiled user expression that evaluates to true/false against an Env.
type Rule struct {
	program *vm.Program
	source  string
}

// Compile parses and type-checks expression against Env's shape, catching
// typos like "TX.ask1" (should be "TX.a1") or a non-boolean result at
// startup instead of on the first evaluation.
func Compile(expression string) (*Rule, error) {
	program, err := expr.Compile(expression, expr.Env(Env{}), expr.AsBool())
	if err != nil {
		return nil, fmt.Errorf("compile expression %q: %w", expression, err)
	}
	return &Rule{program: program, source: expression}, nil
}

func (r *Rule) Eval(env Env) (bool, error) {
	out, err := expr.Run(r.program, env)
	if err != nil {
		return false, fmt.Errorf("eval expression %q: %w", r.source, err)
	}
	result, ok := out.(bool)
	if !ok {
		return false, fmt.Errorf("expression %q returned %T, want bool", r.source, out)
	}
	return result, nil
}
