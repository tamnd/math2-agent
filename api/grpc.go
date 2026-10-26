package api

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	grpcpeer "google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/tamnd/math2-agent/api/m2pb"
)

// grpcCodes are the gRPC codes of the codes of an Error.
var grpcCodes = map[string]codes.Code{Invalid: codes.InvalidArgument, NotFound: codes.NotFound, RateLimited: codes.ResourceExhausted, InternalError: codes.Internal}

// RegisterGRPC registers the service ReadService of api/m2.proto on r, for example a *grpc.Server. Each rpc calls the method of the same name with the fields of the request that are set, and gives the Response of Call with the result as a google.protobuf.Value. Thus a number of the result is a double. An Error of Call is a gRPC status with the code InvalidArgument, NotFound, ResourceExhausted or Internal, and its message. Package m2pb has the client.
func (s *Service) RegisterGRPC(r grpc.ServiceRegistrar) {
	sd := m2pb.File_m2_proto.Services().ByName("ReadService")
	desc := grpc.ServiceDesc{ServiceName: ProtoService, HandlerType: (*any)(nil), Metadata: m2pb.File_m2_proto.Path()}
	for _, m := range methods {
		md := sd.Methods().ByName(protoreflect.Name(camel(m.Name)))
		if md == nil {
			panic("api: m2.proto has no rpc for the method " + m.Name + ". Run make proto.")
		}
		mt, err := protoregistry.GlobalTypes.FindMessageByName(md.Input().FullName())
		if err != nil {
			panic("api: " + err.Error())
		}
		info := &grpc.UnaryServerInfo{Server: s, FullMethod: "/" + ProtoService + "/" + string(md.Name())}
		call := func(_ context.Context, req any) (any, error) {
			return s.callProto(m.Name, req.(proto.Message))
		}
		desc.Methods = append(desc.Methods, grpc.MethodDesc{
			MethodName: string(md.Name()),
			Handler: func(_ any, ctx context.Context, dec func(any) error, ic grpc.UnaryServerInterceptor) (any, error) {
				req := mt.New().Interface()
				if err := dec(req); err != nil {
					return nil, err
				}
				if ic == nil {
					return call(ctx, req)
				}
				return ic(ctx, req, info, call)
			},
		})
	}
	r.RegisterService(&desc, s)
}

// callProto runs the method with the fields of req that are set.
func (s *Service) callProto(method string, req proto.Message) (*m2pb.Response, error) {
	params := map[string]any{}
	req.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch fd.Kind() {
		case protoreflect.Int32Kind:
			params[string(fd.Name())] = int(v.Int())
		case protoreflect.BoolKind:
			params[string(fd.Name())] = v.Bool()
		default:
			params[string(fd.Name())] = v.String()
		}
		return true
	})
	resp, err := s.Call(method, params)
	if err != nil {
		var e *Error
		if !errors.As(err, &e) {
			e = &Error{Code: InternalError, Message: err.Error()}
		}
		return nil, status.Error(grpcCodes[e.Code], e.Message)
	}
	b, err := json.Marshal(resp.Result)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	result := &structpb.Value{}
	if err := protojson.Unmarshal(b, result); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	out := &m2pb.Response{Api: resp.API, Method: resp.Method, Commit: resp.Commit, Build: resp.Build, Result: result}
	for _, a := range resp.About {
		out.About = append(out.About, &m2pb.About{Tag: a.Tag, Hash: a.Hash, Evidence: a.Evidence})
	}
	return out, nil
}

// LimitGRPC returns an interceptor that gives each client the rate limit of opt, as Limit does over HTTP. The key of a client is the IP address of the peer. A client over the limit gets the code ResourceExhausted and the header retry-after with the seconds to wait.
func LimitGRPC(opt Options) grpc.UnaryServerInterceptor {
	lim := newLimiter(opt)
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if lim == nil {
			return handler(ctx, req)
		}
		client := ""
		if p, ok := grpcpeer.FromContext(ctx); ok && p.Addr != nil {
			client = host(p.Addr.String())
		}
		if wait, ok := lim.allow(client, time.Now()); !ok {
			_ = grpc.SetHeader(ctx, metadata.Pairs("retry-after", strconv.Itoa(int(math.Ceil(wait.Seconds())))))
			return nil, status.Error(codes.ResourceExhausted, "too many requests. Wait and try again")
		}
		return handler(ctx, req)
	}
}
