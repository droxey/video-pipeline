## Slide 1: Introduction

<!-- duration: 30.0 -->
<!-- sfx: chime-start -->

Event-Driven Architecture is a design paradigm where services communicate via events rather than direct calls.

## Slide 2: Traditional vs Event-Driven

<!-- duration: 45.0 -->

**Traditional (RPC):** Service A calls Service B directly.

**Event-Driven:** Service A emits an event to a message bus, and Service B subscribes.

## Slide 3: Benefits

<!-- duration: 60.0 -->
<!-- sfx: emphasis -->

Key benefits include loose coupling, scalability, and resilience. Each service handles only what it can, and a failed subscriber never crashes the producer.
