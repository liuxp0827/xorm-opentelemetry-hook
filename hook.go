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

type OpenTelemetryHook struct {
	tracer     trace.Tracer
	propagator propagation.TextMapPropagator
}

func NewOpenTelemetryHook(tp trace.TracerProvider) *OpenTelemetryHook {
	propagator := propagation.NewCompositeTextMapPropagator(Metadata{}, propagation.Baggage{}, propagation.TraceContext{})
	otel.SetTracerProvider(tp)
	tracer := otel.Tracer("db")
	return &OpenTelemetryHook{
		tracer:     tracer,
		propagator: propagator,
	}
}

func WrapEngine(e *xorm.Engine, tp trace.TracerProvider) {
	e.AddHook(NewOpenTelemetryHook(tp))
}

func WrapEngineGroup(eg *xorm.EngineGroup, tp trace.TracerProvider) {
	eg.AddHook(NewOpenTelemetryHook(tp))
}

func (h *OpenTelemetryHook) start(c *contexts.ContextHook) (context.Context, trace.Span) {
	operation := fmt.Sprintf("%v %v", c.SQL, c.Args)
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
