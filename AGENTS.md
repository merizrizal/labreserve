# AGENTS.md

## Sources of truth

There are sources of truth for this project:
- the product requirements documentation 
- the knowledge base or baseline
- the architecture
- the codebase

Read the product requirements documentation before making product decisions.
Product requirements documentation are usually named as:
- prd.md, or
- product-requirements.md,
- or something like that

## Task scope

Work only on the explicitly approved task.

Do not implement future requirements merely because the architecture
describes them.

Do not scaffold speculative modules, schemas, APIs, or infrastructure.

## Development workflow

Work on one approved task at a time.

For an approved implementation task:

1. Inspect the relevant requirements, architecture, and existing code.
2. Establish the relevant current verification baseline where practical.
3. Implement the task.
4. Add or update appropriate tests.
5. Run focused verification.
6. Diagnose and repair failures caused by the change.
7. Run the required repository verification.
8. Inspect the complete final diff.
9. Remove accidental or unrelated changes.
10. Report evidence and remaining limitations.

Routine implementation, testing, and in-scope repairs do not require
additional human permission.

Do not ask for permission between routine implementation,
testing, and in-scope repair steps.

## Escalate when

Stop and report when:

- product requirements materially conflict or are insufficient;
- an approved architectural constraint would need to change;
- correct completion requires expanding the approved task scope;
- required access or tooling is unavailable;
- proceeding would weaken a security or safety boundary.

## Scope discipline

Do not implement features merely because they may be useful later.

Do not silently change product requirements.

Do not push, merge, deploy, or modify credentials unless
explicitly instructed.

Do not overwrite unrelated work.

Never commit secrets or local `.env` files.

## Verification

Do not claim a check passed unless it actually ran successfully.

A passing existing test suite does not prove that every acceptance
property is covered. Add focused regression coverage when a task
introduces or repairs an important invariant.

Do not weaken tests merely to produce a passing result.

## Review

Implementation and independent review should use separate fresh agent
sessions.

An implementation agent may self-check its work, but that does not
replace independent review.

Review findings must be reproduced or substantiated before repair.
Do not blindly change code merely to satisfy a reviewer.
