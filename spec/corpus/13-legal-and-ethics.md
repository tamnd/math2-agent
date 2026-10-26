# K13. Law and ethics

The corpus takes text from thousands of sources and has models write most of the rest. Both are legal questions, and both are questions of trust. This document says what the law says on the day of this spec, what math2 does because of it, and which questions need a lawyer before a phase starts (K13.10).

This document is not legal advice. It is the reading of the sources of K01, part D, by people who are not lawyers. Each fact has its source and the date of reading in K01. Where the law is not settled, this document says so and chooses the safe course.

## K13.1 The three zones

Settled decision 17 gives three licence zones. The corpus keeps them, and puts each new kind of content in one zone:

| Content | Zone | Licence |
|---|---|---|
| prose of nodes, explanations, glue of volumes, text of exercises | prose | CC BY-SA 4.0 |
| Lean code, the Lean statement and the reference solution of an exercise, the code of computation nodes, the Lean companion of a volume | code | Apache-2.0 |
| front matter, mentions, inventory rows, outcomes, the notation register, the conventions register, the JSON-LD of the site, records | metadata | CC0 |

A volume has the licences of its parts (K11.7). A dataset release has a manifest that gives the zone of each file.

## K13.2 Copyright in text that models write

**The United States.** The Copyright Office, in Part 2 of its report on AI (29 January 2025), says that copyright does not extend to material that is purely AI-generated, or where a person had insufficient control over the expressive elements. It says that prompts alone do not give enough control. A person has copyright in what they wrote, and in their creative selection, arrangement or modification of the outputs. The courts agree that the Act needs a human author: the D.C. Circuit held so in *Thaler v. Perlmutter* (18 March 2025), and the Supreme Court denied certiorari on 2 March 2026.

**What this means for math2.**

1. A section that a model wrote and no person changed may have no copyright in the United States. The CC BY-SA licence on it may then have no effect: section 8(a) of the licence says that it does not limit uses that need no permission. Mathematical facts, definitions and short statements have thin protection in any case.
2. What people add has copyright: the text that people write or edit, and the selection, the arrangement and the curation of the corpus. CC BY-SA applies to these.
3. Other jurisdictions answer the question differently, and some have not answered it. For example, the United Kingdom has a rule for "computer-generated works" (CDPA s.9(3)). K01 did not research these.

**The decision.** The prose zone stays CC BY-SA 4.0 for all prose, with a notice in `LICENCE`:

> The licence applies to the extent that copyright subsists in the material. Some sections were written by software and not changed by a person. These sections may be in the public domain in some jurisdictions. The provenance of each section says who or what wrote it.

The other option was CC0 for prose that a model wrote and no person confirmed, and CC BY-SA after a person confirms it. Creative Commons itself suggests CC0 for works with no significant human creativity. The corpus does not take this option, because it makes the licence of a section depend on its provenance, and the zone of a section would change when a person confirms it. The notice tells the truth, and the provenance shows which sections it covers. A change to this decision is a change to settled decision 17.

## K13.3 The real risk: copies

Who owns model output is a small question for math2. The large question is whether model output copies someone else's text. A model that reproduces a textbook proof gives math2 the textbook's text, with the textbook's copyright.

- **Training cases are about inputs.** *Bartz v. Anthropic* (N.D. Cal.) held in June 2025 that training on lawfully acquired books was fair use, and that a library of pirated copies was not. The case settled, with final approval on 20 July 2026. In *Kadrey v. Meta* (June 2025) the court found fair use on that record. In *Thomson Reuters v. Ross* (3d Cir., 29 September 2026), the court held that copying headnotes to train a search tool that does not generate text was not fair use, and said that generative AI is a different case. None of these cases decides the status of outputs.
- **Output cases.** *NYT v. OpenAI* (S.D.N.Y.) is the main case in the United States on regurgitated output, and it has no decision on the merits. In Germany, *GEMA v. OpenAI* (LG München I, 11 November 2025) held that the memorization of song lyrics in a model is a reproduction, and that the output of the lyrics is one too. It held OpenAI liable for the outputs, and OpenAI appealed. In the United Kingdom, *Getty Images v. Stability AI* ([2025] EWHC 2863 (Ch), 4 November 2025) held that the weights of a model do not store an infringing copy, and an appeal is pending. The Court of Justice of the EU heard *Like Company v. Google* (C-250/25), on chatbot output that matches press articles, in March 2026, and a judgment is expected in 2027.
- **Who is exposed.** Whoever publishes a copy is exposed, not only the provider of the model.

**What math2 does.**

1. **The copy check** (K9.4) runs on each body that an agent wrote, against a reference corpus and not only the cited sources. A match with a restricted source blocks the merge.
2. **No restricted text in the context** of a write job (K7.5). A model that rewrites a text that it can see makes a derived work.
3. **Matches with open sources are attributed.** A close match with a CC BY-SA source keeps that source's terms, whatever math2 says, so the node records the attribution (rule L03).
4. **GFDL, NC and all-rights-reserved text never enters the prose zone.** GFDL text, for example from the Stacks Project, cannot be relicensed under CC BY-SA 4.0. Kerodon states no licence, so its text is all rights reserved. Non-commercial text cannot be in a corpus that anyone may sell.
5. **Takedown.** A claim that a node copies a work goes to the curators as a report of the property "ours to publish" (K9.8). The node is quarantined within two working days, while a person checks the claim.

## K13.4 Human sources

The corpus takes from human sources by the licence class of each source (K05):

