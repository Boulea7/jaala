package ns

import "testing"

// electrical is a small table: base A, V, m, s; ohm and Ω over them; W; the prefixes k and m.
func electrical(t *testing.T) *Vocabulary {
	t.Helper()
	v := MustVocabulary(nil)
	for _, u := range [][2]string{{"k", "1e3"}, {"m", "1e-3"}, {"A", ""}, {"V", ""}, {"m", ""}, {"s", ""}, {"ohm", "V/A"}, {"Ω", "V/A"}, {"W", "V*A"}} {
		if err := v.AddUnit(u[0], u[1]); err != nil {
			t.Fatal(err)
		}
	}
	return v
}

func TestUnitsNormalizeToNamedBaseUnits(t *testing.T) {
	v := electrical(t)
	for _, c := range []struct {
		in   string
		f    float64
		want string
	}{
		{"kohm", 1000, "ohm"}, {"mV", 0.001, "V"}, {"A*ohm", 1, "V"}, {"V/A", 1, "ohm"}, {"Ω", 1, "ohm"},
		{"m/s", 1, "m/s"}, {"mm", 0.001, "m"}, {"min", 1, "min"}, {"V*V/ohm", 1, "W"}, {"ohm/s", 1, "ohm/s"},
		{"kV*mA", 1, "W"}, {"m/(s*s)", 1, "m/s^2"}, {"1/s", 1, "1/s"},
	} {
		f, got, err := v.Units().Normalize(1, c.in)
		if err != nil || got != c.want || f != c.f {
			t.Errorf("Normalize(1, %q) = %g %q %v, want %g %q", c.in, f, got, err, c.f, c.want)
		}
	}
	// Every spelling reads back as the unit it names.
	for _, s := range []string{"ohm/s", "m/s^2", "A^2*ohm", "1/s"} {
		if !v.Units().SameUnit(s, s) {
			t.Errorf("%s isn't the same unit as itself", s)
		}
	}
	if !v.Units().SameUnit("ohm", "V/A") || v.Units().SameUnit("ohm", "V") {
		t.Error("SameUnit should read names through the table")
	}
}

func TestAHostNamesItsOwnUnits(t *testing.T) {
	v := electrical(t)
	if err := v.AddUnit("speed", "m/s"); err != nil {
		t.Fatal(err)
	}
	if _, got, _ := v.Units().Normalize(1, "m/s"); got != "speed" {
		t.Errorf("m/s is written %q, want the host's speed", got)
	}
	for _, c := range []struct{ name, def, want string }{
		{"V", "", `unit "V" is defined twice`},
		{"k", "1e6", `prefix "k" is defined twice`},
		{"x2", "", `isn't a name`},
		{"bad", "V^x", `a power is a whole number`},
		{"dozen", "12", `isn't a power of ten`},
	} {
		if err := v.AddUnit(c.name, c.def); err == nil || !contains(err.Error(), c.want) {
			t.Errorf("AddUnit(%q, %q) = %v, want it to say %q", c.name, c.def, err, c.want)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
