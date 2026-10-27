# The exercise role

## 0.2.0

The role writes the form of node.v2 in a tree at node.v2: a node of the kind exercise, with the fields about, difficulty, math_group and check, and the sections Exercise, Hint, Answer and Solution (corpus K8.7). In a tree at node.v1, it writes the form of 0.1.0. The brief gives the form of the tree, and the system prompt says that the brief gives it. The goal accepts both forms, and the field permission set of the role allows the new fields. The role does not write the field external.

## 0.1.0

The first version. The model writes exercises for one node, as example nodes whose Remarks section opens with the line of the role and has the headings Hint, Answer and Solution (corpus K2.3 and K8.7). The run succeeds when the changes are inside the field permission set of exercise, m2 check and the house style find no problem in the new nodes, each exercise is about the target, has a task, a difficulty from 1 to 5, a group of the MATH taxonomy, a check other than lean, an answer unless a person checks it and a solution in numbered steps, and the run adds the number of exercises that the job asks for. Then a solver of another family answers each task, and each exercise that it does not solve goes to a person in queues/exercises.jsonl. The model register has no model of another family yet, thus each exercise goes to a person.
