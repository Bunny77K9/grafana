package tracing

import (
	"context"
	"fmt"
	"time"

	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/setting"
	opentracing "github.com/opentracing/opentracing-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	trace "go.opentelemetry.io/otel/trace"
)

type Tracer interface {
	Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, Span)
	Run(context.Context) error
}

type Span interface {
	End()
	SetAttributes(kv ...attribute.KeyValue)
	// AddEvent(name string, options ...trace.EventOption)
}

var (
	GlobalTracer trace.Tracer
)

type OpentelemetryTracingService struct {
	enabled bool
	address string
	log     log.Logger

	tracerProvider *tracesdk.TracerProvider

	Cfg *setting.Cfg
}

type OpentelemetrySpan struct {
	span trace.Span
}

type OpentracingSpan struct {
	span opentracing.Span
}

func (ts *OpentelemetryTracingService) parseSettingsOpentelemetry() error {
	section, err := ts.Cfg.Raw.GetSection("tracing.opentelemetry.jaeger")
	if err != nil {
		return err
	}
	ts.enabled = section.Key("enabled").MustBool(false)

	return nil
}

func (ts *OpentelemetryTracingService) initTracerProvider() (*tracesdk.TracerProvider, error) {
	// Create the Jaeger exporter
	exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(ts.address)))
	if err != nil {
		return nil, err
	}

	tp := tracesdk.NewTracerProvider(
		// Always be sure to batch in production.
		tracesdk.WithBatcher(exp),
		// Record information about this application in an Resource.
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("grafana"),
			attribute.String("environment", "production"),
		)),
	)

	return tp, nil
}

func (ots *OpentelemetryTracingService) initOpentelemetryTracer() error {
	tp, err := ots.initTracerProvider()
	if err != nil {
		return err
	}
	// Register our TracerProvider as the global so any imported
	// instrumentation in the future will default to using it
	// only if tracing is enabled
	if ots.enabled {
		otel.SetTracerProvider(tp)
	}

	GlobalTracer = otel.GetTracerProvider().Tracer("component-main")

	return nil
}

func (ots *OpentelemetryTracingService) Run(ctx context.Context) error {
	<-ctx.Done()

	ots.log.Info("Closing tracing")
	ctxShutdown, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	if err := ots.tracerProvider.Shutdown(ctxShutdown); err != nil {
		return err
	}

	return nil
}

func (ts *OpentelemetryTracingService) Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, Span) {
	ctx, span := GlobalTracer.Start(ctx, spanName)
	oSpan := OpentelemetrySpan{span: span}
	return ctx, oSpan
}

func (s OpentelemetrySpan) End() {
	s.span.End()
}

func (s OpentelemetrySpan) SetAttributes(kv ...attribute.KeyValue) {
	s.span.SetAttributes(kv...)
}

func (ts *TracingService) Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, Span) {
	span, ctx := opentracing.StartSpanFromContext(ctx, spanName)
	oSpan := OpentracingSpan{
		span: span,
	}
	return ctx, oSpan
}

func (s OpentracingSpan) End() {
	s.span.Finish()
}

func (s OpentracingSpan) SetAttributes(kv ...attribute.KeyValue) {
	for k, v := range kv {
		s.span.SetTag(fmt.Sprint(k), v)
	}
}
