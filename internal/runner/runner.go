// Package runner launches the build inside the yocto-kas container: it assembles
// the `docker run` invocation (mounts for the project, cache, ssh key, and extra
// bind mounts), sources oe-init-build-env, and runs bitbake or an interactive
// shell.
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
	Image   string   // container image to run in
	PokyDir string   // poky checkout dir, relative to project root
	Targets []string // bitbake targets (ignored when Shell)
	Shell   bool     // drop into bash instead of running bitbake
}

// Run executes the build (or shell) for project p.
func Run(p *project.Project, o Options) error {
	pokyEnv := path.Join(conf.WorkDir, o.PokyDir, "oe-init-build-env")
	initLine := fmt.Sprintf("source %s %s >/dev/null", pokyEnv, conf.BuildDir)

	var inner string
	if o.Shell {
		inner = initLine + " && exec bash"
	} else {
		inner = initLine + " && bitbake " + strings.Join(o.Targets, " ")
	}

	args := []string{"run", "--rm"}
	// A TTY gives bitbake its live progress UI (like kas-container). A shell
	// always needs it; a build gets it only when run interactively, so CI logs
	// stay plain line-by-line output.
	if o.Shell || interactive() {
		args = append(args, "-it")
	}
	args = append(args, "-v", p.Dir+":"+conf.WorkDir)
	// Mount DL_DIR and SSTATE_DIR at their host paths. Create them first so they
	// are owned by the caller (uid 1000 = builder); a bind mount of a missing
	// path would otherwise be created root-owned and unwritable in the container.
	seen := map[string]bool{}
	for _, d := range []string{p.DLDir, p.SSTateDir} {
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		_ = os.MkdirAll(d, 0o755)
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
	args = append(args, "-w", conf.BuildDir, o.Image, "bash", "-c", inner)

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
