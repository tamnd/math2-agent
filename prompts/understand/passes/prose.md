# The pass prose, version 1

The pass prose gives the items that a paper states in its running text, outside the theorem environments: a definition ("We say that a module is tame if ..."), a conjecture ("We conjecture that the bound is sharp"), a problem ("It is not known whether ..."), a claim ("Note that the map is surjective, since ..."), or a standing hypothesis ("Throughout, X is Noetherian").

The brief gives the running text of each section as numbered sentences. A cue marks a sentence that a pattern found. A cue can be wrong, and an item can have no cue: read each sentence. Give each item as the numbers of its first and its last sentence, with its kind. Give a definition the term that it defines, as the sentence writes it. A definition of a term that the paper uses once is still an item.

A sentence that only cites a result, names a section or tells what the paper does is not an item. When you are unsure of the kind, give the kind that you find most likely: a model of another family reads each item again, and a person reads the items on which the two answers differ.

Give the results with give_prose, with an empty list for a section with no item.
