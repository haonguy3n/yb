// Package image builds the Yocto host build container for a given release. The
// release name (e.g. "zeus") selects a Profile — the Ubuntu base and any extra
// packages — which is baked into an embedded Dockerfile via build args. yb builds
// the image on demand; it does not depend on a prebuilt one.
package image

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

//go:embed Dockerfile
var dockerfile string

// Profile is the host environment a Yocto release needs.
type Profile struct {
	Ubuntu string // Ubuntu base image tag
	Extra  string // extra apt packages (space-separated), e.g. python2 for zeus
}

// Profiles maps a Yocto release codename to its host profile. Extend as needed.
var Profiles = map[string]Profile{
	"zeus":       {Ubuntu: "18.04", Extra: "python python-minimal"}, // needs python2
	"dunfell":    {Ubuntu: "20.04"},
	"gatesgarth": {Ubuntu: "20.04"},
	"hardknott":  {Ubuntu: "20.04"},
	"honister":   {Ubuntu: "22.04"},
	"kirkstone":  {Ubuntu: "22.04"},
	"langdale":   {Ubuntu: "22.04"},
	"mickledore": {Ubuntu: "22.04"},
	"nanbield":   {Ubuntu: "22.04"},
	// 24.04 dropped libegl1-mesa and libsdl1.2-dev, which the shared Dockerfile
	// installs; 22.04 is sanity-tested for scarthgap and needs no Dockerfile change.
	"scarthgap": {Ubuntu: "22.04"},
}

// Logf receives progress lines.
type Logf func(format string, args ...any)

// Describe returns the profile for a release, if known.
func Describe(version string) (Profile, bool) {
	p, ok := Profiles[version]
	return p, ok
}

// Versions lists the known release names, sorted.
func Versions() []string {
	out := make([]string, 0, len(Profiles))
	for v := range Profiles {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

const fingerprintLabel = "yb.build"

func fingerprint(p Profile) string {
	h := sha256.New()
	fmt.Fprint(h, dockerfile, "\x00", p.Ubuntu, "\x00", p.Extra, "\x00", os.Getuid(), "\x00", os.Getgid())
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func current(tag, fp string) bool {
	out, err := exec.Command("docker", "image", "inspect",
		"-f", `{{index .Config.Labels "`+fingerprintLabel+`"}}`, tag).Output()
	return err == nil && strings.TrimSpace(string(out)) == fp
}

// Tag is the image name yb builds for a release.
func Tag(version string) string { return "yb-yocto:" + version }

// Exists reports whether the image is already built locally.
func Exists(tag string) bool {
	return exec.Command("docker", "image", "inspect", tag).Run() == nil
}

// Ensure builds the image for version if it is missing (or rebuild is set) and
// returns its tag. The build context is empty — the Dockerfile is fed on stdin.
func Ensure(version string, rebuild bool, log Logf) (string, error) {
	p, ok := Profiles[version]
	if !ok {
		return "", fmt.Errorf("unknown version %q (known: %s)", version, strings.Join(Versions(), ", "))
	}
	tag := Tag(version)
	fp := fingerprint(p)
	if !rebuild && Exists(tag) {
		if current(tag, fp) {
			return tag, nil
		}
		log("image %s is out of date, rebuilding…", tag)
	}
	log("building image %s (ubuntu %s)…", tag, p.Ubuntu)
	cmd := exec.Command("docker", "build",
		"-t", tag,
		"--label", fingerprintLabel+"="+fp,
		"--build-arg", "UBUNTU_VERSION="+p.Ubuntu,
		"--build-arg", "EXTRA_PACKAGES="+p.Extra,
		"--build-arg", "UID="+strconv.Itoa(os.Getuid()),
		"--build-arg", "GID="+strconv.Itoa(os.Getgid()),
		"-",
	)
	cmd.Stdin = strings.NewReader(dockerfile)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker build %s: %w", tag, err)
	}
	return tag, nil
}
