package entities

import "testing"

func TestNormalize(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// Trivial
		{"empty", "", ""},
		{"whitespace_only", "   \t\n  ", ""},
		{"already_clean", "stripe", "stripe"},

		// Lowercase + accents
		{"lower", "EDF", "edf"},
		{"mixed_case", "OvH SaS", "ovh sas"},
		{"accent_e", "société générale", "societe generale"},
		{"accent_e_grave", "crédit agricole", "credit agricole"},
		{"accent_circumflex", "hôtellerie", "hotellerie"},
		{"accent_cedilla", "français", "francais"},

		// Punctuation stripped
		{"abbrev_with_dots_dropped", "S.N.C.F.", ""}, // every single letter falls below the 2-char floor
		{"abbrev_no_dots", "SNCF", "sncf"},
		{"colon_dash", "EDF: facture - merci", "edf merci"},
		{"slash", "AMAZON/MARKETPLACE", "amazon marketplace"},
		{"parens", "OVH (CLOUD)", "ovh cloud"},

		// SEPA / noise tokens
		{"sepa_basic", "VIR SEPA EDF FACT 88412", "edf"},
		{"sepa_full", "PRELVT SEPA URSSAF IDF REF: 884412", "urssaf idf"},
		{"carte_cb", "CB CARTE STARBUCKS 12/03", "starbucks"},
		{"facture_word", "Facture FRANCEPRIX", "franceprix"},

		// Dates stripped
		{"date_slash", "EDF 12/03/2024", "edf"},
		{"date_dash", "ENGIE 01-04-24", "engie"},
		{"date_dot", "STRIPE 30.06.2024", "stripe"},
		{"date_short", "ovh 12/03", "ovh"},
		{"date_iso", "STRIPE 2024-03-15", "stripe"},
		{"date_invalid_strips_only_long_digits", "edf 99/99/9999", "edf"}, // 9999 stripped, residual "99 99" dropped as pure-digit tokens

		// Long digits (invoice refs)
		{"long_digits", "EDF 884412 1234567", "edf"},
		{"short_digits_dropped", "TVA 20", "tva"}, // standalone short numbers carry no entity signal

		// Tokens too short
		{"single_letter", "a EDF b", "edf"},
		{"two_letters_kept", "ovh fr", "ovh fr"},

		// Real-world combos
		{"se_domicilie_v1", "SE DOMICILIE FACTURE 48-99", "se domicilie"},
		{"se_domicilie_v2", "SE DOMICILIE", "se domicilie"},
		{"se_domicilie_v3", "SEDOMICILIE", "sedomicilie"},
		{"stripe_payout", "STRIPE PAYOUT 2024-03-15 ref 88234", "stripe payout"},
		{"urssaf_long", "VIRT SEPA URSSAF ILE DE FRANCE COTISATIONS 03/2024", "urssaf ile de france cotisations"},

		// Unicode edge
		{"emoji_drop", "EDF 🌍 FR", "edf fr"},
		{"mixed_script", "EDF 中国 fr", "edf fr"},
		{"nbsp", "EDF FR", "edf fr"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Normalize(c.in)
			if got != c.want {
				t.Errorf("Normalize(%q):\n  got  %q\n  want %q", c.in, got, c.want)
			}
		})
	}
}

func TestNormalizeIsIdempotent(t *testing.T) {
	inputs := []string{
		"VIR SEPA EDF FACT 88412",
		"société générale",
		"SE DOMICILIE FACTURE 48-99",
		"",
		"OvH SaS",
	}
	for _, in := range inputs {
		first := Normalize(in)
		second := Normalize(first)
		if first != second {
			t.Errorf("Normalize is not idempotent for %q:\n  first  %q\n  second %q", in, first, second)
		}
	}
}

func TestNormalizeIsDeterministic(t *testing.T) {
	// Same input must produce the same output across many calls — guards against
	// accidental introduction of non-deterministic state (map iteration, time).
	in := "VIR SEPA URSSAF IDF COTISATIONS 03/2024 REF 884412"
	want := Normalize(in)
	for i := 0; i < 1000; i++ {
		if got := Normalize(in); got != want {
			t.Fatalf("Normalize drifted on iter %d: got %q want %q", i, got, want)
		}
	}
}
