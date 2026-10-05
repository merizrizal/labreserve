CREATE FUNCTION labreserve_lock_booking_owner(p_account_id UUID)
RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog
AS $function$
BEGIN
    PERFORM 1 FROM public.accounts WHERE id = p_account_id FOR NO KEY UPDATE;
    RETURN FOUND;
END;
$function$;

CREATE FUNCTION labreserve_lock_booking_resource(p_resource_id UUID)
RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog
AS $function$
BEGIN
    PERFORM 1 FROM public.resources WHERE id = p_resource_id FOR UPDATE;
    RETURN FOUND;
END;
$function$;

REVOKE ALL ON FUNCTION labreserve_lock_booking_owner(UUID) FROM PUBLIC;
REVOKE ALL ON FUNCTION labreserve_lock_booking_resource(UUID) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION labreserve_lock_booking_owner(UUID) TO labreserve_app;
GRANT EXECUTE ON FUNCTION labreserve_lock_booking_resource(UUID) TO labreserve_app;

COMMENT ON FUNCTION labreserve_lock_booking_owner(UUID) IS
    'Narrow SECURITY DEFINER row-lock capability; labreserve_app receives no UPDATE privilege on accounts.';
COMMENT ON FUNCTION labreserve_lock_booking_resource(UUID) IS
    'Narrow SECURITY DEFINER row-lock capability; labreserve_app receives no UPDATE privilege on resources.';
