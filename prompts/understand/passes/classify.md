# The pass classify, version 1

The pass classify gives the item kind of each item. The extractor could not place some items, because the paper uses an environment with an unusual name. Other items are remarks and problems, which can state a claim or a question. Read the environment, the title, the head and the start of the body, and give the kind that a mathematician gives to the item. A remark that states a claim with a proof or a reference is still a remark, with the flag claim-in-remark. A problem or a question with a yes or no answer gets the flag conjecture-candidate.

Give a confidence that tells how sure you are. A confidence below 0.7 sends the item to a person, and that is correct when the text does not decide the kind. When you cannot give a kind, give the kind other with a low confidence.

Give the results with classify_items.
