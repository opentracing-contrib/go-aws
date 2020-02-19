package otaws

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"net/http"

	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
)

// AddOTHandlers adds open tracing handlers to aws client
//
// Deprecated: use AddOTHandlersToClient instead
func AddOTHandlers(cl *client.Client, opts ...Option) {
	addOTHandlers(&cl.Handlers, opts...)
}

// AddOTHandlersToClient adds open tracing handlers to aws client
func AddOTHandlersToClient(cl *client.Client, opts ...Option) {
	addOTHandlers(&cl.Handlers, opts...)
}

// AddOTHandlersToSession adds open tracing handlers to aws session.
// Open tracing handlers will be propagated to all clients using this session.
func AddOTHandlersToSession(sess *session.Session, opts ...Option) {
	addOTHandlers(&sess.Handlers, opts...)
}

func addOTHandlers(h *request.Handlers, opts ...Option) {
	c := defaultConfig()
	for _, opt := range opts {
		opt(c)
	}

	handler := otHandler(c)
	h.Build.PushFront(handler)
}

func otHandler(c *config) func(*request.Request) {
	tracer := c.tracer

	return func(r *request.Request) {
		var sp opentracing.Span

		ctx := r.Context()
		if ctx == nil || !opentracing.IsGlobalTracerRegistered() {
			sp = tracer.StartSpan(r.Operation.Name)
		} else {
			sp, ctx = opentracing.StartSpanFromContext(ctx, r.Operation.Name)
			r.SetContext(ctx)
		}
		ext.SpanKindRPCClient.Set(sp)
		ext.Component.Set(sp, "go-aws")
		ext.HTTPMethod.Set(sp, r.Operation.HTTPMethod)
		ext.HTTPUrl.Set(sp, r.HTTPRequest.URL.String())
		ext.PeerService.Set(sp, r.ClientInfo.ServiceName)

		_ = inject(tracer, sp, r.HTTPRequest.Header)

		r.Handlers.Complete.PushBack(func(req *request.Request) {
			if req.HTTPResponse != nil {
				ext.HTTPStatusCode.Set(sp, uint16(req.HTTPResponse.StatusCode))
			} else {
				ext.Error.Set(sp, true)
			}
			sp.Finish()
		})

		r.Handlers.Retry.PushBack(func(req *request.Request) {
			sp.LogFields(log.String("event", "retry"))
		})
	}
}

func inject(tracer opentracing.Tracer, span opentracing.Span, header http.Header) error {
	return tracer.Inject(span.Context(), opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(header))
}
