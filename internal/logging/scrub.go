// SPDX-License-Identifier: AGPL-3.0-only
package logging

import (
	"context"
	"log/slog"
)

type Redactor interface {
	Redact(text string) string
}

type scrubHandler struct {
	inner    slog.Handler
	redactor Redactor
}

func WithRedactor(inner slog.Handler, redactor Redactor) slog.Handler {
	if redactor == nil {
		return inner
	}
	return &scrubHandler{inner: inner, redactor: redactor}
}

func (h *scrubHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *scrubHandler) Handle(ctx context.Context, record slog.Record) error {
	scrubbed := slog.NewRecord(record.Time, record.Level, h.redactor.Redact(record.Message), record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		scrubbed.AddAttrs(h.scrubAttr(attr))
		return true
	})
	return h.inner.Handle(ctx, scrubbed)
}

func (h *scrubHandler) scrubAttr(attr slog.Attr) slog.Attr {
	value := attr.Value.Resolve()
	switch value.Kind() {
	case slog.KindString:
		return slog.String(attr.Key, h.redactor.Redact(value.String()))
	case slog.KindAny:
		if err, ok := value.Any().(error); ok {
			return slog.String(attr.Key, h.redactor.Redact(err.Error()))
		}
	case slog.KindGroup:
		group := value.Group()
		scrubbed := make([]slog.Attr, 0, len(group))
		for _, nested := range group {
			scrubbed = append(scrubbed, h.scrubAttr(nested))
		}
		return slog.Attr{Key: attr.Key, Value: slog.GroupValue(scrubbed...)}
	}
	return attr
}

func (h *scrubHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	scrubbed := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		scrubbed = append(scrubbed, h.scrubAttr(attr))
	}
	return &scrubHandler{inner: h.inner.WithAttrs(scrubbed), redactor: h.redactor}
}

func (h *scrubHandler) WithGroup(name string) slog.Handler {
	return &scrubHandler{inner: h.inner.WithGroup(name), redactor: h.redactor}
}
