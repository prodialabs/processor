package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/calebcase/oops"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
	"golang.org/x/term"
)

type RunFlags struct {
	ProcessorResources string `kong:"default='/var/cache/processor/resources',env='PROCESSOR_RESOURCES'"`
	Interactive        bool   `kong:"negatable,short='i'"`
	Controlled         bool   `kong:"negatible"`
}

type RunCLI struct {
	RunFlags

	Meta  string   `kong:"arg,optional"`
	Parts []string `kong:"arg,optional,passthrough"`
}

func (cli *RunCLI) Run(logger zerolog.Logger, flags *Flags) (err error) {
	if !cli.Interactive && term.IsTerminal(int(os.Stdin.Fd())) {
		err = os.Stdin.Close()
		if err != nil {
			return oops.Trace(err)
		}
	}

	var stdin io.ReadCloser = os.Stdin

	g := &errgroup.Group{}
	defer func() {
		oops.ChainP(&err, g.Wait())
	}()

	// If meta was set via the args, then we need to read arg meta and N
	// parts files into parts, and feed these parts to the processor. Next
	// we copy our stdin to the processor dropping any additional meta we
	// see and the first N data parts.
	filterMeta := false
	dropDataParts := 0
	if cli.Meta != "" {
		filterMeta = true
		dropDataParts = len(cli.Parts)

		r, w := io.Pipe()
		if err != nil {
			return oops.Trace(err)
		}
		stdin = r

		g.Go(func() (err error) {
			defer func() {
				oops.ChainP(&err, w.Close())
			}()

			err = func() (err error) {
				meta := Meta{}

				var metaReader io.Reader

				f, err := os.Open(cli.Meta)
				if err != nil {
					if errors.Is(err, fs.ErrNotExist) {
						metaReader = bytes.NewBufferString(cli.Meta)
					} else {
						return oops.Trace(err)
					}
				} else {
					metaReader = f

					defer func() {
						oops.ChainP(&err, f.Close())
					}()
				}

				err = json.NewDecoder(metaReader).Decode(&meta)
				if err != nil {
					return oops.Trace(err)
				}

				err = json.NewEncoder(w).Encode(&meta)
				if err != nil {
					return oops.Trace(err)
				}

				_, err = io.Copy(w, metaReader)
				if err != nil {
					return oops.Trace(err)
				}

				return nil
			}()
			if err != nil {
				return err
			}

			for _, path := range cli.Parts {
				err = func() (err error) {
					f, err := os.Open(path)
					if err != nil {
						return oops.Trace(err)
					}
					defer func() {
						oops.ChainP(&err, f.Close())
					}()

					data := Data{
						Part: Part{
							Kind:        DataKind,
							Type:        DataType,
							ContentType: "application/octet-stream",
							Boundary:    NewBoundary(),
						},
						FileName: filepath.Base(path),
					}

					err = json.NewEncoder(w).Encode(&data)
					if err != nil {
						return oops.Trace(err)
					}

					_, err = io.Copy(w, f)
					if err != nil {
						return oops.Trace(err)
					}

					_, err = w.Write([]byte(data.Boundary))
					if err != nil {
						return oops.Trace(err)
					}

					return nil
				}()
				if err != nil {
					return err
				}
			}

			r := bufio.NewReader(os.Stdin)
			for {
				line, err := r.ReadBytes('\n')
				if err != nil {
					if errors.Is(err, io.EOF) {
						break
					}

					if errors.Is(err, os.ErrClosed) {
						break
					}

					return oops.Trace(err)
				}

				var part Part
				err = json.Unmarshal(line, &part)
				if err != nil {
					return oops.Trace(err)
				}

				err = part.LoadPayload(r)
				if err != nil {
					return err
				}

				if part.Kind == MetaKind && part.Type == LastType {
					break
				}

				if filterMeta && part.Kind == MetaKind {
					continue
				}

				if dropDataParts > 0 {
					dropDataParts -= 1
					continue
				}

				_, err = w.Write(line)
				if err != nil {
					return oops.Trace(err)
				}

				if part.Payload != nil {
					_, err = w.Write(part.Payload)
					if err != nil {
						return oops.Trace(err)
					}
				}
			}

			err = json.NewEncoder(w).Encode(&Last{
				Part: Part{
					Kind: MetaKind,
					Type: "processor.last.v1",
				},
			})
			if err != nil {
				return oops.Trace(err)
			}

			return nil
		})
	}

	flags.ProcessorAssets, err = filepath.Abs(flags.ProcessorAssets)
	if err != nil {
		return oops.Trace(err)
	}

	assetsPath := filepath.Join(flags.ProcessorAssets, ProcessorName)
	sourcePath := filepath.Join(assetsPath, "source")
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

	output, stdout := io.Pipe()
	if err != nil {
		return oops.Trace(err)
	}
	defer func() {
		oops.ChainP(&err, stdout.Close())
	}()

	// Output is filtered depending on whether we should or should not emit
	// ready jobs for a controller.
	g.Go(func() (err error) {
		r := bufio.NewReader(output)
		for {
			line, err := r.ReadBytes('\n')
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}

				if errors.Is(err, os.ErrClosed) {
					break
				}

				return oops.Trace(err)
			}

			var part Data
			err = json.Unmarshal(line, &part)
			if err != nil {
				return oops.Trace(err)
			}

			err = part.LoadPayload(r)
			if err != nil {
				return err
			}

			if !cli.Controlled && part.Kind == ControlKind {
				continue
			}

			_, err = os.Stdout.Write(line)
			if err != nil {
				return oops.Trace(err)
			}

			if part.Payload != nil {
				_, err = os.Stdout.Write(part.Payload)
				if err != nil {
					return oops.Trace(err)
				}
			}
		}

		return nil
	})

	start := time.Now()

	err = execute(
		stdin,
		stdout,
		sourcePath,
		[]string{
			"HOME=" + assetsPath,
			"PATH=" + os.Getenv("PATH"),
			"PROCESSOR_NAME=" + ProcessorName,
			"PROCESSOR_ASSETS=" + flags.ProcessorAssets,
			"PROCESSOR_RESOURCES=" + cli.ProcessorResources,
		},
		executablePath,
	)
	if err != nil {
		return err
	}

	stop := time.Now()

	logger.Info().Float64("elapsed", stop.Sub(start).Seconds()).Msg("executed")

	return nil
}
