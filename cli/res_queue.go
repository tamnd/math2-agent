package cli

import (
	"cmp"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/mdrec"
	"github.com/tamnd/math2-agent/queue"
	"github.com/tamnd/math2-agent/resource"
	resscope "github.com/tamnd/math2-agent/resource/scope"
)

const resQueueUsage = "m2 res queue [-root DIR] [-n N] [-decide TARGET -value V -reason R -by P [-to ID]] <res-scope|res-cluster|res-resolve|res-structure>"

// resQueues gives the file of each queue of RS13.2.
var resQueues = map[string]string{
	"res-scope":     ResScopeQueue,
	"res-cluster":   ResClusterQueue,
	"res-resolve":   ResResolveQueue,
	"res-structure": "build/resources/queues/res-structure.md",
}

func runResQueue(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("m2 res queue", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "the math2 working tree")
	n := fs.Int("n", 50, "the most items to list. 0 means all")
	decide := fs.String("decide", "", "the target of the item to decide: a resource id for res-scope, two record keys or resource ids for res-cluster, or a reference for res-resolve")
	value := fs.String("value", "", "the decision: in, border, out or duplicate for res-scope; merge or split for res-cluster; found or not-found for res-resolve")
	to := fs.String("to", "", "the live resource of a duplicate, or the resource of a reference")
	reason := fs.String("reason", "", "why the person decided so")
	by := fs.String("by", "", "the person who decided")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 1 || resQueues[fs.Arg(0)] == "" {
		fmt.Fprintln(stderr, "usage: "+resQueueUsage)
		return ExitUsage
	}
	name := fs.Arg(0)
	if *decide != "" {
		d, err := queueDecision(name, *decide, *value, *to)
		if err != nil {
			fmt.Fprintf(stderr, "m2 res queue: %v\n", err)
			return ExitUsage
		}
		d.Reason, d.By, d.At = *reason, *by, time.Now().UTC().Format(time.DateOnly)
		if err := resscope.AppendDecision(*root, d); err != nil {
			fmt.Fprintf(stderr, "m2 res queue: %v\n", err)
			return ExitFailure
		}
		fmt.Fprintf(stdout, "wrote the decision %s %s of %s to %s\n", d.Kind, cmp.Or(d.Value, d.Target), d.ID, resscope.DecisionsFile)
		return ExitOK
	}
	items, err := readResQueue(filepath.Join(*root, resQueues[name]))
	if errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(stdout, "the queue %s is empty: %s does not exist\n", name, resQueues[name])
		return ExitOK
	}
	if err != nil {
		fmt.Fprintf(stderr, "m2 res queue: %v\n", err)
		return ExitFailure
	}
	ds, err := resscope.ReadDecisions(*root)
	if err != nil {
		fmt.Fprintf(stderr, "m2 res queue: %v\n", err)
		return ExitFailure
	}
	decided := map[string]bool{}
	for _, d := range ds {
		decided[strings.TrimSpace(d.ID+" "+d.Target)] = true
		decided[d.ID] = true
	}
	fmt.Fprintf(stdout, "%s: %d items\n", name, len(items))
	for i, it := range items {
		if *n > 0 && i == *n {
			fmt.Fprintf(stdout, "and %d more\n", len(items)-i)
			break
		}
		line := fmt.Sprintf("%.3f %s", it.Priority, it.Target)
		if it.Kind != "" {
			line += " [" + it.Kind + "]"
		}
		if it.Name != "" {
			line += " " + it.Name
		}
		if decided[it.Target] {
			line += " (decided, the next run takes it)"
		}
		fmt.Fprintln(stdout, line)
	}
	return ExitOK
}

// readResQueue reads the items of a queue file, the highest priority first.
func readResQueue(path string) ([]queue.Item, error) {
	recs, err := mdrec.Read(path)
	if err != nil {
		return nil, err
	}
	items, err := mdrec.Unmarshal[queue.Item](recs)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	slices.SortStableFunc(items, func(a, b queue.Item) int { return cmp.Compare(b.Priority, a.Priority) })
	return items, nil
}

// queueDecision makes the decision of a person on one item of a queue (RS13.2).
func queueDecision(name, target, value, to string) (resscope.PersonDecision, error) {
	var d resscope.PersonDecision
	switch name {
	case "res-scope":
		if !slices.Contains(resource.Outcomes, value) || value == resource.Deferred {
			return d, fmt.Errorf("the decision of res-scope is in, border, out or duplicate, not %q", value)
		}
		if (value == resource.Duplicate) != (to != "") {
			return d, errors.New("a duplicate needs -to with the live resource, and only a duplicate has -to")
		}
		return resscope.PersonDecision{ID: target, Kind: "scope", Value: value, Target: to}, nil
	case "res-cluster":
		a, b, ok := strings.Cut(strings.TrimSpace(target), " ")
		if !ok || (value != "merge" && value != "split") {
			return d, errors.New("a decision of res-cluster names two record keys or resource ids in -decide and has the value merge or split")
		}
		return resscope.PersonDecision{ID: a, Kind: value, Target: strings.TrimSpace(b)}, nil
	case "res-resolve":
		switch {
		case value == "found" && to != "":
			return resscope.PersonDecision{ID: target, Kind: "resolve", Value: value, Target: to}, nil
		case value == "not-found" && to == "":
			return resscope.PersonDecision{ID: target, Kind: "resolve", Value: value}, nil
		}
		return d, errors.New("a decision of res-resolve is found with -to and the resource, or not-found")
	}
	return d, fmt.Errorf("the items of %s are not decided here. A person adds the source of the structure to the resource", name)
}
