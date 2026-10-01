package datalog

import (
	"fmt"
	"strconv"

	"github.com/panyam/jaala/ns"
)

// coerceConstants checks every constant of a linked program against the type of the position it
// stands in, and rewrites it to that type where the reading is unambiguous (#65). Without it a
// constant of the wrong type matches nothing, and an empty answer to a question asked wrongly reads
// as "the design has none".
//
//   - In an argument typed number, text that parses as a number becomes that number; other text is
//     refused.
//   - In an argument typed string, or naming an entity, a number matches by its text, since names
//     such as pin 1 or net 5 are text that looks numeric.
//   - In a comparison with a variable typed number, the constant side is coerced or refused the
//     same way. A variable typed otherwise leaves the constant alone, so `?name < 5` still has no
//     order (see evalCompare).
//
// A position nothing types keeps its constant as given. A bound value is a constant by the time this
// runs (bindGoal substitutes first), so it is checked like one written in the goal.
func coerceConstants(q Query, reg *ns.Vocabulary) (Query, error) {
	t := newTyper(reg, q.Rules)
	out := q
	out.Rules = make([]Rule, len(q.Rules))
	for i, r := range q.Rules {
		body, err := t.coerceBody(r.Body)
		if err != nil {
			return q, err
		}
		r.Body = body
		out.Rules[i] = r
	}
	goal, err := t.coerceBody(q.Goal)
	if err != nil {
		return q, err
	}
	out.Goal = goal
	return out, nil
}

// coerceBody coerces one body's constants. A rebuilt literal is a copy, so it keeps its at.
func (t *typer) coerceBody(body Body) (Body, error) {
	out := Body{Literals: make([]Literal, len(body.Literals))}
	for i, l := range body.Literals {
		switch {
		case l.Pos != nil:
			a, err := t.coerceAtom(*l.Pos)
			if err != nil {
				return body, err
			}
			l.Pos = &a
		case l.Neg != nil:
			a, err := t.coerceAtom(*l.Neg)
			if err != nil {
				return body, err
			}
			l.Neg = &a
		case l.Compare != nil:
			c, err := t.coerceCompare(*l.Compare, body)
			if err != nil {
				return body, err
			}
			l.Compare = &c
		}
		out.Literals[i] = l
	}
	return out, nil
}

func (t *typer) coerceAtom(a Atom) (Atom, error) {
	labels, types, ok := t.argTypes(a.Relation, nil)
	if !ok {
		return a, nil
	}
	out := Atom{Relation: a.Relation, Args: make([]Term, len(a.Args))}
	copy(out.Args, a.Args)
	for j, arg := range a.Args {
		if arg.Const == nil || j >= len(types) {
			continue
		}
		v, ok := coerceValue(*arg.Const, types[j])
		if !ok {
			return a, fmt.Errorf("query: %s cannot be %q (it holds a number)", argName(a.Relation, labels, j), arg.Const.S)
		}
		out.Args[j] = Term{Const: &v}
	}
	return out, nil
}

// coerceCompare coerces the constant side of a comparison whose other side is a variable typed
// number.
func (t *typer) coerceCompare(c Compare, body Body) (Compare, error) {
	for _, side := range []struct{ v, k *Term }{{&c.Left, &c.Right}, {&c.Right, &c.Left}} {
		if side.v.Var == "" || side.v.Var == "_" || side.k.Const == nil {
			continue
		}
		if t.ofVar(side.v.Var, body, nil).Type != ns.TypeNumber {
			continue
		}
		v, ok := coerceValue(*side.k.Const, ns.ArgType{Type: ns.TypeNumber})
		if !ok {
			return c, fmt.Errorf("query: ?%s cannot be compared with %q: it is a number (%s)", side.v.Var, side.k.Const.S, t.numberSource(side.v.Var, body))
		}
		*side.k = Term{Const: &v}
	}
	return c, nil
}

// numberSource names the argument that types v as a number, for an error message. It looks where
// ofVar does: base relations and predicates before derived relations.
func (t *typer) numberSource(v Var, body Body) string {
	for _, derived := range []bool{false, true} {
		for _, lit := range body.Literals {
			a := lit.Pos
			if a == nil || t.isDerived(a.Relation) != derived {
				continue
			}
			labels, types, ok := t.argTypes(a.Relation, nil)
			if !ok {
				continue
			}
			for j, arg := range a.Args {
				if arg.Var == v && j < len(types) && types[j].Type == ns.TypeNumber {
					return argName(a.Relation, labels, j)
				}
			}
		}
	}
	return "a number argument"
}

// coerceValue converts v to what an argument of type at holds. ok is false for text a number
// argument cannot read. An absent value, and an argument whose type says nothing about scalars, keep
// v as given.
func coerceValue(v ns.Value, at ns.ArgType) (ns.Value, bool) {
	if v.Absent {
		return v, true
	}
	switch {
	case at.Type == ns.TypeNumber:
		if v.Num != nil {
			return v, true
		}
		f, err := strconv.ParseFloat(v.S, 64)
		if err != nil {
			return v, false
		}
		return ns.Value{S: v.S, Num: &f, BaseUnit: v.BaseUnit}, true
	case at.Type == ns.TypeString || at.Kind != "" || at.KindFrom != "":
		return ns.Value{S: v.S}, true
	}
	return v, true
}

// argName names argument j of rel for an error message: by its label, or by position when the
// relation labels none.
func argName(rel string, labels []string, j int) string {
	if j < len(labels) && labels[j] != "" {
		return fmt.Sprintf("%s's %q argument", displayName(rel), labels[j])
	}
	return fmt.Sprintf("%s's argument %d", displayName(rel), j+1)
}
