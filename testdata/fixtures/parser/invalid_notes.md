## Slide 1: Introduction to Go

Narrator: Missing approval metadata entirely.

## Slide 2: Setting Up

Narrator: Good line with full approval.
<!-- provenance: human -->
<!-- approved-by: alice@nebula.com -->
<!-- approved-at: 2024-03-01T10:00:00Z -->

Narrator: Bad provenance value.
<!-- provenance: robot-overlord -->
<!-- approved-by: carol@nebula.com -->
<!-- approved-at: 2024-03-01T10:05:00Z -->

Narrator: Bad timestamp.
<!-- provenance: human -->
<!-- approved-by: dave@nebula.com -->
<!-- approved-at: not-a-timestamp -->

## Slide 3: Different Title

<!-- unknown-key: foo -->

Narrator: Orphaned meta before dialogue line – next line triggers the error.
<!-- approved-by: eve@nebula.com -->
<!-- provenance: human -->
<!-- approved-at: 2024-03-01T10:10:00Z -->

This line is unrecognised (not dialogue, not meta, not blank).
