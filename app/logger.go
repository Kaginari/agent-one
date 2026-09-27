package app

import (
	"io"

	"charm.land/log/v2"
)

// stdLogWarn adapts a Charm Log to the standard logger an http.Server takes: its errors at warn.
var stdLogWarn = log.StandardLogOptions{ForceLevel: log.WarnLevel}

// newLogger is a command's own output (the board): levelled, coloured, timed.
func newLogger(w io.Writer, prefix string) *log.Logger {
	return log.NewWithOptions(w, log.Options{Prefix: prefix, ReportTimestamp: true, TimeFormat: "15:04:05"})
}
