CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE bookings (
    id UUID PRIMARY KEY,
    resource_id UUID NOT NULL REFERENCES resources(id),
    owner_account_id UUID NOT NULL REFERENCES accounts(id),
    request_id UUID NOT NULL,
    start_at TIMESTAMPTZ NOT NULL,
    end_at TIMESTAMPTZ NOT NULL,
    purpose TEXT NOT NULL,
    state TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    cancelled_by_account_id UUID REFERENCES accounts(id),
    cancelled_at TIMESTAMPTZ,
    CONSTRAINT bookings_owner_request_id_uq UNIQUE (owner_account_id, request_id),
    CONSTRAINT bookings_state_check CHECK (state IN ('confirmed', 'cancelled')),
    CONSTRAINT bookings_interval_duration_check CHECK (
        end_at > start_at
        AND end_at - start_at >= INTERVAL '30 minutes'
        AND end_at - start_at <= INTERVAL '8 hours'
    ),
    CONSTRAINT bookings_purpose_length_check CHECK (char_length(purpose) BETWEEN 1 AND 200),
    CONSTRAINT bookings_cancellation_metadata_check CHECK (
        (state = 'confirmed' AND cancelled_by_account_id IS NULL AND cancelled_at IS NULL)
        OR
        (state = 'cancelled' AND cancelled_by_account_id IS NOT NULL AND cancelled_at IS NOT NULL)
    ),
    CONSTRAINT bookings_confirmed_resource_period_excl EXCLUDE USING gist (
        resource_id WITH =,
        tstzrange(start_at, end_at, '[)') WITH &&
    ) WHERE (state = 'confirmed')
);

CREATE TABLE activity_events (
    id UUID PRIMARY KEY,
    actor_account_id UUID NOT NULL REFERENCES accounts(id),
    action TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    booking_id UUID REFERENCES bookings(id),
    resource_id UUID REFERENCES resources(id),
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT activity_events_exactly_one_target_check CHECK (num_nonnulls(booking_id, resource_id) = 1),
    CONSTRAINT activity_events_details_object_check CHECK (jsonb_typeof(details) = 'object')
);

GRANT SELECT, INSERT ON bookings, activity_events TO labreserve_app;
