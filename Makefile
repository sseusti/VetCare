# Команды для аналитической части. Запуск из корня репозитория: make all
#
# psql запускается внутри контейнера и берёт пользователя и базу из его переменных (.env).
PSQL = docker compose exec -T postgres sh -c 'psql -U "$$POSTGRES_USER" -d "$$POSTGRES_DB" -v ON_ERROR_STOP=1 -q'

.PHONY: all up data schema load marts checks psql

all: up data schema load marts checks ## всё с нуля: база, данные, схема, загрузка, витрины, проверки

up: ## поднять PostgreSQL и дождаться, пока он будет готов
	docker compose up -d --wait

data: ## сгенерировать CSV в data-generator/data/raw
	cd data-generator && .venv/bin/python generate.py

schema: ## пересоздать схему oltp
	$(PSQL) < dwh/01_schema.sql

load: ## загрузить CSV в oltp
	$(PSQL) < dwh/02_load.sql

marts: ## пересобрать staging и витрины (схемы stg и mart)
	$(PSQL) < dwh/04_marts.sql

checks: ## проверки качества данных и витрин
	$(PSQL) < dwh/03_checks.sql
	$(PSQL) < dwh/05_mart_checks.sql

psql: ## открыть консоль базы
	docker compose exec postgres sh -c 'psql -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"'
