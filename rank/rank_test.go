package rank

import "testing"

func TestCanonOneSpelling(t *testing.T) {
	for in, want := range map[string]string{
		"auditor": "auditor", "Auditor": "auditor", " Auditor ": "auditor",
		"principal_domain_owner": "principal-domain-owner", "PrincipalDomainOwner": "principal-domain-owner", "principaldomainowner": "principal-domain-owner",
		"principalcoordinator": "principal-coordinator", "Zone": "zone", "orchestrator": "orchestrator",
		"night_watch": "night-watch", "NightWatch": "night-watch", "ZONE": "zone", "": "",
	} {
		if got := Canon(in); got != want {
			t.Errorf("Canon(%q) = %q, want %q", in, got, want)
		}
	}
	al := Aliases("principal_coordinator")
	if len(al) != 3 || al[0] != "principal-coordinator" || al[1] != "principal_coordinator" || al[2] != "principalcoordinator" {
		t.Fatalf("aliases: %v", al)
	}
	if !Equal("PrincipalCoordinator", "principal_coordinator") || Equal("coord", "principal-coordinator") {
		t.Fatal("Equal")
	}
}

func TestFamilyFromFlags(t *testing.T) {
	for want, f := range map[string]Flags{
		FamilyRoot:    {Root: true, Commands: true},
		FamilyAuthor:  {Authors: true},
		FamilyGate:    {HoldsGate: true, Commands: true},
		FamilyPlanner: {Commands: true, ReportsToRoot: true},
		FamilySteward: {Authors: true, HoldsGate: true, ReportsToRoot: true},
		FamilyAuditor: {ReadOnly: true, ReportsToRoot: true},
		FamilyGeneric: {},
	} {
		if got := FamilyOf(f); got != want {
			t.Errorf("%+v → %s, want %s", f, got, want)
		}
	}
	if !ReadOnly([]string{"read", "glob", "grep"}) || ReadOnly([]string{"read", "bash"}) || ReadOnly([]string{"*"}) || ReadOnly(nil) || ReadOnly([]string{"wr*"}) {
		t.Fatal("ReadOnly")
	}
}
