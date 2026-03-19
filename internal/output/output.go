package output

import (
	"fmt"
	"strings"
)

const (
	OK   = "+"
	WARN = "!"
	FAIL = "x"
	INFO = "-"
	SKIP = "~"
)

type Builder struct {
	parts []string
}

func New() *Builder {
	return &Builder{}
}

func (b *Builder) Header(title string) {
	b.parts = append(b.parts, "## "+title)
}

func (b *Builder) Section(title string) {
	b.parts = append(b.parts, "\n### "+title)
}

func (b *Builder) OK(format string, args ...any) {
	b.parts = append(b.parts, "["+OK+"] "+fmt.Sprintf(format, args...))
}

func (b *Builder) Warn(format string, args ...any) {
	b.parts = append(b.parts, "["+WARN+"] "+fmt.Sprintf(format, args...))
}

func (b *Builder) Fail(format string, args ...any) {
	b.parts = append(b.parts, "["+FAIL+"] "+fmt.Sprintf(format, args...))
}

func (b *Builder) Info(format string, args ...any) {
	b.parts = append(b.parts, "["+INFO+"] "+fmt.Sprintf(format, args...))
}

func (b *Builder) Skip(format string, args ...any) {
	b.parts = append(b.parts, "["+SKIP+"] "+fmt.Sprintf(format, args...))
}

func (b *Builder) KV(key string, value any) {
	b.parts = append(b.parts, fmt.Sprintf("**%s**: %v", key, value))
}

func (b *Builder) Bullet(text string) {
	b.parts = append(b.parts, "- "+text)
}

func (b *Builder) Text(text string) {
	b.parts = append(b.parts, text)
}

func (b *Builder) String() string {
	return strings.Join(b.parts, "\n")
}

func (b *Builder) Empty() bool {
	return len(b.parts) == 0
}

func Error(format string, args ...any) string {
	return fmt.Sprintf("**Error**: "+format, args...)
}

func CleanBody(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "(empty)"
	}
	// Strip HTML comments
	for {
		start := strings.Index(text, "<!--")
		if start == -1 {
			break
		}
		end := strings.Index(text[start:], "-->")
		if end == -1 {
			break
		}
		text = text[:start] + text[start+end+3:]
	}
	text = strings.TrimSpace(text)
	// Collapse 3+ consecutive newlines into 2
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}
	return text
}
