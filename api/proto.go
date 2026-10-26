package api

import (
	"bytes"
	"fmt"
	"strings"
)

// ProtoService is the full name of the gRPC service of the read methods in api/m2.proto.
const ProtoService = "m2." + Version + ".ReadService"

// Proto returns the text of api/m2.proto, the interface definition of the read methods for gRPC. It is made from the table of methods, thus it agrees with the HTTP handler and the MCP server. Each method is an rpc with the name of the method in camel case, for example GetFormal for get_formal. Its request has one field for each parameter, in the order of the parameters. A required parameter has the field behaviour REQUIRED, and an integer or a boolean that is not required has explicit presence, so that a missing value gets its default. The result of each rpc is a Response with the result as a google.protobuf.Value, the JSON value of the HTTP answer.
func Proto() []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, `// The read methods of the Mathematical API of math2 (spec 11 of tamnd/math2-agent).
// This file is made from the table of methods in methods.go. Do not edit it. To make it again, run make proto.

syntax = "proto3";

package m2.%s;

import "google/api/annotations.proto";
import "google/api/field_behavior.proto";
import "google/protobuf/struct.proto";

option go_package = "github.com/tamnd/math2-agent/api/m2pb";

// ReadService has the read methods. Each method is also the HTTP endpoint GET /%s/{method}, an MCP tool and an m2 subcommand with the same name.
service ReadService {
`, Version, Version)
	for i, m := range methods {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "  // %s\n  rpc %s(%sRequest) returns (Response) {\n    option (google.api.http) = {get: \"/%s/%s\"};\n  }\n", m.Summary, camel(m.Name), camel(m.Name), Version, m.Name)
	}
	b.WriteString("}\n")
	for _, m := range methods {
		fmt.Fprintf(&b, "\n// %sRequest has the parameters of %s.\nmessage %sRequest {\n", camel(m.Name), m.Name, camel(m.Name))
		for i, p := range m.Params {
			if i > 0 {
				b.WriteString("\n")
			}
			typ := map[string]string{String: "string", Integer: "int32", Boolean: "bool"}[p.Type]
			if !p.Required && p.Type != String {
				typ = "optional " + typ
			}
			opt := ""
			if p.Required {
				opt = " [(google.api.field_behavior) = REQUIRED]"
			}
			fmt.Fprintf(&b, "  // %s\n  %s %s = %d%s;\n", sentence(p.Summary), typ, p.Name, i+1, opt)
		}
		b.WriteString("}\n")
	}
	b.WriteString(`
// Response is the answer to one call. It has the fields of the JSON answer of the HTTP API.
message Response {
  // The version of the API.
  string api = 1;

  // The name of the method.
  string method = 2;

  // The commit of the tree that the build read.
  string commit = 3;

  // The output hash of the build.
  string build = 4;

  // Each node in the result, in the order of the result.
  repeated About about = 5;

  // The result, as the JSON value of the HTTP answer.
  google.protobuf.Value result = 6;
}

// About is a node that a response is about, as the build saw it.
message About {
  // The tag of the node.
  string tag = 1;

  // The content hash of the node file.
  string hash = 2;

  // The evidence level, or the empty string when the node has none.
  string evidence = 3;
}
`)
	return b.Bytes()
}

// camel returns the name of a method in camel case, for example GetFormal for get_formal.
func camel(name string) string {
	var b strings.Builder
	for w := range strings.SplitSeq(name, "_") {
		b.WriteString(upper(w))
	}
	return b.String()
}

// sentence returns s with its first letter in upper case and a full stop at the end.
func sentence(s string) string {
	s = upper(s)
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}

// upper returns s with its first letter in upper case.
func upper(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
