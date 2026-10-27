(* A toy hierarchy in the form of Hierarchy Builder. The text is ours. *)
From HB Require Import structures.

HB.mixin Record HasToyOp T := { toyop : T -> T -> T }.

HB.structure Definition ToyType := { T of HasToyOp T }.

HB.factory Record ToyFromNat T := { from_nat : nat -> T }.

HB.builders Context T of ToyFromNat T.
Definition built_op (x y : T) := x.
Lemma built_lemma : True. Proof. by []. Qed.
HB.instance Definition _ := HasToyOp.Build T built_op.
HB.end.

HB.lock Definition locked_op := 0.

Variant toy_spec : Type := ToySpec.

Inductive tree := Leaf | Node of tree & tree.

Record point := Point { px : nat; py : nat }.

Structure pack := Pack { sort :> Type }.

Proposition toy_prop : "a string with (* no comment"" in it" = "x" -> True.
Proof. by []. Qed.

Remark toy_remark : True. Proof. exact: I. Qed.

Canonical pack_nat := Pack nat.
