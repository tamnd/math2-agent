# Fixture nodes

These are the fixture sets of spec 19 M0. The tests of the `schema`, `check`, `format` and `cli` packages use them. `go test ./check` runs each broken node against the valid tree and expects exactly the planted rule and line.

- `valid/` is a math2 working tree with 117 nodes and no error. It has at least 5 nodes of each of the 11 kinds. The topics are group theory, number theory, real analysis, linear algebra, graph theory and proof methods. Each reference resolves inside the tree. Two nodes have provisional ids, and two nodes are retired. Each file is in canonical form.
- `aliases/` has 3 nodes that refer to nodes of `valid/` by alias, and `registers/aliases.reg`. The aliases are in top-level fields, in the nested fields of an `implies` edge and in link targets. Some links are in code, and m2 fmt must not change them. `aliases/want/` has the output of m2 fmt for each node. Before m2 fmt, m2 check gives S01 and B02 findings for the aliases. After m2 fmt, it gives no findings.
- `broken/` has 200 nodes. Each one is a copy of a valid node with a new tag and one planted error. `broken/expect.yaml` gives the rule that catches the error and the line of the error. The links and the references of a broken node point to nodes in `valid/`. Thus check a broken node together with the valid tree.

The number of broken nodes for each rule:

| Rule | Nodes | Examples of planted errors |
|---|---|---|
| S01 | 40 | a missing name, an unknown field, a bad MSC code, a YAML error, an anchor, a tag field that is not the file name |
| S02 | 25 | a definition with no defines, a statement with no role, a proof with no proves |
| S03 | 25 | a field such as status, verified or fidelity |
| S04 | 4 | a file larger than 64 KB |
| B01 | 36 | an unknown command, `\newcommand`, a brace that is not closed, a command in only one of KaTeX and MathJax |
| B02 | 30 | a link to a URL or to a dead tag, a URL in the Statement section |
| B03 | 25 | an unknown section, sections in the wrong order, a missing required section |
| G01 | 15 | a reference to a tag that is not in the tree |

When you add a broken node, add its entry to `expect.yaml`. Plant one error only, so that the node gives exactly one finding.
