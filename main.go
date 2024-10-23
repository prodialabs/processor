package main

import (
	"context"
	"io"
	stdlog "log"
	"os"

	"github.com/alecthomas/kong"
	"github.com/goccy/go-json"
	"github.com/mattn/go-isatty"
	"github.com/rs/zerolog"

	"github.com/prodialabs/processor/build"
	"github.com/prodialabs/processor/inspect"
)

type Flags struct {
	Log struct {
		Level  zerolog.Level `default:"info" enum:"trace,debug,info,warn,error,fatal,panic" env:"LOG_LEVEL"`
		Format string        `default:"console" enum:"json,console" env:"LOG_FORMAT"`
	} `embed:"" prefix:"log."`
}

type CLI struct {
	Flags

	Build   build.CLI   `kong:"cmd=''"`
	Inspect inspect.CLI `kong:"cmd=''"`
}

func Run() (err error) {
	ctx := context.Background()

	cli := &CLI{}

	kctx := kong.Parse(
		cli,

		kong.Bind(&ctx),
		kong.Bind(&cli.Flags),
		kong.Bind(&cli.Build.Flags),
		kong.Bind(&cli.Inspect.Flags),
	)

	zerolog.SetGlobalLevel(cli.Flags.Log.Level)

	var output io.Writer = os.Stderr

	switch cli.Flags.Log.Format {
	case "json":
		// This is the default already.
	case "console":
		output = zerolog.ConsoleWriter{
			Out:     output,
			NoColor: !isatty.IsTerminal(os.Stderr.Fd()),
		}
	}

	logger := zerolog.New(output).With().Timestamp().Logger()
	stdlog.SetFlags(0)
	stdlog.SetOutput(logger)

	kctx.Bind(logger)

	err = kctx.Run()
	if err != nil {
		logger.Error().Err(err).Msgf("error: %+v", err)

		return err
	}

	return nil
}

func main() {
	zerolog.InterfaceMarshalFunc = func(v any) ([]byte, error) {
		return json.MarshalWithOption(v, json.DisableHTMLEscape())
	}

	err := Run()
	if err != nil {
		os.Exit(1)
	}
}
