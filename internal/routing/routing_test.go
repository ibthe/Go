package routing

import "testing"

func TestDecideDomainSuffix(t *testing.T) {
	rules := []Rule{{Domain: ".example.com", Action: Proxy}}
	for _, host := range []string{"example.com", "www.example.com", "WWW.EXAMPLE.COM."} {
		got, err := Decide(host, rules)
		if err != nil { t.Fatalf("Decide(%q): %v", host, err) }
		if got.Action != Proxy { t.Errorf("Decide(%q) = %q, want proxy", host, got.Action) }
	}
}

func TestDoesNotMatchLookalikeDomain(t *testing.T) {
	got, err := Decide("notexample.com", []Rule{{Domain: "example.com", Action: Proxy}})
	if err != nil { t.Fatal(err) }
	if got.Action != Direct { t.Fatalf("got %q, want direct", got.Action) }
}

func TestFirstMatchingRuleWins(t *testing.T) {
	rules := []Rule{
		{Domain: "private.example", Action: Block},
		{Domain: "example", Action: Proxy},
	}
	got, err := Decide("private.example", rules)
	if err != nil { t.Fatal(err) }
	if got.Action != Block { t.Fatalf("got %q, want block", got.Action) }
}

func TestInvalidAction(t *testing.T) {
	_, err := Decide("example.com", []Rule{{Domain: "example.com", Action: "mystery"}})
	if err == nil { t.Fatal("expected invalid action error") }
}
