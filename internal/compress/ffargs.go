package compress

import (
	"fmt"
	"strings"
	"unicode"
)

type ffQuoteState uint8

const (
	ffUnquoted ffQuoteState = iota
	ffSingleQuoted
	ffDoubleQuoted
)

func ParseFFOptions(value string) ([]string, error) {
	var args []string
	var builder strings.Builder
	state := ffUnquoted
	tokenStarted := false

	flush := func() error {
		if !tokenStarted {
			return nil
		}
		if builder.Len() == 0 {
			return fmt.Errorf("empty ffmpeg option")
		}
		args = append(args, builder.String())
		builder.Reset()
		tokenStarted = false
		return nil
	}

	runes := []rune(value)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch state {
		case ffUnquoted:
			switch {
			case unicode.IsSpace(r):
				if err := flush(); err != nil {
					return nil, err
				}
			case r == '\'':
				state = ffSingleQuoted
				tokenStarted = true
			case r == '"':
				state = ffDoubleQuoted
				tokenStarted = true
			case r == '\\':
				if i+1 == len(runes) {
					return nil, fmt.Errorf("dangling escape in ffmpeg options")
				}
				i++
				builder.WriteRune(runes[i])
				tokenStarted = true
			default:
				builder.WriteRune(r)
				tokenStarted = true
			}
		case ffSingleQuoted, ffDoubleQuoted:
			switch r {
			case '\'':
				if state == ffSingleQuoted {
					state = ffUnquoted
				} else {
					builder.WriteRune(r)
				}
			case '"':
				if state == ffDoubleQuoted {
					state = ffUnquoted
				} else {
					builder.WriteRune(r)
				}
			case '\\':
				if i+1 == len(runes) {
					return nil, fmt.Errorf("dangling escape in ffmpeg options")
				}
				i++
				builder.WriteRune(runes[i])
			default:
				builder.WriteRune(r)
			}
		}
	}

	switch state {
	case ffSingleQuoted:
		return nil, fmt.Errorf("unclosed single quote in ffmpeg options")
	case ffDoubleQuoted:
		return nil, fmt.Errorf("unclosed double quote in ffmpeg options")
	}
	if err := flush(); err != nil {
		return nil, err
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("empty ffmpeg option")
	}
	return args, nil
}
