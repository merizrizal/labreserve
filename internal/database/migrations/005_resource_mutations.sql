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
            MESSAGE = 'migration 005 must be applied by the non-superuser labreserve_migrator role';
    END IF;
END;
$migration$;

CREATE FUNCTION public.labreserve_create_resource(
    p_actor_id UUID,
    p_code TEXT,
    p_name TEXT,
    p_description TEXT,
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
    updated_at TIMESTAMPTZ
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog
AS $function$
DECLARE
    v_resource_id UUID := pg_catalog.gen_random_uuid();
    v_edge_whitespace CONSTANT TEXT := U&'\0009\000A\000B\000C\000D\0020\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000';
BEGIN
    IF p_actor_id IS NULL OR NOT EXISTS (
        SELECT 1
        FROM public.accounts AS actor
        WHERE actor.id = p_actor_id AND actor.role = 'coordinator'
    ) THEN
        RAISE EXCEPTION USING ERRCODE = '42501', MESSAGE = 'coordinator authority required';
    END IF;

    IF p_code IS NULL OR p_code = ''
       OR p_code IS DISTINCT FROM pg_catalog.btrim(p_code, v_edge_whitespace)
       OR p_name IS NULL OR p_name = ''
       OR p_name IS DISTINCT FROM pg_catalog.btrim(p_name, v_edge_whitespace)
       OR p_description IS NULL
       OR p_description IS DISTINCT FROM pg_catalog.btrim(p_description, v_edge_whitespace)
       OR p_active IS NULL OR p_occurred_at IS NULL THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'resource creation values must be canonical and complete';
    END IF;

    INSERT INTO public.resources (id, code, name, description, active, created_at, updated_at)
    VALUES (v_resource_id, p_code, p_name, p_description, p_active, p_occurred_at, p_occurred_at);

    INSERT INTO public.activity_events (id, actor_account_id, action, occurred_at, resource_id, details)
    VALUES (
        pg_catalog.gen_random_uuid(),
        p_actor_id,
        'resource.created',
        p_occurred_at,
        v_resource_id,
        pg_catalog.jsonb_build_object(
            'code', p_code,
            'name', p_name,
            'description', p_description,
            'active', p_active
        )
    );

    RETURN QUERY
    SELECT v_resource_id, p_code, p_name, p_description, p_active, p_occurred_at, p_occurred_at;
END;
$function$;

CREATE FUNCTION public.labreserve_edit_resource(
    p_actor_id UUID,
    p_resource_id UUID,
    p_name TEXT,
    p_description TEXT,
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
SECURITY DEFINER
SET search_path = pg_catalog
AS $function$
DECLARE
    v_resource public.resources%ROWTYPE;
    v_before_name TEXT;
    v_before_description TEXT;
    v_edge_whitespace CONSTANT TEXT := U&'\0009\000A\000B\000C\000D\0020\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000';
BEGIN
    IF p_actor_id IS NULL OR NOT EXISTS (
        SELECT 1
        FROM public.accounts AS actor
        WHERE actor.id = p_actor_id AND actor.role = 'coordinator'
    ) THEN
        RAISE EXCEPTION USING ERRCODE = '42501', MESSAGE = 'coordinator authority required';
    END IF;

    IF p_name IS NULL OR p_name = ''
       OR p_name IS DISTINCT FROM pg_catalog.btrim(p_name, v_edge_whitespace)
       OR p_description IS NULL
       OR p_description IS DISTINCT FROM pg_catalog.btrim(p_description, v_edge_whitespace)
       OR p_occurred_at IS NULL THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'resource edit values must be canonical and complete';
    END IF;

    SELECT stored_resource.*
    INTO v_resource
    FROM public.resources AS stored_resource
    WHERE stored_resource.id = p_resource_id
    FOR UPDATE;

    IF NOT FOUND THEN
        RETURN;
    END IF;

    IF v_resource.name = p_name AND v_resource.description = p_description THEN
        RETURN QUERY
        SELECT v_resource.id, v_resource.code, v_resource.name, v_resource.description,
               v_resource.active, v_resource.created_at, v_resource.updated_at, FALSE;
        RETURN;
    END IF;

    v_before_name := v_resource.name;
    v_before_description := v_resource.description;

    UPDATE public.resources AS stored_resource
    SET name = p_name, description = p_description, updated_at = p_occurred_at
    WHERE stored_resource.id = p_resource_id
    RETURNING stored_resource.id, stored_resource.code, stored_resource.name,
              stored_resource.description, stored_resource.active,
              stored_resource.created_at, stored_resource.updated_at
    INTO v_resource;

    INSERT INTO public.activity_events (id, actor_account_id, action, occurred_at, resource_id, details)
    VALUES (
        pg_catalog.gen_random_uuid(),
        p_actor_id,
        'resource.descriptive_edited',
        p_occurred_at,
        v_resource.id,
        pg_catalog.jsonb_build_object(
            'before', pg_catalog.jsonb_build_object(
                'name', v_before_name,
                'description', v_before_description
            ),
            'after', pg_catalog.jsonb_build_object(
                'name', p_name,
                'description', p_description
            )
        )
    );

    RETURN QUERY
    SELECT v_resource.id, v_resource.code, v_resource.name, v_resource.description,
           v_resource.active, v_resource.created_at, v_resource.updated_at, TRUE;
END;
$function$;

REVOKE ALL ON FUNCTION public.labreserve_create_resource(UUID, TEXT, TEXT, TEXT, BOOLEAN, TIMESTAMPTZ) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.labreserve_edit_resource(UUID, UUID, TEXT, TEXT, TIMESTAMPTZ) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.labreserve_create_resource(UUID, TEXT, TEXT, TEXT, BOOLEAN, TIMESTAMPTZ) TO labreserve_app;
GRANT EXECUTE ON FUNCTION public.labreserve_edit_resource(UUID, UUID, TEXT, TEXT, TIMESTAMPTZ) TO labreserve_app;

COMMENT ON FUNCTION public.labreserve_create_resource(UUID, TEXT, TEXT, TEXT, BOOLEAN, TIMESTAMPTZ) IS
    'Coordinator-checked Resource creation that atomically appends its resource.created Activity event.';
COMMENT ON FUNCTION public.labreserve_edit_resource(UUID, UUID, TEXT, TEXT, TIMESTAMPTZ) IS
    'Coordinator-checked descriptive edit that locks Resource, suppresses unchanged writes, and atomically appends Activity.';
