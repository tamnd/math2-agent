# Third-party proto files

The files in `google/api/` come from [googleapis/googleapis](https://github.com/googleapis/googleapis) at the commit `dff1cd0c7ff8c26ff4c9f7a87cbe175fb5124d4e`, with no change. Their licence is the Apache License 2.0, as in each file. `api/m2.proto` imports them for the HTTP annotations and the field behaviour. The Go code for them is in the module `google.golang.org/genproto/googleapis/api`, thus `buf generate` makes no Go code from them.
