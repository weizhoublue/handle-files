package logx

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

type Logger struct {
	Out io.Writer
	Err io.Writer
}

type Field struct {
	Key   string
	Value string
}

func (l Logger) Info(event string, fields ...Field) {
	l.log(l.Out, "INFO", event, fields, nil)
}

func (l Logger) InfoProgress(event string, fields []Field, progress []Field) {
	l.log(l.Out, "INFO", event, fields, progress)
}

func (l Logger) Warn(event string, fields ...Field) {
	l.log(l.Err, "WARN", event, fields, nil)
}

func (l Logger) WarnProgress(event string, fields []Field, progress []Field) {
	l.log(l.Err, "WARN", event, fields, progress)
}

func (l Logger) Error(event string, fields ...Field) {
	l.log(l.Err, "ERROR", event, fields, nil)
}

func (l Logger) ErrorProgress(event string, fields []Field, progress []Field) {
	l.log(l.Err, "ERROR", event, fields, progress)
}

func (l Logger) log(w io.Writer, level, event string, fields, progress []Field) {
	sortedFields := append([]Field(nil), fields...)
	sort.SliceStable(sortedFields, func(i, j int) bool {
		return sortedFields[i].Key < sortedFields[j].Key
	})
	sortedProgress := append([]Field(nil), progress...)
	sort.SliceStable(sortedProgress, func(i, j int) bool {
		return sortedProgress[i].Key < sortedProgress[j].Key
	})

	var builder strings.Builder
	fmt.Fprintf(&builder, "%s, %s", level, event)
	for _, field := range sortedFields {
		fmt.Fprintf(&builder, " %s=%s", field.Key, field.Value)
	}
	if len(sortedProgress) > 0 {
		builder.WriteString(" [")
		for _, field := range sortedProgress {
			fmt.Fprintf(&builder, " %s=%s", field.Key, field.Value)
		}
		builder.WriteString(" ]")
	}
	builder.WriteByte('\n')
	_, _ = io.WriteString(w, builder.String())
}
