**Building “LabReserve”: a shared-resource booking application for a small engineering team.**

Engineers use it to reserve things such as a network test bench, a shared Kubernetes lab, or a demonstration environment. They can see availability, make a reservation, and cancel it. A coordinator manages the resources and investigates booking history.

“Reserve the Kubernetes lab” means **record its reservation**, not connect to Kubernetes.

---

# Client brief: LabReserve, version 0.1

## 1. The problem I am paying you to solve

> I manage an engineering team that shares several testing and demonstration resources.
>
> Currently, engineers coordinate reservations through chat messages. Sometimes two people believe they have booked the same environment. Sometimes someone finishes early but forgets to release their reservation. When there is a disagreement, we cannot easily establish what happened.
>
> I want a small internal web application where engineers can see resource availability and reserve a time interval.
>
> I need the application to prevent conflicting reservations, enforce ownership rules, and retain a history of important changes.
>
> I do not need a complete calendar platform, an infrastructure automation system, or an enterprise service-management product.

Assume approximately **20 users and 20 resources**. These are scope assumptions, not a scalability benchmark.

### What success looks like to me

I should be able to demonstrate this sequence:

> Engineer A books the network lab. Engineer B sees the booking and cannot reserve an overlapping interval. Engineer A cancels the booking. Engineer B can now reserve that interval. The coordinator can inspect the history and see who performed each action.

That is the central product story. Everything in the first release should support it.

---

## 2. Who uses the application?

### Engineer

An engineer can browse resources, view the team’s reservations, create a booking for themselves, and cancel their own future bookings.

They cannot manage resources or cancel another person’s booking.

### Coordinator

A coordinator has the same booking capabilities as an engineer, plus permission to create and manage resources, cancel another user’s future booking, and inspect the activity log.

For the proof of concept, seed three accounts: two engineers and one coordinator. Use fictional identities and development-only credentials.

**There is no account-registration, password-recovery, or user-administration feature in version 0.1.**

All authenticated users may see booking owners, times, and purposes. This is an internal scheduling tool; do not put confidential information into the demonstration data.

---

## 3. Functional requirements

These are the client requirements. Implementation choices belong in your architectural design specification.

### FR-01: Sign in and sign out

Users must authenticate before accessing resource or booking information.

The application must determine permissions from the authenticated account, not from a role or user ID supplied by the browser. Signing out must invalidate the session.

A role-selection dropdown that merely pretends to authenticate is not acceptable for this exercise. Permission handling is one of the behaviors we want to implement and test.

**Acceptance example:** An engineer submits a direct request to create a resource. The application rejects it and creates neither the resource nor a successful resource-creation event.

### FR-02: Browse resources and their schedules

Each resource has an immutable unique code, a name, a description, and an active/inactive state.

Example seed resources:

| Code | Name | Description |
|---|---|---|
| `NET-01` | Network Test Bench | Shared environment for network integration tests |
| `K8S-01` | Kubernetes Integration Lab | Shared cluster for application integration testing |
| `DEMO-01` | Customer Demo Environment | Shared environment for demonstrations |

Users can select a resource and a date to view its bookings.

A simple list or table is sufficient. **Do not build a drag-and-drop calendar in the first release.**

The interface must distinguish an inactive resource from an active resource that is already booked.

### FR-03: Create a booking

An authenticated user can reserve an active resource for themselves.

A booking contains the resource, owner, start time, end time, and a short purpose. The purpose is required and must contain between 1 and 200 characters after trimming surrounding whitespace.

The application must enforce the following rules:

| Rule | Required behavior |
|---|---|
| Start time | At least five minutes in the future |
| Booking horizon | Start time no more than 30 days in the future |
| Duration | Between 30 minutes and eight hours, inclusive |
| Interval validity | End must be after start |
| Resource eligibility | Resource must be active |
| Conflicts | No overlapping non-cancelled booking for the same resource |
| Ownership | Owner is the authenticated user |

The server’s clock is authoritative.

Bookings may cross midnight. There are no business-hour restrictions in version 0.1.

A person **may reserve different resources during the same interval**. We are preventing resource conflicts, not implementing personal-calendar conflicts.

#### The interval boundary is important

A booking from 10:00 to 11:00 and another from 11:00 to 12:00 are allowed.

A booking from 10:00 to 11:00 and another from 10:30 to 11:30 are not.

In other words, treat booking intervals as **start-inclusive and end-exclusive**.

