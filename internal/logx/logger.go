package logx

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

type Logger struct {
	Out io.Writer
	Err io.Writer
	Now func() time.Time
}

type Field struct {
	Key   string
	Value string
}

func (l Logger) Info(event string, fields ...Field) {
	l.log(l.Out, "INFO", event, fields...)
}

func (l Logger) Warn(event string, fields ...Field) {
	l.log(l.Err, "WARN", event, fields...)
}

func (l Logger) Error(event string, fields ...Field) {
	l.log(l.Err, "ERROR", event, fields...)
}

func (l Logger) log(w io.Writer, level, event string, fields ...Field) {
	sorted := append([]Field(nil), fields...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Key < sorted[j].Key
	})

	var builder strings.Builder
	now := time.Now
	if l.Now != nil {
		now = l.Now
	}
	fmt.Fprintf(&builder, "time=%s level=%s event=%s", now().UTC().Format(time.RFC3339), level, event)
	for _, field := range sorted {
		fmt.Fprintf(&builder, " %s=%s", field.Key, field.Value)
	}
	builder.WriteByte('\n')
	_, _ = io.WriteString(w, builder.String())
}
