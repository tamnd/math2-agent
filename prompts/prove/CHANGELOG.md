# The prove role

## 0.1.1

The style guide tells the model to unfold the statement before a tactic that needs an instance of its proposition, such as `decide`, because instance search does not unfold a definition. It also says that a job can permit `native_decide`, as the first message tells.

## 0.1.0

The first version. The model works on one pinned statement, a definition of a proposition, in the work tree of the run, with the read methods of the API, the files of the tree and the Lean check. The run succeeds when the binding of the target names a theorem whose type is the statement, the library builds, the proof uses only the allowed axioms, and the closure of the statement is not changed.
