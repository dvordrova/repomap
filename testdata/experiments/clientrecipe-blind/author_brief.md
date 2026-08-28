# Blind repository author brief

Build a small standalone Go order-fulfilment service that makes live production
calls to five external systems:

- payment authorization;
- tax quotation;
- address verification;
- parcel booking;
- receipt delivery.

Three integrations are fully maintained. Two deliberately omit different
hardening or verification behaviours. The executable must reach all five
through real application logic. Use a distinct idiomatic design for each
integration instead of cloning one template.

Maintained integrations translate deployment settings, own vendor SDK
construction behind application-facing APIs, are assembled into startup, are
behaviourally tested, and record attempts and failures. Some, but not all,
contain retry or timeout behaviour.

Add at least six realistic non-production lookalikes drawn from at least four
independently chosen categories. Everything must compile and test offline
through local module stubs.

Choose package, file, type, function, and directory names and layouts yourself.
Return only the repository and its local stub dependencies. Do not create an
oracle, inspect an extractor, or run the repository through an analyzer.
