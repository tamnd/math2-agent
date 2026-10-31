# The understand role

## 0.4.0

The pass unfold of tier T3 (corpus arxiv AX7.5), with its prompt in passes/unfold.md and the prompt of its check in checks/unfold.md. A job reads each statement, definition, conjecture and problem with its context, the standing hypotheses whose scope holds it and the items that it uses or names by reference from the edges plane, and gives its self-contained form in TeX with unfold_items. The tool checks that the inlined items are items of the paper, that the form names no item by its number and that its macros are standard. A model of another family gives a verdict; a verdict other than equivalent goes back to the model once, and a second one, and each unfolding while math2 has no checker of another family, goes to the queue unfold-review. A record of the class verbatim keeps only the hash, the inlined ids and the verdict, and a paper of the class record has no unfolding (AQ08).

## 0.3.0

The pass prose of tier T2 (corpus arxiv AX7.4), with its prompt in passes/prose.md and the prompt of its check in checks/prose.md. A job reads the running text that ax prose of arxiv-reader prints, as numbered sentences with the cues of step 1, and gives the prose items of each section as ranges of sentence numbers with a kind, with give_prose. A model of another family checks each item; an item that the check does not accept, and each item while math2 has no checker of another family, goes to the queue prose-review.

## 0.2.0

Each pass has its prompt in passes/PASS.md, which comes at the start of the brief, and the schema of the input of its tool in schema/understand/PASS.v1.json of math2-agent. The system prompt has only the rules of each pass. A second result of an item that is not valid gives the item a record with the mark model-failed: PASS, and the job goes on (corpus arxiv AX7.1).

## 0.1.0

The first version, with the passes classify and concepts of tier T1 (corpus arxiv AX7.2 and AX7.3). A job is one pass on one arXiv paper. Classify gives the item kind of each object that the extractor could not place, and of each remark and problem, with a confidence and the flags claim-in-remark and conjecture-candidate. Concepts gives the MSC2020 codes and the concepts of each item, with a Wikidata item or a name with the flag new-concept, and the terms of a definition. The tool writes one record for each item to records/understand/arxiv/PASS/SHARD.jsonl. The run succeeds when each item of the pass has a record of the run and the changes are inside the field permission set of understand.
