---
paper: "2601.00002"
version: v2
title: 'Degrees in finite graphs'
authors:
  - B. Writer
primary_category: math.CO
categories:
  - math.CO cs.DM
msc_class: "05C07"
access: share-alike
licence: CC-BY-SA-4.0
licence_of_source: cc-by-sa
licence_from: abs
section: 1
section_title: Degrees
kind: section
lang: en
tag: S201
local_id: s1
path: render
---

A graph here is finite and has no loops.

**Definition 1** {#def-1 .definition tag=D201 env=definition}

The degree of a vertex $v$ is the number of edges that have $v$ as an end.

**Lemma 2** (Handshake) {#lem-1 .statement tag=L201 env=lemma}

The sum of the degrees of the vertices of a graph is twice the number of its edges.

**Proof of [Lemma 2](#lem-1)** {#proof-1 .proof tag=P201}

Each edge has two ends, so each edge adds $2$ to the sum in [Definition 1](#def-1).

**Theorem 3** {#thm-1 .statement tag=T201 env=theorem}

In each graph the number of vertices of odd degree is even.

[Corollary 4](#cor-1) in [Section 2](#s2) is a special case.

**Proof.** {#proof-2 .proof tag=P202}

The sum in [Lemma 2](#lem-1) is even. The vertices of even degree add an even number to it, so the vertices of odd degree add an even number too.