#### Concurrent submissions must remain correct

When two users submit valid but conflicting bookings simultaneously against an otherwise healthy system, one booking succeeds and the other receives a clear conflict response.

**Checking availability in the browser is not sufficient. The final write must preserve the rule.**

As the client, I care about the guarantee. Your ADS should explain how you enforce it.

#### Duplicate submissions must not create duplicate bookings

The application must handle a repeated submission of the same booking request without creating a second booking or a second creation event.

Use a stable request identifier scoped to the authenticated user. Repeating the identifier with the same booking data returns the existing booking’s identifier. Reusing it with different booking data is rejected.

Keep this behavior valid until the demonstration database is explicitly reset.

This gives you a realistic retry-handling problem without needing a message queue or an external service.

### FR-04: View bookings

Users need two views: the selected resource’s schedule and **My Bookings**.

A booking has only two stored business states:

```text
Confirmed → Cancelled
```

For confirmed bookings, display a time-based label:

| Label | Meaning |
|---|---|
| Upcoming | Current time is before the start |
| In use | Current time is at or after the start, but before the end |
| Past | Current time is at or after the end |

Cancelled bookings always display as cancelled.

Do not introduce a background worker merely to change “upcoming” into “in use.” These labels are derived from the booking times.

Retain past and cancelled bookings. Display long lists in pages of 25 records, with a documented stable ordering.

### FR-05: Cancel a booking

An engineer can cancel their own booking **only before its start time**.

A coordinator can cancel anyone’s booking before its start time. When cancelling someone else’s booking, the coordinator must provide a reason.

Once a booking has started, neither role can cancel it in version 0.1. Early release is a possible later enhancement, not part of cancellation.

Cancellation must preserve the booking record and immediately release its interval for new reservations.

Repeating an authorized cancellation of an already-cancelled booking must be harmless and must not create another cancellation event.

**Acceptance example:** Engineer A cancels an upcoming reservation. Engineer B can then reserve the same resource for the same interval.

### FR-06: Manage resources

Only a coordinator can create resources, edit their names and descriptions, or activate/deactivate them.

There is no resource-deletion feature. Historical bookings must remain understandable.

A resource may be deactivated only when it has no non-cancelled booking whose end time is still in the future. This includes both upcoming bookings and bookings currently in use.

The application must report why deactivation was rejected.

This rule must also survive concurrency. A simultaneous booking and deactivation must not leave an inactive resource with a newly accepted future booking.

For this release, a coordinator must cancel future bookings before deactivation and wait for an in-use booking to finish.

### FR-07: Inspect activity history

The coordinator needs an activity view showing successful resource and booking changes.

Each event must identify the actor, action, affected record, and server timestamp. Include relevant change details, such as a cancellation reason or the old and new resource name.

The activity record and its associated change must succeed or fail together. A booking must not be created successfully while its required creation event is missing.

There must be no product interface for editing or deleting activity events.

This is **an application activity history**, not a claim of cryptographically tamper-proof auditing. Database-administrator tamper resistance is outside this proof of concept.

---

## 4. Cross-cutting requirements and boundaries

### Time handling

All displayed booking times must use **Asia/Jakarta**, visibly labelled in the interface.

Persist unambiguous instants. Any HTTP interface accepting timestamp strings must require an explicit offset or an equally unambiguous documented representation.

A user viewing the application from a browser configured for another timezone must still see the same Jakarta schedule.

Automated tests must control time rather than depend on hard-coded dates that eventually become the past.

### User experience

The application must be usable through a browser without SQL queries or API tools for normal workflows.

I expect a login screen, resource/schedule view, booking form and detail view, My Bookings, resource management, and coordinator activity view. These can share pages; I am not prescribing a frontend component structure.

Forms must have labels and useful validation errors. A rejected booking should explain whether the problem is invalid input, an inactive resource, or a reservation conflict.

Do not expose raw database errors to users.

### Security and local operation

Enforce permissions on the server. Protect state-changing browser requests against cross-site request forgery. Treat resource descriptions and booking purposes as plain text rather than executable markup.

Do not store plaintext passwords or write passwords and session tokens to logs.

The demonstration must bind locally by default and use synthetic data. It must not require production credentials, access to your OpenStack management network, or external service accounts.

After the initial dependency and container-image downloads, ordinary application operation must not require an external service.

### Reproducibility

