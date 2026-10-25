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
section: 2
section_title: Divisors
kind: section
lang: en
tag: S102
local_id: s2
path: source
---

All numbers in this section are integers.

**Definition 2.1** {#def-1 .definition tag=D101 env=definition}

We say that $d$ divides $n$, and we write $d \mid n$, when $n = dk$ for some integer $k$.

**Lemma 2.2** {#lem-1 .statement tag=L101 env=lemma}

If $d \mid a$ and $d \mid b$, then $d \mid a + b$.

**Proof.** {#proof-1 .proof tag=P101}

By [Definition 2.1](#def-1) there are integers $x$ and $y$ with $a = dx$ and $b = dy$. Then
$$
a + b = d(x + y),
$$
{#eq-1 .equation tag=E101}
and $x + y$ is an integer.

**Remark 2.3** {#rem-1 .remark tag=R101 env=remark}

The converse of [Lemma 2.2](#lem-1) is false: $2$ divides $1 + 1$ and does not divide $1$.

**Proposition 2.4** {#prop-1 .statement tag=Q101 env=proposition}

If $d \mid a$, then for each integer $m$
$$
d \mid ma.
$$
{#eq-2 .equation tag=E102}
In particular $d \mid -a$.

**Proof of [Proposition 2.4](#prop-1)** {#proof-2 .proof tag=P102}

Write $a = dx$ as in [Definition 2.1](#def-1). Then $ma = d(mx)$. The same step as in [(1)](#eq-1) gives the rest, and [Section 1](#s1) explains why we need it.
