---
paper: "2601.00001"
version: v1
title: 'Small facts about divisibility'
authors:
  - A. Writer
primary_category: math.NT
categories:
  - math.NT
msc_class: "11A05 (Primary) 11A41"
access: open
licence: CC-BY-4.0
licence_of_source: cc-by
licence_from: abs
section: A
section_title: Appendix
kind: section
lang: en
tag: S104
local_id: a1
path: source
---

**Proof of [Theorem 3.2](#thm-1)** {#proof-4 .proof tag=P104}

Suppose that $p_1, \dots, p_k$ are all the primes, and let $N = p_1 \cdots p_k + 1$. Some prime $q$ divides $N$, by [Lemma A.1](#lem-2). Then $q$ is some $p_i$, so $q$ divides $N - p_1 \cdots p_k = 1$ by [Lemma 2.2](#lem-1) and [(2)](#eq-2). This is false, because $q > 1$ by [Definition 3.1](#def-2).

**Hilfssatz A.1** {#lem-2 .statement tag=L102 env=hilfssatz}

Each integer $n > 1$ has a prime divisor.

**Proof.** {#proof-5 .proof tag=P105}

Take the smallest divisor $d > 1$ of $n$. A divisor of $d$ that is more than $1$ also divides $n$, so it is $d$. Thus $d$ is prime as in [Definition 3.1](#def-2).

**Theorem A.2** {#thm-2 .statement env=theorem}

Each integer $n > 1$ is a product of primes.
