# Security policy

## Supported versions

Only the latest release gets security fixes.

## Report a problem

Do not open a public issue for a security problem. Use [private vulnerability reporting](https://github.com/tamnd/math2-agent/security/advisories/new) on GitHub.

Include these items in the report:

1. The version of `m2`, or the commit.
2. The steps that cause the problem.
3. What an attacker can do.

You get a reply within 7 days.

## Scope

These items are in scope:

1. The `m2` binary and its release archives.
2. The sandbox for Lean, for computations and for agent tools. Lean metaprograms can run code at elaboration time, so a sandbox escape is a security problem.
3. A way to make the build give a status that the evidence does not support. An example is a proof that the kernel stack accepts for a false statement.
4. Prompt injection from source text that makes an agent write outside its role.

A kernel soundness bug in Lean itself goes to the Lean project. Tell us too, because each release records its checker versions and we must grade the affected results again.
