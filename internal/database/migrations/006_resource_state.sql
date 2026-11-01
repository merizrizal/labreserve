DO $migration$
BEGIN
    IF current_user <> 'labreserve_migrator'
       OR EXISTS (
           SELECT 1
           FROM pg_catalog.pg_roles
           WHERE rolname = current_user AND rolsuper
       ) THEN
        RAISE EXCEPTION USING
            ERRCODE = '42501',
            MESSAGE = 'migration 006 must be applied by the non-superuser labreserve_migrator role';
    END IF;
END;
$migration$;

CREATE FUNCTION public.labreserve_set_resource_state(
    p_actor_id UUID,
    p_resource_id UUID,
    p_active BOOLEAN,
    p_occurred_at TIMESTAMPTZ
)
RETURNS TABLE (
    resource_id UUID,
    code TEXT,
    name TEXT,
    description TEXT,
    active BOOLEAN,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    changed BOOLEAN
)
LANGUAGE plpgsql
VOLATILE
SECURITY DEFINER
SET search_path = pg_catalog
AS $function$
DECLARE
    v_resource public.resources%ROWTYPE;
    v_before_active BOOLEAN;
    v_action TEXT;
BEGIN
    IF p_actor_id IS NULL OR NOT EXISTS (
        SELECT 1
        FROM public.accounts AS actor
        WHERE actor.id = p_actor_id AND actor.role = 'coordinator'
    ) THEN
        RAISE EXCEPTION USING ERRCODE = '42501', MESSAGE = 'coordinator authority required';
    END IF;

    IF p_resource_id IS NULL OR p_active IS NULL OR p_occurred_at IS NULL THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'resource state values must be complete';
    END IF;

    SELECT stored_resource.*
    INTO v_resource
    FROM public.resources AS stored_resource
    WHERE stored_resource.id = p_resource_id
    FOR UPDATE;

    IF NOT FOUND THEN
        RETURN;
    END IF;

    IF v_resource.active = p_active THEN
        RETURN QUERY
        SELECT v_resource.id, v_resource.code, v_resource.name, v_resource.description,
               v_resource.active, v_resource.created_at, v_resource.updated_at, FALSE;
        RETURN;
    END IF;

    v_before_active := v_resource.active;
    v_action := CASE WHEN p_active THEN 'resource.activated' ELSE 'resource.deactivated' END;

    IF NOT p_active AND EXISTS (
        SELECT 1
        FROM public.bookings AS booking
        WHERE booking.resource_id = p_resource_id
          AND booking.state = 'confirmed'
          AND booking.end_at > p_occurred_at
    ) THEN
        RAISE EXCEPTION USING
            ERRCODE = 'P0001',
            MESSAGE = 'resource has blocking confirmed bookings';
    END IF;

    UPDATE public.resources AS stored_resource
    SET active = p_active, updated_at = p_occurred_at
    WHERE stored_resource.id = p_resource_id
    RETURNING stored_resource.id, stored_resource.code, stored_resource.name,
              stored_resource.description, stored_resource.active,
              stored_resource.created_at, stored_resource.updated_at
    INTO v_resource;

    INSERT INTO public.activity_events (id, actor_account_id, action, occurred_at, resource_id, details)
    VALUES (
        pg_catalog.gen_random_uuid(),
        p_actor_id,
        v_action,
        p_occurred_at,
        v_resource.id,
        pg_catalog.jsonb_build_object(
            'before', pg_catalog.jsonb_build_object('active', v_before_active),
            'after', pg_catalog.jsonb_build_object('active', p_active)
        )
    );

    RETURN QUERY
    SELECT v_resource.id, v_resource.code, v_resource.name, v_resource.description,
           v_resource.active, v_resource.created_at, v_resource.updated_at, TRUE;
END;
$function$;

REVOKE ALL ON FUNCTION public.labreserve_set_resource_state(UUID, UUID, BOOLEAN, TIMESTAMPTZ) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.labreserve_set_resource_state(UUID, UUID, BOOLEAN, TIMESTAMPTZ) TO labreserve_app;

COMMENT ON FUNCTION public.labreserve_set_resource_state(UUID, UUID, BOOLEAN, TIMESTAMPTZ) IS
    'Coordinator-checked explicit Resource state transition that guards Confirmed occupancy and atomically appends Activity.';
