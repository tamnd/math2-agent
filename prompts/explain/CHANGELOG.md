# The explain role

## 0.1.0

The first version. The model writes levelled explanations into the Explanations section of the target node, each under a heading such as "### L2 agent", and it can add example nodes. The run succeeds when each explanation cites nodes of the tree, the cited nodes of an L1 or L2 explanation are in the curricula of the level, and the grounding check in checks/ground.md finds each sentence supported by the cited nodes. A sentence that the cited nodes contradict fails the run.
