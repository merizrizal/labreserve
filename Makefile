.PHONY: up down restart migrate seed reset-demo verify ensure-env

ensure-env:
	@if [ ! -f .env ]; then cp .env.example .env; printf 'Created development-only .env from .env.example. Review it before use.\n'; fi

up: ensure-env
	docker compose build app migrate seed
	docker compose up --wait -d db
	docker compose --profile tools run --rm migrate
	docker compose --profile tools run --rm seed
	docker compose up --wait -d app
	@printf 'LabReserve is available at the APP_ORIGIN value in .env (default http://127.0.0.1:8080).\n'

down:
	docker compose down --remove-orphans

restart:
	docker compose restart db
	docker compose up --wait -d db
	docker compose restart app

migrate: ensure-env
	docker compose up --wait -d db
	docker compose --profile tools run --rm migrate

seed: ensure-env
	docker compose --profile tools run --rm seed

reset-demo: ensure-env
	@printf 'This permanently deletes the LabReserve demo database volume.\nType DELETE LABRESERVE DEMO DATA to continue: '; \
	read -r answer; \
	if [ "$$answer" != "DELETE LABRESERVE DEMO DATA" ]; then printf 'Reset cancelled.\n'; exit 1; fi
	docker compose --project-name labreserve --file compose.yaml down --volumes --remove-orphans
	$(MAKE) up

verify: ensure-env
	bash scripts/verify.sh
