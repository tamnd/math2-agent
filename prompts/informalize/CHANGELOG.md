# The informalize role

## 0.1.0

The first version. The model writes the node of a declaration of Mathlib that no person described: a statement node for a theorem or a proposition, and a definition or concept node for another definition, with its name, its main section, its uses and one exact binding to the declaration with no fidelity reviews. When a node binds the declaration already, the model writes its sections. The run succeeds when the changes are inside the field permission set of informalize, m2 check finds no problem in the changed nodes, one node binds the declaration, its kind fits the declaration, its name is not a Lean name, it has the main section of its card, its uses have each node that binds a constant of the type, a definition names its term in bold, it names no option of a convention other than the default, and the run changed no section that a person wrote or confirmed. The F1 battery checks the pair after the run.
