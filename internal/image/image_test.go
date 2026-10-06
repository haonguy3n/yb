package image

import "testing"

func TestFingerprintFollowsProfile(t *testing.T) {
	a := fingerprint(Profile{Ubuntu: "22.04"})
	if a != fingerprint(Profile{Ubuntu: "22.04"}) {
		t.Fatal("fingerprint is not stable")
	}
	if a == fingerprint(Profile{Ubuntu: "18.04"}) {
		t.Fatal("ubuntu base is not part of the fingerprint")
	}
	if a == fingerprint(Profile{Ubuntu: "22.04", Extra: "python"}) {
		t.Fatal("extra packages are not part of the fingerprint")
	}
}
