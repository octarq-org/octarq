## Description

Briefly describe the change and its rationale.

Fixes #(issue)

---

## User Path (required)

- User: ___ (who, in what situation)
- Path: ___ (from entering the product to getting the result)
- Evidence: ___ (screenshot / log / curl output / e2e link)
- Cut or deferred by this change: ___

Write `infra` in the User column for a change with no user-visible behaviour.

---

## Type of Change

- [ ] Bug fix (non-breaking change which fixes an issue)
- [ ] New feature (non-breaking change which adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Refactor / Code quality enhancement
- [ ] Documentation update

---

## Pre-Merge Checklist

- [ ] My code follows the code style and conventions of this project.
- [ ] I have not added backwards compatibility fallbacks or soft degradation shims (Pre-v1.0 Fail-Closed).
- [ ] All user-facing strings adhere to ASD-STE100 guidelines.
- [ ] I have updated the corresponding module `SPEC.md` if behavior or schema changed.
- [ ] I have added tests that prove my fix or feature works.
- [ ] New and existing tests pass locally with my changes.
- [ ] Linters pass without warnings.
