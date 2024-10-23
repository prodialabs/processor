package build

import (
	"bufio"
	"embed"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/calebcase/oops"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

//go:embed template
var templateFS embed.FS

type Flags struct {
	ProcessorName string `kong:"required"`
	SourceDir     string `kong:"default='.'"`
	TargetDir     string `kong:"default='.'"`
	TmpDir        string `kong:"default=''"`
	TmpDirCleanup bool   `kong:"default='true',negatable"`
}

type CLI struct {
	Flags

	BuildDir string
}

func (cli *CLI) exec(path, name string, args ...string) (err error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = filepath.Join(cli.TmpDir, filepath.Clean(path))

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
		scanner := bufio.NewScanner(stdout)

		for scanner.Scan() {
			os.Stdout.Write(append(scanner.Bytes(), '\n'))
		}

		return scanner.Err()
	})

	group.Go(func() (err error) {
		_, err = io.Copy(os.Stderr, stderr)

		return err
	})

	return group.Wait()
}

func (cli *CLI) Run(logger zerolog.Logger) (err error) {
	cli.SourceDir, err = filepath.Abs(cli.SourceDir)
	if err != nil {
		return oops.Trace(err)
	}

	cli.TargetDir, err = filepath.Abs(cli.TargetDir)
	if err != nil {
		return oops.Trace(err)
	}

	if cli.TmpDir == "" {
		cli.TmpDir, err = os.MkdirTemp("", "processor-build-*")
		if err != nil {
			return oops.Trace(err)
		}
	}

	cli.TmpDir, err = filepath.Abs(cli.TmpDir)
	if err != nil {
		return oops.Trace(err)
	}

	logger = logger.With().Str("tmp_dir", cli.TmpDir).Logger()
	logger.Info().Msg("created temporary space")
	if cli.TmpDirCleanup {
		defer os.RemoveAll(cli.TmpDir)
	}

	err = os.CopyFS(cli.TmpDir, templateFS)
	if err != nil {
		return oops.Trace(err)
	}

	cli.BuildDir = filepath.Join(cli.TmpDir, "build")

	err = os.Mkdir(cli.BuildDir, 0700)
	if err != nil {
		return oops.Trace(err)
	}

	sourceFS := os.DirFS(cli.SourceDir)
	if err != nil {
		return oops.Trace(err)
	}

	err = os.Mkdir(filepath.Join(cli.BuildDir, "source"), 0700)
	if err != nil {
		return oops.Trace(err)
	}

	err = os.CopyFS(filepath.Join(cli.BuildDir, "source"), sourceFS)
	if err != nil {
		return oops.Trace(err)
	}

	tmpTemplateFS := os.DirFS(filepath.Join(cli.TmpDir, "template"))
	err = os.CopyFS(cli.BuildDir, tmpTemplateFS)
	if err != nil {
		return oops.Trace(err)
	}

	logger.Info().Msg("initializing...")

	moduleName := fmt.Sprintf("git.prodia.com/processor/%s", cli.ProcessorName)
	err = cli.exec("build", "go", "mod", "init", moduleName)
	if err != nil {
		return err
	}

	logger.Info().Msg("tidying...")

	err = cli.exec("build", "go", "mod", "tidy")
	if err != nil {
		return err
	}

	logger.Info().Msg("building...")

	executablePath := filepath.Join(cli.TargetDir, cli.ProcessorName)

	err = cli.exec(
		"build",
		"go",
		"build",
		"-race",
		"-ldflags",
		"-X main.ProcessorName="+cli.ProcessorName,
		"-gcflags",
		"all=-N -l",
		"-o",
		executablePath,
	)
	if err != nil {
		return err
	}

	reg, err := os.Open(filepath.Join(cli.SourceDir, "registration.json"))
	if err != nil {
		return oops.Trace(err)
	}
	defer func() {
		oops.ChainP(&err, oops.Trace(reg.Close()))
	}()

	bin, err := os.OpenFile(executablePath, os.O_RDWR|os.O_APPEND, 0700)
	if err != nil {
		return oops.Trace(err)
	}
	defer func() {
		oops.ChainP(&err, oops.Trace(bin.Close()))
	}()

	offset, err := bin.Seek(0, io.SeekEnd)
	if err != nil {
		return oops.Trace(err)
	}

	size, err := io.Copy(bin, reg)
	if err != nil {
		return oops.Trace(err)
	}

	//    . major version
	//    |
	//    | . minor version
	//    | |
	//    v v
	// PROm nooo oooo zzzz
	//       \______/ \__/
	//           ^      ^
	//           |      |
	//           |      . size 2^32 = 4 GiB
	//           |
	//           . offset 2^58 = 64 PiB

	buf := []byte("PRO")

	buf = append(buf, 0, 1)

	buf = append(buf, binary.BigEndian.AppendUint64([]byte{}, uint64(offset))[1:]...)
	if err != nil {
		return oops.Trace(err)
	}

	buf = binary.BigEndian.AppendUint32(buf, uint32(size))
	if err != nil {
		return oops.Trace(err)
	}

	_, err = bin.Write(buf)
	if err != nil {
		return oops.Trace(err)
	}

	return nil
}