Provide a documented startup procedure, migrations, seed data, test procedure, and explicit demonstration-reset procedure.

Normal startup and repeated seeding must not erase existing bookings or duplicate seed accounts/resources. Only the explicit reset operation may destroy demonstration data.

Application and database restarts must preserve committed data.

### Explicitly out of scope

Version 0.1 excludes recurring bookings, booking approvals, rescheduling, maintenance windows, email or chat notifications, calendar synchronization, attachments, billing, multi-tenancy, mobile applications, and actual infrastructure control.

It also excludes microservices, Kubernetes deployment, and a custom agent-orchestration framework.

**Do not implement these “for future flexibility.” Put proposed additions in a backlog.**

---

## 5. How I will accept the product

I would establish these scenarios before implementation and use them to review the result.

| ID | Scenario | Required result |
|---|---|---|
| AC-01 | An engineer creates a valid booking | Booking persists, appears in the UI, and has exactly one creation event |
| AC-02 | Another user requests a partially overlapping, contained, enclosing, or identical interval | Every conflicting case is rejected |
| AC-03 | A booking starts exactly when another ends | It is accepted |
| AC-04 | Identical intervals target different resources | Both are accepted |
| AC-05 | Two users submit conflicting bookings concurrently | Exactly one succeeds; only one booking is stored |
| AC-06 | The same user retries the same request identifier and data | Existing booking is returned; no duplicate booking or event |
| AC-07 | An engineer attempts to cancel someone else’s booking directly through HTTP | Rejected without changing the booking |
| AC-08 | An authorized user cancels a future booking and repeats the cancellation | Interval becomes available; exactly one cancellation event |
| AC-09 | A coordinator deactivates a resource with an upcoming or in-use booking | Deactivation is rejected |
| AC-10 | Booking creation races with resource deactivation | The final state satisfies both resource and booking rules |
| AC-11 | The browser uses a non-Jakarta timezone | Entered and displayed times still represent the intended Jakarta interval |
| AC-12 | Recording a required activity event fails | The associated business change does not commit |
| AC-13 | Application and database restart | Previously committed bookings and history remain |
| AC-14 | A booking purpose contains HTML-like content | It appears as text and does not execute |

These are not the entire test suite. They are the **client acceptance cases that your implementation and review must cover**.

For AC-05 and AC-10, I would require tests using the real database and independent concurrent operations. A unit test using a fake repository would not be sufficient evidence for those guarantees.

