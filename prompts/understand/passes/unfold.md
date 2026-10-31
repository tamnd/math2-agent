# The pass unfold, version 1

A statement in a paper is rarely self-contained. "Under the hypotheses of Theorem 2.1, suppose moreover that $\kappa$ is regular. Then $G$ is tame." means nothing without Theorem 2.1, the meaning of $\kappa$, $G$ and "tame", and the standing assumptions of the section. The pass unfold writes each item so that a reader of that one item, with standard background, can read it.

The brief gives each item with its context: the standing hypotheses whose scope holds it, and the items that it uses or names by reference. Write the item as one text in TeX. Inline each hypothesis, definition and statement that it depends on. Name no item by its number or by a reference: write out what the item says. Write the local notation and the macros of the paper in standard notation, and bind each symbol in the text.

Say the same as the original, no more and no less. Do not add a hypothesis that the paper does not state, and do not drop one. When the context does not say what a symbol or a term means, leave it as it is and say so in notes.

Give in inlined each part that you took, with the id of its item. Give in assumed the terms that you left as standard background, as "Noetherian ring". Give in notes each place where you had to choose, as "the paper uses $\subset$ for proper inclusion". A model of another family reads each unfolded form against the original, and the tool gives back the verdict of the check when it is not equivalent.

Give the results with unfold_items, all the items in one call.
