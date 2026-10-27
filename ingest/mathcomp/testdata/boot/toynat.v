(* A small file in the form of the library, for the tests of the adapter.
   The text is ours. (* A nested comment with a Lemma fake : True. *) *)
From mathcomp Require Import ssreflect.

Set Implicit Arguments.

Definition double n := n + n.

Fixpoint half n := if n is n'.+2 then (half n').+1 else 0.

Section Doubling.

Variable m : nat.

Lemma double0 : double 0 = 0. Proof. by []. Qed.

Lemma doubleS n :
  double n.+1 = (double n).+2.
Proof. by rewrite /double addnS addSn. Qed.

Let local_fact : True. Proof. by []. Qed.

#[local] Lemma hidden_twice : double 1 = 2.
Proof. by []. Qed.

End Doubling.

Notation "n .*2" := (double n) (at level 2, format "n .*2") : nat_scope.

Module Toy.
Module Export Inner.
Theorem half_double n : half (double n) = n.
Proof. elim: n => // n IHn. Qed.
End Inner.
Corollary half0 : half 0 = 0. Proof. by []. Qed.
End Toy.

Module Alias := Toy.

Fact double_fact : double 2 = 4. Proof. by []. Qed.
