# Команды для аналитической части. Запуск из корня репозитория: make all
#
# psql запускается внутри контейнера и берёт пользователя и базу из его переменных (.env).
PSQL = docker compose exec -T postgres sh -c 'psql -U "$$POSTGRES_USER" -d "$$POSTGRES_DB" -v ON_ERROR_STOP=1 -q'

.PHONY: all up data schema load marts metrics checks ab analysis-venv psql

all: up data schema load marts metrics checks ## всё с нуля: база, данные, схема, загрузка, витрины, метрики, проверки

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

metrics: ## пересобрать представления с метриками (схема metrics); нужен после marts
	for f in dwh/metrics/*.sql; do echo "$$f"; $(PSQL) < "$$f" || exit 1; done

checks: ## проверки качества данных, витрин и метрик
	$(PSQL) < dwh/03_checks.sql
	$(PSQL) < dwh/05_mart_checks.sql
	$(PSQL) < dwh/06_metric_checks.sql

ab: ## пересчитать A/B-анализ: выполнить ноутбук на текущих данных базы и обновить графики
	cd analysis && .venv/bin/jupyter nbconvert --to notebook --execute --inplace ab_sms_reminders.ipynb

analysis-venv: ## создать окружение для анализа (один раз)
	cd analysis && python3 -m venv .venv && .venv/bin/pip install -r requirements.txt

psql: ## открыть консоль базы
	docker compose exec postgres sh -c 'psql -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"'
