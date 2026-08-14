## Slide 1

<!-- source: seg-0001 -->
<!-- approved-by: alice -->
<!-- approved-at: 2026-08-01T10:00:00Z -->

Welcome to the course on event-driven architecture. [PAUSE:0.5] Today we will explore how modern distributed systems communicate through events rather than direct calls.

## Slide 2

<!-- source: seg-0002, seg-0003 -->

Why not call the next service directly? [SFX:emphasis] Great question. Direct coupling creates hidden dependencies that make systems brittle and hard to scale.

## Slide 3

<!-- source: seg-0003 -->
<!-- approved-by: alice -->
<!-- approved-at: 2026-08-02T09:00:00Z -->

The benefits are significant. [PAUSE:0.3] First, loose coupling means teams can work independently. Second, scalability improves because services only process what they can handle. Third, resilience increases since a failed subscriber does not crash the producer.
