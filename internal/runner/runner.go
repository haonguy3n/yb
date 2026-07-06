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
	if o.Shell {
		args = append(args, "-it")
	}
	args = append(args, "-v", p.Dir+":"+conf.WorkDir)
	args = append(args, "-v", p.Cache+":"+p.Cache)
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
