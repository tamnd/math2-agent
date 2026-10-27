(* A theory in the form of the AFP, for the tests of the adapter. The text is ours.
   (* A nested comment with lemma fake: "True" *) *)
section \<open>A toy theory\<close>

theory Toy
  imports Main
begin

text \<open>A text block with a lemma inside: lemma not_this: "True". And a
  nested \<open>cartouche\<close> too.\<close>

definition double :: "nat \<Rightarrow> nat" where
  "double n = n + n"

lemma double_zero [simp]: "double 0 = 0"
  by (simp add: double_def)

lemma "double 1 = 2"
  by (simp add: double_def)

theorem double_add:
  shows "double (a + b) = double a + double b"
  by (simp add: double_def)

fun half :: "nat \<Rightarrow> nat" where
  "half 0 = 0" | "half (Suc 0) = 0" | "half (Suc (Suc n)) = Suc (half n)"

function (sequential) count :: "nat \<Rightarrow> nat" where
  "count n = n"
  by auto

datatype 'a tree = Leaf | Node "'a tree" 'a "'a tree"

datatype ('a, 'b) pair2 = Pair2 'a 'b

record point =
  px :: nat
  py :: nat

locale monoid_toy =
  fixes f :: "'a \<Rightarrow> 'a \<Rightarrow> 'a" (infixl "\<cdot>" 70)
  assumes assoc: "(a \<cdot> b) \<cdot> c = a \<cdot> (b \<cdot> c)"
begin

lemma assoc_again: "(a \<cdot> b) \<cdot> c = a \<cdot> (b \<cdot> c)"
  by (rule assoc)

end

lemma (in monoid_toy) assoc_twice: "((a \<cdot> b) \<cdot> c) \<cdot> d = a \<cdot> (b \<cdot> (c \<cdot> d))"
  by (simp add: assoc)

context monoid_toy
begin

corollary in_context: "a \<cdot> b = a \<cdot> b"
  by simp

end

context
  fixes x :: nat
begin

proposition anonymous_context: "x = x"
  by simp

end

experiment
begin

lemma hidden_lemma: "True"
  by simp

end

inductive even_toy :: "nat \<Rightarrow> bool" where
  "even_toy 0" | "even_toy n \<Longrightarrow> even_toy (Suc (Suc n))"

lemma double_zero: "double 0 = 0"
  by simp

end
