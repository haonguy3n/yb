package config

import (
	"reflect"
	"testing"
)

func TestLoadMergeIncludes(t *testing.T) {
	c, err := Load("../../testdata/top.yml")
	if err != nil {
		t.Fatal(err)
	}

	if c.Machine != "top-machine" {
		t.Errorf("machine: got %q, want top-machine (override)", c.Machine)
	}
	if c.Distro != "base-distro" {
		t.Errorf("distro: got %q, want base-distro (inherited)", c.Distro)
	}
	if want := []string{"image-a", "image-b"}; !reflect.DeepEqual(c.Targets, want) {
		t.Errorf("targets: got %v, want %v (list replaces scalar)", c.Targets, want)
	}
	if ref := c.Repos["poky"].Ref(); ref != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Errorf("poky ref (commit): got %q", ref)
	}
	if ref := c.Repos["meta-freescale"].Ref(); ref != "master" {
		t.Errorf("meta-freescale ref (branch): got %q", ref)
	}

	// iritech layers must UNION across include + override, and drop the excluded one.
	iri := c.Repos["iritech"]
	for _, want := range []string{"meta-test", "meta-main"} {
		if !iri.Layers[want] {
			t.Errorf("iritech layer %q should be enabled (union)", want)
		}
	}
	if iri.Layers["meta-disabled"] {
		t.Errorf("iritech layer meta-disabled should be excluded")
	}
	if iri.Dir() != "." {
		t.Errorf("url-less repo dir: got %q, want .", iri.Dir())
	}
}

func TestLayerResolution(t *testing.T) {
	c, err := Load("../../testdata/top.yml")
	if err != nil {
		t.Fatal(err)
	}
	got := c.Layers()
	want := []string{
		"meta-main", "meta-test", // iritech (dir "."), sorted
		"repos/meta-freescale",           // no layers: -> repo root
		"repos/poky/meta", "repos/poky/meta-poky",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("layers:\n got  %v\n want %v", got, want)
	}

	if dir, err := c.PokyDir(); err != nil || dir != "repos/poky" {
		t.Errorf("PokyDir: got %q, %v", dir, err)
	}
}
