package hook

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"xorm.io/xorm"
	"xorm.io/xorm/contexts"
)

// Option is tracing option.
type Option func(*options)

type options struct {
	tracerProvider trace.TracerProvider
	propagator     propagation.TextMapPropagator
}

// WithPropagator with tracer propagator.
func WithPropagator(propagator propagation.TextMapPropagator) Option {
	return func(opts *options) {
		opts.propagator = propagator
	}
}

// WithTracerProvider with tracer provider.
// Deprecated: use otel.SetTracerProvider(provider) instead.
func WithTracerProvider(provider trace.TracerProvider) Option {
	return func(opts *options) {
		opts.tracerProvider = provider
	}
}

type OpenTelemetryHook struct {
	tracer trace.Tracer
	opt    *options
}

func NewOpenTelemetryHook(opts ...Option) *OpenTelemetryHook {
	opt := options{
		propagator: propagation.NewCompositeTextMapPropagator(Metadata{}, propagation.Baggage{}, propagation.TraceContext{}),
	}
	for _, o := range opts {
		o(&opt)
	}
	if opt.tracerProvider != nil {
		otel.SetTracerProvider(opt.tracerProvider)
	}

	tracer := otel.Tracer("db")
	return &OpenTelemetryHook{
		tracer: tracer,
		opt:    &opt,
	}
}

func WrapEngine(e *xorm.Engine, opts ...Option) {
	e.AddHook(NewOpenTelemetryHook(opts...))
}

func WrapEngineGroup(eg *xorm.EngineGroup, opts ...Option) {
	eg.AddHook(NewOpenTelemetryHook(opts...))
}

func (h *OpenTelemetryHook) start(c *contexts.ContextHook) (context.Context, trace.Span) {
	operation := fmt.Sprintf("SQL: %v %v", c.SQL, c.Args)
	return h.tracer.Start(c.Ctx,
		operation,
		trace.WithSpanKind(trace.SpanKindClient),
	)
}

func (h *OpenTelemetryHook) BeforeProcess(c *contexts.ContextHook) (context.Context, error) {
	return trace.ContextWithSpan(h.start(c)), nil
}

func (h *OpenTelemetryHook) AfterProcess(c *contexts.ContextHook) error {
	var (
		span  = trace.SpanFromContext(c.Ctx)
		tn    = time.Now()
		attrs = make([]attribute.KeyValue, 0)
	)

	if c.ExecuteTime > 0 {
		attrs = append(attrs, attribute.Key("execute_time_ms").String(c.ExecuteTime.String()))
	}
	attrs = append(attrs, attribute.Key("args").String(fmt.Sprintf("%v", c.Args)))
	attrs = append(attrs, attribute.Key("sql").String(fmt.Sprintf("%v %v", c.SQL, c.Args)))
	attrs = append(attrs, attribute.Key("go.orm").String("xorm"))

	if c.Err != nil {
		span.RecordError(c.Err, trace.WithTimestamp(tn))
	}
	span.SetAttributes(attrs...)
	span.End(trace.WithStackTrace(true), trace.WithTimestamp(tn))
	return nil
}
