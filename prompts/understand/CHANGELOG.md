# The understand role

## 0.2.0

Each pass has its prompt in passes/PASS.md, which comes at the start of the brief, and the schema of the input of its tool in schema/understand/PASS.v1.json of math2-agent. The system prompt has only the rules of each pass. A second result of an item that is not valid gives the item a record with the mark model-failed: PASS, and the job goes on (corpus arxiv AX7.1).

## 0.1.0

The first version, with the passes classify and concepts of tier T1 (corpus arxiv AX7.2 and AX7.3). A job is one pass on one arXiv paper. Classify gives the item kind of each object that the extractor could not place, and of each remark and problem, with a confidence and the flags claim-in-remark and conjecture-candidate. Concepts gives the MSC2020 codes and the concepts of each item, with a Wikidata item or a name with the flag new-concept, and the terms of a definition. The tool writes one record for each item to records/understand/arxiv/PASS/SHARD.jsonl. The run succeeds when each item of the pass has a record of the run and the changes are inside the field permission set of understand.
