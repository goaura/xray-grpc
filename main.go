package xray_grpc

import (
	"context"
	"errors"

	"github.com/aws/aws-xray-sdk-go/header"
	"github.com/aws/aws-xray-sdk-go/xray"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

const (
	GrpcMethod      = "POST"
	CustomUserAgent = "Vendrive-gRPC-XRAY-Interceptor"
)

// Returns a UnaryClientInterceptor that supports populating gRPC metadata with AWS XRAY information.
// Parameter hostFromTarget allows you to translate the grpc.ClientConn target into your preferred outbound
// server name. DNS Information, URL, gRPC error codes, and Content Length are currently not supported.
// Usage:
//
// customHostFromTarget = func (target string) string {
//     withoutPort := target[:strings.IndexByte(target, ':')]
//	   return strings.ReplaceAll(withoutPort, ".my-namespace.local", "")
// }
//
// conn, err := grpc.Dial("my-service.my-namespace.local:3000",
//                        grpc.WithInsecure(),
//                        grpc.WithUnaryInterceptor(xray_grpc.NewGrpcXrayUnaryClientInterceptor(customHostFromTarget)))
//
func NewGrpcXrayUnaryClientInterceptor(hostFromTarget func(string) string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, resp interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {

		// Retrieve the host (subsegment name) from the connection target
		host := hostFromTarget(cc.Target())

		// Copied from XRAY SDK
		err := xray.Capture(ctx, host, func(ctx context.Context) error {
			seg := xray.GetSegment(ctx)

			// If no segment is found, continue on
			if seg == nil {
				return invoker(ctx, method, req, resp, cc, opts...)
			}

			// TODO: Implement httptrace equivalent (DNS Lookup, etc)

			seg.Lock()

			// gRPC is always POST
			seg.GetHTTP().GetRequest().Method = GrpcMethod
			// TODO: Populate URL

			// Populate Metadata for the gRPC server, see https://github.com/grpc/grpc-go/blob/master/Documentation/grpc-metadata.md
			ctx = metadata.AppendToOutgoingContext(ctx, xray.TraceIDHeaderKey, seg.DownstreamHeader().String())

			seg.Unlock()

			err := invoker(ctx, method, req, resp, cc, opts...)
			// Naive Status Codes
			seg.Lock()
			if err != nil {
				seg.GetHTTP().GetResponse().Status = 400
			} else {
				seg.GetHTTP().GetResponse().Status = 200
			}
			// TODO: Populate Content Length
			seg.Unlock()

			return err
		})

		return err
	}
}

// Returns a UnaryServerInterceptor that supports reading gRPC metadata that contains AWS XRAY information.
// Intended to recieve requests from a gRPC client that uses NewGrpcXrayUnaryClientInterceptor. Currently only
// supports NewFixedSegmentNamer for parameter sn. Populating URL, gRPC error codes, and Content Length in segments
// are currently not supported.
// Usage:
//
// s := grpc.NewServer(grpc.UnaryInterceptor(xray_grpc.NewGrpcXrayUnaryServerInterceptor(xray.NewFixedSegmentNamer("my-service"))))
//
func NewGrpcXrayUnaryServerInterceptor(sn xray.SegmentNamer) grpc.UnaryServerInterceptor {
	return grpc.UnaryServerInterceptor(func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {

		// Only supports NewFixedSegmentNamer
		name := sn.Name("only NewFixedSegmentNamer is supported")

		// See https://github.com/grpc/grpc-go/blob/master/Documentation/grpc-metadata.md
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, errors.New("unable to read metadata")
		}

		traceString := ""
		if traceHeaderValueList, ok := md[xray.TraceIDHeaderKey]; ok {
			// Assume Metadata Key only has one value
			if len(traceHeaderValueList) > 0 {
				traceString = traceHeaderValueList[0]
			}
		}
		traceHeader := header.FromString(traceString)

		// Copy Segment creation from XRAY SDK: https://github.com/aws/aws-xray-sdk-go/blob/master/xray/segment.go
		ctx, seg := xray.NewSegmentFromHeader(ctx, name, nil, traceHeader)
		defer seg.Close(nil)

		seg.Lock()

		// gRPC RPC's are always POST
		seg.GetHTTP().GetRequest().Method = GrpcMethod

		p, ok := peer.FromContext(ctx)
		if ok {
			seg.GetHTTP().GetRequest().ClientIP = p.Addr.String()
		}
		seg.GetHTTP().GetRequest().UserAgent = CustomUserAgent
		// TODO: Populate URL

		// Handle Request
		seg.Unlock()
		resp, err := handler(ctx, req)
		seg.Lock()

		// Naive Status Codes
		if err != nil {
			seg.GetHTTP().GetResponse().Status = 400
		} else {
			seg.GetHTTP().GetResponse().Status = 200
		}
		// TODO: Populate Content Length
		seg.Unlock()

		return resp, err
	})
}
