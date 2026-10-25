# Contributing to math2-agent

## Before you start

1. Find the issue for the work. If there is no issue, open one first.
2. Write a comment on the issue to say that you do the work. This prevents two people doing the same work.
3. Read the milestone tracking issue. Do not start work for a later milestone before the current milestone closes, unless a maintainer agrees.

## Make a change

1. Make a branch from `main`.
2. Keep each pull request to one change.
3. Run `make test` and `make lint`. Both must pass.
4. Add a test for each change in behaviour.
5. Open a pull request. Fill in the template.

## Rules for code

1. Do not add a command that exits with code 0 when it does nothing. A command that is not written exits with code 3 and names its milestone.
2. Do not add a dependency without a reason in the pull request. Each dependency is code that we must check.
3. Pin each GitHub Action to a full commit SHA, with the tag in a comment.
4. Give each workflow and each job the smallest permissions that it needs.

## AI tools

You can use AI tools to write code. You must say so in the pull request. You must understand each line that you submit, and you must be able to explain it without the tool. A maintainer can close a pull request that the author cannot explain.

Agents that run from this toolchain open pull requests from a bot account. Each such pull request links to its run record.

## Commit messages

Write the first line in the imperative, for example "Add tag minting". Keep it to 72 characters or fewer. Write the body as full sentences, and do not break a sentence across lines.
