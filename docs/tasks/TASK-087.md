---
id: TASK-087
type: task
title: Implement Ingress Path Normalization and Unhosted Root Protection
status: completed
version: 1.0

project: PROJECT-001
owner: development-lead

created: 2026-09-08
updated: 2026-09-08

depends_on:
  - REQ-085

owns:
  - pkg/ingress/translator.go

references:
  - REQ-085
  - ADR-080
---

# TASK-087 - Implement Ingress Path Normalization and Unhosted Root Protection

## Overview

Enforce strict path canonicalization and disallow unhosted root route registration in `TranslateIngress` in `pkg/ingress/translator.go`.

## Scope & Implementation Breakdown

1. **Path Canonicalization**:
   - Clean incoming `pathRule.Path` with `path.Clean` to eliminate `..` traversal segments and duplicate slashes.
   - Ensure paths start with `/`.
2. **Unhosted Root Enforcement**:
   - If canonical prefix is `/` or empty and `host == ""`, reject the rule.
   - Allow root `/` if an explicit non-empty `host` (e.g. `api.company.com`) is specified.
