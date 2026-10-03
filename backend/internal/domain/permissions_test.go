package domain

import "testing"

func TestPermissionPrerequisites(t *testing.T) {
	for bits := 0; bits < 8; bits++ {
		p := Permission{SectionID: "clients", Read: bits&4 != 0, Write: bits&2 != 0, Delete: bits&1 != 0}
		valid := bits == 0 || bits == 4 || bits == 6 || bits == 7
		if err := ValidatePermissions([]Permission{p}); (err == nil) != valid {
			t.Errorf("unexpected validation for %+v: %v", p, err)
		}
	}
	if err := ValidatePermissions([]Permission{{SectionID: "clients", Read: true}, {SectionID: "clients"}}); err == nil {
		t.Fatal("conflicting duplicate grants accepted")
	}
}