For browser tests, verify user-visible behavior rather than implementation details. Playwright’s official guidance recommends that approach, along with isolated tests and controlled test data. [Playwright](https://playwright.dev/docs/best-practices)

---

## 6. Suggested technical approach

| Area | Suggested default |
|---|---|
| Application | Go, as one application with clear internal boundaries |
| UI | Server-rendered HTML with a small amount of JavaScript |
| Database | PostgreSQL |
| Local environment | Docker Compose |
| Verification | Go tests, real-database integration tests, and Playwright browser tests |
| Delivery | Git repository with one automated verification entry point |

Go’s `html/template` provides contextual escaping for HTML output. I would use it rather than add a separate frontend framework solely for this experiment; do not bypass its escaping for user-provided text. [Go Packages](https://pkg.go.dev/html/template?utm_source=chatgpt.com)

PostgreSQL is a particularly relevant choice because its documentation explicitly covers time-range reservations and exclusion constraints that prohibit overlapping ranges for the same resource. Your ADS can evaluate that mechanism without inventing a concurrency solution from scratch. [PostgreSQL](https://www.postgresql.org/docs/current/rangetypes.html)

Docker Compose can define the application, database, networks, and volumes together and start them from that configuration. That is enough deployment machinery for this proof of concept. [Docker Documentation](https://docs.docker.com/compose/?utm_source=chatgpt.com)

These are **recommended defaults, not additional client requirements**. You still own the architectural decisions.

I would not start with a separate SPA, Redis, a message broker, and multiple services. They are not needed to demonstrate the required product.

---

## Two later client change requests

Once version 0.1 passes, these would be my next requests. **They are not authorized for the initial release.**

**CR-01: Atomic rescheduling.**  
Allow an engineer to move their future booking. When the replacement interval is unavailable, retain the original reservation unchanged. A successful move must have an activity event.

This exercises a new transaction without discarding the existing behavior.

**CR-02: Maintenance windows.**  
Allow coordinators to block a resource for a time interval. Reject a maintenance window that overlaps an existing reservation; do not silently cancel bookings.

This exercises changes to the availability model, UI, and regression suite.

Both requests are substantial enough to expose architectural weaknesses, but bounded enough to implement and review independently.

---

## 10. Your first assignment from me as the client

> **Subject: LabReserve proof of concept — initial delivery**
>
> Please develop LabReserve according to the version 0.1 brief.
>
> Before implementation, submit a short architectural design describing the application boundaries, main data records, authentication and authorization approach, time handling, and how you intend to preserve booking correctness under concurrency.
>
> Identify contradictions or unresolved product decisions rather than silently inventing requirements.
>
> Break the work into reviewable deliveries. Do not add features outside the agreed scope.
>
> The first implementation delivery is limited to reproducible local startup, migrations and seed data, sign-in/sign-out, role-aware access, and browsing resources with an empty schedule.
>
> Include automated verification and a demonstration procedure. Booking creation is the next delivery, not part of the first one.
>
> For every implementation task, continue through testing and in-scope repair. Escalate genuine requirement conflicts, boundary changes, or blockers rather than stopping between routine steps.
>
> A delivery is ready for review only when you provide the candidate revision, executed checks, observed results, and remaining limitations.

---

## 11. Post-Delivery 2 client request — My Bookings and self-service cancellation

### Delivery context and request

The client reports Delivery 2 — booking creation, populated resource schedules, and retry-safe submission — complete. The initial assignment above remains the historical Delivery 1 boundary, not the current implementation scope.

> Our engineers can now create reservations, but they need a convenient place to see all their bookings.
> I want each engineer to see their own booking history, understand which reservations are upcoming, currently in use, past, or cancelled, and cancel upcoming reservations they no longer need.
> When a reservation is cancelled, its time interval should become available for another engineer.
> The application must preserve historical records and prevent users from cancelling someone else's reservations.

This request activates the next portion of existing v0.1 **FR-04 (View bookings)** and **FR-05 (Cancel a booking)**. It is not the deferred CR-01 rescheduling or CR-02 maintenance-window request, and it does not introduce another stored booking state.

### Proposed Delivery 3 boundary

The following is a documentation proposal for review, not approval to implement:

- Provide an authenticated **My Bookings** view across resources, containing only the current account's bookings, including past and cancelled records. Show resource identity, full interval, purpose, and status; use the existing Jakarta time contract and 25-record pages with a documented stable order.
- Derive Upcoming, In use, and Past from authoritative server time and the booking interval. Cancelled always displays Cancelled. Only Confirmed and Cancelled are persisted.
- Let engineers cancel their own Confirmed bookings only while server time is strictly before start. The same self-service capability applies to coordinators for their own bookings; coordinator intervention on another account's booking remains a later delivery.
- Retain the booking, original request identity/data, and history. Commit cancellation metadata and exactly one required cancellation activity event together. A failed required event must leave the booking Confirmed and its interval reserved.
- Release the interval when cancellation commits. Another engineer can then book it through the existing creation workflow. A rejected or rolled-back cancellation must not release availability.
- Enforce ownership and eligibility on the server, including direct requests and retries; state-changing browser requests retain authentication and CSRF protection. A repeated authorized cancellation must not create another event.

### Acceptance emphasis

Demonstrate Engineer A finding their booking in My Bookings, cancelling it before start, and still seeing the retained Cancelled record. Engineer B can then reserve that interval. Engineer B must not see A's booking in their own My Bookings or cancel A's booking through a direct request. Shared resource schedules remain visible to all authenticated users under the original visibility policy.

Use controlled time to check labels and cancellation just before and exactly at start. Verify retained 25-record pages, cancellation/rebooking with real PostgreSQL, concurrent cancellation with exactly one event, and rollback when event recording fails. Existing booking-creation/replay guarantees must remain intact.

### Boundaries and unresolved policy

This request does not authorize early release, rescheduling, deletion, coordinator cancellation of others' bookings, resource management, or an activity-history screen. Coordinator intervention and the activity screen remain required for full v0.1, but are outside this proposed delivery. Atomic cancellation history is required even before that screen exists.

The existing brief leaves one response detail unresolved: **an authorized retry of an already-cancelled booking after its former start time**. Proposed policy: return harmless success without a new mutation/event, while still enforcing ownership. Confirm this before cancellation implementation; the prohibition on cancelling a started Confirmed booking is unchanged.
