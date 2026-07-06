// Command yb is a kas-compatible Yocto build orchestrator: it reads existing kas
// YAML, checks out the pinned repos, generates local.conf/bblayers.conf, and
// runs bitbake inside a release-aligned container. See docs/design/2026-07-06-yb.md.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/haonguy3n/yb/internal/conf"
	"github.com/haonguy3n/yb/internal/config"
	"github.com/haonguy3n/yb/internal/image"
	"github.com/haonguy3n/yb/internal/project"
	"github.com/haonguy3n/yb/internal/repo"
	"github.com/haonguy3n/yb/internal/runner"
)

// version is stamped at release time via -ldflags "-X main.version=<tag>".
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "build":
		err = cmdBuild(os.Args[2:])
	case "shell":
		err = cmdShell(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Println("yb " + version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "yb: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "yb: "+err.Error())
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `yb — kas-compatible Yocto build orchestrator

Usage:
  yb build [targets...]   checkout repos, generate conf, run bitbake in the container
  yb shell                open a bitbake build shell in the container
  yb version

Run it from the project directory. yb builds the file carrying a yb: block (name
kas files as positional *.yml args to override or overlay, kas-style a.yml b.yml);
version/cache/ssh_key/mounts come from that block.

  --force   force git checkout/pull to the pinned commit/branch (build only)
`)
}

// loaded resolves the merged kas config and orchestration. Entry kas files are
// the positional *.yml args, or the auto-detected file carrying a yb: block. The
// project directory is the current directory.
func loaded(entries []string) (*project.Project, *config.Config, error) {
	const dir = "."
	if len(entries) == 0 {
		e, err := config.FindEntry(dir)
		if err != nil {
			return nil, nil, err
		}
		entries = []string{e}
	}
	paths := make([]string, len(entries))
	for i, e := range entries {
		if filepath.IsAbs(e) {
			paths[i] = e
		} else {
			paths[i] = filepath.Join(dir, e)
		}
	}
	c, err := config.LoadFiles(paths)
	if err != nil {
		return nil, nil, err
	}
	p, err := project.New(dir, c)
	if err != nil {
		return nil, nil, err
	}
	return p, c, nil
}

// splitEntry separates positional kas files (existing *.yml/.yaml, optionally
// colon-joined kas-style) from bitbake targets.
func splitEntry(args []string) (entries, targets []string) {
	for _, a := range args {
		if files, ok := asKasFiles(a); ok {
			entries = append(entries, files...)
		} else {
			targets = append(targets, a)
		}
	}
	return entries, targets
}

// asKasFiles reports whether tok names one or more existing kas files (colon-
// joined, kas style), and returns them.
func asKasFiles(tok string) ([]string, bool) {
	parts := strings.Split(tok, ":")
	for _, p := range parts {
		if !isKasFile(p) {
			return nil, false
		}
	}
	return parts, true
}

func isKasFile(a string) bool {
	if !strings.HasSuffix(a, ".yml") && !strings.HasSuffix(a, ".yaml") {
		return false
	}
	info, err := os.Stat(a)
	return err == nil && !info.IsDir()
}

// resolveImage returns the image to run in: an explicit image wins; otherwise yb
// builds (if missing) the image for the project's version.
func resolveImage(p *project.Project, log image.Logf) (string, error) {
	if p.Image != "" {
		return p.Image, nil
	}
	if p.Version == "" {
		return "", fmt.Errorf("set 'version' (%s) or 'image' in the kas file's yb: block",
			strings.Join(image.Versions(), ", "))
	}
	return image.Ensure(p.Version, false, log)
}

func cmdBuild(argv []string) error {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	force := fs.Bool("force", false, "force git checkout/pull to the pinned ref")
	_ = fs.Parse(argv)

	entries, targets := splitEntry(fs.Args())
	p, c, err := loaded(entries)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		targets = c.Targets
	}
	if len(targets) == 0 {
		return fmt.Errorf("no targets: none in the kas file and none on the command line")
	}

	log := func(format string, a ...any) { fmt.Printf("• "+format+"\n", a...) }

	if err := repo.Checkout(p.Dir, c.Repos, p.SSHKey, *force, log); err != nil {
		return err
	}
	buildDir := filepath.Join(p.Dir, "build")
	if err := conf.Write(buildDir, conf.LocalConf(c, p.Cache, runtime.NumCPU()), conf.BBLayers(c)); err != nil {
		return err
	}
	pokyDir, err := c.PokyDir()
	if err != nil {
		return err
	}
	img, err := resolveImage(p, log)
	if err != nil {
		return err
	}
	log("building %v for %s/%s in %s", targets, c.Machine, c.Distro, img)
	return runner.Run(p, runner.Options{Image: img, PokyDir: pokyDir, Targets: targets})
}

func cmdShell(argv []string) error {
	fs := flag.NewFlagSet("shell", flag.ExitOnError)
	_ = fs.Parse(argv)

	entries, _ := splitEntry(fs.Args())
	p, c, err := loaded(entries)
	if err != nil {
		return err
	}
	log := func(format string, a ...any) { fmt.Printf("• "+format+"\n", a...) }
	// Ensure confs exist so the sourced build env is complete.
	buildDir := filepath.Join(p.Dir, "build")
	if err := conf.Write(buildDir, conf.LocalConf(c, p.Cache, runtime.NumCPU()), conf.BBLayers(c)); err != nil {
		return err
	}
	pokyDir, err := c.PokyDir()
	if err != nil {
		return err
	}
	img, err := resolveImage(p, log)
	if err != nil {
		return err
	}
	return runner.Run(p, runner.Options{Image: img, PokyDir: pokyDir, Shell: true})
}
