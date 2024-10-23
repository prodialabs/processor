package inspect

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"os"

	"github.com/calebcase/oops"
	"github.com/rs/zerolog"
)

type Flags struct {
	ExecutablePath string `kong:"arg='',required"`
}

type CLI struct {
	Flags
}

func (cli *CLI) Run(logger zerolog.Logger) (err error) {
	executable, err := os.Open(cli.ExecutablePath)
	if err != nil {
		return oops.Trace(err)
	}
	defer func() {
		oops.ChainP(&err, oops.Trace(executable.Close()))
	}()

	executableSize, err := executable.Seek(0, io.SeekEnd)
	if err != nil {
		return oops.Trace(err)
	}
	if executableSize < 16 {
		return oops.New("not a processor: executable < 16")
	}

	_, err = executable.Seek(-16, io.SeekEnd)
	if err != nil {
		return oops.Trace(err)
	}

	buf := make([]byte, 16)

	_, err = executable.Read(buf)
	if err != nil {
		return oops.Trace(err)
	}
	if len(buf) != 16 {
		return oops.New("not a processor: magic < 16")
	}
	if string(buf[0:3]) != "PRO" {
		return oops.New("not a processor: magic != PRO")
	}

	major := buf[3]
	if major > 0 {
		return oops.New("not a processor: major version != 0")
	}

	minor := buf[4]

	offset := binary.BigEndian.Uint64(append([]byte{0}, buf[5:5+7]...))
	if int64(offset) > executableSize {
		return oops.New("not a processor: offset > executable: %d > %d", offset, executableSize)
	}

	size := binary.BigEndian.Uint32(buf[4+8:])
	if int64(offset)+int64(size) > executableSize {
		return oops.New(
			"not a processor: offset + size > executable: %d > %d",
			int64(offset)+int64(size),
			executableSize,
		)
	}

	logger.Debug().
		Uint8("major", major).
		Uint8("minor", minor).
		Uint64("offset", offset).
		Uint32("size", size).
		Msg("registration")

	reg := make([]byte, size)

	_, err = executable.ReadAt(reg, int64(offset))
	if err != nil {
		return oops.Trace(err)
	}

	tmp := map[string]any{}
	err = json.Unmarshal(reg, &tmp)
	if err != nil {
		return oops.Trace(err)
	}

	reg, err = json.MarshalIndent(tmp, "", "  ")
	if err != nil {
		return oops.Trace(err)
	}

	_, err = os.Stdout.Write(append(reg, '\n'))
	if err != nil {
		return oops.Trace(err)
	}

	return nil
}
