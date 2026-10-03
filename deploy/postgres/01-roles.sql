\getenv database_name POSTGRES_DB
\getenv migration_password MIGRATION_DB_PASSWORD
\getenv app_password APP_DB_PASSWORD

CREATE ROLE labreserve_migrator LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT PASSWORD :'migration_password';
CREATE ROLE labreserve_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT PASSWORD :'app_password';
ALTER DATABASE :"database_name" OWNER TO labreserve_migrator;
GRANT CONNECT ON DATABASE :"database_name" TO labreserve_app;

\connect :database_name
ALTER SCHEMA public OWNER TO labreserve_migrator;
REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT USAGE ON SCHEMA public TO labreserve_app;
