# The curate role

## 0.3.0

Curate also turns an example of the record plane into an example node, an exercise into an exercise node (an example node with the role line in a tree at node.v1, corpus K8.7) and a proof into a proof node. The goal checks the form of an exercise: about, difficulty, group, check, the task, the answer and a solution in numbered steps. A proof node needs the field proves, and when the graph has no node of the statement, the model reports that it is stuck. The brief and the system prompt no longer give an example provisional id, because the model used the same id in different runs. The model now picks eight random hex digits. The brief gives the path of the file of a node, because in the batch the model did not find the file of a node that it had to add a source to.

## 0.2.0

When a node of the matching kind states the result of a record already, the model adds the record as a source at the end of the sources of that node, and the judge must find that the node states the same result. Before, the model reported that it was stuck. A new node also gets the URL of the record in its source, so that the inventory rule textbook-node finds the node from the locator of a textbook mention.

## 0.1.0

The first version. The model turns a statement or a definition of the record plane into a new statement or concept node in its own words, or it fixes the fields uses, about, msc, name and aliases of a node and proposes duplicates. The run succeeds when m2 check finds nothing in the changed nodes. For a record, the new node must name the locator of the record in its sources, must not copy the text of the record, and the judge in checks/judge.md must find that it states the same result as the record.
