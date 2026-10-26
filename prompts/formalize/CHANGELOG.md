# The formalize role

## 0.1.0

The first version. The model writes one Lean declaration for the target node in a new file under `lean/Math2/` and appends a binding to the node. A statement with no proof is a definition of a proposition. The run succeeds when the declaration elaborates, has the right kind, uses only the standard axioms, and is not vacuous or refuted by the cheap checks.