| Class | What math2 may hold | Example |
|---|---|---|
| `open` | the text, with attribution, when the licence is compatible with CC BY-SA 4.0 | Wikipedia (CC BY-SA), set.mm (CC0), Mathlib docstrings (Apache-2.0) |
| `share-alike-nc` | records, locators and links. No text | OpenStax (CC BY-NC-SA), the WeBWorK Open Problem Library (CC BY-NC-SA 3.0) |
| `gfdl` | records, locators and links. No text | the Stacks Project, the Manifold Atlas |
| `verbatim` | short quotations where the law of quotation allows them, with the source | a famous statement in its original words |
| `record` | records and locators only | most textbooks and papers, Kerodon, the DLMF |

Three further points apply to each class:

- **Facts are free, words are not.** That a theorem is true, what it says and where it appears are facts. The words of a source are its author's. A node that states a standard theorem in math2's words takes nothing that copyright protects.
- **Collections and databases.** The selection and the arrangement of a collection can have copyright, and in the EU a database can have a database right against the extraction of a substantial part. So an importer takes from a catalogue only what the terms of the catalogue allow, and the source document (K05) records those terms. A table of contents goes into the record plane as locators for each item, not as a copy of the table.
- **Licences per item.** Some sources have a licence for each entry, for example the entries of the Archive of Formal Proofs, or the posts of Math.StackExchange by date. The importer records the licence of each item, not of the source.

## K13.5 The EU AI Act

- **Duties of providers.** The duties for providers of general-purpose AI models (Articles 53 to 55) apply from 2 August 2025, and the Commission can enforce them from 2 August 2026. They fall on the providers of models. math2 uses models and is not their provider. A team that changes a model becomes a provider only if the change uses more than a third of the training compute of the original. A fine-tune of an open prover by math2 would be far below that.
- **Transparency (Article 50).** It applies from 2 August 2026. Providers of systems that generate text must mark the outputs in a machine-readable form (Article 50(2)). The Digital Omnibus on AI (Regulation (EU) 2026/1744) reportedly gives this duty a grace period to 2 December 2026, and sources disagree on its scope. Deployers that publish text generated by AI to inform the public on matters of public interest must disclose that it is generated (Article 50(4)). The duty does not apply when the text had human review or editorial control and a person holds editorial responsibility for the publication.
- **What math2 does.** It is not settled whether a mathematics library informs the public "on matters of public interest". math2 meets the duty either way. Each field that an agent wrote is marked as such (rule P04), the site puts a machine-readable flag on each field that no person confirmed, and each area has a named editor who holds editorial responsibility (spec 16).

## K13.6 The terms of the providers

K7.9 gives the rules that follow from the terms. In short:

- The terms of Anthropic, OpenAI and Google give the customer the rights in the outputs that the provider has. None of them stops publication under an open licence.
- From 12 November 2026, the usage policy of Anthropic forbids the use of outputs to train an AI model without its prior authorization. OpenAI and Google forbid the use of outputs to build competing models. math2 does not train on the output of hosted models without written authorization.
- Some open-weight licences add duties on outputs. The model register (K7.7) records them.
- **Users of math2.** A person who downloads math2 under CC BY-SA has no contract with the provider of a model. It is not settled whether a provider's terms reach that person, and math2 claims nothing either way. The release notes say which provider wrote which fields, so that users can decide for themselves.

## K13.7 Disclosure

The disclosure of agent work is generated and complete (settled decision 24):

- **Each field.** The provenance names the role, the model and the run of each field that an agent wrote, and the person who confirmed it, if any.
- **Each page.** The site shows "written by software, not yet checked by a person" on each unconfirmed field, in words and in the machine-readable flag of K13.5.
- **Each volume.** The attribution page lists people first, by their contributions, and then the disclosure of agent runs (K11.7).
- **Each release.** The release notes give the share of fields that agents wrote, by layer and kind, and the share of those that a person confirmed.

## K13.8 Ethics

Four duties are not in the law, and the corpus keeps them:

1. **Do not flood the commons.** Wikipedia adopted a speedy-deletion criterion for unreviewed pages that a model generated (G15, August 2025). arXiv asked that review and position papers in computer science be peer-reviewed first (October 2025). Communities object to unreviewed machine text in bulk, not to the use of AI as such. math2 never writes agent text into the sources that it imports from (spec 16.5), and it never presents an unconfirmed section as reviewed.
2. **Credit to the people whose work the corpus uses.** Each node names the sources that its row has, even where the licence does not require it. A mathematician whose result is in the corpus is named with the result (K10.6).
3. **Honest numbers.** The error rates of K9.7 are published with their intervals, including the rates that are bad. A count that goes down is explained in the release notes (rule R03).
4. **Be polite to services.** Importers keep the rate limits and the robots rules of each source, identify themselves with the User-Agent of the toolchain (with no personal address), and prefer a published dump to crawling.

## K13.9 Rules

| Rule | H/S | Checks |
|---|---|---|
| L06 | H | each item from a source with a licence for each item records the licence of the item (K13.4) |
| L07 | S | a node with a takedown report that is older than two working days and not quarantined |

## K13.10 Questions for a lawyer

Each of these needs advice before the phase that the table names:

| Question | Before |
|---|---|
| Is the notice of K13.2 enough, or does each section that a model wrote need its own notice? | the first release with agent prose (phase 2) |
| Does the database right of the EU limit the import of catalogues such as zbMATH, the OEIS and MathSciNet, beyond their terms? | phase 1 |
| Do the terms of a provider reach users of math2 who have no contract with it? | the first dataset release |
| Is the publication of a corpus of outputs an "availability to third parties" under the Qwen 3.8 and Kimi K3 licences? | any use of these models |
| Which licence governs DeepSeek-Prover-V2: the MIT file or the Model Agreement that its README names? | any use of its outputs for training |
| Is a mathematics library within Article 50(4)? | not needed: math2 meets the duty either way |
| What is the position in the United Kingdom and the EU on the copyright of output that a model generated? | phase 3 |
