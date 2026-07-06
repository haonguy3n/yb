// Package runner launches the build. With an image it runs inside that container
// (mounting the project, dl/sstate, ssh key, and extra mounts, sourcing
// oe-init-build-env, then bitbake). With no image it runs natively on the host.
package runner

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/haonguy3n/yb/internal/conf"
	"github.com/haonguy3n/yb/internal/project"
)

// SSHKeyDest is where the ssh key is mounted for the builder user (matches the
// IdentityFile in the yocto-kas image's ssh config).
const SSHKeyDest = "/home/builder/.ssh/iri"

// Options controls a single run.
type Options struct {
	Image   string   // container image to run in; empty => native host build
	PokyDir string   // poky checkout dir, relative to project root
	Targets []string // bitbake targets (ignored when Shell)
	Shell   bool     // drop into bash instead of running bitbake
}

// Run executes the build (or shell) for project p, in the container when an
// image is set, otherwise natively on the host.
func Run(p *project.Project, o Options) error {
	// DL_DIR/SSTATE_DIR must exist and be owned by the caller (a bind mount of a
	// missing path is created root-owned; native bitbake also needs them).
	for _, d := range []string{p.DLDir, p.SSTateDir} {
		if d != "" {
			_ = os.MkdirAll(d, 0o755)
		}
	}
	if o.Image == "" {
		return runHost(p, o)
	}
	return runContainer(p, o)
}

// script builds the `source oe-init-build-env … && bitbake …` (or shell) command
// run in a bash under root (WorkDir in a container, the project dir on the host).
func script(o Options, root string) string {
	init := fmt.Sprintf("source %s %s >/dev/null",
		path.Join(root, o.PokyDir, "oe-init-build-env"), path.Join(root, "build"))
	if o.Shell {
		return init + " && exec bash"
	}
	return init + " && bitbake " + strings.Join(o.Targets, " ")
}

func runHost(p *project.Project, o Options) error {
	cmd := exec.Command("bash", "-c", script(o, p.Dir))
	cmd.Dir = p.Dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func runContainer(p *project.Project, o Options) error {
	args := []string{"run", "--rm"}
	// A TTY gives bitbake its live progress UI (like kas-container). A shell
	// always needs it; a build gets it only when run interactively, so CI logs
	// stay plain line-by-line output.
	if o.Shell || interactive() {
		args = append(args, "-it")
	}
	args = append(args, "-v", p.Dir+":"+conf.WorkDir)
	seen := map[string]bool{}
	for _, d := range []string{p.DLDir, p.SSTateDir} {
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		args = append(args, "-v", d+":"+d)
	}
	for _, m := range p.Mounts {
		host, opt := m, ""
		if strings.HasSuffix(m, ":ro") {
			host, opt = strings.TrimSuffix(m, ":ro"), ":ro"
		}
		args = append(args, "-v", host+":"+host+opt)
	}
	if p.SSHKey != "" {
		if _, err := os.Stat(p.SSHKey); err == nil {
			args = append(args, "-v", p.SSHKey+":"+SSHKeyDest+":ro")
		} else {
			fmt.Fprintf(os.Stderr, "yb: ssh key %s not found; building without it\n", p.SSHKey)
		}
	}
	args = append(args, "-w", path.Join(conf.WorkDir, "build"), o.Image, "bash", "-c", script(o, conf.WorkDir))

	cmd := exec.Command("docker", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// interactive reports whether both stdin and stdout are terminals, so a TTY can
// be allocated (`docker run -t` fails otherwise, e.g. when output is piped).
func interactive() bool {
	return isTTY(os.Stdin) && isTTY(os.Stdout)
}

func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
