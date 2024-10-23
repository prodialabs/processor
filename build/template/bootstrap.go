package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/calebcase/oops"
	"github.com/rs/zerolog"
)

type BootstrapFlags struct{}

type BootstrapCLI struct {
	BootstrapFlags
}

func (cli *BootstrapCLI) Run(logger zerolog.Logger, flags *Flags) (err error) {
	flags.ProcessorAssets, err = filepath.Abs(flags.ProcessorAssets)
	if err != nil {
		return oops.Trace(err)
	}

	assetsPath := filepath.Join(flags.ProcessorAssets, ProcessorName)
	err = os.MkdirAll(assetsPath, 0700)
	if err != nil {
		return oops.Trace(err)
	}

	sourcePath := filepath.Join(assetsPath, "source")
	_ = os.RemoveAll(sourcePath)

	err = os.CopyFS(assetsPath, SourceFS)
	if err != nil {
		return oops.Trace(err)
	}

	registrationPath := filepath.Join(sourcePath, "registration.json")
	reg, err := os.Open(registrationPath)
	if err != nil {
		return oops.Trace(err)
	}
	defer func() {
		oops.ChainP(&err, oops.Trace(reg.Close()))
	}()

	var registration Registration
	err = json.NewDecoder(reg).Decode(&registration)
	if err != nil {
		return oops.Trace(err)
	}

	executablePath := filepath.Join(sourcePath, registration.Executable)
	err = os.Chmod(executablePath, 0700)
	if err != nil {
		return oops.Trace(err)
	}

	err = execute(
		os.Stdin,
		os.Stdout,
		sourcePath,
		[]string{
			"HOME=" + assetsPath,
			"PATH=" + os.Getenv("PATH"),
			"PROCESSOR_NAME=" + ProcessorName,
			"PROCESSOR_ASSETS=" + flags.ProcessorAssets,
			"PROCESSOR_BOOTSTRAP=true",
		},
		executablePath,
	)
	if err != nil {
		return err
	}

	return nil
}
