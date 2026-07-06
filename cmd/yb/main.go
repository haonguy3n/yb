// Command yb is a kas-compatible Yocto build orchestrator: it reads existing kas
// YAML, checks out the pinned repos, generates local.conf/bblayers.conf, and
// runs bitbake inside our yocto-kas container. See docs/design/2026-07-06-yb.md.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/anhhao17/yb/internal/conf"
	"github.com/anhhao17/yb/internal/config"
	"github.com/anhhao17/yb/internal/image"
	"github.com/anhhao17/yb/internal/project"
	"github.com/anhhao17/yb/internal/repo"
	"github.com/anhhao17/yb/internal/runner"
)

const version = "0.1.0"

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

Common flags (build/shell):
  -C dir        project directory (default ".")
  -f file       kas file (overrides kas_file in yb.yaml)
  -version v    Yocto release; yb builds an aligned image (overrides yb.yaml)
  -image name   use this prebuilt image instead of building one
  -machine m    override MACHINE
  --rebuild     rebuild the version image even if it already exists
build-only:
  --dry-run     print the resolved plan without changing or running anything
  --no-checkout skip the git checkout step

Known versions: `)
	fmt.Fprintln(os.Stderr, joinVersions())
}

// override collects the flag values that can override yb.yaml.
type override struct {
	dir, kasFile, version, image, machine string
	rebuild                               bool
}

// loaded resolves the project and merged kas config shared by build and shell.
func loaded(o override) (*project.Project, *config.Config, error) {
	p, err := project.Load(o.dir)
	if err != nil {
		return nil, nil, err
	}
	if o.kasFile != "" {
		p.KasFile = o.kasFile
	}
	if o.version != "" {
		p.Version = o.version
	}
	if o.image != "" {
		p.Image = o.image
	}
	if p.KasFile == "" {
		return nil, nil, fmt.Errorf("no kas file: set kas_file in yb.yaml or pass -f")
	}
	c, err := config.Load(filepath.Join(p.Dir, p.KasFile))
	if err != nil {
		return nil, nil, err
	}
	if o.machine != "" {
		c.Machine = o.machine
	}
	return p, c, nil
}

// resolveImage returns the image to run in: an explicit image wins; otherwise yb
// builds (or, on dry-run, reports) the image for the project's version.
func resolveImage(p *project.Project, rebuild, dryRun bool, log image.Logf) (string, error) {
	if p.Image != "" {
		return p.Image, nil
	}
	if p.Version == "" {
		return "", fmt.Errorf("set 'version' (%s) or 'image' in yb.yaml", joinVersions())
	}
	if dryRun {
		tag := image.Tag(p.Version)
		prof, ok := image.Describe(p.Version)
		if !ok {
			return "", fmt.Errorf("unknown version %q (known: %s)", p.Version, joinVersions())
		}
		if image.Exists(tag) {
			log("image %s (already built)", tag)
		} else {
			log("image %s (would build: ubuntu %s)", tag, prof.Ubuntu)
		}
		return tag, nil
	}
	return image.Ensure(p.Version, rebuild, log)
}

func joinVersions() string { return strings.Join(image.Versions(), ", ") }

func cmdBuild(argv []string) error {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	dir := fs.String("C", ".", "project directory")
	kasFile := fs.String("f", "", "kas file")
	version := fs.String("version", "", "Yocto release")
	imageFlag := fs.String("image", "", "prebuilt container image")
	machine := fs.String("machine", "", "override MACHINE")
	rebuild := fs.Bool("rebuild", false, "rebuild the version image")
	dryRun := fs.Bool("dry-run", false, "print the plan, run nothing")
	noCheckout := fs.Bool("no-checkout", false, "skip git checkout")
	_ = fs.Parse(argv)

	p, c, err := loaded(override{*dir, *kasFile, *version, *imageFlag, *machine, *rebuild})
	if err != nil {
		return err
	}

	targets := fs.Args()
	if len(targets) == 0 {
		targets = c.Targets
	}
	if len(targets) == 0 {
		return fmt.Errorf("no targets: none in %s and none on the command line", p.KasFile)
	}

	log := func(format string, a ...any) { fmt.Printf("• "+format+"\n", a...) }

	if !*noCheckout {
		if err := repo.Checkout(p.Dir, c.Repos, p.SSHKey, *dryRun, log); err != nil {
			return err
		}
	}

	localConf := conf.LocalConf(c, p.Cache, runtime.NumCPU())
	bblayers := conf.BBLayers(c)
	buildDir := filepath.Join(p.Dir, "build")
	if *dryRun {
		fmt.Printf("\n--- %s ---\n%s", filepath.Join("build", "conf", "local.conf"), localConf)
		fmt.Printf("\n--- %s ---\n%s\n", filepath.Join("build", "conf", "bblayers.conf"), bblayers)
	} else {
		if err := conf.Write(buildDir, localConf, bblayers); err != nil {
			return err
		}
	}

	pokyDir, err := c.PokyDir()
	if err != nil {
		return err
	}
	img, err := resolveImage(p, *rebuild, *dryRun, log)
	if err != nil {
		return err
	}
	if !*dryRun {
		log("building %v for %s/%s in %s", targets, c.Machine, c.Distro, img)
	}
	return runner.Run(p, runner.Options{
		Image:   img,
		PokyDir: pokyDir,
		Targets: targets,
		DryRun:  *dryRun,
	})
}

func cmdShell(argv []string) error {
	fs := flag.NewFlagSet("shell", flag.ExitOnError)
	dir := fs.String("C", ".", "project directory")
	kasFile := fs.String("f", "", "kas file")
	version := fs.String("version", "", "Yocto release")
	imageFlag := fs.String("image", "", "prebuilt container image")
	machine := fs.String("machine", "", "override MACHINE")
	rebuild := fs.Bool("rebuild", false, "rebuild the version image")
	_ = fs.Parse(argv)

	p, c, err := loaded(override{*dir, *kasFile, *version, *imageFlag, *machine, *rebuild})
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
	img, err := resolveImage(p, *rebuild, false, log)
	if err != nil {
		return err
	}
	return runner.Run(p, runner.Options{Image: img, PokyDir: pokyDir, Shell: true})
}
