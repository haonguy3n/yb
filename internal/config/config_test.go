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

func TestPokyDirOpenEmbeddedCore(t *testing.T) {
	// A poky-less layout: openembedded-core ships oe-init-build-env too, so it
	// should be accepted as the build-env dir (poky bundles its copy).
	c := &Config{Repos: map[string]*Repo{
		"openembedded-core": {Name: "openembedded-core", URL: "u", Path: "repos/oe-core", Branch: "scarthgap", Layers: map[string]bool{"meta": true}},
		"meta-oe":           {Name: "meta-oe", URL: "u", Path: "repos/meta-oe", Branch: "scarthgap"},
	}}
	if dir, err := c.PokyDir(); err != nil || dir != "repos/oe-core" {
		t.Errorf("PokyDir with openembedded-core: got %q, %v", dir, err)
	}
}

func TestYBBlockAndEntry(t *testing.T) {
	c, err := Load("../../testdata/top.yml")
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != "kirkstone" {
		t.Errorf("yb.version: got %q", c.Version)
	}
	if c.DLDir != "/tmp/dl" || c.SSTateDir != "/tmp/ss" {
		t.Errorf("yb.dl/sstate: got %q / %q", c.DLDir, c.SSTateDir)
	}
	if len(c.Mounts) != 1 || c.Mounts[0] != "/keys:ro" {
		t.Errorf("yb.mounts: got %v", c.Mounts)
	}

	entry, err := FindEntry("../../testdata")
	if err != nil || entry != "top.yml" {
		t.Errorf("FindEntry: got %q, %v (want top.yml)", entry, err)
	}
}

func TestLoadFilesOverlay(t *testing.T) {
	// kas-style `top.yml overlay.yml`: overlay wins on scalars, headers union.
	c, err := LoadFiles([]string{"../../testdata/top.yml", "../../testdata/overlay.yml"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Machine != "overlaid-machine" {
		t.Errorf("machine: got %q, want overlaid-machine (overlay wins)", c.Machine)
	}
	if c.Version != "kirkstone" {
		t.Errorf("version: got %q, want kirkstone (from top's yb block)", c.Version)
	}
	if _, ok := c.LocalConfHeader["x"]; !ok {
		t.Error("local_conf_header should keep x from top")
	}
	if _, ok := c.LocalConfHeader["ci"]; !ok {
		t.Error("local_conf_header should gain ci from overlay")
	}
}

func TestBBLayersConfHeader(t *testing.T) {
	c, err := Load("../../testdata/top.yml")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.BBLayersConfHeader["standard"]; !ok {
		t.Fatal("bblayers_conf_header should carry 'standard' from top.yml")
	}
}

func TestDefaultRepoBranch(t *testing.T) {
	c, err := Load("../../testdata/defaults.yml")
	if err != nil {
		t.Fatal(err)
	}
	// Repos without a branch inherit defaults.repos.branch...
	for _, name := range []string{"meta-tegra", "openembedded-core"} {
		if got := c.Repos[name].Ref(); got != "wrynose" {
			t.Errorf("%s ref: got %q, want wrynose (default branch)", name, got)
		}
	}
	// ...and one that pins its own branch keeps it.
	if got := c.Repos["bitbake"].Ref(); got != "2.8" {
		t.Errorf("bitbake ref: got %q, want 2.8 (own branch beats default)", got)
	}
	// A repo whose only layer is disabled contributes no layer; the url-less
	// project repo contributes its listed layers relative to ".".
	want := []string{
		"repos/meta-tegra",             // no layers: -> repo root
		"repos/openembedded-core/meta", // bitbake's only layer is disabled
		"layers/meta-demo-ci", "layers/meta-tegra-support", "layers/meta-tegrademo",
	}
	if got := c.Layers(); !reflect.DeepEqual(got, want) {
		t.Errorf("layers:\n got  %v\n want %v", got, want)
	}
}
