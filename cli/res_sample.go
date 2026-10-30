package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/resource/census"
	"github.com/tamnd/math2-agent/resource/census/fetch"
	resscope "github.com/tamnd/math2-agent/resource/scope"
)

const (
	resSampleUsage = "m2 res sample -catalogue NAME -dir DIR [-n N] [-seed N] [-arxiv DIR] [-gap D]"
	resLookupUsage = "m2 res lookup -dir DIR [-n N] [-seed N] [-against A,B] [-env FILE] [-gap D]"
)

func runResSample(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 res sample", flag.ContinueOnError)
	fs.SetOutput(stderr)
	name := fs.String("catalogue", "", "the catalogue: "+strings.Join(fetch.Names(fetch.Samplers("")), ", "))
	dir := fs.String("dir", "", "the directory of the census records. The sample goes to <catalogue>.jsonl in it.")
	n := fs.Int("n", 10000, "the size of the sample")
	seed := fs.Uint64("seed", 1, "the seed of the sample")
	axDir := fs.String("arxiv", "", "the metadata plane of a checkout of tamnd/arxiv, for the catalogue arxiv")
	gap := fs.Duration("gap", time.Second, "the shortest time between two requests")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	s, ok := fetch.Samplers(*axDir)[*name]
	if fs.NArg() != 0 || !ok || *dir == "" || *n < 1 {
		fmt.Fprintln(stderr, "usage: "+resSampleUsage)
		return ExitUsage
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	var buf bytes.Buffer
	count := 0
	info, err := s.Sample(ctx, fetch.NewClient(*gap), *n, *seed, func(r census.Record) error {
		count++
		data, err := json.Marshal(r)
		if err != nil {
			return err
		}
		buf.Write(data)
		buf.WriteByte('\n')
		return nil
	})
	if err != nil {
		fmt.Fprintf(stderr, "m2 res sample: %v\n", err)
		return ExitFailure
	}
	out := filepath.Join(*dir, *name+".jsonl")
	if err := writeOut(out, buf.Bytes()); err != nil {
		fmt.Fprintf(stderr, "m2 res sample: %v\n", err)
		return ExitFailure
	}
	fmt.Fprintf(stdout, "%s: %d records match %s; wrote %d records to %s\n", *name, info.Total, info.Filter, count, out)
	return ExitOK
}

func runResLookup(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 res lookup", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", "", "the directory of the census records, one file <catalogue>.jsonl for each catalogue")
	n := fs.Int("n", 5000, "the in records of each catalogue to look up")
	seed := fs.Uint64("seed", 1, "the seed of the choice of the records")
	against := fs.String("against", strings.Join(fetch.Names(fetch.Lookers()), ","), "the catalogues to look in")
	envFile := fs.String("env", "", "a JSON file with the registers of the scope tests")
	gap := fs.Duration("gap", time.Second, "the shortest time between two requests")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 0 || *dir == "" || *n < 1 {
		fmt.Fprintln(stderr, "usage: "+resLookupUsage)
		return ExitUsage
	}
	all := fetch.Lookers()
	var lookers []fetch.Looker
	for _, a := range strings.Split(*against, ",") {
		l, ok := all[strings.TrimSpace(a)]
		if !ok {
			fmt.Fprintf(stderr, "m2 res lookup: no lookup for the catalogue %q\n", a)
			return ExitUsage
		}
		lookers = append(lookers, l)
	}
	env, err := readEnv(*envFile)
	if err != nil {
		fmt.Fprintf(stderr, "m2 res lookup: %v\n", err)
		return ExitFailure
	}
	files, err := filepath.Glob(filepath.Join(*dir, "*.jsonl"))
	if err != nil || len(files) == 0 {
		fmt.Fprintf(stderr, "m2 res lookup: %s has no file <catalogue>.jsonl\n", *dir)
		return ExitFailure
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	c := fetch.NewClient(*gap)
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".jsonl")
		rs, err := readRecords(f)
		if err != nil {
			fmt.Fprintf(stderr, "m2 res lookup: %v\n", err)
			return ExitFailure
		}
		for i := range rs {
			rs[i].Found = nil
		}
		k, err := fetch.Mark(ctx, c, rs, env, name, lookers, *n, *seed)
		if err != nil {
			fmt.Fprintf(stderr, "m2 res lookup: %s: %v\n", name, err)
			return ExitFailure
		}
		var buf bytes.Buffer
		for _, r := range rs {
			data, err := json.Marshal(r)
			if err != nil {
				fmt.Fprintf(stderr, "m2 res lookup: %v\n", err)
				return ExitFailure
			}
			buf.Write(data)
			buf.WriteByte('\n')
		}
		if err := writeOut(f, buf.Bytes()); err != nil {
			fmt.Fprintf(stderr, "m2 res lookup: %v\n", err)
			return ExitFailure
		}
		var in []string
		for _, l := range lookers {
			if l.Name() != name {
				in = append(in, l.Name())
			}
		}
		slices.Sort(in)
		fmt.Fprintf(stdout, "%s: looked up %d records in %s\n", name, k, strings.Join(in, ", "))
	}
	return ExitOK
}

func readEnv(path string) (resscope.Env, error) {
	var env resscope.Env
	if path == "" {
		return env, nil
	}
	data, err := os.ReadFile(path) // #nosec G304 -- the user names the file.
	if err != nil {
		return env, err
	}
	return env, json.Unmarshal(data, &env)
}

func readRecords(path string) ([]census.Record, error) {
	f, err := os.Open(path) // #nosec G304 -- the user names the directory.
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []census.Record
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 64<<20)
	for n := 1; sc.Scan(); n++ {
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		var r census.Record
		dec := json.NewDecoder(bytes.NewReader(sc.Bytes()))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&r); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, n, err)
		}
		out = append(out, r)
	}
	return out, sc.Err()
}

func writeOut(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil { // #nosec G306 -- census records are public metadata.
		return err
	}
	return os.Rename(tmp, path)
}
