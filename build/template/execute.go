package main

import (
	"io"
	"os"
	"os/exec"

	"github.com/calebcase/oops"
	"golang.org/x/sync/errgroup"
)

func execute(
	in io.Reader,
	out io.Writer,
	path string,
	env []string,
	name string,
	args ...string,
) (err error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = path
	cmd.Env = env

	cmd.Stdin = in

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return oops.Trace(err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return oops.Trace(err)
	}

	err = cmd.Start()
	if err != nil {
		return oops.Trace(err)
	}
	defer func() {
		oops.ChainP(&err, oops.Trace(cmd.Wait()))
	}()

	group := errgroup.Group{}
	group.Go(func() (err error) {
		_, err = io.Copy(out, stdout)

		return err
	})

	group.Go(func() (err error) {
		_, err = io.Copy(os.Stderr, stderr)

		return err
	})

	return group.Wait()
}
