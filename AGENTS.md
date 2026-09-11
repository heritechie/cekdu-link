# cekdu-link — OpenCode Project Instructions

## Project Overview

`cekdu-link` is a **standalone generic short-link service**.

It provides simple short URLs that redirect to destination URLs with basic click tracking.

Example:

```text
short.domain/motor-sept
```

redirects to a destination URL.

The service is **domain-agnostic**. It must NOT contain client-specific business concepts such as:

* campaigns
* partners
* users
* organizations
* tenants
* any other business entity owned by a consuming platform

It is **not** a Bitly clone and **not** a standalone URL-shortener SaaS with metering/billing.

Primary value:

```text
Short Link → Redirect → Click Count
```

---

## Consumers

`cekdulu.co.id` is one client/consumer of this service. The relationship is:

```text
Consuming Platform
        │
        │ API
        ▼
   cekdu-link
        │
        ▼
   PostgreSQL
```

The same service could be consumed by another application without changing its domain model.

Do not model consumer-specific concepts in the database or API. Consumers may map links to their own business objects outside this service (e.g., in their own database keyed by `code` or link `id`).

---

## Product Positioning

Always treat `cekdu-link` as a **generic short-link infrastructure component**.

Do not expand the product into a generic public URL-shortening SaaS unless explicitly instructed.

The primary use cases are:

* Short link creation
* Redirect
* Basic click tracking
* Simple link management

---

## MVP Scope

Keep the MVP intentionally small.

The initial scope is only:

1. Create Link
2. Redirect
3. Track Click
4. Manage Link

Do not add features merely because they are technically easy to implement.

Features such as the following are NOT part of the MVP:

* Billing
* Subscription
* Public SaaS user management
* Advanced analytics
* QR code generation
* Custom domains
* Public API
* Link previews
* Link-in-bio pages
* Team management
* Complex dashboards
* Event streaming
* Real-time analytics
* A/B testing
* Marketing automation
* Consumer identity / tenancy

Only introduce these when there is a concrete product requirement.

---

## Repository

This project intentionally lives in a separate repository:

```text
cekdu-link
```

Keep it independently deployable from any consuming application.

The service should remain small and independently maintainable.

---

## Architecture

Use a **single service** architecture.

Initial architecture:

```text
Client / Admin
        │
        ▼
   cekdu-link
        │
        ▼
   PostgreSQL
```

Redirect flow:

```text
User
 │
 ▼
/{code}
 │
 ▼
cekdu-link
 │
 ├── lookup link
 │
 ├── record click
 │
 └── redirect
        │
        ▼
 destination URL
```

Do NOT introduce microservices.

Do NOT create separate services for:

* redirect
* analytics
* link management

Keep them inside the same application until real scale or organizational requirements justify separation.

---

## Technology

Preferred stack:

* Go
* PostgreSQL
* Docker
* Cloudflare

Prefer Go standard library where practical.

For HTTP:

```text
net/http
```

is preferred for the MVP.

Do not introduce Gin, Echo, Fiber, Chi, or another HTTP framework unless there is a concrete benefit that justifies the dependency.

Prefer simple, explicit code over abstractions.

---

## Engineering Philosophy

Follow:

> Build the smallest useful system.

Prioritize:

1. Simplicity
2. Reliability
3. Low operational cost
4. Maintainability
5. Clear code
6. Easy future evolution

Avoid:

* premature abstraction
* premature optimization
* premature scaling
* unnecessary dependencies
* unnecessary infrastructure

When multiple solutions are possible, prefer the simplest solution that satisfies the requirement.

---

## Database

Use PostgreSQL.

The link model contains only generic short-link concerns:

```text
id
code
destination_url
click_count
status
created_at
updated_at
```

Rule:

* `id` — UUID, PostgreSQL-generated, PRIMARY KEY
* `code` — public identifier, VARCHAR, NOT NULL, UNIQUE
* `destination_url` — opaque to the service except basic validation, TEXT NOT NULL
* `click_count` — BIGINT NOT NULL DEFAULT 0, must never be negative
* `status` — simple string (`active`/`inactive`), NOT NULL DEFAULT `active`, no ENUM
* `created_at`, `updated_at` — TIMESTAMPTZ NOT NULL DEFAULT now(), no triggers

Do NOT add fields such as `campaign_id`, `partner_id`, `source`, `user_id`, `organization_id`, `owner_id`, or `tenant_id` unless a demonstrated generic requirement exists that cannot be handled by consumers.

Do not introduce event-level click storage in the MVP unless explicitly requested.

The MVP analytics mechanism is simply:

```text
click_count
```

---

## Redirect

Use:

```text
302 Found
```

or:

```text
307 Temporary Redirect
```

for redirects.

Do NOT use `301` as the default.

Destinations may change, and permanent redirects can be cached aggressively.

---

## Click Tracking

For MVP, tracking should remain simple.

Basic flow:

```text
GET /{code}
    ↓
lookup link
    ↓
increment click_count
    ↓
redirect
```

Do not build a complex analytics pipeline.

Do not introduce:

* Kafka
* RabbitMQ
* Redis
* Temporal
* event buses
* background analytics workers

unless a real requirement appears.

If analytics requirements later expand, event-level tracking may include:

* timestamp
* country
* device
* referrer

But this is NOT MVP.

---

## Security and Abuse

A public short URL service has abuse risks.

Keep the service protected against obvious abuse without turning the MVP into a security platform.

Consider:

* rate limiting
* link status / disabling
* basic URL validation
* domain blocklist where necessary
* abuse reporting
* reasonable request limits

Do not overbuild these mechanisms before there is a real requirement.

If the service is deployed on a brand-associated domain, protecting the domain reputation is important.