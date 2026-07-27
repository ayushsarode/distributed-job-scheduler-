package assert

import (
	"fmt"
	"os"
	"runtime"
	"github.com/rs/zerolog"
	assertlib "github.com/stretchr/testify/assert"
)

type assertLogger struct{}

var _ (assertlib.TestingT) = (*assertLogger)(nil)

var defaultLogger = zerolog.New(os.Stdout).With().Timestamp().Logger()

const callerSkip = 4
// Errorf implements assert.TestingT.
func (a *assertLogger) Errorf(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	_, file, line, ok := runtime.Caller(callerSkip)
	if ok {
		defaultLogger.Fatal().
			Str("file", file).
			Int("line", line).
			Msg(message)
	} else {
		defaultLogger.Fatal().Msg(message)
	}
}

func NotNil(v any, messageArgs ...any) {
	assertlib.NotNil(&assertLogger{}, v, messageArgs...)
}

func True(v bool, messageArgs ...any) {
	assertlib.True(&assertLogger{}, v, messageArgs...)
}

func NoError(err error, args ...any) {
	assertlib.NoError(&assertLogger{}, err, args...)
}

func NotEmpty(v string, messageArgs ...any) {
	assertlib.NotEmpty(&assertLogger{}, v, messageArgs...)
}

func Fail(message string, messageArgs ...any) {
	assertlib.Fail(&assertLogger{}, message, messageArgs...)
}
