# The explore role

## 0.1.0

The first version. The model writes computations, runs them in the sandbox with the tool run_computation, looks up each claim with the tool lookup, and files conjecture and counterexample nodes. The run succeeds when m2 check finds nothing in the changed nodes, each conjecture links to a computation and each counterexample refutes a statement through one, each new computation verifies, and the novelty check ran on each output. The judge of the novelty check is in checks/judge.md.
