package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	"github.com/tamnd/math2-agent/api"
	"github.com/tamnd/math2-agent/api/m2pb"
)

// TestProto checks that m2.proto is the file that the table of methods makes. Give -update to write it again, and then run buf generate in api/.
func TestProto(t *testing.T) {
	got := api.Proto()
	if *update {
		if err := os.WriteFile("m2.proto", got, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile("m2.proto")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("m2.proto is not the file that the table of methods makes. Run make proto.")
	}
}

// TestProtoCode checks that the code in m2pb agrees with the table of methods, so that the code is not older than m2.proto.
func TestProtoCode(t *testing.T) {
	sd := m2pb.File_m2_proto.Services().ByName("ReadService")
	if sd == nil || string(sd.FullName()) != api.ProtoService {
		t.Fatalf("m2pb has no service %s", api.ProtoService)
	}
	ms := api.Methods()
	if sd.Methods().Len() != len(ms) {
		t.Errorf("m2pb has %d rpcs and the table has %d methods", sd.Methods().Len(), len(ms))
	}
	kinds := map[string]protoreflect.Kind{api.String: protoreflect.StringKind, api.Integer: protoreflect.Int32Kind, api.Boolean: protoreflect.BoolKind}
	for i, m := range ms {
		if i >= sd.Methods().Len() {
			break
		}
		md := sd.Methods().Get(i)
		rule, _ := proto.GetExtension(md.Options(), annotations.E_Http).(*annotations.HttpRule)
		if want := "/" + api.Version + "/" + m.Name; rule.GetGet() != want {
			t.Errorf("%s: the HTTP path is %q and not %q", md.Name(), rule.GetGet(), want)
		}
		fields := md.Input().Fields()
		if fields.Len() != len(m.Params) {
			t.Errorf("%s: %s has %d fields and the method has %d parameters", m.Name, md.Input().Name(), fields.Len(), len(m.Params))
			continue
		}
		for j, p := range m.Params {
			fd := fields.Get(j)
			behaviour, _ := proto.GetExtension(fd.Options(), annotations.E_FieldBehavior).([]annotations.FieldBehavior)
			required := len(behaviour) == 1 && behaviour[0] == annotations.FieldBehavior_REQUIRED
			if string(fd.Name()) != p.Name || fd.Kind() != kinds[p.Type] || required != p.Required || fd.HasPresence() != (p.Type != api.String && !p.Required) {
				t.Errorf("%s: the field %d is %s %s, required %v, presence %v, and the parameter is %+v", m.Name, j+1, fd.Kind(), fd.Name(), required, fd.HasPresence(), p)
			}
		}
	}
}

// dial serves the service over gRPC in memory and returns a client.
func dial(t *testing.T, opt ...grpc.ServerOption) *grpc.ClientConn {
	t.Helper()
	ln := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(opt...)
	open(t).RegisterGRPC(srv)
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return ln.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// TestGRPC makes each call of the contract that gRPC can make, and checks that the answer is the HTTP answer in testdata/contract.
func TestGRPC(t *testing.T) {
	conn := dial(t)
	sd := m2pb.File_m2_proto.Services().ByName("ReadService")
	grpcCodes := map[float64]codes.Code{400: codes.InvalidArgument, 404: codes.NotFound}
	covered := map[string]bool{}
	for _, c := range contract {
		if c.method == "" {
			continue
		}
		var md protoreflect.MethodDescriptor
		for i := range sd.Methods().Len() {
			if m := sd.Methods().Get(i); rpcPath(m) == "/"+api.Version+"/"+c.method {
				md = m
			}
		}
		if md == nil {
			continue
		}
		mt, err := protoregistry.GlobalTypes.FindMessageByName(md.Input().FullName())
		if err != nil {
			t.Fatal(err)
		}
		req := mt.New()
		if !fill(t, req, c.query, c.body) {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			var file struct {
				Status float64        `json:"status"`
				Body   map[string]any `json:"body"`
			}
			b, err := os.ReadFile(filepath.Join("testdata", "contract", c.name+".json")) // #nosec G304 -- the path is a test file.
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(b, &file); err != nil {
				t.Fatal(err)
			}
			resp := &m2pb.Response{}
			err = conn.Invoke(context.Background(), "/"+api.ProtoService+"/"+string(md.Name()), req.Interface(), resp)
			if file.Status != 200 {
				e, _ := file.Body["error"].(map[string]any)
				if status.Code(err) != grpcCodes[file.Status] || status.Convert(err).Message() != e["message"] {
					t.Errorf("the error is %v, and the HTTP error is %v %v", err, file.Status, e)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			covered[c.method] = true
			resp.Build = "BUILD"
			b, err = protojson.MarshalOptions{UseProtoNames: true}.Marshal(resp)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatal(err)
			}
			// proto3 does not send an empty list.
			for _, v := range []map[string]any{got, file.Body} {
				if a, _ := v["about"].([]any); len(a) == 0 {
					delete(v, "about")
				}
			}
			if !reflect.DeepEqual(round(got), file.Body) {
				t.Errorf("the gRPC answer is not the HTTP answer.\ngot:  %v\nwant: %v", got, file.Body)
			}
		})
	}
	for _, m := range api.Methods() {
		if !covered[m.Name] {
			t.Errorf("no gRPC call covers the method %s", m.Name)
		}
	}
}

// rpcPath returns the HTTP path of an rpc.
func rpcPath(md protoreflect.MethodDescriptor) string {
	rule, _ := proto.GetExtension(md.Options(), annotations.E_Http).(*annotations.HttpRule)
	return rule.GetGet()
}

// fill sets the fields of req to the parameters of the query or the body. It returns false when gRPC cannot send the parameters, for example a parameter that is not a field.
func fill(t *testing.T, req protoreflect.Message, query, body string) bool {
	t.Helper()
	params := map[string]any{}
	if body != "" {
		if err := json.Unmarshal([]byte(body), &params); err != nil {
			return false
		}
	} else {
		q, err := url.ParseQuery(query)
		if err != nil {
			t.Fatal(err)
		}
		for k, v := range q {
			params[k] = v[0]
		}
	}
	for k, v := range params {
		fd := req.Descriptor().Fields().ByName(protoreflect.Name(k))
		if fd == nil {
			return false
		}
		s, isString := v.(string)
		switch fd.Kind() {
		case protoreflect.Int32Kind:
			n, ok := v.(float64)
			if isString {
				i, err := strconv.Atoi(s)
				n, ok = float64(i), err == nil
			}
			if !ok {
				return false
			}
			req.Set(fd, protoreflect.ValueOfInt32(int32(n)))
		case protoreflect.BoolKind:
			b, ok := v.(bool)
			if isString {
				x, err := strconv.ParseBool(s)
				b, ok = x, err == nil
			}
			if !ok {
				return false
			}
			req.Set(fd, protoreflect.ValueOfBool(b))
		default:
			if !isString {
				return false
			}
			req.Set(fd, protoreflect.ValueOfString(s))
		}
	}
	return true
}

func TestLimitGRPC(t *testing.T) {
	client := m2pb.NewReadServiceClient(dial(t, grpc.UnaryInterceptor(api.LimitGRPC(api.Options{Rate: 1, Burst: 2}))))
	var got []codes.Code
	for range 3 {
		_, err := client.Status(context.Background(), &m2pb.StatusRequest{Ref: "Q529ES"})
		got = append(got, status.Code(err))
	}
	if !reflect.DeepEqual(got, []codes.Code{codes.OK, codes.OK, codes.ResourceExhausted}) {
		t.Errorf("the codes are %v, and they must be OK, OK, ResourceExhausted", got)
	}
}
