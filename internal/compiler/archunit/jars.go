package archunit

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ergoprobat/ergo/internal/compiler"
)

// maxJarBytes bounds one jar. The largest pinned jar is under 5 MiB.
const maxJarBytes = 32 << 20

const (
	archunitJarName = "archunit-1.5.1.jar"
	slf4jJarName    = "slf4j-api-2.0.19.jar"
	consoleJarName  = "junit-platform-console-standalone-6.1.3.jar"
)

type pinnedJar struct {
	name   string
	sha256 string
}

// pinnedJars is the sha256 of every jar evaluation runs. jars.lock under testdata/tool carries the
// same values for install.sh, and a test keeps the two equal.
var pinnedJars = []pinnedJar{
	{archunitJarName, "a4dbfc51c90005ad6ac9967672a7efb43aa6928287a57238b28e60e0a75c5c6f"},
	{slf4jJarName, "e91ff6d720609e7a194ffe758c3ed5c84e798617ae07b0a0f6a4fe229741b4bb"},
	{consoleJarName, "e62b96ac475dbcde8599ea905d088f65d90778f86e259b856a49fa5c4ea256ec"},
}

// stageJars reads each pinned jar from source, checks its sha256, and writes the verified bytes to dest.
// The JVM only ever sees dest, so a change to source after the check does not reach it.
func stageJars(source, dest string) error {
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return &compiler.Error{Code: CodeRun, Err: err}
	}
	for _, jar := range pinnedJars {
		body, err := readJar(filepath.Join(source, jar.name))
		if err != nil {
			return &compiler.Error{Code: CodeJar, Err: fmt.Errorf("%s: %w", jar.name, err)}
		}
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != jar.sha256 {
			return &compiler.Error{Code: CodeJar, Err: fmt.Errorf("%s does not match the pinned sha256", jar.name)}
		}
		if err := os.WriteFile(filepath.Join(dest, jar.name), body, 0o600); err != nil {
			return &compiler.Error{Code: CodeRun, Err: err}
		}
	}
	return nil
}

func readJar(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot be read")
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("is not a regular file")
	}
	body, err := io.ReadAll(io.LimitReader(file, maxJarBytes+1))
	if err != nil {
		return nil, errors.New("cannot be read")
	}
	if len(body) > maxJarBytes {
		return nil, fmt.Errorf("is larger than %d bytes", maxJarBytes)
	}
	return body, nil
}
