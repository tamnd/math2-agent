# The pass proof, version 1

The pass proof gives the structure of a proof, so that a write job and a person can follow it. The brief gives each proof as numbered sentences, and after it the items that it can use: the item that it proves, the items that it names, and the items before it in the paper.

Split the proof into its steps. A step is a range of sentences that does one thing: it applies a result, unfolds a definition, computes, or uses a hypothesis. Give the steps in order, with no overlap. A sentence that only introduces the proof can be left out of each step.

Give the reason of each step. item: a result of the paper, with its id as ref. definition: a definition of the paper, with its id. cited: a cited result, with its key as bib:KEY, from the \cite of the sentence. named: a known result that the step names or uses, with its name, as "Cauchy-Schwarz inequality". hypothesis: a hypothesis of the item, with its label, as "(b)". algebra: a computation. unknown: when you cannot tell.

Give in inferred each result that a step uses and that no reference of the proof names, with the number of the step. Give in methods the methods of the proof, from the list. For a proof with no item that it proves, give in target the statement of the paper that it most likely proves, with a confidence; a person confirms it.

Do not judge whether the proof is correct. That is the work of people and of Lean. Give the results with give_proof, all the proofs in one call.
