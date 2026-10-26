# Lean conventions of math2

- A binding names a declaration by its full name, for example `Subgroup.card_subgroup_dvd_card`, with no `@` and no arguments.
- To see the type of a declaration, elaborate `#check @NAME` with lean_check. The `@` shows the implicit arguments, and these are part of the statement.
- Mathlib names a theorem by its conclusion. A rename often moves a word, for example `card_subgroup_dvd_card` to `card_dvd_card`, or moves the declaration to another namespace.
- A deprecated declaration is an alias of its replacement. The replacement has the same type, thus the match of the binding stays the same.
- When the new declaration has a hypothesis more or a weaker conclusion than the node, the match of the binding changes, and bump does not change the match. Do not rebind it. Call report_stuck and say what changed.
