package service

import "testing"

func TestCheckAdoptionVHostConflicts(t *testing.T) {
	for _, dump := range []string{
		"port 443 namevhost example.test (/etc/apache2/sites-enabled/legacy.conf:1)",
		"alias example.test",
		"wild alias *.test",
		"default server example.test (/etc/apache2/sites-enabled/legacy.conf:1)",
		"*:80 example.test (/etc/apache2/sites-enabled/legacy.conf:1)",
	} {
		if err := checkAdoptionVHostConflicts(dump, []string{"example.test"}); err == nil {
			t.Errorf("missed conflict in %q", dump)
		}
	}
	if err := checkAdoptionVHostConflicts("port 80 namevhost other.test (/etc/apache2/sites-enabled/example.test.conf:1)", []string{"example.test"}); err != nil {
		t.Fatal(err)
	}
	if err := checkAdoptionVHostConflicts("alias [invalid", []string{"example.test"}); err == nil {
		t.Fatal("malformed Apache pattern was accepted")
	}
}
