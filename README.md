# gRPC Interceptors for AWX X-Ray Tracing

Provides gRPC Unary Client and Server Interceptors for AWX X-Ray that play nicely with the AWS X-Ray SDK (and existing AWS X-Ray HTTP wrappers)

## Usage

For both Client and Server Interceptors, setup the AWS X-Ray SDK normally:

```
xray.Configure(xray.Config{
    ...
})
```

and import the `xray-grpc` module:

```
import "github.com/vendrive/xray-grpc"
```

Both Client and Server Interceptors use the AWS X-Ray SDK, and support most features. Check `main.go` (code is minimal) if you are curious if your use case is supported.

**Note**: Populating URL, gRPC error codes, and Content Length in segments are currently not supported.

### gRPC Unary Client

```
// Converts the grpc.ClientConn target (first parameter passed to grpc.Dial below) into your preferred outbound server name
customHostFromTarget = func (target string) string {
    withoutPort := target[:strings.IndexByte(target, ':')]
    return strings.ReplaceAll(withoutPort, ".my-namespace.local", "")
}

conn, err := grpc.Dial("my-service.my-namespace.local:3000",
                       grpc.WithInsecure(),
                       grpc.WithUnaryInterceptor(xray_grpc.NewGrpcXrayUnaryClientInterceptor(customHostFromTarget)))
```

### gRPC Unary Server

```
// NewFixedSegmentNamer is the only segment namer currently supported
s := grpc.NewServer(grpc.UnaryInterceptor(xray_grpc.NewGrpcXrayUnaryServerInterceptor(xray.NewFixedSegmentNamer("my-service"))))
```

## Resources
- https://github.com/aws/aws-xray-sdk-go/
- https://github.com/grpc/grpc-go/blob/master/Documentation/grpc-metadata.md